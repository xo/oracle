package ttc

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/xo/oracle/internal/codec"
	"github.com/xo/oracle/internal/config"
	"github.com/xo/oracle/internal/types"

	"github.com/xo/oracle/internal/auth"
	"github.com/xo/oracle/internal/charset"
	"github.com/xo/oracle/internal/tns"
)

type ConnectionState int

const (
	Closed ConnectionState = 0
	Opened ConnectionState = 1
)

type LogonMode int

const (
	NoNewPass   LogonMode = 0x1
	SysDba      LogonMode = 0x20
	SysOper     LogonMode = 0x40
	SysAsm      LogonMode = 0x00400000
	SysBackup   LogonMode = 0x01000000
	SysDg       LogonMode = 0x02000000
	SysKm       LogonMode = 0x04000000
	SysRac      LogonMode = 0x08000000
	UserAndPass LogonMode = 0x100
	WithNewPass LogonMode = 0x2
	PROXY       LogonMode = 0x400
)

// from GODROR
const wrapResultset = "--WRAP-RESULTSET--"

const createLob = "--CREATE-LOB-STREAM--"
const connRef = "--GET-CONNECTION-REF--"

// Querier is the QueryContext of sql.Conn.
type Querier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// ///

type Conn struct {
	State      ConnectionState
	LogonMode  LogonMode
	autoCommit bool
	// txnInProgress is the flag of the end of a call that says that a
	// transaction is open
	txnInProgress bool
	logger        *slog.Logger
	connOption    *config.ConnectionConfig
	session       *tns.Session
	tcpNego       *TCPNego
	dataNego      *DataTypeNego
	// negotiated is true when the replies of the negotiations that a fast
	// logon sends with its first message have been read.
	negotiated        bool
	authObject        *AuthObject
	SessionProperties map[string]string
	dBVersion         *DBVersion
	sessionID         int
	serialID          int
	transactionID     []byte
	sStrConv          *charset.Converter
	nStrConv          *charset.Converter
	cStrConv          *charset.Converter
	goTypeCoder       map[reflect.Type]codec.OracleParameterCoder
	oracleTypeCoder   map[types.TypeID]codec.OracleParameterCoder
	nameTypeCoder     map[string]codec.OracleParameterCoder
	//typeDecoder       map[uint16]type_coder.OracleTypeDecoder
	maxLen struct {
		varchar   int64
		nvarchar  int64
		raw       int64
		number    int64
		date      int64
		timestamp int64
	}
	bad bool
	// dbTimeZone is the zone of the database, which a TIMESTAMP WITH LOCAL
	// TIME ZONE is in, and dbTimeZoneErr the error of a zone that did not
	// load
	dbTimeZone       *time.Location
	dbTimeZoneErr    error
	temporaryLocs    []types.Locator
	token            []byte
	tokenPrivateKey  []byte
	connectionCookie *ConnectionCookie
	cookies          *cookieJar
	// lobFetch is the mode of LOBs of the DSN. connOption.Lob is the mode of
	// the statement that runs, which an option can change (D109)
	lobFetch config.LobFetch
	newKey   []byte
}

func (conn *Conn) buildParameterCoderMap(drv *Registry) {
	conn.goTypeCoder = drv.goTypeCoder
	conn.nameTypeCoder = drv.nameTypeCoder
	conn.oracleTypeCoder = drv.oracleTypeCoder

}

// Prepare take a query string and create a stmt object
func (conn *Conn) Prepare(query string) (driver.Stmt, error) {
	conn.logger.Debug("prepare", "query", query)
	return NewStmt(query, conn), nil
}

// Ping test if connection is online
func (conn *Conn) Ping(ctx context.Context) error {
	conn.logger.Debug("ping")
	if !conn.IsValid() {
		return driver.ErrBadConn
	}
	conn.session.ResetBuffer()
	done := conn.session.StartContext(ctx)
	defer conn.session.EndContext(done)
	return (&simpleObject{
		connection:  conn,
		operationID: 0x93,
		data:        nil,
	}).exec()
}

func (conn *Conn) MaxStringLength() int64 {
	if conn.maxLen.varchar == 0 {
		return 0x7FFF
	}
	return conn.maxLen.varchar
}

// restoreLobFetch puts back the mode of LOBs of the DSN after a statement.
func (conn *Conn) restoreLobFetch() {
	conn.connOption.Lob = conn.lobFetch
}

// ServerRelease returns the major release of the server, such as 11 or 23.
func (conn *Conn) ServerRelease() int {
	if conn.dBVersion == nil {
		return 0
	}
	return conn.dBVersion.MajorVersion
}

func (conn *Conn) MaxRawLength() int64 {
	if conn.maxLen.raw == 0 {
		return 0x7FFF
	}
	return conn.maxLen.raw
}
func (conn *Conn) getDefaultCharsetID() int {

	return conn.tcpNego.ServerCharset
}
func (conn *Conn) DefaultStringCoder() (*charset.Converter, error) {
	return conn.StringCoder(conn.getDefaultCharsetID(), 1)
}

func (conn *Conn) StringCoder(charsetID, charsetForm int) (*charset.Converter, error) {
	errStringCoder := fmt.Errorf("server requested charset id: %d which is not supported by the driver", charsetID)
	if charsetID == 0 {
		if charsetForm == 1 {
			if conn.cStrConv != nil {
				return conn.cStrConv, nil
			}
			if conn.sStrConv != nil {
				return conn.sStrConv, nil
			}
		} else {
			if conn.nStrConv != nil {
				return conn.nStrConv, nil
			}
		}
		return nil, errStringCoder
	}
	if conn.cStrConv != nil && charsetID == conn.cStrConv.LangID {
		return conn.cStrConv, nil
	}
	if conn.sStrConv != nil && charsetID == conn.sStrConv.LangID {
		if conn.cStrConv != nil {
			return conn.cStrConv, nil
		}
		return conn.sStrConv, nil
	}

	if conn.nStrConv != nil && charsetID == conn.nStrConv.LangID {
		return conn.nStrConv, nil
	}
	temp := charset.New(charsetID)
	if temp == nil {
		return nil, errStringCoder
	}
	return temp, nil
}

func (conn *Conn) ServerStringCoder() *charset.Converter {
	return conn.sStrConv
}
func (conn *Conn) ServerNStringCoder() *charset.Converter {
	return conn.nStrConv
}
func (conn *Conn) Logoff() error {
	conn.logger.Debug("logoff")
	session := conn.session
	session.ResetBuffer()
	session.PutTTCFunc(3, 9)
	err := session.Write()
	if err != nil {
		return err
	}
	return conn.read()
}

func (conn *Conn) read() error {
	loop := true
	session := conn.session
	for loop {
		msg, err := session.ReadByte()
		if err != nil {
			return err
		}
		err = conn.ProcessTCCResponse(msg)
		if err != nil {
			return err
		}
		if msg == 4 || msg == 9 {
			loop = false
		}
	}
	return nil
}

// Open the connection = bring it online

// OpenWithContext open the connection with timeout context
func (conn *Conn) OpenWithContext(ctx context.Context) error {
	logger := conn.logger
	switch conn.connOption.DBAPrivilege {
	case config.SYSDBA:
		conn.LogonMode |= SysDba
	case config.SYSOPER:
		conn.LogonMode |= SysOper
	case config.SYSASM:
		conn.LogonMode |= SysAsm
	case config.SYSBACKUP:
		conn.LogonMode |= SysBackup
	case config.SYSDG:
		conn.LogonMode |= SysDg
	case config.SYSKM:
		conn.LogonMode |= SysKm
	case config.SYSRAC:
		conn.LogonMode |= SysRac
	default:
		conn.LogonMode = 0
	}
	conn.connOption.ResetServerIndex()
	conn.session = tns.NewSession(conn.connOption, conn.logger)
	conn.session.Drain = conn.drainBreak
	// the packets of the logon carry the encrypted session key and password,
	// so the logger sees only their length until the logon ends (D14)
	conn.session.HidePackets(true)
	W := conn.connOption.Wallet
	if conn.connOption.SSL && W != nil {
		err := conn.session.LoadSSLData(W.Certificates, W.PrivateKeys, W.CertificateRequests)
		if err != nil {
			return err
		}
	}
	session := conn.session
	// start check for context
	done := session.StartContext(ctx)
	defer session.EndContext(done)
	err := session.Connect(ctx)
	if err != nil {
		return err
	}
	// advanced negotiation
	var ano *auth.AdvNego = nil
	if session.Context.ACFL0&1 != 0 && session.Context.ACFL0&4 == 0 && session.Context.ACFL1&8 == 0 {
		logger.Debug("advanced negotiation")
		ano, err = auth.NewAdvNego(session, conn.connOption)
		if err != nil {
			return err
		}
		err = ano.Write()
		if err != nil {
			return err
		}
		err = ano.Read()
		if err != nil {
			return err
		}
		err = ano.StartServices()
		if err != nil {
			return err
		}
	}
	conn.session.Context.SetAdvancedNegotiator(ano)
	// if fast login enabled this means new features are available
	if conn.session.Context.FastAuthEnabled {
		session.UseBigClrChunks = true
		session.ClrChunkSize = 0x7FFF
		session.TTCVersion = 24
		session.HasFSAPCapability = true
	}
	if conn.isFastLoginEnabled() {
		logger.Debug("fast authentication")
		session.ResetBuffer()
		conn.negotiated = false
		cookie := lookupCookie(conn)
		if cookie != nil {
			conn.connectionCookie = cookie
			logger.Debug("fast authentication with a cookie")
			conn.tcpNego = &TCPNego{
				conn:                  conn,
				ServerFlags:           cookie.ServerFlags,
				ServerCharset:         cookie.ServerCharset,
				ServerNCharset:        cookie.ServerNCharset,
				OracleVersion:         cookie.OracleVersion,
				ProtocolServerString:  cookie.ProtocolServerString,
				ServerCompileTimeCaps: cookie.ServerCompileTimeCaps,
				ServerRuntimeCaps:     cookie.ServerRuntimeCaps,
			}
			if conn.sStrConv == nil {
				conn.sStrConv = charset.New(cookie.ServerCharset)
			}
			if conn.nStrConv == nil {
				conn.nStrConv = charset.New(cookie.ServerNCharset)
			}
			session.StrConv = conn.sStrConv
			session.PutBytes(0x22, 1, 0, 0)
			session.PutBytes([]byte("OracleClientGo\x00")...)
			cookie.writeTTICookie(session)
			conn.dataNego = buildTypeNego(conn.tcpNego, conn)
			conn.dataNego.writeMessage()
		} else {
			logger.Debug("fast authentication with no cookie")
			session.PutBytes(0x22, 1, 0, 0)
			conn.tcpNego = &TCPNego{conn: conn, ServerFlags: 3, ServerCharset: 0x369, ServerNCharset: 0x7D0}
			conn.tcpNego.writeMessage()
			session.PutUint(conn.tcpNego.ServerCharset, 2, false, false)
			session.PutBytes(conn.tcpNego.ServerFlags)
			session.PutUint(conn.tcpNego.ServerNCharset, 2, false, false)
			session.PutBytes(session.TTCVersion)
			conn.dataNego = buildTypeNego(conn.tcpNego, conn)
			conn.dataNego.writeMessage()
		}
	} else {
		err = conn.protocolNegotiation()
		if err != nil {
			return err
		}
		err = conn.dataTypeNegotiation()
		if err != nil {
			return err
		}

	}

	err = conn.doAuth()
	if errors.Is(err, tns.ErrConnReset) {
		// a server that refuses the logon sends a reset and then the error,
		// after the replies of the negotiations or in place of them
		if conn.isFastLoginEnabled() {
			err = conn.readNegotiation()
			if err != nil {
				return err
			}
		}
		err = conn.read()
	}
	if conn.connectionCookie != nil && (errors.Is(err, driver.ErrBadConn) || errors.Is(err, io.EOF)) {
		// if cookie based remove cookie and reconnect
		logger.Info("reconnect without the fast login cookie", "error", err)
		deleteCookie(conn)
		conn.connectionCookie = nil
		if conn.session != nil {
			conn.session.Disconnect()
		}
		return conn.OpenWithContext(ctx)
	}
	if err != nil {
		return err
	}
	if ano != nil {
		externalAuth := len(conn.connOption.UserID) == 0 || len(conn.connOption.Password) == 0
		err = ano.SetKeyFolding(conn.newKey, externalAuth, conn.LogonMode&0x20 > 0 || conn.LogonMode&0x40 > 0)
		if err != nil {
			return err
		}
	}

	conn.State = Opened
	conn.session.ServerFlags = conn.tcpNego.ServerFlags
	if conn.connectionCookie == nil && conn.isFastLoginEnabled() && conn.tcpNego != nil {
		saveCookie(conn)
	}
	conn.dBVersion, err = ReadDBVersion(conn.session)
	if err != nil {
		return err
	}
	conn.session.SetConnected()
	conn.session.HidePackets(false)
	logger.Debug("connected", "version", conn.dBVersion.Text)
	sessionID, err := strconv.ParseUint(conn.SessionProperties["AUTH_SESSION_ID"], 10, 32)
	if err != nil {
		return err
	}
	conn.sessionID = int(sessionID)
	serialNum, err := strconv.ParseUint(conn.SessionProperties["AUTH_SERIAL_NUM"], 10, 32)
	if err != nil {
		return err
	}
	conn.serialID = int(serialNum)
	conn.connOption.InstanceName = conn.SessionProperties["AUTH_SC_INSTANCE_NAME"]
	conn.connOption.ServiceName = conn.SessionProperties["AUTH_SC_SERVICE_NAME"]
	conn.connOption.DomainName = conn.SessionProperties["AUTH_SC_DB_DOMAIN"]
	conn.connOption.DBName = conn.SessionProperties["AUTH_SC_DBUNIQUE_NAME"]
	return nil
}

// Begin is the form of BeginTx with no context, which database/sql does not
// call, because Conn implements driver.ConnBeginTx.
func (conn *Conn) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("beginning a transaction without a context: %w", tns.ErrNotSupported)
}

// BeginTx begins a transaction. Oracle has read committed, its default, and
// serializable, and a read only transaction. Any other isolation level fails
// with an error that wraps ErrNotSupported. A read only transaction is read
// committed, because Oracle has no read only serializable transaction.
func (conn *Conn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	var stmt string
	switch sql.IsolationLevel(opts.Isolation) {
	case sql.LevelDefault, sql.LevelReadCommitted:
		if opts.ReadOnly {
			stmt = "SET TRANSACTION READ ONLY"
		}
	case sql.LevelSerializable:
		if opts.ReadOnly {
			return nil, fmt.Errorf("beginning a read only serializable transaction: %w", tns.ErrNotSupported)
		}
		stmt = "SET TRANSACTION ISOLATION LEVEL SERIALIZABLE"
	default:
		return nil, fmt.Errorf("beginning a transaction with the isolation level %v: %w", sql.IsolationLevel(opts.Isolation), tns.ErrNotSupported)
	}
	conn.logger.Debug("begin")
	conn.autoCommit = false
	if stmt != "" {
		if _, err := conn.ExecContext(ctx, stmt, nil); err != nil {
			conn.autoCommit = true
			return nil, fmt.Errorf("beginning a transaction: %w", err)
		}
	}
	return &Tx{conn: conn}, nil
}

// NewConnection create a new connection from databaseURL string or configuration
func NewConnection(cfg *config.ConnectionConfig) (*Conn, error) {
	var err error
	if cfg == nil {
		return nil, errors.New("database url or configuration is required")
	}
	var token, tokenPrivateKey []byte
	if len(cfg.TokenFile) > 0 {
		token, err = os.ReadFile(cfg.TokenFile)
		if err != nil {
			return nil, err
		}
	}
	if len(cfg.TokenPrivateKeyFile) > 0 {
		tokenPrivateKey, err = os.ReadFile(cfg.TokenPrivateKeyFile)
		if err != nil {
			return nil, err
		}
	}
	if len(token) > 0 && len(tokenPrivateKey) == 0 {
		return nil, errors.New("token private key file is required")
	}
	temp := new(config.ConnectionConfig)
	*temp = *cfg
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Conn{
		State:           Closed,
		connOption:      temp,
		cStrConv:        charset.New(cfg.CharsetID),
		logger:          logger,
		lobFetch:        cfg.Lob,
		autoCommit:      true,
		oracleTypeCoder: nil,
		maxLen: struct {
			varchar   int64
			nvarchar  int64
			raw       int64
			number    int64
			date      int64
			timestamp int64
		}{varchar: 0x7FFF, nvarchar: 0x7FFF, raw: 0x7FFF, number: 0x16, date: 0xB, timestamp: 0xB},
		token:           token,
		tokenPrivateKey: tokenPrivateKey,
	}, nil
}

func (conn *Conn) freeTemporaryLobs() error {
	if len(conn.temporaryLocs) == 0 {
		return nil
	}
	conn.logger.Debug("free temporary LOBs", "count", len(conn.temporaryLocs))
	session := conn.session
	freeTemp := func(locators []types.Locator) {
		totalLen := 0
		for _, locator := range locators {
			totalLen += len(locator)
		}
		session.PutTTCFunc(0x11, 0x60)
		session.PutBytes(1)
		session.PutUint(totalLen, 4, true, true)
		session.PutBytes(0, 0, 0, 0, 0, 0, 0)
		session.PutUint(0x80111, 4, true, true)
		session.PutBytes(0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
		for _, locator := range locators {
			session.PutBytes(locator...)
		}
	}
	start := 0
	end := 0
	session.ResetBuffer()
	for start < len(conn.temporaryLocs) {
		end = min(start+25000, len(conn.temporaryLocs))
		freeTemp(conn.temporaryLocs[start:end])
		start += end
	}
	session.PutTTCFunc(0x3, 0x93)
	err := session.Write()
	if err != nil {
		return err
	}
	err = conn.read()
	if err == nil {
		conn.temporaryLocs = nil
	}
	return err
}

// Close the connection by disconnect network session
// drainBreak reads the answer of a break that came after the call that it
// broke had ended: the marker, the reset, and ORA-01013, which it drops. A
// read that fails makes the connection bad.
func (conn *Conn) drainBreak() error {
	err := conn.read()
	if errors.Is(err, tns.ErrConnReset) {
		err = conn.read()
	}
	var oerr *tns.OracleError
	if err == nil || errors.As(err, &oerr) && oerr.ErrCode == 1013 {
		return nil
	}
	conn.setBad()
	return err
}

// eocsTxnInProgress is the flag of the end of a call that says that a
// transaction is open.
const eocsTxnInProgress = 0x2

func (conn *Conn) Close() (err error) {
	err = conn.freeTemporaryLobs()
	if err != nil {
		conn.logger.Debug("freeing temporary LOBs", "error", err)
	}
	logger := conn.logger
	logger.Debug("close")
	// the server commits an open transaction at a logoff, so a transaction
	// that the server says is open, or that BeginTx began, is rolled back
	// first, as python-oracledb does. go-ora committed it
	if conn.session != nil && conn.IsValid() && (conn.txnInProgress || !conn.autoCommit) {
		conn.session.ResetBuffer()
		if err := (&simpleObject{connection: conn, operationID: 0xF}).exec(); err != nil {
			logger.Debug("rollback at close", "error", err)
		}
	}
	if conn.session != nil {
		err = conn.Logoff()
		if err != nil {
			logger.Debug("logoff", "error", err)
		}
		conn.session.Disconnect()
		conn.session = nil
	}
	conn.State = Closed
	conn.logger.Debug("closed")
	return
}

func (conn *Conn) isFastLoginEnabled() bool {
	return conn.session.Context.FastAuthEnabled && conn.connOption.FastLogin
}

// doAuth a login step that occur during open connection
// readNegotiation reads the replies of the protocol and data type
// negotiations that a fast logon sends with its first message, once. A logon
// with a cookie gets no reply to the protocol negotiation.
func (conn *Conn) readNegotiation() error {
	if conn.negotiated {
		return nil
	}
	if conn.connectionCookie == nil {
		err := conn.tcpNego.readMessage()
		if err != nil {
			return err
		}
		conn.tcpNego.ServerFlags |= 2
	}
	conn.dataNego = buildTypeNego(conn.tcpNego, conn)
	err := conn.dataNego.read()
	if err != nil {
		return err
	}
	conn.negotiated = true
	return nil
}

func (conn *Conn) doAuth() error {
	conn.logger.Debug("authenticate")
	authObject := &AuthObject{
		conn:       conn,
		tcpNego:    conn.tcpNego,
		usePadding: false,
	}
	if !conn.isFastLoginEnabled() {
		conn.session.ResetBuffer()
	}
	if len(conn.connOption.UserID) > 0 && len(conn.connOption.Password) > 0 {
		conn.session.PutTTCFunc(0x3, 0x76)
		conn.session.PutBytes(1)
		conn.session.PutUint(len(conn.connOption.UserID), 4, true, true)
		conn.LogonMode = conn.LogonMode | NoNewPass
		conn.session.PutUint(int(conn.LogonMode), 4, true, true)
		conn.session.PutBytes(1, 1, 5, 1, 1)
		if len(conn.connOption.UserID) > 0 {
			conn.session.PutString(conn.connOption.UserID)
		}
		conn.session.PutKeyValString("AUTH_TERMINAL", conn.connOption.ClientInfo.TerminalName(), 0)
		conn.session.PutKeyValString("AUTH_PROGRAM_NM", conn.connOption.ClientInfo.ProgramName, 0)
		conn.session.PutKeyValString("AUTH_MACHINE", conn.connOption.ClientInfo.HostName, 0)
		conn.session.PutKeyValString("AUTH_PID", fmt.Sprintf("%d", conn.connOption.ClientInfo.PID), 0)
		conn.session.PutKeyValString("AUTH_SID", conn.connOption.ClientInfo.OSUserName, 0)
		err := conn.session.Write()
		if err != nil {
			return err
		}

		if conn.isFastLoginEnabled() {
			err = conn.readNegotiation()
			if err != nil {
				return err
			}
		}
		err = authObject.read()
		if err != nil {
			return err
		}
		conn.session.ResetBuffer()
	}
	// if proxyAuth ==> mode |= PROXY
	err := authObject.Write()
	if err != nil {
		return err
	}
	if (len(conn.connOption.UserID) == 0 || len(conn.connOption.Password) == 0) && conn.isFastLoginEnabled() {
		err = conn.readNegotiation()
		if err != nil {
			return err
		}
	}
	stop := false
	for !stop {
		msg, err := conn.session.ReadByte()
		if err != nil {
			return err
		}
		switch msg {
		case 8:
			dictLen, err := conn.session.ReadInt(2, true, true)
			if err != nil {
				return err
			}
			conn.SessionProperties = make(map[string]string, dictLen)
			for range dictLen {
				key, val, _, err := conn.session.ReadKeyVal()
				if err != nil {
					return err
				}
				conn.SessionProperties[string(key)] = string(val)
			}
		default:
			err = conn.ProcessTCCResponse(msg)
			if err != nil {
				return err
			}
			if msg == 4 || msg == 9 {
				stop = true
			}
		}
	}

	return nil
}

// loadNLSData get nls data for v2
func (conn *Conn) loadNLSData() error {
	_, err := conn.session.ReadInt(2, true, true)
	if err != nil {
		return err
	}
	_, err = conn.session.ReadByte()
	if err != nil {
		return err
	}
	length, err := conn.session.ReadInt(4, true, true)
	if err != nil {
		return err
	}
	_, err = conn.session.ReadByte()
	if err != nil {
		return err
	}
	// the NLS settings, which nothing reads
	for range length {
		_, _, _, err := conn.session.ReadKeyVal()
		if err != nil {
			return err
		}
	}
	_, err = conn.session.ReadInt(4, true, true)
	return err
}

func (conn *Conn) getServerNetworkInformation(code uint8) error {
	session := conn.session
	if code == 0 {
		_, err := session.ReadByte()
		return err
	}
	switch code - 1 {
	case 1:
		// receive OCOSPID
		length, err := session.ReadInt(2, true, true)
		if err != nil {
			return err
		}
		_, err = session.ReadByte()
		if err != nil {
			return err
		}
		_, err = session.ReadBytes(length)
		if err != nil {
			return err
		}
	case 3:
		// receive OCSESSRET session return values
		_, err := session.ReadInt(2, true, true)
		if err != nil {
			return err
		}
		_, err = session.ReadByte()
		if err != nil {
			return err
		}
		length, err := session.ReadInt(2, true, true)
		if err != nil {
			return err
		}
		// the NLS settings, which nothing reads
		for range length {
			_, _, _, err := session.ReadKeyVal()
			if err != nil {
				return err
			}
		}
		flag, err := session.ReadInt(4, true, true)
		if err != nil {
			return err
		}
		sessionID, err := session.ReadInt(4, true, true)
		if err != nil {
			return err
		}
		serialID, err := session.ReadInt(2, true, true)
		if err != nil {
			return err
		}
		if flag&4 == 4 {
			conn.sessionID = sessionID
			conn.serialID = serialID
			// save session id and serial number to connection
		}
	case 4:
		err := conn.loadNLSData()
		if err != nil {
			return err
		}
	case 6:
		length, err := session.ReadInt(4, true, true)
		if err != nil {
			return err
		}
		conn.transactionID, err = session.ReadCLR()
		if len(conn.transactionID) > length {
			conn.transactionID = conn.transactionID[:length]
		}
	case 7:
		_, err := session.ReadInt(2, true, true)
		if err != nil {
			return err
		}
		_, err = session.ReadByte()
		if err != nil {
			return err
		}
		_, err = session.ReadInt(4, true, true)
		if err != nil {
			return err
		}
		_, err = session.ReadInt(4, true, true)
		if err != nil {
			return err
		}
		_, err = session.ReadByte()
		if err != nil {
			return err
		}
		_, err = session.ReadDLC()
		if err != nil {
			return err
		}
	case 8:
		_, err := session.ReadInt(2, true, true)
		if err != nil {
			return err
		}
		_, err = session.ReadByte()
		if err != nil {
			return err
		}
	}
	return nil
}

func (conn *Conn) Exec(text string, args ...driver.Value) (driver.Result, error) {
	stmt := NewStmt(text, conn)
	defer func() {
		_ = stmt.Close()
	}()
	return stmt.Exec(args)
}

func (conn *Conn) CheckNamedValue(nv *driver.NamedValue) error {
	// an option goes to the statement as it is (D109)
	if IsOption(nv.Value) {
		return nil
	}
	// a decimal of another package binds as a NUMBER, with every digit,
	// before its Value
	if d, ok, err := decimalArg(nv.Value); ok || err != nil {
		nv.Value = d
		return err
	}
	// database/sql converts a driver.Valuer, such as sql.Null[T], to its
	// value, except the few that the driver binds itself
	if _, ok := nv.Value.(driver.Valuer); ok && !ownValue(nv.Value) {
		return driver.ErrSkip
	}
	if namedBasic(nv.Value) {
		return driver.ErrSkip
	}

	return nil
}

func (conn *Conn) QueryRowContext(ctx context.Context, query string, args []driver.NamedValue) *Rows {
	stmt := NewStmt(query, conn)
	stmt.autoClose = true
	rows, err := stmt.QueryContext(ctx, args)
	if err != nil {
		dataSet := &Rows{}
		dataSet.currentResultSet().lastErr = err
		return dataSet
	}
	dataSet, ok := rows.(*Rows)
	if !ok {
		dataSet.currentResultSet().lastErr = fmt.Errorf("expected Rows, got %T", rows)
		return dataSet
	}
	dataSet.Advance()
	return dataSet
}

func WrapRefCursor(ctx context.Context, q Querier, cursor *RefCursor) (*sql.Rows, error) {
	rows, err := cursor.Query()
	if err != nil {
		return nil, err
	}
	return q.QueryContext(ctx, wrapResultset, rows)
}

func (conn *Conn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if query == createLob {
		if len(args) != 1 {
			return nil, fmt.Errorf("CreateLob: expected 1 arguments, got %d", len(args))
		}
		if temp, ok := args[0].Value.(*types.LobStreamer); ok {
			*temp = &LobStream{
				conn: conn,
			}
		} else {
			return nil, fmt.Errorf("CreateLob: unsupported data type %T", args[0].Value)
		}
		return nil, nil
	}
	if query == connRef {
		if len(args) != 1 {
			return nil, fmt.Errorf("GetConnectionRef: expected 1 arguments, got %d", len(args))
		}
		tempVal := reflect.ValueOf(args[0].Value)
		tempVal.Elem().Set(reflect.ValueOf(conn))
		return nil, nil
	}
	stmt := NewStmt(query, conn)
	stmt.autoClose = true
	result, err := stmt.ExecContext(ctx, args)
	if err != nil {
		_ = stmt.Close()
		return nil, err
	}
	err = stmt.Close()
	return result, err
}

func (conn *Conn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if query == wrapResultset {
		return args[0].Value.(driver.Rows), nil
	}
	stmt := NewStmt(query, conn)
	stmt.autoClose = true
	rows, err := stmt.QueryContext(ctx, args)
	if err != nil {
		_ = stmt.Close()
	}
	return rows, err
}

func (conn *Conn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if !conn.IsValid() || !conn.session.Connected {
		// database/sql discards the connection and opens another. go-ora
		// opened this one again here, printed the result to standard
		// output, and returned ErrBadConn all the same
		conn.logger.Info("prepare on a broken connection")
		return nil, driver.ErrBadConn
	}
	conn.logger.Debug("prepare", "query", query)
	done := conn.session.StartContext(ctx)
	defer conn.session.EndContext(done)
	return NewStmt(query, conn), nil
}

func (conn *Conn) ProcessTCCResponse(msgCode uint8) error {
	session := conn.session
	var err error
	switch msgCode {
	case 4:
		conn.session.Summary, err = tns.NewSummary(session)
		if err != nil {
			return err
		}
		conn.logger.Debug("summary", "code", session.Summary.RetCode, "message", string(session.Summary.ErrorMessage))
		if conn.session.HasError() {
			return conn.session.ServerError()
		}
		// the server created a unit that it could not compile, and the
		// caller must know (D32)
		if conn.session.Summary.CompilationError() {
			return tns.NewCompilationError()
		}
	case 8:
		size, err := session.ReadInt(2, true, true)
		if err != nil {
			return err
		}
		for x := 0; x < size; x++ {
			_, err = session.ReadInt(4, true, true)
			if err != nil {
				return err
			}
		}
		_, err = session.ReadInt(2, true, true)
		if err != nil {
			return err
		}
		size, err = session.ReadInt(2, true, true)
		for x := 0; x < size; x++ {
			_, val, num, err := session.ReadKeyVal()
			if err != nil {
				return err
			}
			if num == 163 {
				session.TimeZone = val
			}
		}
		if session.TTCVersion >= 4 {
			// get queryID
			size, err = session.ReadInt(4, true, true)
			if err != nil {
				return err
			}
			if size > 0 {
				bty, err := session.ReadBytes(size)
				if err != nil {
					return err
				}
				if len(bty) >= 8 {
					queryID := binary.LittleEndian.Uint64(bty[size-8:])
					_, _ = os.Stderr.WriteString(fmt.Sprintln("query ID: ", queryID))
				}
			}
		}
		if session.TTCVersion >= 7 {
			length, err := session.ReadInt(4, true, true)
			if err != nil {
				return err
			}
			for range length {
				_, err = session.ReadInt(8, true, true)
				if err != nil {
					return err
				}
			}
		}
	case 9:
		if session.HasEOSCapability {
			temp, err := session.ReadInt(4, true, true)
			if err != nil {
				return err
			}
			if session.Summary != nil {
				session.Summary.EndOfCallStatus = temp
			}
			conn.txnInProgress = temp&eocsTxnInProgress != 0
		}
		if session.HasFSAPCapability {
			if session.Summary == nil {
				session.Summary = new(tns.SummaryObject)
			}
			session.Summary.EndToEndECIDSequence, err = session.ReadInt(2, true, true)
			if err != nil {
				return err
			}
		}
	case 15:
		warning, err := tns.NewWarningObject(session)
		if err != nil {
			return err
		}
		// a warning comes at logon too, as for a password that will expire,
		// so it goes to the logger and not to the caller. go-ora wrote it to
		// os.Stderr
		if warning != nil {
			conn.logger.Warn("server warning", "code", warning.Code(), "message", warning.Message())
		}
	case 23:
		opCode, err := session.ReadByte()
		if err != nil {
			return err
		}
		err = conn.getServerNetworkInformation(opCode)
		if err != nil {
			return err
		}
	case 28:
		conn.logger.Debug("fast negotiation refused")
		err = conn.protocolNegotiation()
		if err != nil {
			return err
		}
		err = conn.dataTypeNegotiation()
		if err != nil {
			return err
		}
	default:
		return errors.New(fmt.Sprintf("TTC error: received code %d during response reading", msgCode))
	}
	return nil
	// cancel loop if = 4 or 9
}

func (conn *Conn) setBad() {
	conn.bad = true
}

// ResetSession decides responsible for resetting a connection. Part of a keepConnOnRollback condition to decide if to keep a transaction after rollback.
func (conn *Conn) ResetSession(_ context.Context) error {
	if conn.bad {
		return driver.ErrBadConn
	}
	return nil
}

// IsValid validates if a connection has to be discarded. Part of a keepConnOnRollback condition to decide if to keep a transaction after rollback.
func (conn *Conn) IsValid() bool {
	// the connection is valid when it is not bad and is open
	return !conn.bad && conn.State == Opened
}

func (conn *Conn) dataTypeNegotiation() error {
	var err error
	conn.logger.Debug("data type negotiation")
	conn.dataNego = buildTypeNego(conn.tcpNego, conn)
	err = conn.dataNego.write()
	if err != nil {
		return err
	}
	return conn.dataNego.read()
}

func (conn *Conn) protocolNegotiation() error {
	var err error
	conn.logger.Debug("protocol negotiation")
	conn.tcpNego = &TCPNego{conn: conn}
	err = conn.tcpNego.write()
	if err != nil {
		return err
	}
	err = conn.tcpNego.readMessage()
	if err != nil {
		return err
	}
	conn.logger.Debug("charsets", "server", conn.tcpNego.ServerCharset, "national", conn.tcpNego.ServerNCharset)
	conn.tcpNego.ServerFlags |= 2
	return nil
}

func (conn *Conn) appendTemporaryLoc(loc types.Locator) {
	for _, locator := range conn.temporaryLocs {
		if bytes.Equal(loc, locator) {
			return
		}
	}
	conn.temporaryLocs = append(conn.temporaryLocs, loc)
}

func (conn *Conn) Session() tns.SessionReadWriter {
	return conn.session
}

func (conn *Conn) NewLobStreamer() types.LobStreamer {
	return &LobStream{conn: conn}
}

func (conn *Conn) ParameterCoder(input any) (codec.OracleParameterCoder, error) {
	switch input := input.(type) {
	case reflect.Type:
		if coder, ok := conn.goTypeCoder[input]; ok {
			return coder.Copy(), nil
		}
		if input != nil && input.Kind() != reflect.Interface {
			if coder := builtinCoder(reflect.Zero(input).Interface()); coder != nil {
				coder.Init()
				return coder, nil
			}
		}

		return nil, fmt.Errorf("no parameter coder registered for go type %s", input.String())
	case string:
		if coder, ok := conn.nameTypeCoder[strings.ToUpper(input)]; ok {
			return coder.Copy(), nil
		}
		return nil, fmt.Errorf("no parameter coder registered for name %s", input)
	case types.TypeID:
		if coder, ok := conn.oracleTypeCoder[input]; ok {
			return coder.Copy(), nil
		}
		return nil, fmt.Errorf("no parameter coder registered for oracle type %d", input)
	case int, int16, int32, int64:
		index := reflect.ValueOf(input).Int()
		if coder, ok := conn.oracleTypeCoder[types.TypeID(index)]; ok {
			return coder.Copy(), nil
		}

		return nil, fmt.Errorf("no parameter coder registered for oracle type %d", index)
	case uint, uint16, uint32, uint64:
		index := reflect.ValueOf(input).Uint()
		if coder, ok := conn.oracleTypeCoder[types.TypeID(index)]; ok {
			return coder.Copy(), nil
		}

		return nil, fmt.Errorf("no parameter coder registered for oracle type %d", index)
	default:
		return nil, fmt.Errorf("accepted values for ParameterCoder: reflec.Type, int/uint or string")
	}
}

func (conn *Conn) SendTimeAsUTC() bool {
	return conn.dataNego.serverTZVersion > 0 && conn.dataNego.clientTZVersion == conn.dataNego.serverTZVersion
}

func (conn *Conn) DBTimeZone() (*time.Location, error) {
	return conn.dbTimeZone, conn.dbTimeZoneErr
}

func (conn *Conn) TTCVersion() uint8 {
	return conn.session.TTCVersion
}
