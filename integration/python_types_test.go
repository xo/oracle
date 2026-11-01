package integration

import (
	"bytes"
	"context"
	"database/sql"
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
