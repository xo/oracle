package tns

import "testing"

// TestBadCodes checks that the errors that end a session, as ORA-00028 of a
// killed session and ORA-01012 after it, make the connection bad, and that an
// error of a statement does not.
func TestBadCodes(t *testing.T) {
	for _, test := range []struct {
		code int
		bad  bool
	}{
		{28, true},
		{1012, true},
		{3113, true},
		{3135, true},
		{12570, true},
		{1, false},
		{942, false},
		{1403, false},
		{6550, false},
		{20101, false},
	} {
		if bad := NewOracleError(test.code).Bad(); bad != test.bad {
			t.Errorf("ORA-%05d: bad %t, want %t", test.code, bad, test.bad)
		}
	}
}
