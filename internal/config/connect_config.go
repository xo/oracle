package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

type LobFetch int

const (
	INLINE LobFetch = 0
	STREAM LobFetch = 1
)

type LobReadMode int

const (
	LobReadModeAuto LobReadMode = iota
	LobReadModeNone
)

type KerberosAuthenticator interface {
	Authenticate(server, service string) ([]byte, error)
}

// NTSAuthenticator makes the NTLM messages of NTS, the Windows
// authentication of Oracle Net.
type NTSAuthenticator interface {
	NewNegotiateMessage(domain, machine string) ([]byte, error)
	ProcessChallenge(chaMsgData []byte, user, password string) ([]byte, error)
}

type AdvNegoServiceInfo struct {
	AuthService     []string
	EncServiceLevel int
	IntServiceLevel int
	// Kerberos authenticates with Kerberos, which needs it.
	Kerberos KerberosAuthenticator
	// NTS authenticates with NTS. Nil uses the NTLM of the driver.
	NTS NTSAuthenticator
}
type ConnectionConfig struct {
	ClientInfo
	DatabaseInfo
	SessionInfo
	AdvNegoServiceInfo
	// Logger receives the packets and events of the connection. Nil logs
	// nothing (D14).
	Logger *slog.Logger
	// SkipNLS skips the query of the NLS settings at logon.
	SkipNLS             bool
	PrefetchRows        int
	Lob                 LobFetch
	LobReadMode         LobReadMode
	FastLogin           bool
	TokenFile           string
	TokenPrivateKeyFile string
}

// Clone returns a copy of config that shares no slice and no TLS
// configuration with it, because a connection changes its copy as it
// connects.
func (config *ConnectionConfig) Clone() *ConnectionConfig {
	c := *config
	c.Servers = slices.Clone(config.Servers)
	c.AuthService = slices.Clone(config.AuthService)
	if config.TLSConfig != nil {
		c.TLSConfig = config.TLSConfig.Clone()
	}
	return &c
}

func (config *ConnectionConfig) ConnectionData() string {
	if len(config.connStr) != 0 {
		return config.connStr
	}
	host := config.ActiveServer(false)
	protocol := config.Protocol
	if host.Protocol != "" {
		protocol = host.Protocol
	}
	FulCid := "(CID=(PROGRAM=" + config.ProgramPath + ")(HOST=" + config.HostName + ")(USER=" + config.OSUserName + "))"
	if len(config.Cid) > 0 {
		FulCid = config.Cid
	}
	var address string
	if len(config.UnixAddress) > 0 {
		address = "(ADDRESS=(PROTOCOL=IPC)(KEY=EXTPROC1))"
	} else {
		address = "(ADDRESS=(PROTOCOL=" + protocol + ")(HOST=" + host.Addr + ")(PORT=" + strconv.Itoa(host.Port) + "))"
	}

	connectData := "(CONNECT_DATA="
	if config.SID != "" {
		connectData += "(SID=" + config.SID + ")"
	} else {
		connectData += "(SERVICE_NAME=" + config.ServiceName + ")"
	}
	if config.InstanceName != "" {
		connectData += "(INSTANCE_NAME=" + config.InstanceName + ")"
	}
	// should add connection id also here
	//(CONNECTION_ID=/lKthdECvk6r+ivbdaDGJQ==)
	connectData += FulCid

	// (security=(ssl_server_dn_match=yes))
	security := ""
	if config.SSLVerify {
		security = "(security=(ssl_server_dn_match=yes))"
	}
	return "(DESCRIPTION=" + address + connectData + security + "))"
}

// DefaultPrefetch is the prefetch of a connection that sets none. A
// statement with it sizes each fetch by the size of its rows.
const DefaultPrefetch = 25

// New returns the configuration of a connection with the defaults of the
// protocol: the default prefetch, fast login, a connect timeout of 60
// seconds, TCP, and the language AMERICAN in the territory AMERICA.
func New() *ConnectionConfig {
	return &ConnectionConfig{
		PrefetchRows: DefaultPrefetch,
		FastLogin:    true,
		SessionInfo: SessionInfo{
			ConnectTimeout:        60 * time.Second,
			TransportDataUnitSize: 0x200000,
			SessionDataUnitSize:   0x200000,
			Protocol:              "tcp",
			SSLVerify:             true,
		},
		DatabaseInfo: DatabaseInfo{
			Servers: make([]ServerAddr, 0, 3),
		},
		ClientInfo: ClientInfo{Territory: "AMERICA", Language: "AMERICAN"},
	}
}

// LoadWallet reads the wallet in dir: ewallet.p12 when it exists and password
// is set, and cwallet.sso when not. When the configuration has a user and no
// password, it takes the password of that user from the wallet.
func (config *ConnectionConfig) LoadWallet(dir, password string) error {
	if len(config.ServiceName) == 0 {
		return errors.New("you should specify server/service if you will use wallet")
	}
	var err error
	if _, err = os.Stat(path.Join(dir, "ewallet.p12")); err == nil && len(password) > 0 {
		fileData, err := os.ReadFile(path.Join(dir, "ewallet.p12"))
		if err != nil {
			return err
		}
		config.Wallet = &Wallet{password: []byte(password)}
		if err := config.Wallet.readPKCS12(fileData); err != nil {
			return err
		}
	} else {
		config.Wallet, err = NewWallet(path.Join(dir, "cwallet.sso"))
		if err != nil {
			return err
		}
	}
	if len(config.UserID) > 0 && len(config.Password) == 0 {
		serv := config.Servers[0]
		cred, err := config.Wallet.getCredential(serv.Addr, serv.Port, config.ServiceName, config.UserID)
		if err != nil {
			return err
		}
		if cred == nil {
			return fmt.Errorf("cannot find credentials for server: %s:%d, service: %s,  username: %s",
				serv.Addr, serv.Port, config.ServiceName, config.UserID)
		}
		config.UserID = cred.username
		config.Password = cred.password
	}
	return nil
}

// CharsetID returns the Oracle ID of the charset with name.
func CharsetID(name string) (int, error) {
	return getCharsetID(name)
}

// Validate checks that config names a database, and fills in the client
// fields from this process: the user, the host, the program and the PID.
func (config *ConnectionConfig) Validate() error {
	if len(config.SID) == 0 && len(config.ServiceName) == 0 {
		return errors.New("empty SID and service name")
	}
	if config.AuthType == Kerberos {
		config.AuthService = append(config.AuthService, "KERBEROS5")
	}
	if config.AuthType == TCPS {
		config.AuthService = append(config.AuthService, "TCPS")
	}
	if len(config.TokenFile) > 0 && len(config.TokenPrivateKeyFile) > 0 {

	} else {
		if len(config.UserID) == 0 || len(config.Password) == 0 && config.AuthType == Normal {
			config.AuthType = OS
		}
	}

	if config.AuthType == OS {
		if runtime.GOOS == "windows" {
			config.AuthService = append(config.AuthService, "NTS")
		}
	}

	if config.SSL {
		config.Protocol = "tcps"
	}

	// get client info
	var idx int
	temp := getCurrentUser()

	if temp != nil {
		idx = strings.Index(temp.Username, "\\")
		if idx >= 0 {
			if len(config.DomainName) == 0 {
				config.DomainName = temp.Username[:idx]
			}
			if len(config.ClientInfo.OSUserName) == 0 {
				config.ClientInfo.OSUserName = temp.Username[idx+1:]
			}
		} else {
			if len(config.ClientInfo.OSUserName) == 0 {
				config.ClientInfo.OSUserName = temp.Username
			}
		}
	}
	config.HostName, _ = os.Hostname()

	config.ProgramPath = os.Args[0]
	if config.ProgramName == "" {
		config.ProgramName = filepath.Base(os.Args[0])
	}
	config.DriverName = "OracleClientGo"
	config.PID = os.Getpid()
	return nil
}
