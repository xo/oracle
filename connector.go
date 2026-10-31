package oracle

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/user"
	"strconv"
	"strings"
	"sync"

	"github.com/xo/oracle/internal/auth"
	"github.com/xo/oracle/internal/configurations"
)

// Driver is the oracle driver. The value that init registers holds nothing.
// Each Connector returns a Driver that points back at it, so that
// RegisterType reaches the types of one database through its *sql.DB.
type Driver struct {
	c *Connector
}

// Open returns a new connection to the database that dsn names. database/sql
// does not call Open, because Driver implements driver.DriverContext.
func (d Driver) Open(dsn string) (driver.Conn, error) {
	c, err := d.OpenConnector(dsn)
	if err != nil {
		return nil, err
	}
	return c.(*Connector).open()
}

// OpenConnector parses dsn with ParseDSN and returns a Connector for it. It
// does not connect.
func (Driver) OpenConnector(dsn string) (driver.Connector, error) {
	cfg, err := ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	return NewConnector(cfg)
}

// sessionParam is a parameter that the connector sets on each session with
// ALTER SESSION.
type sessionParam struct {
	key, value string
}

// Connector opens connections to one Oracle database. It holds everything
// that belongs to that database: the configuration, the session parameters,
// the registered types and the type coders (D9). Pass it to sql.OpenDB.
type Connector struct {
	cfg    Config
	conn   *configurations.ConnectionConfig
	wallet *configurations.Wallet
	params []sessionParam
	reg    *registry

	mu     sync.Mutex
	closed bool
}

// ConnectorOption changes a Connector as NewConnector makes it.
type ConnectorOption func(*Connector) error

// WithSessionParam sets the session parameter key to value, with ALTER
// SESSION, on each connection after its logon. The value is written as it is,
// so a string value needs its quotes, as in WithSessionParam("nls_date_format",
// "'YYYY-MM-DD'").
func WithSessionParam(key, value string) ConnectorOption {
	return func(c *Connector) error {
		c.params = append(c.params, sessionParam{key, value})
		return nil
	}
}

// WithWallet reads a wallet, a cwallet.sso file, from r. It takes the place
// of the folder that Config.Wallet names.
func WithWallet(r io.Reader) ConnectorOption {
	return func(c *Connector) error {
		w, err := configurations.NewWalletFromReader(r)
		if err != nil {
			return fmt.Errorf("reading the wallet: %w", err)
		}
		c.wallet = w
		return nil
	}
}

// NewConnector returns a Connector for cfg. It reads the files that cfg names,
// such as a wallet or a certificate, and it does not connect.
func NewConnector(cfg Config, opts ...ConnectorOption) (*Connector, error) {
	c := &Connector{cfg: cfg, reg: newRegistry()}
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	conn, err := cfg.connectionConfig()
	if err != nil {
		return nil, err
	}
	if c.wallet != nil {
		conn.Wallet = c.wallet
	}
	c.conn = conn
	return c, nil
}

// Connect opens a connection, logs on, and sets the session parameters.
func (c *Connector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.newConnection()
	if err != nil {
		return nil, err
	}
	if err := conn.OpenWithContext(ctx); err != nil {
		return nil, err
	}
	if err := c.reg.initFromConn(conn, c.params); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

// open is Connect for Driver.Open, which has no context.
func (c *Connector) open() (driver.Conn, error) {
	conn, err := c.newConnection()
	if err != nil {
		return nil, err
	}
	if err := conn.Open(); err != nil {
		return nil, err
	}
	if err := c.reg.initFromConn(conn, c.params); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

// newConnection returns a connection that is not open yet, with its own copy
// of the configuration, because a connection changes its copy as it connects.
func (c *Connector) newConnection() (*Connection, error) {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return nil, ErrConnectorClosed
	}
	conn, err := NewConnection(c.conn.Clone())
	if err != nil {
		return nil, err
	}
	conn.buildParameterCoderMap(c.reg)
	conn.cusTyp = c.reg.cusTyp
	return conn, nil
}

// Driver returns a Driver that points at c.
func (c *Connector) Driver() driver.Driver {
	return Driver{c: c}
}

// Close makes each later Connect fail. sql.DB.Close calls it. It closes no
// connection, because database/sql closes those.
func (c *Connector) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return nil
}

// connectionConfig returns the configuration of the protocol code for cfg.
//
// For now it writes the DSN that the parser of go-ora reads, and parses that.
// The parser holds the defaults and the checks that the recorded exchanges
// were made with, so the driver sends the same packets as before. A later
// step writes the configuration directly and deletes the parser.
func (cfg Config) connectionConfig() (*configurations.ConnectionConfig, error) {
	if err := cfg.check(); err != nil {
		return nil, fmt.Errorf("checking the configuration: %w", err)
	}
	port := cfg.Port
	if port == 0 {
		port = DefaultPort
	}
	u := &url.URL{
		Scheme: "oracle",
		Host:   net.JoinHostPort(cfg.Host, strconv.Itoa(port)),
		Path:   "/" + cfg.Service,
	}
	if cfg.Descriptor != "" && cfg.Host == "" {
		u.Host = ":0"
	}
	if cfg.User != "" || cfg.Password != "" {
		u.User = url.UserPassword(cfg.User, cfg.Password)
	}
	q := url.Values{}
	set := func(key, v string) {
		if v != "" {
			q.Set(key, v)
		}
	}
	set("SID", cfg.SID)
	set("INSTANCE NAME", cfg.Instance)
	set("CONNSTR", cfg.Descriptor)
	for _, s := range cfg.Servers {
		q.Add("SERVER", s)
	}
	switch cfg.SSLMode {
	case "", SSLDisable:
	case SSLRequire:
		q.Set("SSL", "true")
		q.Set("SSL VERIFY", "false")
	default:
		q.Set("SSL", "true")
		q.Set("SSL VERIFY", "true")
	}
	set("WALLET", cfg.Wallet)
	set("WALLET PASSWORD", cfg.WalletPassword)
	if cfg.Prefetch != 0 {
		q.Set("PREFETCH_ROWS", strconv.Itoa(cfg.Prefetch))
	}
	set("LOB FETCH", strings.ToUpper(string(cfg.LobFetch)))
	set("LOB READ", strings.ToUpper(string(cfg.LobRead)))
	switch cfg.Auth {
	case AuthOS:
		q.Set("AUTH TYPE", "OS")
	case AuthKerberos:
		q.Set("AUTH TYPE", "KERBEROS")
	}
	set("TOKEN FILE", cfg.Token)
	set("TOKEN PRIVATE KEY FILE", cfg.TokenKey)
	set("ENCRYPTION", strings.ToUpper(string(cfg.Encryption)))
	set("DATA INTEGRITY", strings.ToUpper(string(cfg.Checksum)))
	set("LANGUAGE", cfg.Language)
	set("TERRITORY", cfg.Territory)
	set("CHARSET", cfg.Charset)
	set("PROGRAM", cfg.Program)
	set("PROXY CLIENT NAME", cfg.ProxyUser)
	set("UNIX SOCKET", cfg.UnixSocket)
	if cid := cfg.Client.cid(); cid != "" {
		q.Set("CID", cid)
	}
	u.RawQuery = q.Encode()

	conn, err := configurations.ParseConfig(u.String())
	if err != nil {
		return nil, fmt.Errorf("reading the configuration: %w", err)
	}
	if len(conn.OSPassword) > 0 {
		SetNTSAuth(&auth.NTSAuthHash{})
	}
	// the parser gives SYS the SYSDBA privilege by its name, and D11 does not
	conn.DBAPrivilege = configurations.DBAPrivilegeFromString(string(cfg.Privilege))
	if cfg.Timeout != 0 {
		conn.Timeout = cfg.Timeout
	}
	if cfg.ConnectTimeout != 0 {
		conn.ConnectTimeout = cfg.ConnectTimeout
	}
	if cfg.Dialer != nil {
		conn.Dialer = cfg.Dialer
	}
	if cfg.Kerberos != nil {
		conn.Kerberos = cfg.Kerberos
	}
	conn.TLSConfig, err = cfg.tlsConfig()
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// cid returns the client identity of the connect packet, or "" when no field
// is set, so that the protocol code fills in this process.
func (cl Client) cid() string {
	if cl == (Client{}) {
		return ""
	}
	if cl.Program == "" && len(os.Args) > 0 {
		cl.Program = os.Args[0]
	}
	if cl.Host == "" {
		cl.Host, _ = os.Hostname()
	}
	if cl.User == "" {
		if u, err := user.Current(); err == nil {
			cl.User = u.Username
		}
	}
	return "(CID=(PROGRAM=" + cl.Program + ")(HOST=" + cl.Host + ")(USER=" + cl.User + "))"
}

// tlsConfig returns the TLS configuration that cfg asks for, or nil when the
// protocol code builds its own from a wallet.
func (cfg Config) tlsConfig() (*tls.Config, error) {
	if cfg.TLS != nil {
		return cfg.TLS.Clone(), nil
	}
	switch cfg.SSLMode {
	case "", SSLDisable:
		return nil, nil
	}
	files := cfg.SSLRootCert != "" || cfg.SSLCert != "" || cfg.SSLServerName != ""
	if cfg.Wallet != "" && !files && cfg.SSLMode != SSLVerifyCA {
		return nil, nil
	}
	t := &tls.Config{
		ServerName: cfg.SSLServerName,
		MinVersion: tls.VersionTLS12,
	}
	if t.ServerName == "" {
		// the name of the URL, and not the address that a listener redirects
		// to, which fixes sijms/go-ora#765
		t.ServerName = cfg.Host
	}
	if cfg.SSLRootCert != "" {
		pem, err := os.ReadFile(cfg.SSLRootCert)
		if err != nil {
			return nil, fmt.Errorf("reading sslrootcert: %w", err)
		}
		t.RootCAs = x509.NewCertPool()
		if !t.RootCAs.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("reading sslrootcert: no certificate: %w", ErrInvalidValue)
		}
	}
	if cfg.SSLCert != "" {
		cert, err := tls.LoadX509KeyPair(cfg.SSLCert, cfg.SSLKey)
		if err != nil {
			return nil, fmt.Errorf("reading sslcert and sslkey: %w", err)
		}
		t.Certificates = []tls.Certificate{cert}
	}
	switch cfg.SSLMode {
	case SSLRequire:
		t.InsecureSkipVerify = true
	case SSLVerifyCA:
		// check the chain and not the name
		t.InsecureSkipVerify = true
		roots := t.RootCAs
		t.VerifyConnection = func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("verifying the server: no certificate")
			}
			inter := x509.NewCertPool()
			for _, c := range cs.PeerCertificates[1:] {
				inter.AddCert(c)
			}
			_, err := cs.PeerCertificates[0].Verify(x509.VerifyOptions{Roots: roots, Intermediates: inter})
			return err
		}
	}
	return t, nil
}
