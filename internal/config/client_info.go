package config

import (
	"os"
	"os/user"
)

type ClientInfo struct {
	ProgramPath string
	ProgramName string
	OSUserName  string
	OSPassword  string
	HostName    string
	// Terminal is the terminal that the logon reports, or HostName when it
	// is empty
	Terminal    string
	DomainName  string
	DriverName  string
	PID         int
	UseKerberos bool
	Language    string
	Territory   string
	CharsetID   int
	Cid         string
}

func getCurrentUser() *user.User {
	if userName := os.Getenv("USER"); len(userName) > 0 {
		return &user.User{
			Uid:      "",
			Gid:      "",
			Username: userName,
			Name:     userName,
			HomeDir:  "",
		}
	} else {
		temp, _ := user.Current()
		return temp
	}
}

// TerminalName returns the terminal that the logon reports.
func (info ClientInfo) TerminalName() string {
	if info.Terminal != "" {
		return info.Terminal
	}
	return info.HostName
}
