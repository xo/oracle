package config

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

const defaultPort int = 1521

var generalQuoteRegexp = regexp.MustCompile(`(?i)([a-z0-9_]+\s*=\s*)"([^"]*)"`)

type DBAPrivilege int

const (
	NONE      DBAPrivilege = 0
	SYSDBA    DBAPrivilege = 0x20
	SYSOPER   DBAPrivilege = 0x40
	SYSASM    DBAPrivilege = 0x00400000
	SYSBACKUP DBAPrivilege = 0x01000000
	SYSDG     DBAPrivilege = 0x02000000
	SYSKM     DBAPrivilege = 0x04000000
	SYSRAC    DBAPrivilege = 0x08000000
)

type AuthType int

const (
	Normal   AuthType = 0
	OS       AuthType = 1
	Kerberos AuthType = 2
	TCPS     AuthType = 3
)

type ServerAddr struct {
	Protocol string
	Addr     string
	Port     int
}

type DatabaseInfo struct {
	UserID          string
	Password        string
	Servers         []ServerAddr
	serverIndex     int
	SID             string
	ProxyClientName string
	ServiceName     string
	InstanceName    string
	DBName          string
	DBAPrivilege    DBAPrivilege
	AuthType        AuthType
	Wallet          *Wallet
	connStr         string
}

// ExtractServers returns the address of each ADDRESS of the descriptor
// connStr, in order, at any depth. An address keeps HOST, PORT and PROTOCOL,
// and a PORT it lacks is 1521. go-ora matched an address with a regular
// expression that took no other key, so an address with HTTPS_PROXY or
// SEND_BUF_SIZE was dropped.
func ExtractServers(connStr string) ([]ServerAddr, error) {
	var addresses []ServerAddr
	var err error
	walkDescriptor(connStr, func(key string, values map[string]string) {
		if err != nil || key != "ADDRESS" {
			return
		}
		server := ServerAddr{Port: 1521, Protocol: values["PROTOCOL"], Addr: values["HOST"]}
		if port, ok := values["PORT"]; ok {
			if server.Port, err = strconv.Atoi(port); err != nil {
				err = fmt.Errorf("reading the port %q of %s: %w", port, server.Addr, err)
				return
			}
		}
		if server.Addr != "" {
			addresses = append(addresses, server)
		}
	})
	return addresses, err
}

// walkDescriptor calls f with the upper case key of each group of the
// descriptor s, such as ADDRESS for (ADDRESS=(HOST=h)(PORT=1)), and the
// values of its groups that hold a value, keyed by the upper case of their
// keys, in the order in which the groups close.
func walkDescriptor(s string, f func(key string, values map[string]string)) {
	type group struct {
		key    string
		values map[string]string
	}
	var stack []group
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			eq := strings.IndexAny(s[i+1:], "=()")
			if eq < 0 || s[i+1+eq] != '=' {
				continue
			}
			key := strings.ToUpper(strings.TrimSpace(s[i+1 : i+1+eq]))
			i += eq + 1
			// a value runs to the parenthesis that closes the group, and a
			// group with no value holds other groups
			rest := strings.TrimLeft(s[i+1:], " \t\n\r")
			if !strings.HasPrefix(rest, "(") {
				end := strings.IndexByte(s[i+1:], ')')
				if end < 0 {
					return
				}
				value := strings.Trim(strings.TrimSpace(s[i+1:i+1+end]), `"`)
				if len(stack) > 0 {
					stack[len(stack)-1].values[key] = value
				}
				f(key, nil)
				i += end + 1
				continue
			}
			stack = append(stack, group{key: key, values: map[string]string{}})
		case ')':
			if len(stack) == 0 {
				continue
			}
			g := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			f(g.key, g.values)
		}
	}
}

func (info *DatabaseInfo) UpdateDatabaseInfo(connStr string) (err error) {
	connStr = strings.ReplaceAll(connStr, "\r", "")
	connStr = strings.ReplaceAll(connStr, "\n", "")

	connStr = generalQuoteRegexp.ReplaceAllString(connStr, "$1$2")

	info.Servers, err = ExtractServers(connStr)
	if err != nil {
		return err
	}

	if len(info.Servers) == 0 {
		return errors.New("no address passed in connection string")
	}

	info.connStr = connStr

	var serviceNameRegexp *regexp.Regexp
	serviceNameRegexp, err = regexp.Compile(`(?i)\(\s*SERVICE_NAME\s*=\s*([\w.-]+)\s*\)`)
	if err != nil {
		return err
	}

	match := serviceNameRegexp.FindStringSubmatch(connStr)
	if len(match) > 1 {
		info.ServiceName = match[1]
	}

	var sidRegexp *regexp.Regexp
	sidRegexp, err = regexp.Compile(`(?i)\(\s*SID\s*=\s*([\w.-]+)\s*\)`)
	if err != nil {
		return err
	}

	match = sidRegexp.FindStringSubmatch(connStr)
	if len(match) > 1 {
		info.SID = match[1]
	}

	var instanceNameRegexp *regexp.Regexp
	instanceNameRegexp, err = regexp.Compile(`(?i)\(\s*INSTANCE_NAME\s*=\s*([\w.-]+)\s*\)`)
	if err != nil {
		return err
	}

	match = instanceNameRegexp.FindStringSubmatch(connStr)
	if len(match) > 1 {
		info.InstanceName = match[1]
	}

	return nil
}

func (info *DatabaseInfo) UpdateDatabaseInfoForRedirect(redirectAddr string, reconnectData string) error {
	redirectAddr = strings.ReplaceAll(redirectAddr, "\r", "")
	redirectAddr = strings.ReplaceAll(redirectAddr, "\n", "")
	reconnectData = strings.ReplaceAll(reconnectData, "\r", "")
	reconnectData = strings.ReplaceAll(reconnectData, "\n", "")
	var err error
	info.Servers, err = ExtractServers(redirectAddr)
	if err != nil {
		return err
	}
	if len(info.Servers) == 0 {
		return errors.New("no address passed in connection string")
	}
	r, err := regexp.Compile(`(?i)\(\s*SERVICE_NAME\s*=\s*([\w.-]+)\s*\)`)
	if err != nil {
		return err
	}
	match := r.FindStringSubmatch(reconnectData)
	if len(match) > 1 {
		info.ServiceName = match[1]
	}
	r, err = regexp.Compile(`(?i)\(\s*SID\s*=\s*([\w.-]+)\s*\)`)
	if err != nil {
		return err
	}
	match = r.FindStringSubmatch(reconnectData)
	if len(match) > 1 {
		info.SID = match[1]
	}
	r, err = regexp.Compile(`(?i)\(\s*INSTANCE_NAME\s*=\s*([\w.-]+)\s*\)`)
	if err != nil {
		return err
	}
	match = r.FindStringSubmatch(reconnectData)
	if len(match) > 1 {
		info.InstanceName = match[1]
	}
	info.connStr = ""
	return nil
}

func (info *DatabaseInfo) AddServer(server ServerAddr) {
	for i := 0; i < len(info.Servers); i++ {
		if server.IsEqual(&info.Servers[i]) {
			return
		}
	}
	info.Servers = append(info.Servers, server)
}

func (serv *ServerAddr) IsEqual(input *ServerAddr) bool {
	return strings.EqualFold(strings.ToUpper(serv.Addr), strings.ToUpper(input.Addr)) &&
		serv.Port == input.Port
}

func (serv *ServerAddr) NetworkAddr() string {
	return net.JoinHostPort(serv.Addr, strconv.Itoa(serv.Port))
}

func (info *DatabaseInfo) ResetServerIndex() {
	info.serverIndex = 0
}

func (info *DatabaseInfo) ActiveServer(jump bool) *ServerAddr {
	if jump {
		info.serverIndex++
	}
	if info.serverIndex >= len(info.Servers) {
		return nil
	}
	return &info.Servers[info.serverIndex]
}

func DBAPrivilegeFromString(s string) DBAPrivilege {
	switch strings.ToUpper(s) {
	case "SYSDBA":
		return SYSDBA
	case "SYSOPER":
		return SYSOPER
	case "SYSASM":
		return SYSASM
	case "SYSBACKUP":
		return SYSBACKUP
	case "SYSDG":
		return SYSDG
	case "SYSKM":
		return SYSKM
	case "SYSRAC":
		return SYSRAC
	default:
		return NONE
	}
}
