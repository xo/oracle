package integration

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/cockroachdb/apd/v3"

	"github.com/xo/oracle"
)

// TestBatches runs DML in batches, an Exec whose arguments are slices. It
// holds the cases of tests/cursor/test_1200_executemany.py of python-oracledb
// that a batch of DML covers, written again here. Each names the number of
// its test there. A batch of PL/SQL, which python-oracledb runs, binds its
// slices as PL/SQL arrays in this driver.
func TestBatches(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	db.Exec("DROP TABLE go_batch PURGE")
	if err := execCmd(db, "CREATE TABLE go_batch (id NUMBER(9) PRIMARY KEY, s VARCHAR2(100), n NUMBER)"); err != nil {
		t.Fatal(err)
	}
	defer execCmd(db, "DROP TABLE go_batch PURGE")
	reset := func(t *testing.T) {
		t.Helper()
		if _, err := db.ExecContext(ctx, "TRUNCATE TABLE go_batch"); err != nil {
			t.Fatal(err)
		}
	}
	// rows reads the table as "id:s:n" for each row, in the order of id
	rows := func(t *testing.T) []string {
		t.Helper()
		r, err := db.QueryContext(ctx, "SELECT id, s, TO_CHAR(n) FROM go_batch ORDER BY id")
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		var out []string
		for r.Next() {
			var id int
			var s, n sql.Null[string]
			if err := r.Scan(&id, &s, &n); err != nil {
				t.Fatal(err)
			}
			out = append(out, fmt.Sprintf("%d:%s:%s", id, s.V, n.V))
		}
		if err := r.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}
	ids := func(from, to int) []int {
		var v []int
		for i := from; i < to; i++ {
			v = append(v, i)
		}
		return v
	}
	t.Run("1200 a batch of 250", func(t *testing.T) {
		reset(t)
		if _, err := db.ExecContext(ctx, "INSERT INTO go_batch (id) VALUES (:1)", ids(0, 250)); err != nil {
			t.Fatal(err)
		}
		if got := len(rows(t)); got != 250 {
			t.Errorf("got %d rows", got)
		}
	})
	t.Run("1203 two batches of one statement", func(t *testing.T) {
		reset(t)
		stmt, err := db.PrepareContext(ctx, "INSERT INTO go_batch (id) VALUES (:1)")
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Close()
		for _, batch := range [][]int{ids(0, 50), ids(50, 235)} {
			if _, err := stmt.ExecContext(ctx, batch); err != nil {
				t.Fatal(err)
			}
		}
		if got := len(rows(t)); got != 235 {
			t.Errorf("got %d rows", got)
		}
	})
	t.Run("1205 NULLs, then a string, in the next batch", func(t *testing.T) {
		reset(t)
		stmt, err := db.PrepareContext(ctx, "INSERT INTO go_batch (id, s) VALUES (:1, :2)")
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Close()
		if _, err := stmt.ExecContext(ctx, []int{1, 2}, []sql.Null[string]{{}, {}}); err != nil {
			t.Fatal(err)
		}
		if _, err := stmt.ExecContext(ctx, []int{3, 4}, []sql.Null[string]{{}, {V: "Testing", Valid: true}}); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(rows(t), " "); got != "1:: 2:: 3:: 4:Testing:" {
			t.Errorf("got %s", got)
		}
	})
	t.Run("1206 numbers of several types", func(t *testing.T) {
		reset(t)
		big, _, _ := apd.NewFromString("36893488147419103232")
		dec, _, _ := apd.NewFromString("24.5")
		if _, err := db.ExecContext(ctx, "INSERT INTO go_batch (id, n) VALUES (:1, :2)", []int{1, 2, 3, 4, 5}, []any{5, 7.0, 6.5, big, dec}); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(rows(t), " "); got != "1::5 2::7 3::6.5 4::36893488147419103232 5::24.5" {
			t.Errorf("got %s", got)
		}
	})
	t.Run("a NULL last row", func(t *testing.T) {
		// the batch writes every row with the coder of its last value
		// that is not NULL
		reset(t)
		if _, err := db.ExecContext(ctx, "INSERT INTO go_batch (id, s, n) VALUES (:1, :2, :3)", []int{1, 2}, []any{"a", nil}, []any{6.5, nil}); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(rows(t), " "); got != "1:a:6.5 2::" {
			t.Errorf("got %s", got)
		}
	})
	t.Run("1207 strings that grow", func(t *testing.T) {
		reset(t)
		s := []string{"First", "Second", "Third", "Fourth", "Fifth", "Sixth", "Seventh and the longest one"}
		if _, err := db.ExecContext(ctx, "INSERT INTO go_batch (id, s) VALUES (:1, :2)", ids(1, 8), s); err != nil {
			t.Fatal(err)
		}
		got := rows(t)
		if len(got) != 7 || got[6] != "7:Seventh and the longest one:" {
			t.Errorf("got %v", got)
		}
	})
	t.Run("1208 a key error in a batch", func(t *testing.T) {
		reset(t)
		_, err := db.ExecContext(ctx, "INSERT INTO go_batch (id) VALUES (:1)", []int{1, 2, 3, 2, 5})
		if err == nil || !strings.Contains(err.Error(), "ORA-00001") {
			t.Fatalf("got %v, want ORA-00001", err)
		}
		if got := len(rows(t)); got != 3 {
			t.Errorf("got %d rows, want the 3 before the error", got)
		}
	})
	t.Run("1221 a NULL first, then a number", func(t *testing.T) {
		reset(t)
		if _, err := db.ExecContext(ctx, "INSERT INTO go_batch (id, n) VALUES (:1, :2)", []int{1, 2}, []sql.Null[int64]{{}, {V: 25, Valid: true}}); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(rows(t), " "); got != "1:: 2::25" {
			t.Errorf("got %s", got)
		}
	})
	t.Run("1232 batches whose first holds only NULLs", func(t *testing.T) {
		reset(t)
		stmt, err := db.PrepareContext(ctx, "INSERT INTO go_batch (id, n) VALUES (:1, :2)")
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Close()
		var want []string
		for pos := 1; pos <= 20; pos += 4 {
			var id []int
			var n []sql.Null[float64]
			for i := pos; i < pos+4; i++ {
				id = append(id, i)
				if i <= 10 {
					n = append(n, sql.Null[float64]{})
					want = append(want, fmt.Sprintf("%d::", i))
				} else {
					n = append(n, sql.Null[float64]{V: float64(i) * 0.25, Valid: true})
					want = append(want, fmt.Sprintf("%d::%g", i, float64(i)*0.25))
				}
			}
			if _, err := stmt.ExecContext(ctx, id, n); err != nil {
				t.Fatalf("the batch from %d: %v", pos, err)
			}
		}
		if got := strings.Join(rows(t), " "); got != strings.Join(want, " ") {
			t.Errorf("got  %s\nwant %s", got, strings.Join(want, " "))
		}
	})
	t.Run("a batch of 100,000", func(t *testing.T) {
		reset(t)
		s := make([]string, 100_000)
		for i := range s {
			s[i] = fmt.Sprint("row ", i)
		}
		if _, err := db.ExecContext(ctx, "INSERT INTO go_batch (id, s) VALUES (:1, :2)", ids(0, len(s)), s); err != nil {
			t.Fatal(err)
		}
		var n int
		var last string
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*), MAX(s) KEEP (DENSE_RANK LAST ORDER BY id) FROM go_batch").Scan(&n, &last); err != nil || n != len(s) || last != "row 99999" {
			t.Errorf("got %d rows and %q, %v", n, last, err)
		}
	})
}

// TestJSONBatch runs a batch of JSON values that shrink, grow and end in a
// NULL. go-ora wrote every row with the locator of the last, which holds its
// length, so the server refused a longer row, and a NULL last row hung the
// connection.
func TestJSONBatch(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	requireRelease(t, db, 21)
	lobTable(t, db, "go_json_batch", "id NUMBER, j JSON")
	short, err := oracle.NewJSON(`{"a":1}`)
	if err != nil {
		t.Fatal(err)
	}
	long, err := oracle.NewJSON(`{"a":"a value much longer than the short one"}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO go_json_batch VALUES (:1, :2)", []int{1, 2, 3, 4}, []any{long, short, long, nil}); err != nil {
		t.Fatal(err)
	}
	r, err := db.Query("SELECT NVL(JSON_SERIALIZE(j), 'NULL') FROM go_json_batch ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var got []string
	for r.Next() {
		var s string
		if err := r.Scan(&s); err != nil {
			t.Fatal(err)
		}
		got = append(got, s)
	}
	want := []string{`{"a":"a value much longer than the short one"}`, `{"a":1}`, `{"a":"a value much longer than the short one"}`, "NULL"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("got %v\nwant %v", got, want)
	}
}
