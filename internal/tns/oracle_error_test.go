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

// TestAuthenticationRefused checks the method that usql asks an error for:
// true for the codes of a refused credential and for ErrAuthentication, and
// false for a lack of a permission and for any other error.
func TestAuthenticationRefused(t *testing.T) {
	for _, test := range []struct {
		code int
		want bool
	}{
		{1017, true},
		{1005, true},
		{28041, false}, // the logon data of the server did not read
		{28000, false}, // a locked account
		{28001, false}, // an expired password
		{1031, false},  // insufficient privileges
		{1045, false},  // lacks CREATE SESSION
		{942, false},
	} {
		if got := NewOracleError(test.code).AuthenticationRefused(); got != test.want {
			t.Errorf("ORA-%05d: got %t, want %t", test.code, got, test.want)
		}
	}
	if !ErrAuthentication.AuthenticationRefused() || ErrNotSupported.AuthenticationRefused() {
		t.Error("the sentinels: ErrAuthentication must be refused, and ErrNotSupported not")
	}
}

// TestErrorText checks that Error returns the message of the server alone,
// with no line feed at its end and no position, which ErrPos returns.
func TestErrorText(t *testing.T) {
	err := &OracleError{ErrCode: 942, ErrMsg: "ORA-00942: table or view does not exist\n", errPos: 14}
	if got, want := err.Error(), "ORA-00942: table or view does not exist"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if err.ErrPos() != 14 {
		t.Errorf("the position: got %d, want 14", err.ErrPos())
	}
	multi := &OracleError{ErrCode: 6550, ErrMsg: "ORA-06550: line 1, column 7:\nPLS-00201: identifier 'X' must be declared\n"}
	if got, want := multi.Error(), "ORA-06550: line 1, column 7:\nPLS-00201: identifier 'X' must be declared"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
