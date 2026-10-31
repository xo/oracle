package oracle

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseDSN(t *testing.T) {
	tests := []struct {
		dsn  string
		want Config
	}{
		{
			"oracle://scott:tiger@db.example.com/orclpdb1",
			Config{User: "scott", Password: "tiger", Host: "db.example.com", Service: "orclpdb1"},
		},
		{
			"oracle://scott:tiger@db.example.com:1522/orclpdb1/orcl2",
			Config{User: "scott", Password: "tiger", Host: "db.example.com", Port: 1522, Service: "orclpdb1", Instance: "orcl2"},
		},
		{
			// a / in a service name is written as %2F
			"oracle://u@h/a%2Fb/i",
			Config{User: "u", Host: "h", Service: "a/b", Instance: "i"},
		},
		{
			"oracle://u:p%40ss@h?sid=XE",
			Config{User: "u", Password: "p@ss", Host: "h", SID: "XE"},
		},
		{
			"oracle://u:p@h/s?server=h2:1521&server=h3:1522&sslmode=verify-full&sslrootcert=/ca.pem&sslservername=db",
			Config{
				User: "u", Password: "p", Host: "h", Service: "s",
				Servers: []string{"h2:1521", "h3:1522"},
				SSLMode: SSLVerifyFull, SSLRootCert: "/ca.pem", SSLServerName: "db",
			},
		},
		{
			"oracle://u:p@h/s?timeout=30s&connect_timeout=1m&prefetch=100&lob_fetch=stream&lob_read=explicit&privilege=sysdba",
			Config{
				User: "u", Password: "p", Host: "h", Service: "s",
				Timeout: 30 * time.Second, ConnectTimeout: time.Minute, Prefetch: 100,
				LobFetch: LobFetchStream, LobRead: LobReadExplicit, Privilege: PrivilegeSYSDBA,
			},
		},
		{
			"oracle://u:p@h/s?encryption=required&checksum=requested&language=AMERICAN&territory=AMERICA&charset=UTF8&program=app&proxy_user=px&unix_socket=/tmp/s",
			Config{
				User: "u", Password: "p", Host: "h", Service: "s",
				Encryption: ServiceRequired, Checksum: ServiceRequested,
				Language: "AMERICAN", Territory: "AMERICA", Charset: "UTF8",
				Program: "app", ProxyUser: "px", UnixSocket: "/tmp/s",
			},
		},
		{
			// a descriptor names the addresses, so the host can be empty, and
			// the commas of a DN pass through, as sijms/go-ora#741 asked
			"oracle://u:p@?descriptor=" + "%28DESCRIPTION%3D%28ADDRESS%3D%28PROTOCOL%3Dtcps%29%28HOST%3Dhost%29%28PORT%3D1522%29%29%28SECURITY%3D%28SSL_SERVER_CERT_DN%3DCN%3Dadwc%2CO%3DOracle%2CC%3DUS%29%29%29",
			Config{
				User: "u", Password: "p",
				Descriptor: "(DESCRIPTION=(ADDRESS=(PROTOCOL=tcps)(HOST=host)(PORT=1522))(SECURITY=(SSL_SERVER_CERT_DN=CN=adwc,O=Oracle,C=US)))",
			},
		},
		{
			"oracle://u:p@h/s?auth=token&token=/t&token_key=/k&wallet=/w&wallet_password=wp",
			Config{
				User: "u", Password: "p", Host: "h", Service: "s",
				Auth: AuthToken, Token: "/t", TokenKey: "/k", Wallet: "/w", WalletPassword: "wp",
			},
		},
	}
	for _, test := range tests {
		got, err := ParseDSN(test.dsn)
		if err != nil {
			t.Errorf("ParseDSN(%q): %v", test.dsn, err)
			continue
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("ParseDSN(%q)\n got %+v\nwant %+v", test.dsn, got, test.want)
		}
		back, err := ParseDSN(got.DSN())
		if err != nil {
			t.Errorf("ParseDSN(%q.DSN()): %v", test.dsn, err)
			continue
		}
		if !reflect.DeepEqual(back, got) {
			t.Errorf("the DSN of %q does not read back\n got %+v\nwant %+v", test.dsn, back, got)
		}
	}
}

func TestParseDSNErrors(t *testing.T) {
	tests := []struct {
		dsn  string
		want error
	}{
		{"postgres://u:p@h/s", ErrScheme},
		{"oracle:opaque", ErrInvalidValue},
		{"oracle://u:p@h/s?SSL=true", ErrUnknownKey},
		{"oracle://u:p@h/s?SERVICE%20NAME=x", ErrUnknownKey},
		{"oracle://u:p@h/s?tls=true", ErrUnknownKey},
		{"oracle://u:p@h/s?sid=a&sid=b", ErrRepeatedKey},
		{"oracle://u:p@h:0/s", ErrInvalidValue},
		{"oracle://u:p@h:x/s", ErrInvalidValue},
		{"oracle://u:p@h/a/b/c", ErrInvalidValue},
		{"oracle://u:p@h", ErrInvalidValue},
		{"oracle://u:p@/s", ErrInvalidValue},
		{"oracle://u:p@h/s?sslmode=prefer", ErrInvalidValue},
		{"oracle://u:p@h/s?sslmode=allow", ErrInvalidValue},
		{"oracle://u:p@h/s?timeout=30", ErrInvalidValue},
		{"oracle://u:p@h/s?timeout=-1s", ErrInvalidValue},
		{"oracle://u:p@h/s?prefetch=0", ErrInvalidValue},
		{"oracle://u:p@h/s?lob_fetch=pre", ErrInvalidValue},
		{"oracle://u:p@h/s?auth=token&token=/t", ErrInvalidValue},
		{"oracle://u:p@h/s?sslcert=/c", ErrInvalidValue},
		{"oracle://u:p@h/s?server=h2", ErrInvalidValue},
		{"oracle://u:p@h/s?privilege=root", ErrInvalidValue},
		{"oracle://u:p@h/s?encryption=yes", ErrInvalidValue},
	}
	for _, test := range tests {
		_, err := ParseDSN(test.dsn)
		if !errors.Is(err, test.want) {
			t.Errorf("ParseDSN(%q): got %v, want %v", test.dsn, err, test.want)
		}
	}
}

func TestParseDSNHidesThePassword(t *testing.T) {
	for _, dsn := range []string{
		"oracle://u:s3cret@h:bad/s",
		"oracle://u:s3cret@h/s?nope=1",
		"oracle://u:s3cret@h/s\x7f",
	} {
		_, err := ParseDSN(dsn)
		if err == nil {
			t.Errorf("ParseDSN(%q): no error", dsn)
			continue
		}
		if strings.Contains(err.Error(), "s3cret") {
			t.Errorf("ParseDSN(%q): the error holds the password: %v", dsn, err)
		}
	}
}

func TestConnectionConfig(t *testing.T) {
	cfg, err := ParseDSN("oracle://sys:p@h/s?timeout=1500ms&connect_timeout=2s")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := cfg.connectionConfig()
	if err != nil {
		t.Fatal(err)
	}
	if conn.DBAPrivilege != 0 {
		t.Errorf("the user SYS has the privilege %d with no privilege key, and D11 gives it none", conn.DBAPrivilege)
	}
	if conn.Timeout != 1500*time.Millisecond || conn.ConnectTimeout != 2*time.Second {
		t.Errorf("timeouts: got %v and %v", conn.Timeout, conn.ConnectTimeout)
	}
	if len(conn.Servers) != 1 || conn.Servers[0].Addr != "h" || conn.Servers[0].Port != DefaultPort {
		t.Errorf("servers: got %+v", conn.Servers)
	}
	clone := conn.Clone()
	clone.Servers[0].Addr = "other"
	if conn.Servers[0].Addr != "h" {
		t.Error("Clone shares the servers of the original")
	}
}

func TestClientCID(t *testing.T) {
	if got := (Client{}).cid(); got != "" {
		t.Errorf("an empty Client gives %q, and the protocol code fills it in", got)
	}
	want := "(CID=(PROGRAM=p)(HOST=h)(USER=u))"
	if got := (Client{Program: "p", Host: "h", User: "u"}).cid(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTLSConfig(t *testing.T) {
	for _, mode := range []SSLMode{"", SSLDisable} {
		if c, err := (Config{Host: "h", SSLMode: mode}).tlsConfig(); err != nil || c != nil {
			t.Errorf("sslmode %q: got %v, %v, want no TLS", mode, c, err)
		}
	}
	c, err := (Config{Host: "h", SSLMode: SSLRequire}).tlsConfig()
	if err != nil || c == nil || !c.InsecureSkipVerify {
		t.Errorf("require: got %+v, %v", c, err)
	}
	c, err = (Config{Host: "h", SSLMode: SSLVerifyFull}).tlsConfig()
	if err != nil || c == nil || c.InsecureSkipVerify || c.ServerName != "h" {
		t.Errorf("verify-full: got %+v, %v", c, err)
	}
	c, err = (Config{Host: "h", SSLMode: SSLVerifyCA}).tlsConfig()
	if err != nil || c == nil || !c.InsecureSkipVerify || c.VerifyConnection == nil {
		t.Errorf("verify-ca: got %+v, %v", c, err)
	}
	if _, err := (Config{Host: "h", SSLMode: SSLVerifyFull, SSLRootCert: "/no/such/file"}).tlsConfig(); err == nil {
		t.Error("a missing sslrootcert gives no error")
	}
}

func TestConnectorClose(t *testing.T) {
	c, err := NewConnector(Config{Host: "h", Service: "s"})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Connect(t.Context()); !errors.Is(err, ErrConnectorClosed) {
		t.Errorf("Connect after Close: got %v, want ErrConnectorClosed", err)
	}
	if d, ok := c.Driver().(Driver); !ok || d.c != c {
		t.Error("Driver does not point back at its connector")
	}
}
