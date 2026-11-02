package tns

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"database/sql/driver"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xo/oracle/internal/config"

	"github.com/xo/oracle/internal/charset"
)

var readBufferSize = 0x4000

type SessionState struct {
	summary   *SummaryObject
	sendPcks  []packet
	InBuffer  *bytes.Buffer
	OutBuffer *bytes.Buffer
}

type Session struct {
	mu sync.Mutex
	// breakMu is held while a context breaks a call, so that EndContext can
	// wait for the break to end
	breakMu sync.Mutex
	// breakDeadline is the end of the grace of a break, in Unix nanoseconds,
	// or zero
	breakDeadline atomic.Int64
	// breakPending is true from a break until the server answers it. A break
	// that comes after its call ended is answered at the start of the next
	// call, which Drain reads and drops
	breakPending atomic.Bool
	// Drain reads the answer of a break that is pending, the reset and its
	// ORA-01013. The connection sets it
	Drain             func() error
	conn              net.Conn
	sslConn           *tls.Conn
	reader            *bufio.Reader
	remainingBytes    int
	lastPacket        bytes.Buffer
	Context           *SessionContext
	sendPcks          []packet
	breakIndex        int
	TimeZone          []byte
	HasEOSCapability  bool
	HasFSAPCapability bool
	Summary           *SummaryObject
	states            []SessionState
	StrConv           *charset.Converter
	breakConn         bool
	Connected         bool
	SSL               struct {
		CertificateRequest []*x509.CertificateRequest
		PrivateKeys        []*rsa.PrivateKey
		Certificates       []*x509.Certificate
		roots              *x509.CertPool
		tlsCertificates    []tls.Certificate
	}
	// logger is never nil. hide withholds the bytes of each packet from it
	// while a connection logs on, since those packets carry the encrypted
	// session key and password (D14).
	logger *slog.Logger
	hide   atomic.Bool
	basicSession
}

func NewSessionWithInputBufferForDebug(input []byte) *Session {
	options := &config.ConnectionConfig{
		AuthService:           nil,
		SessionDataUnitSize:   0xFFFF,
		TransportDataUnitSize: 0xFFFF,
	}
	ret := &Session{
		conn:       nil,
		lastPacket: bytes.Buffer{},
		Context:    NewSessionContext(options),
		Summary:    nil,
		logger:     slog.New(slog.DiscardHandler),
	}
	ret.inBuffer = bytes.NewBuffer(input)
	ret.terminal = ret
	ret.UseBigClrChunks = false
	ret.ClrChunkSize = 0x40
	return ret
}

func NewSession(config *config.ConnectionConfig, logger *slog.Logger) *Session {
	ret := &Session{
		conn:       nil,
		Context:    NewSessionContext(config),
		Summary:    nil,
		lastPacket: bytes.Buffer{},
		logger:     logger,
	}
	ret.inBuffer = &bytes.Buffer{}
	ret.outBuffer = &bytes.Buffer{}
	ret.UseBigClrChunks = false
	ret.ClrChunkSize = 0x40
	ret.terminal = ret
	return ret
}

// SaveState save current session state and accept new state
// if new state is nil the session will be reset
func (session *Session) SaveState(newState *SessionState) {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.states = append(session.states, SessionState{
		summary:   session.Summary,
		sendPcks:  session.sendPcks,
		InBuffer:  session.inBuffer,
		OutBuffer: session.outBuffer,
	})
	if newState == nil {
		session.Summary = nil
		session.sendPcks = nil
		session.inBuffer = &bytes.Buffer{}
		session.outBuffer = &bytes.Buffer{}
	} else {
		session.Summary = newState.summary
		session.sendPcks = newState.sendPcks
		session.inBuffer = newState.InBuffer
		session.outBuffer = newState.OutBuffer
	}
}

// LoadState load last saved session state and return the current state
// if this is the only session state available set session state memory to nil
func (session *Session) LoadState() (oldState *SessionState) {
	index := len(session.states) - 1
	if index >= 0 {
		oldState = &session.states[index]
	}
	if index >= 0 {
		currentState := session.states[index]
		session.Summary = currentState.summary
		session.sendPcks = currentState.sendPcks
		session.inBuffer = currentState.InBuffer
		session.outBuffer = currentState.OutBuffer
		if index == 0 {
			session.states = nil
		} else {
			session.states = session.states[:index]
		}
	}
	return
}

func (session *Session) resetWrite() {
	session.mu.Lock()
	session.sendPcks = nil
	if session.outBuffer != nil {
		session.outBuffer.Reset()
	}
	session.mu.Unlock()
}

func (session *Session) resetRead() {
	if session.inBuffer != nil {
		session.inBuffer.Reset()
	}
}

// ResetBuffer empty in and out buffer and set read index to 0. A call starts
// with it, so it first drains the answer of a break that came after the call
// that it broke had ended. go-ora read that answer as the reply of the next
// call, which then failed with ORA-01013, or waited for good.
func (session *Session) ResetBuffer() {
	if session.breakPending.Load() && session.Drain != nil {
		session.breakPending.Store(false)
		session.ResetBreak()
		session.breakDeadline.Store(time.Now().Add(breakGrace).UnixNano())
		if err := session.Drain(); err != nil {
			session.logger.Info("draining the answer of a break", "error", err)
		}
		session.breakDeadline.Store(0)
	}
	session.Summary = nil
	session.resetWrite()
	session.resetRead()
}

func (session *Session) Properties() SessionProperties {
	return session.SessionProperties
}
func (session *Session) SetConnected() {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.Connected = true
}

// breakGrace is how long the session waits for the server to answer a break
// that a context sent. A server that does not answer in that time fails the
// read, and the connection is broken.
const breakGrace = 10 * time.Second

// StartContext watches ctx while one call runs on the session, and returns
// the function that ends the watch, which the caller passes to EndContext.
// When ctx ends before that, the session breaks the call: it sends the break
// of Oracle Net, by out of band data where the server supports it and by a
// marker packet where not, and the server answers with ORA-01013.
//
// It starts no goroutine. context.AfterFunc runs the break only when ctx ends
// (D10). A nil ctx, and a ctx that never ends, watch nothing.
func (session *Session) StartContext(ctx context.Context) func() {
	if ctx == nil || ctx.Done() == nil {
		return func() {}
	}
	stop := context.AfterFunc(ctx, session.breakForContext)
	return func() {
		if !stop() {
			// the break started, so wait for it to end before the session
			// takes the next call
			session.breakMu.Lock()
			session.breakMu.Unlock()
		}
	}
}

// EndContext ends the watch that StartContext began.
func (session *Session) EndContext(end func()) {
	if end != nil {
		end()
	}
}

// breakForContext breaks the call that runs when its context ends. Before the
// logon the session has no call to break, so it closes the connection.
func (session *Session) breakForContext() {
	session.breakMu.Lock()
	defer session.breakMu.Unlock()
	session.mu.Lock()
	connected := session.Connected
	logger := session.logger
	session.mu.Unlock()
	if !connected {
		if err := session.WriteFinalPacket(); err != nil {
			logger.Info("writing the final packet", "error", err)
		}
		session.Disconnect()
		return
	}
	session.breakDeadline.Store(time.Now().Add(breakGrace).UnixNano())
	if err := session.BreakConnection(); err != nil {
		logger.Info("breaking the call", "error", err)
	}
}

// deadline returns the deadline of the next read or write: the timeout of the
// configuration, or the end of the grace of a break, whichever is first.
func (session *Session) deadline() time.Time {
	var t time.Time
	if session.Context.connConfig.Timeout > 0 {
		t = time.Now().Add(session.Context.connConfig.Timeout)
	}
	if bd := session.breakDeadline.Load(); bd != 0 {
		if b := time.Unix(0, bd); t.IsZero() || b.Before(t) {
			t = b
		}
	}
	return t
}

func (session *Session) initRead() error {
	var err error
	timeout := session.deadline()
	if session.sslConn != nil {
		err = session.sslConn.SetReadDeadline(timeout)
	} else if session.conn != nil {
		err = session.conn.SetReadDeadline(timeout)
	} else {
		return errors.New("attempt to set timeout on closed connection")
	}
	return err
}

func (session *Session) initWrite() error {
	var err error
	timeout := session.deadline()
	if session.sslConn != nil {
		err = session.sslConn.SetWriteDeadline(timeout)
	} else if session.conn != nil {
		err = session.conn.SetWriteDeadline(timeout)
	} else {
		return errors.New("attempt to set timeout on closed connection")
	}
	return err
}

// LoadSSLData load data required for SSL connection like certificate, private keys and
// certificate requests
func (session *Session) LoadSSLData(certs, keys, certRequests [][]byte) error {
	for _, temp := range certs {
		cert, err := x509.ParseCertificate(temp)
		if err != nil {
			return err
		}
		session.SSL.Certificates = append(session.SSL.Certificates, cert)
		for _, temp2 := range keys {
			key, err := x509.ParsePKCS1PrivateKey(temp2)
			if err != nil {
				return err
			}
			if key.PublicKey.Equal(cert.PublicKey) {
				certPem := pem.EncodeToMemory(&pem.Block{
					Type:  "CERTIFICATE",
					Bytes: temp,
				})
				keyPem := pem.EncodeToMemory(&pem.Block{
					Type:  "RSA PRIVATE KEY",
					Bytes: x509.MarshalPKCS1PrivateKey(key),
				})
				tlsCert, err := tls.X509KeyPair(certPem, keyPem)
				if err != nil {
					return err
				}
				session.SSL.tlsCertificates = append(session.SSL.tlsCertificates, tlsCert)
			}
		}
	}
	for _, temp := range certRequests {
		cert, err := x509.ParseCertificateRequest(temp)
		if err != nil {
			return err
		}
		session.SSL.CertificateRequest = append(session.SSL.CertificateRequest, cert)
	}
	return nil
}

// negotiate it is a step in SSL communication in which tcp connection is
// used to create sslConn object
func (session *Session) negotiate() {
	connOption := session.Context.connConfig
	host := connOption.ActiveServer(false)

	if tlsConfig := connOption.TLSConfig; tlsConfig != nil {
		// a copy, so that the configuration of the caller does not change,
		// which fixes sijms/go-ora#757
		tlsConfig = tlsConfig.Clone()
		if tlsConfig.ServerName == "" {
			tlsConfig.ServerName = host.Addr
		}
		sslConn := tls.Client(session.conn, tlsConfig)
		session.mu.Lock()
		session.sslConn = sslConn
		session.mu.Unlock()
		return
	}

	if session.SSL.roots == nil && len(session.SSL.Certificates) > 0 {
		session.SSL.roots = x509.NewCertPool()
		for _, cert := range session.SSL.Certificates {
			session.SSL.roots.AddCert(cert)
		}
	}
	config := &tls.Config{
		ServerName: host.Addr,
	}
	if len(session.SSL.tlsCertificates) > 0 {
		config.Certificates = session.SSL.tlsCertificates
	}
	if session.SSL.roots != nil {
		config.RootCAs = session.SSL.roots
	}
	if !connOption.SSLVerify {
		config.InsecureSkipVerify = true
	}
	sslConn := tls.Client(session.conn, config)
	session.mu.Lock()
	session.sslConn = sslConn
	session.mu.Unlock()
}

func (session *Session) ResetBreak() {
	session.mu.Lock()
	session.breakConn = false
	session.mu.Unlock()
	session.breakDeadline.Store(0)
}

// IsBreak tell if the connection break elicit
func (session *Session) IsBreak() bool {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.breakConn
}

// BreakConnection elicit connection break to cancel the current operation
func (session *Session) BreakConnection() error {
	session.logger.Info("break")
	// the break is pending before it goes out, because the reader can read
	// its answer, and clear the flag, before the write returns
	session.mu.Lock()
	session.breakConn = true
	session.mu.Unlock()
	session.breakPending.Store(true)
	err := session.sendBreak()
	if err != nil {
		session.ResetBreak()
		session.breakPending.Store(false)
	}
	return err
}

// sendBreak sends the break, by out of band data where the server supports
// it and by a marker packet where not.
func (session *Session) sendBreak() error {
	if session.Context.NegotiatedOptions&0x400 > 0 {
		done, err := sendOOB(session.conn)
		if err != nil || done {
			return err
		}
	}
	return session.writePacket(newMarkerPacket(markerTypeInterrupt, session.Context))
}

// Connect perform network connection on address:port
// check if the client need to use SSL
// then send connect packet to the server and
// receive either accept, redirect or refuse packet
func (session *Session) Connect(ctx context.Context) error {
	connOption := session.Context.connConfig
	session.ResetBuffer()
	session.Disconnect()
	session.logger.Debug("connect")
	var err error
	connected := false
	var host *config.ServerAddr
	loop := true
	dialer := connOption.Dialer
	if dialer == nil {
		dialer = &net.Dialer{}
		if session.Context.connConfig.ConnectTimeout > 0 {
			dialer = &net.Dialer{
				Timeout: session.Context.connConfig.ConnectTimeout,
			}
		} else {
			dialer = &net.Dialer{}
		}
	}
	for loop {
		host = connOption.ActiveServer(false)
		if host == nil {
			if err != nil {
				return err
			}
			return errors.New("no available servers to connect to")
		}
		addr := host.NetworkAddr()
		var conn net.Conn
		if len(session.Context.connConfig.UnixAddress) > 0 {
			conn, err = dialer.DialContext(ctx, "unix", session.Context.connConfig.UnixAddress)
		} else {
			conn, err = dialer.DialContext(ctx, "tcp", addr)
		}
		// publish conn under the mutex to avoid a data race with
		// Disconnect() which may be called concurrently by the
		// StartContext watchdog when the context is canceled during
		// connection establishment (#736)
		session.mu.Lock()
		session.conn = conn
		session.mu.Unlock()

		if err != nil {
			session.logger.Debug("dial", "address", addr, "error", err)
			host = connOption.ActiveServer(true)
			if host == nil {
				break
			}
			continue
		}
		session.logger.Debug("dial", "address", addr)
		connected = true
		loop = false
	}
	if !connected {
		return err
	}
	err = connOption.UpdateSSL(host)
	if err != nil {
		return err
	}
	if connOption.SSL {
		session.logger.Debug("tls")
		session.negotiate()
		session.reader = bufio.NewReaderSize(session.sslConn, readBufferSize)
	} else {
		session.reader = bufio.NewReaderSize(session.conn, readBufferSize)
	}
	// next tries the next server when the handshake with this one fails. A
	// gateway or a VPN can accept the dial to a port that nothing listens
	// on, and reset the connection after it, sijms/go-ora#619
	next := func(err error) error {
		session.logger.Debug("handshake", "address", host.NetworkAddr(), "error", err)
		session.Disconnect()
		if ctx.Err() != nil || connOption.ActiveServer(true) == nil {
			return err
		}
		return session.Connect(ctx)
	}
	session.logger.Debug("open", "connect data", connOption.ConnectionData())
	connectPacket := newConnectPacket(session.Context)
	err = session.writePacket(connectPacket)
	if err != nil {
		return next(err)
	}
	if uint16(connectPacket.length) == connectPacket.dataOffset {
		session.PutBytes(connectPacket.buffer...)
		err = session.Write()
		if err != nil {
			return next(err)
		}
	}
	pck, err := session.readPacket()
	if err != nil {
		return next(err)
	}

	if acceptPacket, ok := pck.(*AcceptPacket); ok {
		session.mu.Lock()
		*session.Context = *acceptPacket.sessionCtx
		session.Context.handshakeComplete = true
		session.mu.Unlock()
		session.logger.Debug("handshake complete")
		return nil
	}
	if redirectPacket, ok := pck.(*RedirectPacket); ok {
		session.logger.Debug("redirect")
		err = session.Context.connConfig.UpdateDatabaseInfoForRedirect(redirectPacket.redirectAddr, redirectPacket.reconnectData)
		if err != nil {
			return err
		}
		session.Context.connConfig.ResetServerIndex()
		session.Context.isRedirect = true
		return session.Connect(ctx)
	}
	if refusePacket, ok := pck.(*RefusePacket); ok {
		refusePacket.extractErrCode()
		var addr string
		var port int
		if host != nil {
			addr = host.Addr
			port = host.Port
		}
		session.logger.Debug("refused", "address", addr, "port", port, "error", refusePacket.Err)
		host = connOption.ActiveServer(true)
		if host == nil {
			session.Disconnect()
			return refusePacket.Err
		}
		return session.Connect(ctx)
	}
	return next(errors.New("connection refused by the server due to unknown reason"))
}

func (session *Session) WriteFinalPacket() error {
	data, err := newDataPacket(nil, session.Context, &session.mu, 0)
	if err != nil {
		return err
	}
	data.dataFlag = 0x40
	return session.writePacket(data)
}

// Disconnect close the network and release resources
func (session *Session) Disconnect() {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.sslConn != nil {
		_ = session.sslConn.Close()
		session.sslConn = nil
	}
	if session.conn != nil {
		_ = session.conn.Close()
		session.conn = nil
	}
}

func (session *Session) WriteRPC(start, end bool) error {
	dataFlag := uint16(0)
	if start {
		dataFlag |= 0x1000
	}
	if end {
		dataFlag |= 0x800
	}
	outputBytes := session.outBuffer.Bytes()
	size := session.outBuffer.Len()
	if size == 0 {
		// send empty data packet
		pck, err := newDataPacket(nil, session.Context, &session.mu, dataFlag)
		if err != nil {
			return err
		}
		return session.writePacket(pck)
	}

	segmentLen := int(session.Context.SessionDataUnit - 64)
	offset := 0
	if size > segmentLen {
		segment := make([]byte, segmentLen)
		for size > segmentLen {
			copy(segment, outputBytes[offset:offset+segmentLen])
			pck, err := newDataPacket(segment, session.Context, &session.mu, dataFlag)
			if err != nil {
				return err
			}
			err = session.writePacket(pck)
			if err != nil {
				session.outBuffer.Reset()
				return err
			}
			size -= segmentLen
			offset += segmentLen
		}
	}
	if size != 0 {
		pck, err := newDataPacket(outputBytes[offset:], session.Context, &session.mu, dataFlag)
		if err != nil {
			return err
		}
		err = session.writePacket(pck)
		if err != nil {
			session.outBuffer.Reset()
			return err
		}
	}
	return nil
}

// Write send data store in output buffer through network
//
// if data bigger than fSessionDataUnit it should be divided into
// segment and each segment sent in data packet
func (session *Session) Write() error {
	return session.WriteRPC(false, false)
}

func (session *Session) processMarker() error {
	var err error
	// the reset answers a break
	session.breakPending.Store(false)
	// send reset connection
	session.resetWrite()
	marker := newMarkerPacket(markerTypeReset, session.Context)
	err = session.writePacket(marker)
	if err != nil {
		return err
	}
	if session.Context.nego != nil {
		err = session.Context.nego.Reset()
		if err != nil {
			return err
		}
	}
	_, err = session.readPacket()
	if err != nil {
		return err
	}

	return nil

}

// Read numBytes of data from input buffer if requested data is larger
// than input buffer session will get the remaining from network stream
func (session *Session) read(numBytes int) ([]byte, error) {
	var err error
	var pck packet
	ret := make([]byte, numBytes)
	index := 0
	actualRead, err := session.inBuffer.Read(ret)
	if err != nil && err != io.EOF {
		return nil, err
	}
	numBytes -= actualRead
	index += actualRead
	for numBytes > 0 {
		// this mean we need to add more data in the buffer

		// if break connection send break message
		if session.IsBreak() {
			session.ResetBreak()
		}
		pck, err = session.readPacket()
		if err != nil {
			return nil, err
		}

		// pck == nil means successful read data packet
		if pck == nil {
			actualRead, err = session.inBuffer.Read(ret[index:])
			numBytes -= actualRead
			index += actualRead
			continue
		}
		if _, ok := pck.(*MarkerPacket); ok {
			err = session.processMarker()
			if err != nil {
				return nil, err
			}
			return nil, ErrConnReset
		}

		return nil, fmt.Errorf("receive abnormal packet type %d instead of data packet", pck.getPacketType())
	}
	return ret, nil
}

// Write a packet to the network stream
func (session *Session) writePacket(pck packet) error {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.sendPcks = append(session.sendPcks, pck)
	tmp := pck.bytes()
	session.LogPacket("write packet", tmp)
	err := session.initWrite()
	if err != nil {
		return err
	}
	if session.sslConn != nil {
		_, err = session.sslConn.Write(tmp)
	} else if session.conn != nil {
		_, err = session.conn.Write(tmp)
	} else {
		return errors.New("attempt to write on closed connection")
	}
	return err
}

// HasError reports whether the last summary holds an error. ORA-01403, no
// data found, is not one: the server sends it with the last rows of a fetch,
// to say that the cursor has no more.
func (session *Session) HasError() bool {
	return session.Summary != nil && (session.Summary.RetCode != 0 && session.Summary.RetCode != 1403)
}

// BatchError returns the errors of the rows of a batch that failed, or nil.
func (session *Session) BatchError() *BatchError {
	if !session.Summary.HasBatchErrors() {
		return nil
	}
	be := &BatchError{}
	for _, e := range session.Summary.bindErrors {
		oerr := &OracleError{ErrCode: e.errorCode, errPos: -1}
		if session.StrConv != nil {
			oerr.ErrMsg = session.StrConv.Decode(e.errorMsg)
		} else {
			oerr.ErrMsg = string(e.errorMsg)
		}
		be.Rows = append(be.Rows, RowError{Row: e.rowOffset, Err: oerr})
	}
	return be
}

// ServerError Return the error in form or OracleError
func (session *Session) ServerError() *OracleError {
	err := &OracleError{}
	if session.HasError() {
		err.ErrCode = session.Summary.RetCode
		if session.StrConv != nil {
			err.ErrMsg = session.StrConv.Decode(session.Summary.ErrorMessage)
		} else {
			err.ErrMsg = string(session.Summary.ErrorMessage)
		}
		err.errPos = session.Summary.errorPos
	}
	return err
}

func (session *Session) readAll(size int) error {
	index := 0
	tempBuffer := make([]byte, size)
	var err error
	var temp int
	session.remainingBytes = size
	for index < size {
		if session.conn == nil && session.sslConn == nil {
			return errors.New("closed connection")
		}
		err = session.initRead()
		if err != nil {
			return err
		}
		temp, err = session.reader.Read(tempBuffer[index:])
		if err != nil {
			if temp > 0 {
				session.remainingBytes -= temp
				index += temp
			}
			session.lastPacket.Write(tempBuffer[:index])
			return err
		}
		session.remainingBytes -= temp
		index += temp
	}
	session.lastPacket.Write(tempBuffer)
	return nil
}

// readPacketData
func (session *Session) readPacketData() error {
	var err error
	if session.remainingBytes > 0 {
		if session.lastPacket.Len() < 8 { // means break occur inside head
		}
		err = session.readAll(session.remainingBytes)
		if err != nil {
			return err
		}
		session.remainingBytes = 0
		return nil
	}
	session.lastPacket.Reset()
	err = session.readAll(8)
	if err != nil {
		// if remaining bytes = 8 means no data read so make it 0
		if session.remainingBytes == 8 {
			session.remainingBytes = 0
		}
		return err
	}
	head := session.lastPacket.Bytes()
	var length uint32

	if session.Context.handshakeComplete && session.Context.Version >= 315 {
		length = binary.BigEndian.Uint32(head)
	} else {
		length = uint32(binary.BigEndian.Uint16(head))
	}
	length -= 8
	err = session.readAll(int(length))
	if err != nil {
		return err
	}
	session.LogPacket("read packet", session.lastPacket.Bytes())
	return nil
}

// read a packet from network stream
func (session *Session) readPacket() (packet, error) {
	var err error
	err = session.readPacketData()
	if err != nil {
		return nil, err
	}
	packetData := session.lastPacket.Bytes()
	pckType := PacketType(packetData[4])
	flag := packetData[5]
	switch pckType {
	case RESEND:
		if session.Context.connConfig.SSL && flag&8 != 0 {
			session.negotiate()
			session.reader = bufio.NewReaderSize(session.sslConn, readBufferSize)
		}
		resend := func() error {
			session.mu.Lock()
			defer session.mu.Unlock()
			for _, pck := range session.sendPcks {
				err := session.initWrite()
				if err != nil {
					return err
				}
				if session.sslConn != nil {
					_, err = session.sslConn.Write(pck.bytes())
				} else if session.conn != nil {
					_, err = session.conn.Write(pck.bytes())
				} else {
					err = errors.New("attempt to write on closed connection")
				}
				if err != nil {
					return err
				}
			}
			return nil
		}
		err = resend()
		if err != nil {
			return nil, err
		}
		return session.readPacket()
	case ACCEPT:
		// packet constructors return nil on malformed input; without this
		// check the typed-nil interface passes the caller's type assertion
		// and panics on first field access
		acceptPck := newAcceptPacketFromData(packetData, session.Context.connConfig)
		if acceptPck == nil {
			return nil, fmt.Errorf("invalid accept packet received from server")
		}
		return acceptPck, nil
	case REFUSE:
		refusePck := newRefusePacketFromData(packetData)
		if refusePck == nil {
			return nil, fmt.Errorf("invalid refuse packet received from server")
		}
		return refusePck, nil
	case REDIRECT:
		pck := newRedirectPacketFromData(packetData)
		if pck == nil {
			return nil, fmt.Errorf("invalid redirect packet received from server")
		}
		dataLen := binary.BigEndian.Uint16(packetData[8:])
		var data string
		if uint16(pck.length) <= pck.dataOffset {
			err = session.readPacketData()
			packetData = session.lastPacket.Bytes()
			dataPck, err := newDataPacketFromData(packetData, session.Context, &session.mu)
			if err != nil {
				return nil, err
			}
			data = string(dataPck.buffer)
		} else {
			data = string(packetData[10 : 10+dataLen])
		}
		length := strings.Index(data, "\x00")
		if pck.flag&2 != 0 && length > 0 {
			pck.redirectAddr = data[:length]
			pck.reconnectData = data[length:]
		} else {
			pck.redirectAddr = data
		}
		return pck, nil
	case DATA:
		dataPck, err := newDataPacketFromData(packetData, session.Context, &session.mu)
		if dataPck != nil {
			if session.Context.connConfig.SSL && (dataPck.dataFlag&0x8000 > 0 || dataPck.flag&0x80 > 0) {
				session.negotiate()
				session.reader = bufio.NewReaderSize(session.sslConn, readBufferSize)
			}
			if dataPck.dataFlag == 0x40 {
				// close connection
				session.Disconnect()
				return nil, driver.ErrBadConn
			}
			_, err = session.inBuffer.Write(dataPck.buffer)
		}
		return nil, err
	case MARKER:
		markerPck := newMarkerPacketFromData(packetData, session.Context)
		if markerPck == nil {
			return nil, fmt.Errorf("invalid marker packet received from server")
		}
		return markerPck, nil
	default:
		return nil, fmt.Errorf("unsupported packet type: %d", pckType)
	}
}

// Logger returns the logger of the session, which is never nil.
func (session *Session) Logger() *slog.Logger {
	return session.logger
}

// HidePackets withholds the bytes of each packet from the logger while hide
// is true. A connection sets it until it has logged on.
func (session *Session) HidePackets(hide bool) {
	session.hide.Store(hide)
}

// LogPacket writes p to the logger at the debug level, or only its length
// while packets are hidden.
func (session *Session) LogPacket(msg string, p []byte) {
	if session.hide.Load() {
		session.logger.Debug(msg, "length", len(p), "bytes", "withheld")
		return
	}
	session.logger.Debug(msg, "length", len(p), "bytes", packetDump(p))
}

// packetDump formats a packet as hex.Dump does, and only when a handler
// writes it.
type packetDump []byte

func (p packetDump) LogValue() slog.Value {
	return slog.StringValue(hex.Dump(p))
}
