package integration

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/cockroachdb/apd/v3"
)

// typeChangeValue gives the value of a scan into *any as text, so that the
// values of different types compare.
func typeChangeValue(v any) string {
	switch v := v.(type) {
	case time.Time:
		return v.Format("2006-01-02 15:04:05")
	case []byte:
		return "raw:" + string(v)
	case *apd.Decimal:
		return v.String()
	}
	return fmt.Sprint(v)
}

// TestTypeChanges runs one query twice on one connection, once on its own
// and once through a statement prepared once, while a view under it changes
// the type of its column, so that the second run gets a describe that does
// not match the cursor of the first. It holds the cases of
// tests/cursor/test_2500_type_changes.py of python-oracledb, written again
// here. Each names the number of its test there.
func TestTypeChanges(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	db.Exec("DROP TABLE go_tc_longs PURGE")
	db.Exec("DROP TABLE go_tc_long_raws PURGE")
	err = execCmd(db, "CREATE TABLE go_tc_longs (id NUMBER, value LONG)",
		"CREATE TABLE go_tc_long_raws (id NUMBER, value LONG RAW)",
		"INSERT INTO go_tc_longs VALUES (1, 'string_long')",
		"INSERT INTO go_tc_long_raws VALUES (1, UTL_RAW.CAST_TO_RAW('string_long_raw'))")
	if err != nil {
		t.Fatal(err)
	}
	defer execCmd(db, "DROP VIEW go_types_changed", "DROP TABLE go_tc_longs PURGE", "DROP TABLE go_tc_long_raws PURGE")
	const date = "TO_DATE('04-01-2022', 'DD-MM-YYYY')"
	tests := []struct {
		name        string
		from, first string
		to, second  string
	}{
		{"2500 VARCHAR to CLOB", "SELECT CAST('string_4600' AS VARCHAR2(15)) AS value FROM dual", "string_4600", "TO_CLOB('clob_4600')", "clob_4600"},
		{"2501 CHAR to CLOB", "SELECT CAST('string_4601' AS CHAR(11)) AS value FROM dual", "string_4601", "TO_CLOB('clob_4601')", "clob_4601"},
		{"2502 LONG to CLOB", "SELECT value FROM go_tc_longs", "string_long", "TO_CLOB('clob_4602')", "clob_4602"},
		{"2503 NVARCHAR to CLOB", "SELECT CAST('string_4603' AS NVARCHAR2(15)) AS value FROM dual", "string_4603", "TO_CLOB('clob_4603')", "clob_4603"},
		{"2504 NCHAR to CLOB", "SELECT CAST('string_4604' AS NCHAR(11)) AS value FROM dual", "string_4604", "TO_CLOB('clob_4604')", "clob_4604"},
		{"2505 RAW to BLOB", "SELECT UTL_RAW.CAST_TO_RAW('string_4605') AS value FROM dual", "raw:string_4605", "TO_BLOB(UTL_RAW.CAST_TO_RAW('blob_4605'))", "raw:blob_4605"},
		{"2506 LONG RAW to BLOB", "SELECT value FROM go_tc_long_raws", "raw:string_long_raw", "TO_BLOB(UTL_RAW.CAST_TO_RAW('blob_4606'))", "raw:blob_4606"},
		{"2507 VARCHAR to NCLOB", "SELECT CAST('string_4607' AS VARCHAR2(15)) AS value FROM dual", "string_4607", "TO_NCLOB('nclob_4607')", "nclob_4607"},
		{"2509 LONG to NCLOB", "SELECT value FROM go_tc_longs", "string_long", "TO_NCLOB('nclob_4609')", "nclob_4609"},
		{"2511 NCHAR to NCLOB", "SELECT CAST('string_4611' AS NCHAR(11)) AS value FROM dual", "string_4611", "TO_NCLOB('nclob_4611')", "nclob_4611"},
		{"2512 VARCHAR to NUMBER", "SELECT CAST('string_4612' AS VARCHAR2(15)) AS value FROM dual", "string_4612", "TO_NUMBER('4612')", "4612"},
		{"2513 NUMBER to VARCHAR", "SELECT TO_NUMBER('4613') AS value FROM dual", "4613", "CAST('string_4613' AS VARCHAR2(15))", "string_4613"},
		{"2514 VARCHAR to DATE", "SELECT CAST('string_4614' AS VARCHAR2(15)) AS value FROM dual", "string_4614", date, "2022-01-04 00:00:00"},
		{"2515 DATE to VARCHAR", "SELECT " + date + " AS value FROM dual", "2022-01-04 00:00:00", "CAST('string_4615' AS VARCHAR2(15))", "string_4615"},
		{"2516 NUMBER to DATE", "SELECT TO_NUMBER('4616') AS value FROM dual", "4616", date, "2022-01-04 00:00:00"},
		{"2517 CLOB to VARCHAR", "SELECT TO_CLOB('clob_4617') AS value FROM dual", "clob_4617", "CAST('string_4617' AS VARCHAR2(15))", "string_4617"},
		{"2518 NCLOB to NVARCHAR", "SELECT TO_NCLOB('nclob_4618') AS value FROM dual", "nclob_4618", "CAST('nstring_4618' AS NVARCHAR2(15))", "nstring_4618"},
		{"2520 BLOB to RAW", "SELECT TO_BLOB(UTL_RAW.CAST_TO_RAW('blob_4620')) AS value FROM dual", "raw:blob_4620", "UTL_RAW.CAST_TO_RAW('string_4620')", "raw:string_4620"},
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, prepared := range []bool{false, true} {
		for _, test := range tests {
			t.Run(fmt.Sprintf("%s prepared=%t", test.name, prepared), func(t *testing.T) {
				if _, err := conn.ExecContext(ctx, "CREATE OR REPLACE VIEW go_types_changed AS "+test.from); err != nil {
					t.Fatal(err)
				}
				var stmt *sql.Stmt
				if prepared {
					if stmt, err = conn.PrepareContext(ctx, "SELECT * FROM go_types_changed"); err != nil {
						t.Fatal(err)
					}
					defer stmt.Close()
				}
				query := func() (string, error) {
					var v any
					var err error
					if stmt != nil {
						err = stmt.QueryRowContext(ctx).Scan(&v)
					} else {
						err = conn.QueryRowContext(ctx, "SELECT * FROM go_types_changed").Scan(&v)
					}
					return typeChangeValue(v), err
				}
				if got, err := query(); err != nil || got != test.first {
					t.Fatalf("before the change: got %q, %v, want %q", got, err, test.first)
				}
				if _, err := conn.ExecContext(ctx, "CREATE OR REPLACE VIEW go_types_changed AS SELECT "+test.to+" AS value FROM dual"); err != nil {
					t.Fatal(err)
				}
				if got, err := query(); err != nil || got != test.second {
					t.Errorf("after the change: got %q, %v, want %q", got, err, test.second)
				}
			})
		}
	}
}
