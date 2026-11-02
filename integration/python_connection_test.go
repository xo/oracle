package integration

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/xo/oracle"
)

// dsnAs returns ORACLE_DSN with the user and the password replaced, and the
// key and value added to its query when key is not empty.
func dsnAs(t *testing.T, user, password, key, value string) string {
	t.Helper()
	u, err := url.Parse(os.Getenv("ORACLE_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword(user, password)
	if key != "" {
		q := u.Query()
		q.Set(key, value)
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// openDSN opens a database for dsn.
func openDSN(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	cfg, err := oracle.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	c, err := oracle.NewConnector(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return sql.OpenDB(c)
}

// createUser creates a user that can connect, and returns a func that drops
// it. It skips the test when the server refuses the user, as an autonomous
// database does a password that its rules refuse.
func createUser(t *testing.T, db *sql.DB, name, password string) func() {
	t.Helper()
	db.Exec("DROP USER " + name + " CASCADE")
	if _, err := db.ExecContext(t.Context(), fmt.Sprintf(`CREATE USER %s IDENTIFIED BY "%s"`, name, password)); err != nil {
		t.Skipf("creating the user %s: %v", name, err)
	}
	if _, err := db.ExecContext(t.Context(), "GRANT CREATE SESSION TO "+name); err != nil {
		t.Fatal(err)
	}
	return func() {
		db.Exec("DROP USER " + name + " CASCADE")
	}
}

// TestConnectRefused holds 1005 and 1006 of
// tests/connection/test_1000_basic.py of python-oracledb, written again
// here: a DSN that does not parse, one with no password, and a wrong
// password. The error of a refused credential has the method
// AuthenticationRefused, which usql asks for.
func TestConnectRefused(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := oracle.ParseDSN("not a valid connect string!!"); err == nil {
		t.Error("a DSN that does not parse: no error")
	}
	u, _ := url.Parse(os.Getenv("ORACLE_DSN"))
	u.User = url.User(u.User.Username())
	noPassword := openDSN(t, u.String())
	defer noPassword.Close()
	if err := noPassword.PingContext(t.Context()); !refused(err) {
		t.Errorf("no password: got %v, want an error that says the credential was refused", err)
	}
	password, _ := url.Parse(os.Getenv("ORACLE_DSN"))
	pw, _ := password.User.Password()
	wrong := openDSN(t, dsnAs(t, password.User.Username(), pw+"X", "", ""))
	defer wrong.Close()
	if err := wrong.PingContext(t.Context()); !errors.Is(err, oracle.ErrAuthentication) || !refused(err) {
		t.Errorf("a wrong password: got %v, want ErrAuthentication", err)
	}
}

// refused reports whether an error in the tree of err has the method
// AuthenticationRefused and returns true, as usql asks it.
func refused(err error) bool {
	if r, ok := err.(interface{ AuthenticationRefused() bool }); ok && r.AuthenticationRefused() {
		return true
	}
	switch e := err.(type) {
	case interface{ Unwrap() error }:
		return refused(e.Unwrap())
	case interface{ Unwrap() []error }:
		for _, err := range e.Unwrap() {
			if refused(err) {
				return true
			}
		}
	}
	return false
}

// TestPasswordSymbols connects with a password that holds / and @, as 1009
// of tests/connection/test_1000_basic.py of python-oracledb does. The DSN
// escapes them.
func TestPasswordSymbols(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const password = "Ab1/cd@Ef2/gh@Ij3"
	defer createUser(t, db, "go_pw_symbols", password)()
	other := openDSN(t, dsnAs(t, "go_pw_symbols", password, "", ""))
	defer other.Close()
	var user string
	if err := other.QueryRowContext(t.Context(), "SELECT USER FROM dual").Scan(&user); err != nil || user != "GO_PW_SYMBOLS" {
		t.Errorf("got %q, %v", user, err)
	}
}

// TestRollbackAtClose closes a connection with a transaction open, as 1013
// and 1014 of tests/connection/test_1000_basic.py of python-oracledb do. The
// server rolls the transaction back.
func TestRollbackAtClose(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := t.Context()
	db.Exec("DROP TABLE go_rollback_close PURGE")
	if err := execCmd(db, "CREATE TABLE go_rollback_close (n NUMBER)"); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP TABLE go_rollback_close PURGE")
	cfg, err := oracle.ParseDSN(os.Getenv("ORACLE_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := oracle.NewConnector(cfg)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := c.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.(driver.ConnBeginTx).BeginTx(ctx, driver.TxOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.(driver.ExecerContext).ExecContext(ctx, "INSERT INTO go_rollback_close VALUES (1)", nil); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM go_rollback_close").Scan(&n); err != nil || n != 0 {
		t.Errorf("got %d rows, %v, want 0", n, err)
	}
}

// TestManyGoroutines runs 20 connections at once, and one connection from 3
// goroutines, as 1015 and 1025 of tests/connection/test_1000_basic.py of
// python-oracledb do with threads.
func TestManyGoroutines(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := t.Context()
	run := func(n int, f func() error) {
		t.Helper()
		var wg sync.WaitGroup
		errs := make(chan error, n)
		for range n {
			wg.Go(func() {
				if err := f(); err != nil {
					errs <- err
				}
			})
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Error(err)
		}
	}
	query := func(q interface {
		QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	}) error {
		rows, err := q.QueryContext(ctx, "SELECT LEVEL FROM dual CONNECT BY LEVEL <= 10")
		if err != nil {
			return err
		}
		defer rows.Close()
		n := 0
		for rows.Next() {
			n++
		}
		if n != 10 {
			return fmt.Errorf("got %d rows, want 10", n)
		}
		return rows.Err()
	}
	// 1015
	run(20, func() error {
		conn, err := db.Conn(ctx)
		if err != nil {
			return err
		}
		defer conn.Close()
		return query(conn)
	})
	// 1025
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	run(3, func() error {
		for range 5 {
			if err := query(conn); err != nil {
				return err
			}
		}
		return nil
	})
}

// TestProxyUser connects as one user through another, as 1043 of
// tests/connection/test_1000_basic.py of python-oracledb does. The DSN names
// the user of the session as proxy_user.
func TestProxyUser(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const password = "Pr0xy/Us3r@pw"
	defer createUser(t, db, "go_px_proxy", password)()
	defer createUser(t, db, "go_px_target", password)()
	if _, err := db.ExecContext(t.Context(), "ALTER USER go_px_target GRANT CONNECT THROUGH go_px_proxy"); err != nil {
		t.Fatal(err)
	}
	other := openDSN(t, dsnAs(t, "go_px_proxy", password, "proxy_user", "go_px_target"))
	defer other.Close()
	var user, proxy string
	if err := other.QueryRowContext(t.Context(), "SELECT USER, SYS_CONTEXT('USERENV', 'PROXY_USER') FROM dual").Scan(&user, &proxy); err != nil ||
		user != "GO_PX_TARGET" || proxy != "GO_PX_PROXY" {
		t.Errorf("got the user %q and the proxy %q, %v", user, proxy, err)
	}
}

// TestErrorInResponse reads a query whose last row fails, in one fetch of
// 1500 rows, as 1058 of tests/connection/test_1000_basic.py of
// python-oracledb does. The error comes with the rows, and the connection
// serves the next query.
func TestErrorInResponse(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := t.Context()
	db.Exec("DROP TABLE go_error_response PURGE")
	if err := execCmd(db, "CREATE TABLE go_error_response (int_col NUMBER(9), number_col NUMBER(9))",
		`INSERT INTO go_error_response SELECT LEVEL, CASE WHEN LEVEL < 1500 THEN 2 ELSE 0 END FROM dual CONNECT BY LEVEL <= 1500`); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP TABLE go_error_response PURGE")
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	rows, err := conn.QueryContext(oracle.WithOptions(ctx, oracle.WithPrefetch(1500)), `
		SELECT int_col, 1 / number_col FROM go_error_response WHERE int_col < 1500
		UNION ALL
		SELECT int_col, 1 / number_col FROM go_error_response WHERE int_col = 1500`)
	if err == nil {
		for rows.Next() {
		}
		err = rows.Err()
		rows.Close()
	}
	requireErr(t, "a division by zero", err, "ORA-01476")
	var user string
	if err := conn.QueryRowContext(ctx, "SELECT USER FROM dual").Scan(&user); err != nil || !strings.EqualFold(user, mustUser(t)) {
		t.Errorf("after: got %q, %v", user, err)
	}
}

// mustUser returns the user of ORACLE_DSN in upper case.
func mustUser(t *testing.T) string {
	t.Helper()
	u, err := url.Parse(os.Getenv("ORACLE_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ToUpper(u.User.Username())
}

// TestFailedCommit commits a transaction that a deferred constraint fails,
// as sijms/go-ora#778 names. The commit fails with ORA-02091, the row is not
// stored, and the pool serves the next query, since the connection of the
// failed commit is dropped.
func TestFailedCommit(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := t.Context()
	db.SetMaxOpenConns(1)
	db.Exec("DROP TABLE go_failed_commit PURGE")
	if err := execCmd(db, `CREATE TABLE go_failed_commit (n NUMBER,
		CONSTRAINT go_failed_commit_n CHECK (n > 0) DEFERRABLE INITIALLY DEFERRED)`); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP TABLE go_failed_commit PURGE")
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO go_failed_commit VALUES (-1)"); err != nil {
		t.Fatal(err)
	}
	requireErr(t, "the commit", tx.Commit(), "ORA-02091")
	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM go_failed_commit").Scan(&n); err != nil || n != 0 {
		t.Errorf("got %d rows, %v, want 0", n, err)
	}
}
