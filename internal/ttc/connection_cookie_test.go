package ttc

import (
	"testing"

	"github.com/xo/oracle/internal/config"
	"github.com/xo/oracle/internal/tns"
)

// TestCookiesPerRegistry is sijms/go-ora#692: two databases with the same
// service name shared one fast login cookie, and so the charset of the first.
func TestCookiesPerRegistry(t *testing.T) {
	uuid := []byte("0123456789abcdef")
	conn := func(reg *Registry, charset int) *Conn {
		t.Helper()
		c, err := reg.NewConnection(&config.ConnectionConfig{DatabaseInfo: config.DatabaseInfo{ServiceName: "XE"}})
		if err != nil {
			t.Fatal(err)
		}
		c.session = &tns.Session{Context: &tns.SessionContext{UUID: uuid}}
		c.tcpNego = &TCPNego{ServerCharset: charset}
		return c
	}
	reg1, reg2 := NewRegistry(), NewRegistry()
	saveCookie(conn(reg1, 873))
	if got := lookupCookie(conn(reg1, 0)); got == nil || got.ServerCharset != 873 {
		t.Errorf("the registry that saved the cookie finds %+v", got)
	}
	if got := lookupCookie(conn(reg2, 0)); got != nil {
		t.Errorf("another registry finds the cookie with charset %d", got.ServerCharset)
	}
	deleteCookie(conn(reg1, 0))
	if got := lookupCookie(conn(reg1, 0)); got != nil {
		t.Errorf("a deleted cookie is still there")
	}
}
