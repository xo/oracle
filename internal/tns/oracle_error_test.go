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

// TestErrVerbose checks the form of SQL*Plus that usql shows for
// \errverbose: the line of the statement with a * under the position of the
// error, the number of the line, and the message with its stack. The
// positions are those that 11g sent for each statement.
func TestErrVerbose(t *testing.T) {
	for _, test := range []struct {
		name  string
		query string
		pos   int
		msg   string
		want  string
	}{
		{
			"table", "select * from nosuch", 14,
			"ORA-00942: table or view does not exist\n",
			"select * from nosuch\n              *\nERROR at line 1:\nORA-00942: table or view does not exist",
		},
		{
			"second line", "select 1 from dual\nwhere nosuch = 1", 25,
			"ORA-00904: \"NOSUCH\": invalid identifier\n",
			"where nosuch = 1\n      *\nERROR at line 2:\nORA-00904: \"NOSUCH\": invalid identifier",
		},
		{
			// the server counts bytes, and the * counts characters
			"multibyte", "select 'éé', nosuch from dual", 15,
			"ORA-00904: \"NOSUCH\": invalid identifier\n",
			"select 'éé', nosuch from dual\n             *\nERROR at line 1:\nORA-00904: \"NOSUCH\": invalid identifier",
		},
		{
			"stack", "begin\n  raise_application_error(-20001, 'boom');\nend;", 0,
			"ORA-20001: boom\nORA-06512: at line 2\n",
			"begin\n*\nERROR at line 1:\nORA-20001: boom\nORA-06512: at line 2",
		},
		{
			"tab", "select\t*\r\nfrom\tnosuch", 15,
			"ORA-00942: table or view does not exist\n",
			"from\tnosuch\n    \t*\nERROR at line 2:\nORA-00942: table or view does not exist",
		},
		{
			"end", "select * from", 13,
			"ORA-00903: invalid table name\n",
			"select * from\n             *\nERROR at line 1:\nORA-00903: invalid table name",
		},
		{"no query", "", 0, "ORA-01017: invalid username/password; logon denied\n", ""},
		{"no position", "select 1 from dual", -1, "ORA-24344: success with compilation error", ""},
		{"past the end", "select 1 from dual", 40, "ORA-00942: table or view does not exist\n", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := &OracleError{ErrCode: 1, ErrMsg: test.msg, errPos: test.pos}
			if got := err.ErrVerbose(test.query); got != test.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, test.want)
			}
		})
	}
}
