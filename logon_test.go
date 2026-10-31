package oracle_test

import (
	"database/sql"
	"errors"
	"net/url"
	"os"
	"testing"

	"github.com/xo/oracle"
)

// TestLogonRefused holds a refused logon. From 23ai the server takes the
// first logon of a connector in one round trip, with the negotiations, and
// the driver read the reply to the protocol negotiation a second time after
// the refusal, which gave "message code error: received code 4" in place of
// ORA-01017. A logon with the cookie of an earlier one is refused the same
// way.
//
// The user has a lower case name, which is sijms/go-ora#683: the server
// folds a name to upper case unless it is quoted, as SQL Developer quotes it.
func TestLogonRefused(t *testing.T) {
	dsn := os.Getenv("ORACLE_DSN")
	if dsn == "" {
		t.Skip("ORACLE_DSN is empty")
	}
	admin := openLive(t)
	const user, pass = "go_logon", "Refused_Pass_123"
	drop := `BEGIN
	FOR s IN (SELECT sid, serial# AS serial FROM v$session WHERE username = 'go_logon') LOOP
		EXECUTE IMMEDIATE 'ALTER SYSTEM KILL SESSION ''' || s.sid || ',' || s.serial || ''' IMMEDIATE';
	END LOOP;
	EXECUTE IMMEDIATE 'DROP USER "go_logon"';
EXCEPTION WHEN OTHERS THEN NULL;
END;`
	if _, err := admin.ExecContext(t.Context(), drop); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE USER "go_logon" IDENTIFIED BY "` + pass + `"`,
		`GRANT CREATE SESSION TO "go_logon"`,
	} {
		if _, err := admin.ExecContext(t.Context(), q); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { admin.Exec(drop) })
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	open := func(name, password string) *sql.DB {
		u.User = url.UserPassword(name, password)
		cfg, err := oracle.ParseDSN(u.String())
		if err != nil {
			t.Fatal(err)
		}
		c, err := oracle.NewConnector(cfg)
		if err != nil {
			t.Fatal(err)
		}
		db := sql.OpenDB(c)
		// each query opens a new connection, which logs on again
		db.SetMaxIdleConns(0)
		t.Cleanup(func() { db.Close() })
		return db
	}
	whoami := func(db *sql.DB) (string, error) {
		var s string
		err := db.QueryRowContext(t.Context(), "SELECT USER FROM dual").Scan(&s)
		return s, err
	}
	for _, name := range []string{user, `"` + user + `x"`, "nobody"} {
		_, err := whoami(open(name, pass))
		if !errors.Is(err, oracle.ErrAuthentication) {
			t.Errorf("%s: got %v, want ORA-01017", name, err)
		}
	}
	if _, err := whoami(open(`"`+user+`"`, "wrong")); !errors.Is(err, oracle.ErrAuthentication) {
		t.Errorf("a wrong password: got %v, want ORA-01017", err)
	}
	db := open(`"`+user+`"`, pass)
	if got, err := whoami(db); err != nil || got != user {
		t.Fatalf("got %q, %v, want %q", got, err, user)
	}
	if _, err := admin.ExecContext(t.Context(), `ALTER USER "go_logon" IDENTIFIED BY "`+pass+`x"`); err != nil {
		t.Fatal(err)
	}
	if _, err := whoami(db); !errors.Is(err, oracle.ErrAuthentication) {
		t.Errorf("a changed password: got %v, want ORA-01017", err)
	}
	usable(t, admin)
}
