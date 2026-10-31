package oracle_test

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/xo/oracle"
)

// These tests hold the faults of sijms/go-ora that this driver fixed, each
// named for its issue, against a live server.

// TestUntypedNullRow is sijms/go-ora#776: a query whose only columns are
// NULL returned no row and no error.
func TestUntypedNullRow(t *testing.T) {
	db := openLive(t)
	tests := []struct {
		query string
		want  [][]any
	}{
		{"SELECT NULL a FROM dual", [][]any{{nil}}},
		{"SELECT '' a FROM dual", [][]any{{nil}}},
		{"SELECT CASE WHEN 1=0 THEN 0 END a FROM dual", [][]any{{nil}}},
		{"SELECT CASE WHEN 1=0 THEN 0 END a, NULL b FROM dual CONNECT BY LEVEL <= 3", [][]any{{nil, nil}, {nil, nil}, {nil, nil}}},
		{"SELECT 'x' b, NULL a FROM dual", [][]any{{"x", nil}}},
		{"SELECT NULL a FROM dual WHERE 1=0", nil},
	}
	for _, test := range tests {
		rows, err := db.QueryContext(t.Context(), test.query)
		if err != nil {
			t.Errorf("%s: %v", test.query, err)
			continue
		}
		cols, _ := rows.Columns()
		var got [][]any
		for rows.Next() {
			row := make([]any, len(cols))
			dest := make([]any, len(row))
			for i := range row {
				dest[i] = &row[i]
			}
			if err := rows.Scan(dest...); err != nil {
				t.Errorf("%s: %v", test.query, err)
			}
			got = append(got, row)
		}
		if err := rows.Err(); err != nil {
			t.Errorf("%s: %v", test.query, err)
		}
		_ = rows.Close()
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s: got %v, want %v", test.query, got, test.want)
		}
	}
}

// TestReturningStringWithNoSize is sijms/go-ora#703: RETURNING into a string
// through sql.Out with no size broke the connection, and later cut the value.
func TestReturningStringWithNoSize(t *testing.T) {
	db := openLive(t)
	ctx := t.Context()
	const table = "xo_oracle_returning"
	if _, err := db.ExecContext(ctx, "BEGIN EXECUTE IMMEDIATE 'DROP TABLE "+table+" PURGE'; EXCEPTION WHEN OTHERS THEN NULL; END;"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE "+table+" (id NUMBER(10), name VARCHAR2(50))"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DROP TABLE " + table + " PURGE") })
	var id int64
	var name string
	_, err := db.ExecContext(ctx, "INSERT INTO "+table+" (id, name) VALUES (:1, :2) RETURNING id, name INTO :3, :4",
		7, "TestTest", sql.Out{Dest: &id}, sql.Out{Dest: &name})
	if err != nil {
		t.Fatal(err)
	}
	if id != 7 || name != "TestTest" {
		t.Errorf("got %d and %q, want 7 and TestTest", id, name)
	}
}

type (
	namedString string
	namedInt    int64
)

// TestNamedTypes is sijms/go-ora#699: a named type such as type ID string
// gave unsupported go type.
func TestNamedTypes(t *testing.T) {
	db := openLive(t)
	ctx := t.Context()
	var s string
	if err := db.QueryRowContext(ctx, "SELECT :1 FROM dual", namedString("abc")).Scan(&s); err != nil || s != "abc" {
		t.Errorf("named string: got %q, %v", s, err)
	}
	var n int64
	if err := db.QueryRowContext(ctx, "SELECT :1 FROM dual", namedInt(42)).Scan(&n); err != nil || n != 42 {
		t.Errorf("named int: got %d, %v", n, err)
	}
	var d time.Duration
	if err := db.QueryRowContext(ctx, "SELECT :1 FROM dual", 90*time.Second).Scan(&d); err != nil || d != 90*time.Second {
		t.Errorf("time.Duration, which the driver binds as an interval: got %v, %v", d, err)
	}
}

// TestUnopenedRefCursor is sijms/go-ora#671: a REF CURSOR that the procedure
// never opened broke the connection. It is an error, and the connection
// stays usable.
func TestUnopenedRefCursor(t *testing.T) {
	db := openLive(t)
	ctx := t.Context()
	if _, err := db.ExecContext(ctx, `CREATE OR REPLACE PROCEDURE xo_oracle_cursor(c OUT SYS_REFCURSOR) AS BEGIN NULL; END;`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DROP PROCEDURE xo_oracle_cursor") })
	var cur oracle.RefCursor
	_, err := db.ExecContext(ctx, "BEGIN xo_oracle_cursor(:1); END;", sql.Out{Dest: &cur})
	if err == nil {
		t.Error("an unopened cursor gives no error")
	}
	usable(t, db)
}

// TestBatchLengths is sijms/go-ora#695: an empty array in a batch panicked,
// and arrays of different lengths were cut to the shortest.
func TestBatchLengths(t *testing.T) {
	db := openLive(t)
	ctx := t.Context()
	const table = "xo_oracle_batch"
	if _, err := db.ExecContext(ctx, "BEGIN EXECUTE IMMEDIATE 'DROP TABLE "+table+" PURGE'; EXCEPTION WHEN OTHERS THEN NULL; END;"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE "+table+" (id NUMBER(10), name VARCHAR2(50))"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DROP TABLE " + table + " PURGE") })
	insert := "INSERT INTO " + table + " (id, name) VALUES (:1, :2)"
	if _, err := db.ExecContext(ctx, insert, []int64{}, []string{}); err == nil {
		t.Error("an empty batch gives no error")
	}
	if _, err := db.ExecContext(ctx, insert, []int64{1, 2, 3}, []string{"a", "b"}); err == nil {
		t.Error("arrays of 3 and 2 rows give no error")
	}
	if _, err := db.ExecContext(ctx, insert, []int64{1, 2}, []string{"a", "b", "c"}); err == nil {
		t.Error("arrays of 2 and 3 rows give no error")
	}
	res, err := db.ExecContext(ctx, insert, []int64{1, 2}, []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := res.RowsAffected(); err != nil || n != 2 {
		t.Errorf("a batch of 2 rows affects %d, %v", n, err)
	}
	usable(t, db)
}

// TestBatchError is sijms/go-ora#698: a batch whose row broke a key waited
// until the network timed out on 11g, where it returns ORA-00001.
func TestBatchError(t *testing.T) {
	db := openLive(t)
	const table = "xo_oracle_batch_error"
	if _, err := db.ExecContext(t.Context(), "BEGIN EXECUTE IMMEDIATE 'DROP TABLE "+table+" PURGE'; EXCEPTION WHEN OTHERS THEN NULL; END;"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "CREATE TABLE "+table+" (id NUMBER(10) PRIMARY KEY, name VARCHAR2(20), val NUMBER(10))"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DROP TABLE " + table + " PURGE") })
	merge := `MERGE INTO ` + table + ` t
		USING (SELECT :id id, :name name, :val val FROM dual) s
		ON (s.id = t.id AND s.name = t.name)
		WHEN MATCHED THEN UPDATE SET t.val = s.val
		WHEN NOT MATCHED THEN INSERT (id, name, val) VALUES (s.id, s.name, s.val)`
	tests := []struct {
		name  string
		query string
		args  []any
	}{
		{"insert", "INSERT INTO " + table + " (id, name, val) VALUES (:1, :2, :3)", []any{[]int64{1, 1}, []string{"a", "b"}, []int64{1, 2}}},
		{"merge", merge, []any{sql.Named("id", []int64{2, 2}), sql.Named("name", []string{"a", "b"}), sql.Named("val", []int64{1, 2})}},
	}
	for _, test := range tests {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		_, err := db.ExecContext(ctx, test.query, test.args...)
		cancel()
		var serr *oracle.ServerError
		if !errors.As(err, &serr) || serr.ErrCode != 1 {
			t.Errorf("%s: got %v, want ORA-00001", test.name, err)
		}
	}
	usable(t, db)
}
