package integration

import (
	"database/sql"
	"math"
	"testing"
)

// TestColumnType checks the name that ColumnTypeDatabaseTypeName gives each
// type, which is the name of the data dictionary. go-ora gave none.
func TestColumnType(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	columns := []struct{ def, want string }{
		{"CHAR(32)", "CHAR"},
		{"NCHAR(32)", "NCHAR"},
		{"VARCHAR2(32)", "VARCHAR2"},
		{"NVARCHAR2(32)", "NVARCHAR2"},
		{"NUMBER", "NUMBER"},
		{"NUMBER(5, 2)", "NUMBER"},
		{"FLOAT", "FLOAT"},
		{"BINARY_FLOAT", "BINARY_FLOAT"},
		{"BINARY_DOUBLE", "BINARY_DOUBLE"},
		{"DATE", "DATE"},
		{"TIMESTAMP", "TIMESTAMP"},
		{"TIMESTAMP WITH TIME ZONE", "TIMESTAMP WITH TIME ZONE"},
		{"TIMESTAMP WITH LOCAL TIME ZONE", "TIMESTAMP WITH LOCAL TIME ZONE"},
		{"INTERVAL YEAR TO MONTH", "INTERVAL YEAR TO MONTH"},
		{"INTERVAL DAY TO SECOND", "INTERVAL DAY TO SECOND"},
		{"RAW(16)", "RAW"},
		{"CLOB", "CLOB"},
		{"NCLOB", "NCLOB"},
		{"BLOB", "BLOB"},
	}
	def := ""
	for i, c := range columns {
		if i > 0 {
			def += ", "
		}
		def += "c" + string(rune('a'+i)) + " " + c.def
	}
	_, _ = db.ExecContext(ctx, "DROP TABLE TTB_COLUMN_TYPE PURGE")
	if _, err := db.ExecContext(ctx, "CREATE TABLE TTB_COLUMN_TYPE ("+def+")"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DROP TABLE TTB_COLUMN_TYPE PURGE") })
	rows, err := db.QueryContext(ctx, "SELECT * FROM TTB_COLUMN_TYPE")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	types, err := rows.ColumnTypes()
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range columns {
		if got := types[i].DatabaseTypeName(); got != c.want {
			t.Errorf("%s: got %q, want %q", c.def, got, c.want)
		}
	}
	rows.Close()
	rows, err = db.QueryContext(ctx, "SELECT ROWID FROM dual")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	types, err = rows.ColumnTypes()
	if err != nil {
		t.Fatal(err)
	}
	if got := types[0].DatabaseTypeName(); got != "ROWID" && got != "UROWID" {
		t.Errorf("ROWID: got %q", got)
	}
}

// TestColumnTypeLength checks the length that ColumnTypeLength gives a column
// of each type, of a table and of an expression. A string gives its length in
// characters and a RAW in bytes, so that a caller can tell a RAW(16), which
// holds a GUID, from another RAW. A LOB gives math.MaxInt64, and a type of
// fixed length gives none. go-ora gave a length for a string only.
func TestColumnTypeLength(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	type length struct {
		n  int64
		ok bool
	}
	columns := []struct {
		def  string
		want length
	}{
		{"VARCHAR2(10 BYTE)", length{10, true}},
		{"VARCHAR2(10 CHAR)", length{10, true}},
		{"NVARCHAR2(10)", length{10, true}},
		{"CHAR(5)", length{5, true}},
		{"NCHAR(5)", length{5, true}},
		{"RAW(16)", length{16, true}},
		{"RAW(2000)", length{2000, true}},
		{"CLOB", length{math.MaxInt64, true}},
		{"NCLOB", length{math.MaxInt64, true}},
		{"BLOB", length{math.MaxInt64, true}},
		{"NUMBER(5)", length{}},
		{"DATE", length{}},
		{"TIMESTAMP", length{}},
	}
	def := ""
	for i, c := range columns {
		if i > 0 {
			def += ", "
		}
		def += "c" + string(rune('a'+i)) + " " + c.def
	}
	_, _ = db.ExecContext(ctx, "DROP TABLE TTB_COLUMN_LENGTH PURGE")
	if _, err := db.ExecContext(ctx, "CREATE TABLE TTB_COLUMN_LENGTH ("+def+")"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DROP TABLE TTB_COLUMN_LENGTH PURGE") })
	check := func(query string, want []length, names []string) {
		t.Helper()
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		types, err := rows.ColumnTypes()
		if err != nil {
			t.Fatal(err)
		}
		for i, w := range want {
			n, ok := types[i].Length()
			if n != w.n || ok != w.ok {
				t.Errorf("%s: got %d, %t, want %d, %t", names[i], n, ok, w.n, w.ok)
			}
		}
	}
	var want []length
	var names []string
	for _, c := range columns {
		want = append(want, c.want)
		names = append(names, c.def)
	}
	check("SELECT * FROM TTB_COLUMN_LENGTH", want, names)
	check("SELECT SYS_GUID(), HEXTORAW('AB'), CAST(NULL AS RAW(16)), 'abc', CAST('x' AS VARCHAR2(20)) FROM dual",
		[]length{{16, true}, {1, true}, {16, true}, {3, true}, {20, true}},
		[]string{"SYS_GUID()", "HEXTORAW('AB')", "CAST(NULL AS RAW(16))", "'abc'", "CAST('x' AS VARCHAR2(20))"})
	check("SELECT TEXT FROM ALL_VIEWS WHERE ROWNUM < 2",
		[]length{{math.MaxInt64, true}}, []string{"LONG"})
}

// TestQueryAllColumnTypes queries a table with a column of each of 18 types,
// BFILE and LONG RAW among them (sijms/go-ora#335).
func TestQueryAllColumnTypes(t *testing.T) {
	var create = func(db *sql.DB) error {
		return execCmd(db, `CREATE TABLE TTB_335 (
	  varchar2_col VARCHAR2(10),
	  char_col CHAR(10),
	  nchar_col NCHAR(10),
	  number_col NUMBER(10, 2),
	  integer_col INTEGER,
	  smallint_col SMALLINT,
	  float_col FLOAT(10),
	  date_col DATE,
	  timestamp_col TIMESTAMP,
	  timestamp_with_time_zone_col TIMESTAMP WITH TIME ZONE,
	  timestamp_with_local_time_zone_col TIMESTAMP WITH LOCAL TIME ZONE,
	  interval_year_to_month_col INTERVAL YEAR TO MONTH,
	  interval_day_to_second_col INTERVAL DAY TO SECOND,
	  clob_col CLOB,
	  blob_col BLOB,
	  bfile_col BFILE,
	  long_raw_col LONG RAW,
	  raw_col RAW(10)
	)NOCOMPRESS`,
			`INSERT INTO TTB_335 (
		varchar2_col, 
		char_col, 
		nchar_col, 
		number_col, 
		integer_col, 
		smallint_col, 
		float_col, 
		date_col, 
		timestamp_col, 
		timestamp_with_time_zone_col, 
		timestamp_with_local_time_zone_col, 
		interval_year_to_month_col, 
		interval_day_to_second_col, 
		clob_col,
		blob_col,
		bfile_col,
		long_raw_col,
		raw_col)
		VALUES (
		'hello', 
		'world', 
		'unicode',
		3.14, 
		42, 10,
		3.14159, 
		SYSDATE,
		SYSTIMESTAMP,
		SYSTIMESTAMP, 
		SYSTIMESTAMP, 
		INTERVAL '1-2' YEAR TO MONTH, INTERVAL '1 12:30:00' DAY TO SECOND, 
		'lorem ipsum',
		EMPTY_BLOB(), 
		BFILENAME('MY_DIR', 'MY_FILE'), 
		EMPTY_BLOB(), 
		'DEADBEEF')`,
		)
	}

	var drop = func(db *sql.DB) error {
		return execCmd(db, `DROP TABLE TTB_335 PURGE`)
	}
	var query = func(db *sql.DB) error {
		rows, err := db.Query("SELECT * FROM TTB_335")
		if err != nil {
			return err
		}
		defer func() {
			err = rows.Close()
			if err != nil {
				t.Error(err)
			}
		}()
		return nil
	}

	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	// an identifier of over 30 characters needs 12c
	requireRelease(t, db, 12)
	defer func() {
		err = db.Close()
		if err != nil {
			t.Error(err)
		}
	}()
	err = create(db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err = drop(db)
		if err != nil {
			t.Error(err)
		}
	}()
	err = query(db)
	if err != nil {
		t.Fatal(err)
	}
}
