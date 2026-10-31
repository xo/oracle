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
	"os"
	"os/user"
	"strconv"
	"sync"

	"github.com/xo/oracle/internal/config"
	"github.com/xo/oracle/internal/ttc"
)

// Driver is the oracle driver. The value that init registers holds nothing.
// Each Connector returns a Driver that points back at it, so that
// RegisterType reaches the types of one database through its *sql.DB.
type Driver struct {
	c *Connector
}

// Open fails, because it has no context. database/sql does not call it,
// because Driver implements driver.DriverContext, and sql.Open works.
func (Driver) Open(string) (driver.Conn, error) {
	return nil, fmt.Errorf("opening a connection without a context: use sql.Open or sql.OpenDB: %w", ErrNotSupported)
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

// Connector opens connections to one Oracle database. It holds everything
// that belongs to that database: the configuration, the session parameters,
// the registered types and the type coders (D9). Pass it to sql.OpenDB.
type Connector struct {
	cfg    Config
	conn   *config.ConnectionConfig
	wallet *config.Wallet
	params []ttc.SessionParam
	reg    *ttc.Registry

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
		c.params = append(c.params, ttc.SessionParam{Key: key, Value: value})
		return nil
	}
}

// WithWallet reads a wallet, a cwallet.sso file, from r. It takes the place
// of the folder that Config.Wallet names.
func WithWallet(r io.Reader) ConnectorOption {
	return func(c *Connector) error {
		w, err := config.NewWalletFromReader(r)
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
	c := &Connector{cfg: cfg, reg: ttc.NewRegistry()}
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
	if err := c.reg.InitFromConn(conn, c.params); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

// newConnection returns a connection that is not open yet, with its own copy
// of the configuration, because a connection changes its copy as it connects.
func (c *Connector) newConnection() (*ttc.Connection, error) {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return nil, ErrConnectorClosed
	}
	return c.reg.NewConnection(c.conn.Clone())
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
func (cfg Config) connectionConfig() (*config.ConnectionConfig, error) {
	if err := cfg.check(); err != nil {
		return nil, fmt.Errorf("checking the configuration: %w", err)
	}
	conn := config.New()
	conn.UserID, conn.Password = cfg.User, cfg.Password
	conn.Logger = cfg.Logger
	if cfg.Host != "" {
		port := cfg.Port
		if port == 0 {
			port = DefaultPort
		}
		conn.Servers = append(conn.Servers, config.ServerAddr{Addr: cfg.Host, Port: port})
	}
	conn.ServiceName = cfg.Service
	desc := cfg.Descriptor
	if cfg.Alias != "" {
		dir := cfg.TNSAdmin
		if dir == "" {
			dir = os.Getenv("TNS_ADMIN")
		}
		if dir == "" {
			return nil, fmt.Errorf("finding the alias %q: no tns_admin and no TNS_ADMIN: %w", cfg.Alias, ErrInvalidValue)
		}
		var err error
		if desc, err = config.LookupAlias(dir, cfg.Alias); err != nil {
			return nil, fmt.Errorf("finding the alias: %w", err)
		}
	}
	if desc != "" {
		// the descriptor names the addresses, and can name the service, the
		// SID and the instance
		if err := conn.UpdateDatabaseInfo(desc); err != nil {
			return nil, fmt.Errorf("reading the descriptor: %w", err)
		}
	}
	if cfg.SID != "" {
		conn.SID = cfg.SID
	}
	if cfg.Instance != "" {
		conn.InstanceName = cfg.Instance
	}
	for _, s := range cfg.Servers {
		host, p, err := net.SplitHostPort(s)
		if err != nil {
			return nil, fmt.Errorf("reading the server %q: %w", s, ErrInvalidValue)
		}
		port, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("reading the port of the server %q: %w", s, ErrInvalidValue)
		}
		conn.Servers = append(conn.Servers, config.ServerAddr{Addr: host, Port: port})
	}
	if len(conn.Servers) == 0 {
		return nil, fmt.Errorf("no address: %w", ErrInvalidValue)
	}
	switch cfg.Auth {
	case AuthOS:
		conn.AuthType = config.OS
	case AuthKerberos:
		conn.AuthType = config.Kerberos
	}
	conn.EncServiceLevel = cfg.Encryption.level()
	conn.IntServiceLevel = cfg.Checksum.level()
	switch cfg.SSLMode {
	case "", SSLDisable:
	case SSLRequire:
		conn.SSL, conn.SSLVerify = true, false
	default:
		conn.SSL, conn.SSLVerify = true, true
	}
	conn.DBAPrivilege = config.DBAPrivilegeFromString(string(cfg.Privilege))
	if cfg.Timeout != 0 {
		conn.Timeout = cfg.Timeout
	}
	if cfg.ConnectTimeout != 0 {
		conn.ConnectTimeout = cfg.ConnectTimeout
	}
	if cfg.Prefetch != 0 {
		conn.PrefetchRows = cfg.Prefetch
	}
	if cfg.LobFetch == LobFetchStream {
		conn.Lob = config.STREAM
	}
	if cfg.LobRead == LobReadExplicit {
		conn.LobReadMode = config.LobReadModeNone
	}
	conn.UnixAddress = cfg.UnixSocket
	conn.SkipNLS = cfg.SkipNLS
	conn.TokenFile, conn.TokenPrivateKeyFile = cfg.Token, cfg.TokenKey
	conn.ProxyClientName = cfg.ProxyUser
	if cfg.Language != "" {
		conn.Language = cfg.Language
	}
	if cfg.Territory != "" {
		conn.Territory = cfg.Territory
	}
	if cfg.Charset != "" {
		id, err := config.CharsetID(cfg.Charset)
		if err != nil {
			return nil, fmt.Errorf("reading the charset: %w: %w", ErrInvalidValue, err)
		}
		conn.CharsetID = id
	}
	conn.ProgramName = cfg.Program
	conn.Cid = cfg.Client.cid()
	if cfg.Wallet != "" {
		if err := conn.LoadWallet(cfg.Wallet, cfg.WalletPassword); err != nil {
			return nil, fmt.Errorf("reading the wallet: %w", err)
		}
	}
	if err := conn.Validate(); err != nil {
		return nil, fmt.Errorf("checking the configuration: %w", err)
	}
	if cfg.Dialer != nil {
		conn.Dialer = cfg.Dialer
	}
	if cfg.Kerberos != nil {
		conn.Kerberos = cfg.Kerberos
	}
	conn.NTS = cfg.NTS
	var err error
	conn.TLSConfig, err = cfg.tlsConfig()
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// level returns the number that the protocol code uses for l. The zero value
// is accepted, the default of Oracle Net.
func (l ServiceLevel) level() int {
	switch l {
	case ServiceRejected:
		return 1
	case ServiceRequested:
		return 2
	case ServiceRequired:
		return 3
	}
	return 0
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
