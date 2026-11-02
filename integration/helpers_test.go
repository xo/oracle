package integration

import (
	"database/sql"
	"fmt"
	"net/url"

	"os"
	"testing"
	"time"

	"github.com/xo/oracle"
)

// urlOptions holds the keys that a test adds to the query of ORACLE_DSN.
var urlOptions = map[string]string{}

type Execuer interface {
	Exec(query string, args ...any) (sql.Result, error)
	Prepare(query string) (*sql.Stmt, error)
}

func createMainTable(db *sql.DB) error {
	sqlText := `CREATE TABLE TTB_MAIN(
	ID	number(10)	NOT NULL,
	NAME		VARCHAR2(500),
	VAL			number(10,2),
	LDATE   		date,
	DATA			RAW(100),
	PRIMARY KEY(ID)
	)`
	return execCmd(db, sqlText)
}

func dropMainTable(db *sql.DB) error {
	return execCmd(db, "drop table TTB_MAIN purge")
}

// getDB opens the database that ORACLE_DSN names, with the keys of
// urlOptions added to its query, and with opts. It skips the test when
// ORACLE_DSN is empty. dbrun prints the DSN of each release. See
// docs/DESIGN.md, under Testing.
func getDB(t testing.TB, opts ...oracle.ConnectorOption) (*sql.DB, error) {
	t.Helper()
	dsn := os.Getenv("ORACLE_DSN")
	if dsn == "" {
		t.Skip("ORACLE_DSN is empty")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, fmt.Errorf("parsing ORACLE_DSN: %w", err)
	}
	q := u.Query()
	for k, v := range urlOptions {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	cfg, err := oracle.ParseDSN(u.String())
	if err != nil {
		return nil, err
	}
	c, err := oracle.NewConnector(cfg, opts...)
	if err != nil {
		return nil, err
	}
	return sql.OpenDB(c), nil
}

func execCmd(db *sql.DB, stmts ...string) error {
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			if len(stmts) > 1 {
				return fmt.Errorf("error: %v in execuation of stmt: %s", err, stmt)
			} else {
				return err
			}
		}
	}
	return nil
}

func isEqualTime(t1, t2 time.Time, compareNano bool) bool {
	ret := t1.Year() == t2.Year() &&
		t1.Month() == t2.Month() &&
		t1.Day() == t2.Day() &&
		t1.Hour() == t2.Hour() &&
		t1.Minute() == t2.Minute() &&
		t1.Second() == t2.Second()
	if compareNano {
		return ret && t1.Nanosecond() == t2.Nanosecond()
	}
	return ret
}

// requireRelease skips the test when the server is older than the major
// release, such as 12 for a feature that 11g lacks.
func requireRelease(t *testing.T, db *sql.DB, major int) {
	t.Helper()
	var version int
	err := db.QueryRowContext(t.Context(), `SELECT TO_NUMBER(REGEXP_SUBSTR(version, '^[0-9]+'))
		FROM product_component_version WHERE product LIKE 'Oracle%' AND ROWNUM = 1`).Scan(&version)
	if err != nil {
		t.Fatalf("reading the release: %v", err)
	}
	if version < major {
		t.Skipf("needs release %d, and the server is %d", major, version)
	}
}
