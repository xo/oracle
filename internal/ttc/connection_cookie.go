package ttc

import (
	"crypto/md5"
	"encoding/base64"
	"sync"

	"github.com/xo/oracle/internal/tns"
)

type ConnectionCookie struct {
	ServerCharset         int
	ServerFlags           uint8
	ServerNCharset        int
	OracleVersion         int
	ProtocolServerString  string
	ServerCompileTimeCaps []byte
	ServerRuntimeCaps     []byte
	cookieKey             string
}

// cookieJar holds the fast login cookies of the connections of one Registry,
// and so of one Connector (D9). A cookie names the charsets of the server.
// go-ora kept one store for the whole process, keyed by the UUID of the
// server and its service name, so a second database with the same name, such
// as XE, took the charset of the first, which was sijms/go-ora#692.
type cookieJar struct {
	mu      sync.Mutex
	cookies map[string]*ConnectionCookie
}

func getCookieKey(conn *Connection) string {
	uuid := conn.session.Context.UUID
	if len(uuid) == 0 {
		return ""
	}
	serviceName := conn.connOption.ServiceName
	if serviceName == "" {
		serviceName = conn.connOption.SID
	}
	if serviceName == "" {
		return ""
	}
	raw := base64.StdEncoding.EncodeToString(uuid) + "|" + serviceName
	h := md5.Sum([]byte(raw))
	return string(base64.StdEncoding.EncodeToString(h[:]))
}

func lookupCookie(conn *Connection) *ConnectionCookie {
	key := getCookieKey(conn)
	if key == "" || conn.cookies == nil {
		return nil
	}
	conn.cookies.mu.Lock()
	defer conn.cookies.mu.Unlock()
	return conn.cookies.cookies[key]
}

func deleteCookie(conn *Connection) {
	key := getCookieKey(conn)
	if key == "" || conn.cookies == nil {
		return
	}
	conn.cookies.mu.Lock()
	defer conn.cookies.mu.Unlock()
	delete(conn.cookies.cookies, key)
}

func saveCookie(conn *Connection) {
	key := getCookieKey(conn)
	if key == "" || conn.cookies == nil || conn.tcpNego == nil {
		return
	}
	nego := conn.tcpNego
	cookie := &ConnectionCookie{
		ServerCharset:         nego.ServerCharset,
		ServerFlags:           nego.ServerFlags,
		ServerNCharset:        nego.ServerNCharset,
		OracleVersion:         nego.OracleVersion,
		ProtocolServerString:  nego.ProtocolServerString,
		ServerCompileTimeCaps: append([]byte(nil), nego.ServerCompileTimeCaps...),
		ServerRuntimeCaps:     append([]byte(nil), nego.ServerRuntimeCaps...),
		cookieKey:             key,
	}
	conn.cookies.mu.Lock()
	if conn.cookies.cookies == nil {
		conn.cookies.cookies = make(map[string]*ConnectionCookie)
	}
	conn.cookies.cookies[key] = cookie
	conn.cookies.mu.Unlock()
}

func (c *ConnectionCookie) writeTTICookie(session *tns.Session) {
	session.PutBytes(30, 1)
	session.PutBytes(uint8(c.OracleVersion))
	session.PutUint(c.ServerCharset, 2, false, false)
	session.PutBytes(c.ServerFlags)
	session.PutUint(c.ServerNCharset, 2, false, false)
	session.PutBytes(uint8(len(c.ProtocolServerString) + 1))
	session.PutBytes([]byte(c.ProtocolServerString)...)
	session.PutBytes(0)
	session.PutBytes(uint8(len(c.ServerCompileTimeCaps)))
	session.PutBytes(c.ServerCompileTimeCaps...)
	session.PutBytes(uint8(len(c.ServerRuntimeCaps)))
	session.PutBytes(c.ServerRuntimeCaps...)
}
