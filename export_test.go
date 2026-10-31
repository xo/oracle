package oracle

import "github.com/xo/oracle/internal/ttc"

// ResetCookies forgets the fast login cookie of every server, so that a
// test that compares the packets of a logon starts from an empty store.
func ResetCookies() {
	ttc.ResetCookies()
}
