package go_ora

// ResetCookies forgets the fast login cookie of every server. The store is
// one for the whole process, so a test that compares the packets of a logon
// must start from an empty store. D9 moves the store to the Connector.
func ResetCookies() {
	cookieMu.Lock()
	clear(cookieStore)
	cookieMu.Unlock()
}
