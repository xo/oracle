package oracle

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// DefaultPort is the port of an Oracle listener when the DSN names none.
const DefaultPort = 1521

// SSLMode says whether a connection uses TLS, and what it checks. The values
// are the ones of the sslmode key of pgx and lib/pq (D11).
type SSLMode string

// The modes. An Oracle listener speaks TCP or TCPS on one port, never both,
// so the allow and prefer modes of pgx do not exist here.
const (
	// SSLDisable speaks TCP with no TLS.
	SSLDisable SSLMode = "disable"
	// SSLRequire speaks TLS and checks no certificate.
	SSLRequire SSLMode = "require"
	// SSLVerifyCA speaks TLS and checks that a known authority signed the
	// certificate of the server, and not its name.
	SSLVerifyCA SSLMode = "verify-ca"
	// SSLVerifyFull speaks TLS and checks the authority and the name.
	SSLVerifyFull SSLMode = "verify-full"
)

// LobFetch says how a query returns a LOB column.
type LobFetch string

// The ways to fetch a LOB.
const (
	// LobFetchInline returns the whole value in the row.
	LobFetchInline LobFetch = "inline"
	// LobStream returns a locator, and the driver reads the value with more
	// round trips.
	LobFetchStream LobFetch = "stream"
)

// LobRead says whether the driver reads the value of a LOB locator for the
// caller.
type LobRead string

// The ways to read a LOB.
const (
	// LobReadAuto reads the value when the row is scanned.
	LobReadAuto LobRead = "auto"
	// LobReadExplicit leaves the locator for the caller to read.
	LobReadExplicit LobRead = "explicit"
)

// Auth is how a connection proves who it is.
type Auth string

// The ways to authenticate.
const (
	// AuthPassword sends the user and the password. It is the default.
	AuthPassword Auth = "password"
	// AuthOS uses the user of the operating system, through NTS on Windows.
	AuthOS Auth = "os"
	// AuthKerberos uses Kerberos, through Config.Kerberos.
	AuthKerberos Auth = "kerberos"
	// AuthToken sends the token of Config.Token, signed with Config.TokenKey.
	AuthToken Auth = "token"
)

// Privilege is an administrative privilege that a session asks for.
type Privilege string

// The administrative privileges.
const (
	PrivilegeSYSDBA    Privilege = "sysdba"
	PrivilegeSYSOPER   Privilege = "sysoper"
	PrivilegeSYSASM    Privilege = "sysasm"
	PrivilegeSYSBACKUP Privilege = "sysbackup"
	PrivilegeSYSDG     Privilege = "sysdg"
	PrivilegeSYSKM     Privilege = "syskm"
	PrivilegeSYSRAC    Privilege = "sysrac"
)

// ServiceLevel is how much the client wants an Oracle Net service, such as
// encryption or checksums. The server compares it with its own level.
type ServiceLevel string

// The service levels of Oracle Net.
const (
	ServiceAccepted  ServiceLevel = "accepted"
	ServiceRejected  ServiceLevel = "rejected"
	ServiceRequested ServiceLevel = "requested"
	ServiceRequired  ServiceLevel = "required"
)

// Dialer opens the network connection to a listener. A caller sets it to
// reach the server through a proxy or a tunnel.
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// Kerberos returns the Kerberos ticket for a service.
type Kerberos interface {
	Authenticate(server, service string) ([]byte, error)
}

// Client is what the connect packet says about the client. An empty field
// takes the value of this process: the path of the program, the name of the
// host, and the user of the operating system.
type Client struct {
	Program string
	Host    string
	User    string
}

// Config is the configuration of one database. ParseDSN fills it from a DSN,
// and a caller can change it or build one, and pass it to NewConnector. The
// zero value of a field is its default.
type Config struct {
	User     string
	Password string

	// Host and Port are the address of the listener. Servers are more
	// addresses, as host:port, that the driver tries in order.
	Host    string
	Port    int
	Servers []string

	// Service is the service name, Instance the instance name, and SID the
	// SID, which some old databases use in place of a service name.
	Service  string
	Instance string
	SID      string

	// Descriptor is a TNS descriptor, such as (DESCRIPTION=...), that names
	// the addresses and the service in place of the fields above.
	Descriptor string

	// SSLMode and the SSL fields say whether and how the connection uses TLS.
	SSLMode       SSLMode
	SSLRootCert   string
	SSLCert       string
	SSLKey        string
	SSLServerName string

	// TLS is a TLS configuration for the connection. When it is set, it takes
	// the place of the SSL files, and the connector clones it for each dial.
	TLS *tls.Config

	// Wallet is a folder that holds cwallet.sso or ewallet.p12, and
	// WalletPassword the password of ewallet.p12.
	Wallet         string
	WalletPassword string

	// Timeout is the deadline of each round trip, and ConnectTimeout the limit
	// on the dial, the TLS handshake and the logon. Zero means no timeout and
	// 60 seconds.
	Timeout        time.Duration
	ConnectTimeout time.Duration

	// Prefetch is the number of rows that one fetch asks for. Zero means 25.
	Prefetch int

	LobFetch LobFetch
	LobRead  LobRead

	Auth      Auth
	Privilege Privilege

	// Token and TokenKey are the files of a token and of its private key.
	Token    string
	TokenKey string

	// Encryption and Checksum are the levels of the native encryption and
	// checksums of Oracle Net.
	Encryption ServiceLevel
	Checksum   ServiceLevel

	// Language, Territory and Charset are the NLS settings of the client.
	Language  string
	Territory string
	Charset   string

	// Program is the program name that the session reports, and ProxyUser
	// the user of a proxy session.
	Program   string
	ProxyUser string

	// UnixSocket is a Unix socket to dial in place of TCP.
	UnixSocket string

	// Client is what the connect packet says about the client.
	Client Client

	// Dialer opens each network connection. Nil uses a net.Dialer.
	Dialer Dialer

	// Kerberos returns the ticket when Auth is AuthKerberos.
	Kerberos Kerberos

	// NTS makes the messages of NTS when Auth is AuthOS and the server asks
	// for it. Nil uses the NTLM of the driver.
	NTS NTS

	// Logger receives the packets and events of each connection. Nil logs
	// nothing (D14).
	Logger *slog.Logger
}

// The keys of the query of a DSN. Every key is lower case, with no space
// (D11).
const (
	keySID            = "sid"
	keyDescriptor     = "descriptor"
	keyServer         = "server"
	keySSLMode        = "sslmode"
	keySSLRootCert    = "sslrootcert"
	keySSLCert        = "sslcert"
	keySSLKey         = "sslkey"
	keySSLServerName  = "sslservername"
	keyWallet         = "wallet"
	keyWalletPassword = "wallet_password"
	keyTimeout        = "timeout"
	keyConnectTimeout = "connect_timeout"
	keyPrefetch       = "prefetch"
	keyLobFetch       = "lob_fetch"
	keyLobRead        = "lob_read"
	keyAuth           = "auth"
	keyPrivilege      = "privilege"
	keyToken          = "token"
	keyTokenKey       = "token_key"
	keyEncryption     = "encryption"
	keyChecksum       = "checksum"
	keyLanguage       = "language"
	keyTerritory      = "territory"
	keyCharset        = "charset"
	keyProgram        = "program"
	keyProxyUser      = "proxy_user"
	keyUnixSocket     = "unix_socket"
)

// keys holds every key that a DSN can hold.
var keys = []string{
	keySID, keyDescriptor, keyServer,
	keySSLMode, keySSLRootCert, keySSLCert, keySSLKey, keySSLServerName,
	keyWallet, keyWalletPassword,
	keyTimeout, keyConnectTimeout, keyPrefetch, keyLobFetch, keyLobRead,
	keyAuth, keyPrivilege, keyToken, keyTokenKey,
	keyEncryption, keyChecksum,
	keyLanguage, keyTerritory, keyCharset,
	keyProgram, keyProxyUser, keyUnixSocket,
}

// ParseDSN parses a DSN of the form
//
//	oracle://user:password@host:port/service/instance?key=value
//
// The scheme must be oracle. The port is 1521 when the DSN names none. The
// path is the service name, and an instance name after it. A service name
// that holds a / is written with %2F. A key that ParseDSN does not know, and
// a key that appears twice, other than server, are errors. An error never
// holds the DSN, which can hold a password.
func ParseDSN(dsn string) (Config, error) {
	var cfg Config
	u, err := url.Parse(dsn)
	if err != nil {
		// the error of net/url quotes the URL, and the URL can hold a password
		return cfg, fmt.Errorf("parsing the dsn: %w", ErrInvalidValue)
	}
	if u.Scheme != "oracle" {
		return cfg, fmt.Errorf("parsing the dsn: the scheme %q: %w", u.Scheme, ErrScheme)
	}
	if u.Opaque != "" {
		return cfg, fmt.Errorf("parsing the dsn: no host: %w", ErrInvalidValue)
	}
	if u.User != nil {
		cfg.User = u.User.Username()
		cfg.Password, _ = u.User.Password()
	}
	cfg.Host = u.Hostname()
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return cfg, fmt.Errorf("parsing the dsn: the port %q: %w", p, ErrInvalidValue)
		}
		cfg.Port = n
	}
	if err := cfg.parsePath(u.EscapedPath()); err != nil {
		return cfg, err
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return cfg, fmt.Errorf("parsing the query of the dsn: %w", ErrInvalidValue)
	}
	for key, vals := range q {
		if !slices.Contains(keys, key) {
			return cfg, fmt.Errorf("parsing the dsn: the key %q: %w", key, ErrUnknownKey)
		}
		if len(vals) > 1 && key != keyServer {
			return cfg, fmt.Errorf("parsing the dsn: the key %q: %w", key, ErrRepeatedKey)
		}
		if err := cfg.set(key, vals); err != nil {
			return cfg, fmt.Errorf("parsing the dsn: the key %q: %w", key, err)
		}
	}
	if err := cfg.check(); err != nil {
		return cfg, fmt.Errorf("parsing the dsn: %w", err)
	}
	return cfg, nil
}

// parsePath reads the service name and the instance name from the escaped
// path of the URL. Each segment is unescaped on its own, so %2F is a / that
// belongs to the name.
func (cfg *Config) parsePath(p string) error {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return nil
	}
	segs := strings.Split(p, "/")
	if len(segs) > 2 {
		return fmt.Errorf("parsing the dsn: the path has %d segments, and it holds the service and the instance: %w", len(segs), ErrInvalidValue)
	}
	names := make([]string, len(segs))
	for i, s := range segs {
		name, err := url.PathUnescape(s)
		if err != nil || name == "" {
			return fmt.Errorf("parsing the dsn: segment %d of the path: %w", i+1, ErrInvalidValue)
		}
		names[i] = name
	}
	cfg.Service = names[0]
	if len(names) == 2 {
		cfg.Instance = names[1]
	}
	return nil
}

// set reads the values of one key into cfg.
func (cfg *Config) set(key string, vals []string) error {
	v := vals[0]
	var err error
	switch key {
	case keySID:
		cfg.SID = v
	case keyDescriptor:
		cfg.Descriptor = v
	case keyServer:
		for _, s := range vals {
			if _, _, err := net.SplitHostPort(s); err != nil {
				return fmt.Errorf("the server %q is not host:port: %w", s, ErrInvalidValue)
			}
		}
		cfg.Servers = vals
	case keySSLMode:
		cfg.SSLMode, err = oneOf(SSLMode(v), SSLDisable, SSLRequire, SSLVerifyCA, SSLVerifyFull)
	case keySSLRootCert:
		cfg.SSLRootCert = v
	case keySSLCert:
		cfg.SSLCert = v
	case keySSLKey:
		cfg.SSLKey = v
	case keySSLServerName:
		cfg.SSLServerName = v
	case keyWallet:
		cfg.Wallet = v
	case keyWalletPassword:
		cfg.WalletPassword = v
	case keyTimeout:
		cfg.Timeout, err = duration(v)
	case keyConnectTimeout:
		cfg.ConnectTimeout, err = duration(v)
	case keyPrefetch:
		cfg.Prefetch, err = strconv.Atoi(v)
		if err != nil || cfg.Prefetch < 1 {
			err = fmt.Errorf("%q is not a positive integer: %w", v, ErrInvalidValue)
		}
	case keyLobFetch:
		cfg.LobFetch, err = oneOf(LobFetch(v), LobFetchInline, LobFetchStream)
	case keyLobRead:
		cfg.LobRead, err = oneOf(LobRead(v), LobReadAuto, LobReadExplicit)
	case keyAuth:
		cfg.Auth, err = oneOf(Auth(v), AuthPassword, AuthOS, AuthKerberos, AuthToken)
	case keyPrivilege:
		cfg.Privilege, err = oneOf(Privilege(v), PrivilegeSYSDBA, PrivilegeSYSOPER, PrivilegeSYSASM,
			PrivilegeSYSBACKUP, PrivilegeSYSDG, PrivilegeSYSKM, PrivilegeSYSRAC)
	case keyToken:
		cfg.Token = v
	case keyTokenKey:
		cfg.TokenKey = v
	case keyEncryption:
		cfg.Encryption, err = oneOf(ServiceLevel(v), ServiceAccepted, ServiceRejected, ServiceRequested, ServiceRequired)
	case keyChecksum:
		cfg.Checksum, err = oneOf(ServiceLevel(v), ServiceAccepted, ServiceRejected, ServiceRequested, ServiceRequired)
	case keyLanguage:
		cfg.Language = v
	case keyTerritory:
		cfg.Territory = v
	case keyCharset:
		cfg.Charset = v
	case keyProgram:
		cfg.Program = v
	case keyProxyUser:
		cfg.ProxyUser = v
	case keyUnixSocket:
		cfg.UnixSocket = v
	}
	return err
}

// check returns an error when cfg cannot name a database.
func (cfg *Config) check() error {
	if cfg.Descriptor == "" {
		if cfg.Host == "" {
			return fmt.Errorf("no host: %w", ErrInvalidValue)
		}
		if cfg.Service == "" && cfg.SID == "" {
			return fmt.Errorf("no service and no sid: %w", ErrInvalidValue)
		}
	}
	if cfg.Auth == AuthToken && (cfg.Token == "" || cfg.TokenKey == "") {
		return fmt.Errorf("auth=token needs token and token_key: %w", ErrInvalidValue)
	}
	if (cfg.SSLCert == "") != (cfg.SSLKey == "") {
		return fmt.Errorf("sslcert and sslkey go together: %w", ErrInvalidValue)
	}
	return nil
}

// oneOf returns v when it is one of allowed.
func oneOf[T ~string](v T, allowed ...T) (T, error) {
	if slices.Contains(allowed, v) {
		return v, nil
	}
	return "", fmt.Errorf("%q is not one of %v: %w", v, allowed, ErrInvalidValue)
}

// duration reads a Go duration that is zero or more.
func duration(v string) (time.Duration, error) {
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("%q is not a duration such as 30s: %w", v, ErrInvalidValue)
	}
	return d, nil
}

// DSN returns cfg as a DSN that ParseDSN reads back to the same Config. It
// writes no field that a DSN cannot hold, such as TLS, Dialer or Logger.
func (cfg Config) DSN() string {
	u := &url.URL{Scheme: "oracle", Host: cfg.Host}
	if cfg.Port != 0 {
		u.Host = net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	}
	if cfg.User != "" || cfg.Password != "" {
		u.User = url.UserPassword(cfg.User, cfg.Password)
		if cfg.Password == "" {
			u.User = url.User(cfg.User)
		}
	}
	if cfg.Service != "" {
		u.RawPath = "/" + url.PathEscape(cfg.Service)
		if cfg.Instance != "" {
			u.RawPath += "/" + url.PathEscape(cfg.Instance)
		}
		u.Path, _ = url.PathUnescape(u.RawPath)
	}
	q := url.Values{}
	add := func(key, v string) {
		if v != "" {
			q.Set(key, v)
		}
	}
	add(keySID, cfg.SID)
	add(keyDescriptor, cfg.Descriptor)
	for _, s := range cfg.Servers {
		q.Add(keyServer, s)
	}
	add(keySSLMode, string(cfg.SSLMode))
	add(keySSLRootCert, cfg.SSLRootCert)
	add(keySSLCert, cfg.SSLCert)
	add(keySSLKey, cfg.SSLKey)
	add(keySSLServerName, cfg.SSLServerName)
	add(keyWallet, cfg.Wallet)
	add(keyWalletPassword, cfg.WalletPassword)
	if cfg.Timeout != 0 {
		add(keyTimeout, cfg.Timeout.String())
	}
	if cfg.ConnectTimeout != 0 {
		add(keyConnectTimeout, cfg.ConnectTimeout.String())
	}
	if cfg.Prefetch != 0 {
		add(keyPrefetch, strconv.Itoa(cfg.Prefetch))
	}
	add(keyLobFetch, string(cfg.LobFetch))
	add(keyLobRead, string(cfg.LobRead))
	add(keyAuth, string(cfg.Auth))
	add(keyPrivilege, string(cfg.Privilege))
	add(keyToken, cfg.Token)
	add(keyTokenKey, cfg.TokenKey)
	add(keyEncryption, string(cfg.Encryption))
	add(keyChecksum, string(cfg.Checksum))
	add(keyLanguage, cfg.Language)
	add(keyTerritory, cfg.Territory)
	add(keyCharset, cfg.Charset)
	add(keyProgram, cfg.Program)
	add(keyProxyUser, cfg.ProxyUser)
	add(keyUnixSocket, cfg.UnixSocket)
	u.RawQuery = q.Encode()
	return u.String()
}
