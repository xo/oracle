package integration

import (
	"context"
	"database/sql"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/xo/oracle"
)

// TestCursorOther holds cases of tests/cursor/test_1800_other.py of
// python-oracledb, written again here. A python cursor that runs again is a
// prepared statement here, or a statement on one sql.Conn.
func TestCursorOther(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := t.Context()
	db.Exec("DROP TABLE go_other PURGE")
	if err := execCmd(db, "CREATE TABLE go_other (int_col NUMBER(9) NOT NULL, string_col VARCHAR2(400))"); err != nil {
		t.Fatal(err)
	}
	defer execCmd(db, "DROP TABLE go_other PURGE")
	reset := func(t *testing.T) {
		t.Helper()
		if _, err := db.ExecContext(ctx, "TRUNCATE TABLE go_other"); err != nil {
			t.Fatal(err)
		}
	}
	strs := func(t *testing.T) func(*sql.Rows, error) [][]string {
		return func(rows *sql.Rows, err error) [][]string {
			t.Helper()
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			cols, _ := rows.Columns()
			var got [][]string
			for rows.Next() {
				row := make([]sql.Null[string], len(cols))
				ptrs := make([]any, len(cols))
				for i := range row {
					ptrs[i] = &row[i]
				}
				if err := rows.Scan(ptrs...); err != nil {
					t.Fatal(err)
				}
				var s []string
				for _, v := range row {
					s = append(s, v.V)
				}
				got = append(got, s)
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			return got
		}
	}
	same := func(a, b [][]string) bool {
		return slices.EqualFunc(a, b, slices.Equal)
	}

	t.Run("1815 a bool into a NUMBER", func(t *testing.T) {
		reset(t)
		for _, b := range []bool{false, true} {
			if _, err := db.ExecContext(ctx, "INSERT INTO go_other VALUES (:1, :2)", b, "a bool"); err != nil {
				t.Fatal(err)
			}
		}
		got := strs(t)(db.QueryContext(ctx, "SELECT int_col FROM go_other ORDER BY int_col"))
		if !same(got, [][]string{{"0"}, {"1"}}) {
			t.Errorf("got %q", got)
		}
	})

	t.Run("1832 NULL and other OUT strings", func(t *testing.T) {
		var out [4]sql.Null[string]
		if _, err := db.ExecContext(ctx, "BEGIN :1 := NULL; :2 := 'Value 1'; :3 := NULL; :4 := 'Value 2'; END;",
			oracle.Out{Dest: &out[0], Size: 30}, oracle.Out{Dest: &out[1], Size: 30},
			oracle.Out{Dest: &out[2], Size: 30}, oracle.Out{Dest: &out[3], Size: 30}); err != nil {
			t.Fatal(err)
		}
		if out[0].Valid || out[1].V != "Value 1" || out[2].Valid || out[3].V != "Value 2" {
			t.Errorf("got %+v", out)
		}
	})

	t.Run("1834 repeated DDL", func(t *testing.T) {
		for range 2 {
			reset(t)
			if _, err := db.ExecContext(ctx, "INSERT INTO go_other (int_col) VALUES (1)"); err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("1835 SQL that is not ASCII", func(t *testing.T) {
		var s string
		if err := db.QueryRowContext(ctx, "SELECT 'FÖÖ' FROM dual").Scan(&s); err != nil || s != "FÖÖ" {
			t.Errorf("got %q, %v", s, err)
		}
	})

	t.Run("1836 the case of a name", func(t *testing.T) {
		var s string
		if err := db.QueryRowContext(ctx, "SELECT :test FROM dual", sql.Named("TEST", "a")).Scan(&s); err != nil || s != "a" {
			t.Errorf("got %q, %v", s, err)
		}
	})

	// go-ora matched a quoted name in any case
	t.Run("1837 the case of a quoted name", func(t *testing.T) {
		var s string
		err := db.QueryRowContext(ctx, `SELECT :"test" FROM dual`, sql.Named("TEST", "a")).Scan(&s)
		if err == nil {
			t.Errorf("got %q, and a quoted name keeps its case", s)
		}
		if err := db.QueryRowContext(ctx, `SELECT :"test" FROM dual`, sql.Named("test", "a")).Scan(&s); err != nil || s != "a" {
			t.Errorf("the same case: got %q, %v", s, err)
		}
	})

	t.Run("1838 a keyword as a name", func(t *testing.T) {
		var s string
		err := db.QueryRowContext(ctx, "SELECT :ROWID FROM dual", "a").Scan(&s)
		requireErr(t, "ROWID", err, "ORA-01745")
	})

	t.Run("1839 fewer rows to a fetch than the query has", func(t *testing.T) {
		qctx := oracle.WithOptions(ctx, oracle.WithPrefetch(1))
		for range 2 {
			got := strs(t)(db.QueryContext(qctx, "SELECT 1 FROM dual UNION SELECT 2 FROM dual"))
			if !same(got, [][]string{{"1"}, {"2"}}) {
				t.Errorf("got %q", got)
			}
		}
	})

	// a prepared statement whose run failed parsed again into the cursor of
	// the error, and go-ora sent its text with no pointer, so the second run
	// failed with ORA-03120
	t.Run("1841 a run again after an error", func(t *testing.T) {
		for range 2 {
			_, err := db.ExecContext(ctx, "SELECT * FROM go_no_such_table")
			requireErr(t, "a missing table", err, "ORA-00942")
		}
		stmt, err := db.PrepareContext(ctx, "INSERT INTO go_other (int_col) VALUES (NULL)")
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Close()
		for range 2 {
			_, err := stmt.ExecContext(ctx)
			requireErr(t, "a NULL", err, "ORA-01400")
		}
	})

	t.Run("1842 a view that drops a column", func(t *testing.T) {
		defer db.Exec("DROP VIEW go_other_1007")
		if _, err := db.ExecContext(ctx, `CREATE OR REPLACE VIEW go_other_1007 AS
			SELECT 1 AS sample_number, 'String' AS sample_string, 'Another String' AS another_string FROM dual`); err != nil {
			t.Fatal(err)
		}
		stmt, err := db.PrepareContext(ctx, "SELECT * FROM go_other_1007")
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Close()
		if got := strs(t)(stmt.QueryContext(ctx)); !same(got, [][]string{{"1", "String", "Another String"}}) {
			t.Errorf("before: got %q", got)
		}
		if _, err := db.ExecContext(ctx, `CREATE OR REPLACE VIEW go_other_1007 AS
			SELECT 1 AS sample_number, 'Another String' AS another_string FROM dual`); err != nil {
			t.Fatal(err)
		}
		if got := strs(t)(stmt.QueryContext(ctx)); !same(got, [][]string{{"1", "Another String"}}) {
			t.Errorf("after: got %q", got)
		}
	})

	t.Run("1843 RETURNING of no row in PL/SQL", func(t *testing.T) {
		reset(t)
		var n sql.NullInt64
		if _, err := db.ExecContext(ctx, `BEGIN
			UPDATE go_other SET int_col = :1 WHERE string_col = :2 RETURNING int_col INTO :3;
		END;`, 1, "test string 4352", sql.Out{Dest: &n}); err != nil || n.Valid {
			t.Errorf("got %+v, %v", n, err)
		}
	})

	t.Run("1844 rows that repeat", func(t *testing.T) {
		// python-oracledb sets no rows with the execute, and here one row
		// comes with each fetch
		qctx := oracle.WithOptions(ctx, oracle.WithPrefetch(1))
		stmt, err := db.PrepareContext(ctx, `SELECT 'A', 'B', 'C' FROM dual UNION ALL
			SELECT 'A', 'B', 'C' FROM dual UNION ALL SELECT 'A', 'B', 'C' FROM dual`)
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Close()
		want := [][]string{{"A", "B", "C"}, {"A", "B", "C"}, {"A", "B", "C"}}
		for range 2 {
			if got := strs(t)(stmt.QueryContext(qctx)); !same(got, want) {
				t.Errorf("got %q", got)
			}
		}
	})

	t.Run("1853 a column added to a prepared query", func(t *testing.T) {
		db.Exec("DROP TABLE go_other_1853 PURGE")
		if err := execCmd(db, "CREATE TABLE go_other_1853 (col1 VARCHAR2(10))", "INSERT INTO go_other_1853 VALUES ('val 1')"); err != nil {
			t.Fatal(err)
		}
		defer db.Exec("DROP TABLE go_other_1853 PURGE")
		stmt, err := db.PrepareContext(ctx, "SELECT * FROM go_other_1853")
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Close()
		if got := strs(t)(stmt.QueryContext(ctx)); !same(got, [][]string{{"val 1"}}) {
			t.Errorf("before: got %q", got)
		}
		if err := execCmd(db, "ALTER TABLE go_other_1853 ADD col2 VARCHAR2(10)", "UPDATE go_other_1853 SET col2 = 'val 2'"); err != nil {
			t.Fatal(err)
		}
		if got := strs(t)(stmt.QueryContext(ctx)); !same(got, [][]string{{"val 1", "val 2"}}) {
			t.Errorf("after: got %q", got)
		}
	})

	t.Run("1855 PL/SQL with strings that grow", func(t *testing.T) {
		stmt, err := db.PrepareContext(ctx, "BEGIN :1 := LENGTH(:2); END;")
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Close()
		for _, s := range []string{"one", "three", "end", "four", "five", "six"} {
			var n int
			if _, err := stmt.ExecContext(ctx, sql.Out{Dest: &n}, s); err != nil || n != len(s) {
				t.Errorf("%q: got %d, %v", s, n, err)
			}
		}
	})

	t.Run("1866 a LOB after an error", func(t *testing.T) {
		q := "SELECT TO_CLOB(:1), 1 / (DBMS_LOB.GETLENGTH(TO_CLOB(:2)) - 1) FROM dual"
		rows, err := db.QueryContext(ctx, q, "a", "a")
		if err == nil {
			for rows.Next() {
			}
			err = rows.Err()
			rows.Close()
		}
		requireErr(t, "a division by zero", err, "ORA-01476")
		var s string
		var n int
		if err := db.QueryRowContext(ctx, q, "bb", "bb").Scan(&s, &n); err != nil || s != "bb" || n != 1 {
			t.Errorf("got %q and %d, %v", s, n, err)
		}
	})

	t.Run("1870 PL/SQL after rows that repeat", func(t *testing.T) {
		reset(t)
		for i := range 20 {
			if _, err := db.ExecContext(ctx, "INSERT INTO go_other VALUES (:1, 'test_cursor_1870')", i+1); err != nil {
				t.Fatal(err)
			}
		}
		conn, err := db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		if got := strs(t)(conn.QueryContext(ctx, "SELECT int_col, string_col FROM go_other ORDER BY int_col")); len(got) != 20 {
			t.Errorf("got %d rows, want 20", len(got))
		}
		var n int
		if _, err := conn.ExecContext(ctx, "BEGIN :1 := 4370; END;", sql.Out{Dest: &n}); err != nil || n != 4370 {
			t.Errorf("got %d, %v", n, err)
		}
	})
}

// TestCursorOtherLOBs holds the cases of LOBs of
// tests/cursor/test_1800_other.py of python-oracledb (1840, 1851, 1857 to
// 1859), written again here.
func TestCursorOtherLOBs(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := t.Context()
	create := func(t *testing.T) {
		t.Helper()
		db.Exec("DROP TABLE go_other_lobs PURGE")
		if err := execCmd(db, "CREATE TABLE go_other_lobs (int_col NUMBER(9) NOT NULL, clob_col CLOB, blob_col BLOB, extra NUMBER(9))"); err != nil {
			t.Fatal(err)
		}
	}
	create(t)
	defer db.Exec("DROP TABLE go_other_lobs PURGE")

	t.Run("1840 a BLOB as bytes again", func(t *testing.T) {
		data := []byte("An arbitrary set of blob data for test case 4348")
		for range 2 {
			if err := execCmd(db, "DELETE FROM go_other_lobs"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, "INSERT INTO go_other_lobs (int_col, blob_col) VALUES (1, :1)", data); err != nil {
				t.Fatal(err)
			}
			var got []byte
			if err := db.QueryRowContext(ctx, "SELECT blob_col FROM go_other_lobs").Scan(&got); err != nil || string(got) != string(data) {
				t.Errorf("got %q, %v", got, err)
			}
		}
	})

	// after the define of a query with a LOB, go-ora sent each later run as
	// a fetch with no execute, which failed with ORA-01002 once a bind
	// changed type
	t.Run("1851 a bind that changes type", func(t *testing.T) {
		if err := execCmd(db, "DELETE FROM go_other_lobs",
			"INSERT INTO go_other_lobs (int_col, clob_col) VALUES (1, 'Short value 1')",
			"INSERT INTO go_other_lobs (int_col, clob_col) VALUES (56, 'Short value 56')"); err != nil {
			t.Fatal(err)
		}
		stmt, err := db.PrepareContext(ctx, "SELECT int_col, clob_col FROM go_other_lobs WHERE int_col = :int_col")
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Close()
		for _, test := range []struct {
			arg  any
			want string
		}{{"1", "Short value 1"}, {"56", "Short value 56"}, {1, "Short value 1"}} {
			var n int
			var s string
			if err := stmt.QueryRowContext(ctx, sql.Named("int_col", test.arg)).Scan(&n, &s); err != nil || s != test.want {
				t.Errorf("%v: got %q, %v", test.arg, s, err)
			}
		}
	})

	t.Run("1857 the order of binds in PL/SQL", func(t *testing.T) {
		if err := execCmd(db, "DELETE FROM go_other_lobs"); err != nil {
			t.Fatal(err)
		}
		data := strings.Repeat("x", 9000)
		insert := "INSERT INTO go_other_lobs (int_col, clob_col, extra) VALUES (:1, :2, :3)"
		if _, err := db.ExecContext(ctx, insert, 1, data, 5); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, "BEGIN "+insert+"; END;", 2, data, 6); err != nil {
			t.Fatal(err)
		}
		rows, err := db.QueryContext(ctx, "SELECT int_col, clob_col, extra FROM go_other_lobs ORDER BY int_col")
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		want := [][2]int{{1, 5}, {2, 6}}
		i := 0
		for ; rows.Next(); i++ {
			var n, extra int
			var s string
			if err := rows.Scan(&n, &s, &extra); err != nil {
				t.Fatal(err)
			}
			if i >= len(want) || n != want[i][0] || extra != want[i][1] || s != data {
				t.Errorf("row %d: got %d, %d characters, %d", i, n, len(s), extra)
			}
		}
		if err := rows.Err(); err != nil || i != 2 {
			t.Errorf("got %d rows, %v", i, err)
		}
	})

	t.Run("1858 and 1859 a table with a LOB built again", func(t *testing.T) {
		if err := execCmd(db, "DELETE FROM go_other_lobs"); err != nil {
			t.Fatal(err)
		}
		insert := func(t *testing.T) {
			t.Helper()
			for i, s := range []string{"CLOB value 1", "CLOB value 2"} {
				if _, err := db.ExecContext(ctx, "INSERT INTO go_other_lobs (int_col, clob_col) VALUES (:1, :2)", i+1, s); err != nil {
					t.Fatal(err)
				}
			}
		}
		insert(t)
		stmt, err := db.PrepareContext(ctx, "SELECT int_col, clob_col FROM go_other_lobs ORDER BY int_col")
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Close()
		read := func(t *testing.T, ctx context.Context, asLOB bool) {
			t.Helper()
			rows, err := stmt.QueryContext(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			var got []string
			for rows.Next() {
				var n int
				var s string
				if asLOB {
					var c oracle.Clob
					if err := rows.Scan(&n, &c); err != nil {
						t.Fatal(err)
					}
					b, err := io.ReadAll(c.NewReader(ctx))
					if err != nil {
						t.Fatal(err)
					}
					s = string(b)
				} else if err := rows.Scan(&n, &s); err != nil {
					t.Fatal(err)
				}
				got = append(got, s)
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, []string{"CLOB value 1", "CLOB value 2"}) {
				t.Errorf("got %q", got)
			}
		}
		for _, asLOB := range []bool{false, true} {
			read(t, ctx, asLOB)
			read(t, ctx, asLOB)
		}
		create(t)
		insert(t)
		for _, asLOB := range []bool{false, true} {
			read(t, ctx, asLOB)
		}
	})
}

// TestDomainColumn reads a table whose column has a domain and annotations,
// as 1861 of tests/cursor/test_1800_other.py of python-oracledb does. The
// driver has no API for the domain or the annotations of a column, and reads
// the rows.
func TestDomainColumn(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	requireRelease(t, db, 23)
	db.Exec("DROP TABLE go_other_domain PURGE")
	db.Exec("DROP DOMAIN go_simple_domain")
	if err := execCmd(db, "CREATE DOMAIN go_simple_domain AS NUMBER(3, 0) NOT NULL",
		`CREATE TABLE go_other_domain (id NUMBER(6), age NUMBER(3) DOMAIN go_simple_domain
			ANNOTATIONS (anno_1 'first annotation', anno_2 'second annotation', anno_3))`,
		"INSERT INTO go_other_domain VALUES (1, 25)"); err != nil {
		t.Fatal(err)
	}
	defer execCmd(db, "DROP TABLE go_other_domain PURGE", "DROP DOMAIN go_simple_domain")
	var id, age int
	if err := db.QueryRowContext(t.Context(), "SELECT * FROM go_other_domain").Scan(&id, &age); err != nil || id != 1 || age != 25 {
		t.Errorf("got %d and %d, %v", id, age, err)
	}
}
