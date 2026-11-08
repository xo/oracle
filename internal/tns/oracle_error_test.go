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

// TestErrVerbose checks the form of psql that usql shows for \errverbose:
// ERROR and the first error, the line of the statement with a ^ under the
// position, then DETAIL, HINT and CONTEXT. The positions and the messages are
// those that 11g and 26ai sent for each statement.
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
			"ERROR:  ORA-00942: table or view does not exist\nLINE 1: select * from nosuch\n                      ^",
		},
		{
			"help", "select blah", 7,
			"ORA-00904: \"BLAH\": invalid identifier\nHelp: https://docs.oracle.com/error-help/db/ora-00904/\n",
			"ERROR:  ORA-00904: \"BLAH\": invalid identifier\nLINE 1: select blah\n               ^\nHINT:  https://docs.oracle.com/error-help/db/ora-00904/",
		},
		{
			"second line", "select 1 from dual\nwhere nosuch = 1", 25,
			"ORA-00904: \"NOSUCH\": invalid identifier\n",
			"ERROR:  ORA-00904: \"NOSUCH\": invalid identifier\nLINE 2: where nosuch = 1\n              ^",
		},
		{
			// the server counts bytes, and the ^ counts characters
			"multibyte", "select 'éé', nosuch from dual", 15,
			"ORA-00904: \"NOSUCH\": invalid identifier\n",
			"ERROR:  ORA-00904: \"NOSUCH\": invalid identifier\nLINE 1: select 'éé', nosuch from dual\n                     ^",
		},
		{
			"crlf", "select *\r\nfrom nosuch", 15,
			"ORA-00942: table or view does not exist\n",
			"ERROR:  ORA-00942: table or view does not exist\nLINE 2: from nosuch\n             ^",
		},
		{
			"end", "select * from", 13,
			"ORA-00903: invalid table name\n",
			"ERROR:  ORA-00903: invalid table name\nLINE 1: select * from\n                     ^",
		},
		{
			// a position of 0 is an error with none
			"stack", "begin\n  raise_application_error(-20001, 'boom');\nend;", 0,
			"ORA-20001: boom\nORA-06512: at \"SYS.P\", line 3\nORA-06512: at line 2\n",
			"ERROR:  ORA-20001: boom\nCONTEXT:  ORA-06512: at \"SYS.P\", line 3\nORA-06512: at line 2",
		},
		{
			"compile", "begin\n  nosuch;\nend;", 8,
			"ORA-06550: line 2, column 3:\nPLS-00201: identifier 'NOSUCH' must be declared\nORA-06550: line 2, column 3:\nPL/SQL: Statement ignored\n",
			"ERROR:  ORA-06550: line 2, column 3:\nLINE 2:   nosuch;\n          ^\nDETAIL:  PLS-00201: identifier 'NOSUCH' must be declared\nDETAIL:  ORA-06550: line 2, column 3:\nDETAIL:  PL/SQL: Statement ignored",
		},
		{"no query", "", 0, "ORA-01017: invalid username/password; logon denied\n", "ERROR:  ORA-01017: invalid username/password; logon denied"},
		{"no position", "select 1 from dual", -1, "ORA-24344: success with compilation error", "ERROR:  ORA-24344: success with compilation error"},
		{"past the end", "select 1 from dual", 40, "ORA-00942: table or view does not exist\n", "ERROR:  ORA-00942: table or view does not exist"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := &OracleError{ErrCode: 1, ErrMsg: test.msg, errPos: test.pos}
			if got := err.ErrVerbose(test.query); got != test.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, test.want)
			}
		})
	}
}
