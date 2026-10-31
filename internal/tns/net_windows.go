//go:build windows

package tns

import (
	"net"
)

func sendOOB(conn net.Conn) (done bool, err error) {
	return
}
