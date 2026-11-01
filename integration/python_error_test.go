package integration

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/xo/oracle"
)

// TestServerErrors holds cases of tests/misc/test_1100_error.py of
// python-oracledb (1100, 1101 and 1108), written again here: the position of
// a parse error, and the codes of the errors of raise_application_error.
func TestServerErrors(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.ExecContext(t.Context(), "begin t_Missing := 5; end;")
	var serr *oracle.ServerError
	switch {
	case !errors.As(err, &serr):
		t.Errorf("a parse error: got %v, want a *ServerError", err)
	case serr.ErrCode != 6550 || serr.ErrPos() != 6:
		t.Errorf("a parse error: got ORA-%05d at %d, want ORA-06550 at 6", serr.ErrCode, serr.ErrPos())
	}
	for _, code := range []int{20000, 20101, 20500, 20999} {
		_, err := db.ExecContext(t.Context(), fmt.Sprintf("begin raise_application_error(-%d, 'Test!'); end;", code))
		switch {
		case !errors.As(err, &serr):
			t.Errorf("ORA-%d: got %v, want a *ServerError", code, err)
		case serr.ErrCode != code || serr.ErrPos() != 0 || !strings.Contains(err.Error(), "Test!"):
			t.Errorf("ORA-%d: got ORA-%05d at %d, %v", code, serr.ErrCode, serr.ErrPos(), err)
		}
	}
}

// TestKilledSession kills the session of a connection from another, as 1109
// of tests/misc/test_1100_error.py of python-oracledb does. The query after
// it fails, the connection is then bad, and the pool runs the next query on a
// new connection. go-ora marked the connection bad but sent the next query
// on it all the same, and the server answered ORA-01012.
func TestKilledSession(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	admin, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	ctx := t.Context()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var sid, serial int
	if err := conn.QueryRowContext(ctx, `SELECT sid, serial# FROM v$session
		WHERE sid = SYS_CONTEXT('USERENV', 'SID')`).Scan(&sid, &serial); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.ExecContext(ctx, fmt.Sprintf("ALTER SYSTEM KILL SESSION '%d,%d'", sid, serial)); err != nil {
		t.Fatal(err)
	}
	var user string
	if err := conn.QueryRowContext(ctx, "SELECT user FROM dual").Scan(&user); err == nil {
		t.Fatal("a query after the kill: no error")
	} else {
		t.Logf("a query after the kill: %v", err)
	}
	// database/sql closes the connection when the driver says that it is
	// bad, and some servers close it at the kill
	if err := conn.QueryRowContext(ctx, "SELECT user FROM dual").Scan(&user); !errors.Is(err, driver.ErrBadConn) && !errors.Is(err, sql.ErrConnDone) {
		t.Errorf("a second query after the kill: got %v, want ErrBadConn or ErrConnDone", err)
	}
	conn.Close()
	if err := db.QueryRowContext(ctx, "SELECT user FROM dual").Scan(&user); err != nil {
		t.Errorf("a query of the pool after the kill: %v", err)
	}
}

// TestCompilationError creates a procedure, a function and a type that do not
// compile, as 1104 to 1106 of tests/misc/test_1100_error.py of
// python-oracledb do. The server creates each one, and the statement returns
// ORA-24344, which matches ErrCompilation (D32). go-ora returned no error, so
// the caller did not know. A unit that compiles returns none.
func TestCompilationError(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := t.Context()
	for _, test := range []struct {
		bad, good, drop string
	}{
		{"CREATE OR REPLACE PROCEDURE go_bad_proc AS BEGIN NULL END;",
			"CREATE OR REPLACE PROCEDURE go_bad_proc AS BEGIN NULL; END;",
			"DROP PROCEDURE go_bad_proc"},
		{"CREATE OR REPLACE FUNCTION go_bad_func RETURN NUMBER AS BEGIN RETURN NULL END;",
			"CREATE OR REPLACE FUNCTION go_bad_func RETURN NUMBER AS BEGIN RETURN NULL; END;",
			"DROP FUNCTION go_bad_func"},
		{"CREATE OR REPLACE TYPE go_bad_type AS OBJECT (x go_no_such_type)",
			"CREATE OR REPLACE TYPE go_bad_type AS OBJECT (x NUMBER)",
			"DROP TYPE go_bad_type"},
	} {
		_, err := db.ExecContext(ctx, test.bad)
		var serr *oracle.ServerError
		if !errors.Is(err, oracle.ErrCompilation) || !errors.As(err, &serr) || serr.ErrCode != 24344 {
			t.Errorf("%s: got %v, want ORA-24344", test.bad, err)
		}
		var status string
		name := strings.Fields(test.drop)[2]
		if err := db.QueryRowContext(ctx, "SELECT status FROM user_objects WHERE object_name = :1",
			strings.ToUpper(name)).Scan(&status); err != nil || status != "INVALID" {
			t.Errorf("%s: the status %q, %v, want INVALID", name, status, err)
		}
		if _, err := db.ExecContext(ctx, test.good); err != nil {
			t.Errorf("%s: %v", test.good, err)
		}
		if _, err := db.ExecContext(ctx, test.drop); err != nil {
			t.Errorf("%s: %v", test.drop, err)
		}
	}
}
