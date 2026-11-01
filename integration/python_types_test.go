package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/xo/oracle"
)

// TestLongValues binds LONG and LONG RAW values that grow by 25,000
// characters, and a batch of LONG values up to 160 KB, reads them back, and
// checks the names of the column types. It holds the cases of
// tests/data_types/test_1600_longs.py of python-oracledb (1600 to 1604),
// written again here.
func TestLongValues(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	for _, kind := range []string{"LONG", "LONG RAW"} {
		t.Run(kind, func(t *testing.T) {
			db.Exec("DROP TABLE go_longs PURGE")
			if err := execCmd(db, "CREATE TABLE go_longs (id NUMBER(9), value "+kind+")"); err != nil {
				t.Fatal(err)
			}
			defer execCmd(db, "DROP TABLE go_longs PURGE")
			value := func(s string) any {
				if kind == "LONG RAW" {
					return []byte(s)
				}
				return s
			}
			var want []string
			long := ""
			for i := 1; i <= 5; i++ {
				long += strings.Repeat(string(rune('A'+i-1)), 25000)
				want = append(want, long)
				if _, err := db.ExecContext(ctx, "INSERT INTO go_longs VALUES (:1, :2)", i, value(long)); err != nil {
					t.Fatalf("%d characters: %v", len(long), err)
				}
			}
			rows, err := db.QueryContext(ctx, "SELECT id, value FROM go_longs ORDER BY id")
			if err != nil {
				t.Fatal(err)
			}
			types, err := rows.ColumnTypes()
			if err != nil || types[1].DatabaseTypeName() != kind {
				t.Errorf("the type of the column: got %q, %v", types[1].DatabaseTypeName(), err)
			}
			n := 0
			for rows.Next() {
				var id int
				var got []byte
				if err := rows.Scan(&id, &got); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, []byte(want[id-1])) {
					t.Errorf("row %d: got %d bytes, want %d", id, len(got), len(want[id-1]))
				}
				n++
			}
			rows.Close()
			if n != 5 {
				t.Errorf("got %d rows", n)
			}
		})
	}
	// a batch of LONG values, 1601
	db.Exec("DROP TABLE go_longs PURGE")
	if err := execCmd(db, "CREATE TABLE go_longs (id NUMBER(9), value LONG)"); err != nil {
		t.Fatal(err)
	}
	defer execCmd(db, "DROP TABLE go_longs PURGE")
	var ids []int
	var values []string
	for i := range 5 {
		ids = append(ids, i+1)
		values = append(values, strings.Repeat(string(rune('A'+i)), 32768*(i+1)))
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO go_longs VALUES (:1, :2)", ids, values); err != nil {
		t.Fatal(err)
	}
	var total int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM go_longs").Scan(&total); err != nil || total != 5 {
		t.Errorf("a batch of LONG: got %d rows, %v", total, err)
	}
	var last string
	if err := db.QueryRowContext(ctx, "SELECT value FROM go_longs WHERE id = 5").Scan(&last); err != nil || last != values[4] {
		t.Errorf("a batch of LONG: got %d characters, %v", len(last), err)
	}
}

// TestProcedureLongValues calls procedures that take a CLOB and a BLOB with a
// string and a []byte longer than 32,767 bytes, which bind as temporary LOBs
// in PL/SQL (D24). It holds the cases 1425 and 1426 of
// tests/cursor/test_1400_callproc.py of python-oracledb, written again here.
func TestProcedureLongValues(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	err = execCmd(db,
		"CREATE OR REPLACE PROCEDURE go_clob_size(c IN CLOB, n OUT NUMBER) AS BEGIN n := DBMS_LOB.GETLENGTH(c); END;",
		"CREATE OR REPLACE PROCEDURE go_blob_size(b IN BLOB, n OUT NUMBER) AS BEGIN n := DBMS_LOB.GETLENGTH(b); END;")
	if err != nil {
		t.Fatal(err)
	}
	defer execCmd(db, "DROP PROCEDURE go_clob_size", "DROP PROCEDURE go_blob_size")
	text := strings.Repeat("1425", 16000)
	data := bytes.Repeat([]byte("1426"), 16250)
	var n int
	if _, err := db.Exec("BEGIN go_clob_size(:1, :2); END;", text, oracle.Out{Dest: &n}); err != nil || n != len(text) {
		t.Errorf("a string: got %d, %v", n, err)
	}
	if _, err := db.Exec("BEGIN go_blob_size(:1, :2); END;", data, oracle.Out{Dest: &n}); err != nil || n != len(data) {
		t.Errorf("a []byte: got %d, %v", n, err)
	}
}

// TestNClobReturning inserts NVARCHAR2 and NCLOB values, the NCLOBs empty, in
// another order than the columns, with RETURNING, twice. It is the case 2028
// of tests/data_types/test_2000_char.py of python-oracledb, which was
// python-oracledb issue 50, ORA-24816, written again here.
func TestNClobReturning(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	requireRelease(t, db, 12)
	db.Exec("DROP TABLE go_issue_50 PURGE")
	err = execCmd(db, `CREATE TABLE go_issue_50 (
	id NUMBER(11) PRIMARY KEY,
	str1 NVARCHAR2(256), str2 NVARCHAR2(256), str3 NVARCHAR2(256),
	nclob1 NCLOB, nclob2 NCLOB)`)
	if err != nil {
		t.Fatal(err)
	}
	defer execCmd(db, "DROP TABLE go_issue_50 PURGE")
	const insert = `INSERT INTO go_issue_50 (id, str2, str3, nclob1, nclob2, str1)
	VALUES (:arg0, :arg1, :arg2, :arg3, :arg4, :arg5) RETURNING id INTO :arg6`
	for i, args := range [][]string{{"555a4c78", "f319ef0e", "23009914"}, {"d5ff845a", "94275767", "bf161ff6"}} {
		var id int
		_, err := db.Exec(insert, i+1, args[0], args[1], args[2], "", "", oracle.Out{Dest: &id})
		if err != nil || id != i+1 {
			t.Errorf("row %d: got %d, %v", i+1, id, err)
		}
	}
}

// TestRowIDs selects the ROWIDs of a table and the UROWIDs of an index
// organized table, binds each back, stores them in ROWID and UROWID
// columns, binds invalid ones, and reads a NULL one. It holds the cases of
// tests/data_types/test_3400_rowid.py of python-oracledb, written again
// here.
func TestRowIDs(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	drop := func() {
		execCmd(db, "DROP TABLE go_rid_heap PURGE", "DROP TABLE go_rid_iot PURGE", "DROP TABLE go_rids PURGE")
	}
	drop()
	err = execCmd(db, "CREATE TABLE go_rid_heap (id NUMBER(9))",
		"CREATE TABLE go_rid_iot (id NUMBER(9) PRIMARY KEY, s VARCHAR2(250), d DATE) ORGANIZATION INDEX",
		"CREATE TABLE go_rids (id NUMBER(9), r ROWID, u UROWID)")
	if err != nil {
		t.Fatal(err)
	}
	defer drop()
	date := time.Date(2017, 4, 4, 0, 0, 0, 0, time.UTC)
	for i := 1; i <= 4; i++ {
		db.ExecContext(ctx, "INSERT INTO go_rid_heap VALUES (:1)", i)
		s := strings.Repeat("A", 250)
		if i < 4 {
			s = "String #" + string(rune('0'+i))
		}
		db.ExecContext(ctx, "INSERT INTO go_rid_iot VALUES (:1, :2, :3)", i, s, date.AddDate(0, 0, i))
	}
	for _, table := range []string{"go_rid_heap", "go_rid_iot"} {
		rows, err := db.QueryContext(ctx, "SELECT ROWID, id FROM "+table)
		if err != nil {
			t.Fatal(err)
		}
		var rids []string
		var ids []int
		for rows.Next() {
			var rid string
			var id int
			if err := rows.Scan(&rid, &id); err != nil {
				t.Fatal(err)
			}
			rids, ids = append(rids, rid), append(ids, id)
		}
		rows.Close()
		if len(rids) != 4 {
			t.Fatalf("%s: got %d ROWIDs", table, len(rids))
		}
		for i, rid := range rids {
			var id int
			if err := db.QueryRowContext(ctx, "SELECT id FROM "+table+" WHERE ROWID = :1", rid).Scan(&id); err != nil || id != ids[i] {
				t.Errorf("%s: the ROWID %s gave %d, %v", table, rid, id, err)
			}
		}
	}
	// a ROWID and a UROWID column, 3403 and 3404
	if _, err := db.ExecContext(ctx, "INSERT INTO go_rids (id, r, u) SELECT id, ROWID, ROWID FROM go_rid_heap"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO go_rids (id, u) SELECT id + 10, ROWID FROM go_rid_iot"); err != nil {
		t.Fatal(err)
	}
	var mismatch int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM go_rids x
		WHERE x.id < 10 AND NOT EXISTS (SELECT 1 FROM go_rid_heap h WHERE h.ROWID = x.r AND h.ROWID = x.u AND h.id = x.id)`).Scan(&mismatch)
	if err != nil || mismatch != 0 {
		t.Errorf("the ROWIDs of the columns: %d do not match, %v", mismatch, err)
	}
	var u string
	if err := db.QueryRowContext(ctx, "SELECT u FROM go_rids WHERE id = 14").Scan(&u); err != nil {
		t.Fatal(err)
	}
	var s string
	if err := db.QueryRowContext(ctx, "SELECT s FROM go_rid_iot WHERE ROWID = :1", u).Scan(&s); err != nil || s != strings.Repeat("A", 250) {
		t.Errorf("a UROWID of a long key: got %d characters, %v", len(s), err)
	}
	// invalid ROWIDs, 3402
	for _, in := range []any{12345, "523lkhlf"} {
		if _, err := db.ExecContext(ctx, "INSERT INTO go_rids (id, r) VALUES (99, :1)", in); err == nil {
			t.Errorf("%v: no error", in)
		}
	}
	// a NULL ROWID, 3407
	if _, err := db.ExecContext(ctx, "INSERT INTO go_rids (id) VALUES (100)"); err != nil {
		t.Fatal(err)
	}
	var r, nu sql.Null[string]
	if err := db.QueryRowContext(ctx, "SELECT r, u FROM go_rids WHERE id = 100").Scan(&r, &nu); err != nil || r.Valid || nu.Valid {
		t.Errorf("a NULL ROWID: got %v and %v, %v", r, nu, err)
	}
}

// TestNegativeIntervals binds negative intervals and reads them back. It holds
// the cases 1311 of tests/data_types/test_1300_interval_ds.py and 1410 of
// tests/data_types/test_1400_interval_ym.py of python-oracledb, written
// again here.
func TestNegativeIntervals(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, in := range []time.Duration{
		-24*time.Hour + 86314*time.Second + 431152*time.Microsecond,
		-8*24*time.Hour - 126*time.Second - 1811*time.Microsecond,
	} {
		var got time.Duration
		if err := db.QueryRow("SELECT :1 FROM dual", in).Scan(&got); err != nil || got != in {
			t.Errorf("%v: got %v, %v", in, got, err)
		}
	}
	in := oracle.IntervalYM{Years: -12, Months: -5}
	var got oracle.IntervalYM
	if err := db.QueryRow("SELECT :1 FROM dual", in).Scan(&got); err != nil || got != in {
		t.Errorf("%v: got %v, %v", in, got, err)
	}
}

// TestDateBounds reads and binds dates at the ends of the range of DATE and
// TIMESTAMP, a year before 0, and TIMESTAMP WITH TIME ZONE values in
// regions, with nanoseconds. It holds the case 1118 of
// tests/data_types/test_1100_date.py of python-oracledb, which expects an
// error where Python holds no year before 1, and the cases of
// test_2100_timestamp.py and test_2300_timestamp_tz.py, written again here.
func TestDateBounds(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var d time.Time
	if err := db.QueryRow("SELECT TO_DATE('-4712-01-01', 'SYYYY-MM-DD') FROM dual").Scan(&d); err != nil || d.Year() != -4712 || d.Month() != 1 || d.Day() != 1 {
		t.Errorf("year -4712: got %v, %v", d, err)
	}
	for _, in := range []time.Time{
		time.Date(-4712, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC),
		time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC),
		time.Date(1, 1, 1, 0, 0, 0, 1, time.UTC),
	} {
		var out time.Time
		if err := db.QueryRow("SELECT CAST(:1 AS TIMESTAMP(9)) FROM dual", in).Scan(&out); err != nil || !out.Equal(in) {
			t.Errorf("%v: got %v, %v", in, out, err)
		}
	}
	for _, zone := range []string{"Europe/Prague", "America/New_York", "Asia/Kolkata"} {
		loc, err := time.LoadLocation(zone)
		if err != nil {
			t.Skip(err)
		}
		for _, in := range []time.Time{
			time.Date(2022, 1, 7, 18, 30, 10, 250000000, loc),
			time.Date(2022, 7, 7, 18, 30, 10, 123456789, loc),
		} {
			var out time.Time
			if err := db.QueryRow("SELECT CAST(:1 AS TIMESTAMP(9) WITH TIME ZONE) FROM dual", oracle.TimestampTZ(in)).Scan(&out); err != nil || !out.Equal(in) {
				t.Errorf("%v: got %v, %v", in, out, err)
			}
		}
	}
}

// TestBooleanColumn binds and reads the BOOLEAN of 23ai: true, false and
// NULL, into a column, out of PL/SQL, and through RETURNING. It holds the
// cases 2409 to 2415 of tests/data_types/test_2400_boolean.py of
// python-oracledb, written again here.
func TestBooleanColumn(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	requireRelease(t, db, 23)
	db.Exec("DROP TABLE go_bools PURGE")
	if err := execCmd(db, "CREATE TABLE go_bools (id NUMBER, b BOOLEAN)"); err != nil {
		t.Fatal(err)
	}
	defer execCmd(db, "DROP TABLE go_bools PURGE")
	for i, in := range []sql.Null[bool]{{V: true, Valid: true}, {V: false, Valid: true}, {}} {
		var ret sql.Null[bool]
		_, err := db.Exec("INSERT INTO go_bools VALUES (:1, :2) RETURNING b INTO :3", i, in, oracle.Out{Dest: &ret})
		if err != nil || ret != in {
			t.Errorf("%v: returned %v, %v", in, ret, err)
		}
		var out sql.Null[bool]
		if err := db.QueryRow("SELECT b FROM go_bools WHERE id = :1", i).Scan(&out); err != nil || out != in {
			t.Errorf("%v: read %v, %v", in, out, err)
		}
		var plsql sql.Null[bool]
		if _, err := db.Exec("BEGIN :1 := :2; END;", oracle.Out{Dest: &plsql}, in); err != nil || plsql != in {
			t.Errorf("%v: out of PL/SQL %v, %v", in, plsql, err)
		}
	}
}

// TestJSONFieldNames reads JSON that the server encoded and that the driver
// encoded, with field names longer than 255 bytes, which OSON writes in
// another form, from a table and from a table with compressed JSON, whose
// OSON has relative offsets. It holds the cases of
// tests/data_types/test_3200_json_26ai.py of python-oracledb, written again
// here.
func TestJSONFieldNames(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	requireRelease(t, db, 23)
	drop := func() { execCmd(db, "DROP TABLE go_json_names PURGE", "DROP TABLE go_json_compressed PURGE") }
	drop()
	// a JSON column needs a tablespace of automatic segment space
	// management, which SYSTEM, the one of system on 26ai Free, is not
	space := ""
	if err := execCmd(db, "CREATE TABLE go_json_names (id NUMBER, j JSON)"); err != nil {
		space = " TABLESPACE USERS"
		if err := execCmd(db, "CREATE TABLE go_json_names (id NUMBER, j JSON)"+space); err != nil {
			t.Fatal(err)
		}
	}
	compressed := execCmd(db, "CREATE TABLE go_json_compressed (id NUMBER, j JSON) JSON (j) STORE AS (COMPRESS HIGH)"+space) == nil
	defer drop()
	many := func(long, short bool) map[string]any {
		m := map[string]any{}
		for i := range 26 {
			for j := range 26 {
				name := string(rune('A'+i)) + string(rune('A'+j))
				if short {
					m[name] = 6.75
				}
				if long {
					m[name+strings.Repeat("X", 254)] = 12.25
				}
			}
		}
		return m
	}
	oneLong := many(false, true)
	oneLong[strings.Repeat("B", 256)] = 6704.0
	values := map[string]any{
		"3200 one long name":           map[string]any{strings.Repeat("A", 256): 6700.0},
		"3201 a short and a long":      map[string]any{"short_name": "Short name", strings.Repeat("A", 256): 6701.0},
		"3202 many long names":         many(true, false),
		"3203 many long and short":     many(true, true),
		"3204 one long and many short": oneLong,
		"3210 lists":                   map[string]any{strings.Repeat("C", 256): 6710.0, "num_list": []any{1.5, 2.25, 3.75, 5.5}, "str_list": []any{"string 1", "string 2"}},
	}
	var shared, distinct []any
	for i := range 15 {
		shared = append(shared, map[string]any{"a": 6711.0, "b": "String Value"})
		distinct = append(distinct, map[string]any{"a": float64(6711 + i), "b": fmt.Sprint("String Value ", i)})
	}
	values["3211 shared fields and values"] = shared
	values["3212 shared fields"] = distinct
	for name, value := range values {
		text, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		tables := []string{"go_json_names"}
		if compressed {
			tables = append(tables, "go_json_compressed")
		}
		for _, table := range tables {
			db.Exec("DELETE FROM " + table)
			// encoded by the server, from text. Text past 32,767 bytes binds
			// as a LONG in SQL, which JSON() takes as a CLOB only (D24)
			clob, err := oracle.CreateClob(context.Background(), db, string(text), false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("INSERT INTO "+table+" VALUES (1, JSON(:1))", clob); err != nil {
				t.Fatalf("%s, %s: %v", name, table, err)
			}
			// encoded by the driver
			doc, err := oracle.NewJSON(value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("INSERT INTO "+table+" VALUES (2, :1)", doc); err != nil {
				t.Fatalf("%s, %s, bound: %v", name, table, err)
			}
			for id := 1; id <= 2; id++ {
				var got any
				if err := db.QueryRow("SELECT j FROM "+table+" WHERE id = :1", id).Scan(&got); err != nil {
					t.Fatalf("%s, %s, %d: %v", name, table, id, err)
				}
				var back any
				if err := json.Unmarshal([]byte(fmt.Sprint(got)), &back); err != nil {
					t.Fatalf("%s, %s, %d: %v", name, table, id, err)
				}
				if !reflect.DeepEqual(back, value) {
					t.Errorf("%s, %s, %d: the documents differ", name, table, id)
				}
			}
		}
	}
}
