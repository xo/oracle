package integration

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/xo/oracle"
)

// TestReturning runs DML RETURNING of one row. It holds the cases of
// tests/cursor/test_2200_dml_returning.py of python-oracledb that return one
// row, written again here. Each names the number of its test there. A
// RETURNING of more than one row is an error in this driver, which returns
// no array. They found three faults. An OUT too small for its value got the
// value cut with no error (2202). An UPDATE of no row cleared the
// destinations, and failed (2204). RETURNING(id)INTO, with no spaces, was a
// statement with no RETURNING, and the server refused its binds (2222).
func TestReturning(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	drop := func() {
		execCmd(db, "DROP TABLE go_ret PURGE", "DROP TABLE go_ret_iot PURGE")
	}
	drop()
	err = execCmd(db, "CREATE TABLE go_ret (id NUMBER(9), s VARCHAR2(400), c CLOB)",
		"CREATE TABLE go_ret_iot (id NUMBER(9) PRIMARY KEY, s VARCHAR2(10), d DATE) ORGANIZATION INDEX")
	if err != nil {
		t.Fatal(err)
	}
	defer drop()
	t.Run("2200 insert", func(t *testing.T) {
		var id int
		var s string
		_, err := db.ExecContext(ctx, "INSERT INTO go_ret (id, s) VALUES (:1, :2) RETURNING id, s INTO :3, :4",
			5, "A test string", oracle.Out{Dest: &id}, oracle.Out{Dest: &s, Size: 400})
		if err != nil || id != 5 || s != "A test string" {
			t.Errorf("got %d and %q, %v", id, s, err)
		}
	})
	t.Run("2202 an OUT too small", func(t *testing.T) {
		var s string
		_, err := db.ExecContext(ctx, "INSERT INTO go_ret (id, s) VALUES (:1, :2) RETURNING s INTO :3",
			6, "A different test string", oracle.Out{Dest: &s, Size: 2})
		if err == nil {
			t.Errorf("got %q and no error", s)
		}
	})
	t.Run("2203 update", func(t *testing.T) {
		var id int
		var s string
		_, err := db.ExecContext(ctx, "UPDATE go_ret SET s = :1 WHERE id = :2 RETURNING id, s INTO :3, :4",
			"The updated value", 5, oracle.Out{Dest: &id}, oracle.Out{Dest: &s, Size: 400})
		if err != nil || id != 5 || s != "The updated value" {
			t.Errorf("got %d and %q, %v", id, s, err)
		}
	})
	t.Run("2204 update no row", func(t *testing.T) {
		var id sql.Null[int64]
		var s sql.Null[string]
		_, err := db.ExecContext(ctx, "UPDATE go_ret SET s = :1 WHERE id = :2 RETURNING id, s INTO :3, :4",
			"no row", 999, oracle.Out{Dest: &id}, oracle.Out{Dest: &s, Size: 400})
		if err != nil || id.Valid || s.Valid {
			t.Errorf("got %v and %v, %v", id, s, err)
		}
	})
	t.Run("2208 a ROWID", func(t *testing.T) {
		var rowid string
		_, err := db.ExecContext(ctx, "INSERT INTO go_ret (id, s) VALUES (278, 'String 278') RETURNING ROWID INTO :1",
			oracle.Out{Dest: &rowid, Size: 100})
		if err != nil {
			t.Fatal(err)
		}
		var s string
		if err := db.QueryRowContext(ctx, "SELECT s FROM go_ret WHERE ROWID = :1", rowid).Scan(&s); err != nil || s != "String 278" {
			t.Errorf("got %q, %v", s, err)
		}
	})
	t.Run("2212 an error", func(t *testing.T) {
		var s string
		_, err := db.ExecContext(ctx, "INSERT INTO go_ret (id, s) VALUES (:1, :2) RETURNING s INTO :3",
			7, strings.Repeat("A", 401), oracle.Out{Dest: &s, Size: 400})
		if err == nil || !strings.Contains(err.Error(), "ORA-12899") {
			t.Errorf("got %v, want ORA-12899", err)
		}
		var n int
		if err := db.QueryRowContext(ctx, "SELECT 1 FROM dual").Scan(&n); err != nil {
			t.Errorf("the connection after the error: %v", err)
		}
	})
	t.Run("2219 a CLOB", func(t *testing.T) {
		var s string
		var c oracle.Clob
		_, err := db.ExecContext(ctx, "INSERT INTO go_ret (id, c) VALUES (8, :1) RETURNING c, c INTO :2, :3",
			"A short CLOB - 1619", oracle.Out{Dest: &s, Size: 100}, oracle.Out{Dest: &c})
		if err != nil || s != "A short CLOB - 1619" {
			t.Errorf("got %q, %v", s, err)
		}
		if v, err := c.Value(); err != nil || v != "A short CLOB - 1619" {
			t.Errorf("an oracle.Clob: got %v, %v", v, err)
		}
	})
	t.Run("2220 a UROWID of an index organized table", func(t *testing.T) {
		var rowid string
		date := time.Date(2017, 4, 11, 0, 0, 0, 0, time.UTC)
		_, err := db.ExecContext(ctx, "INSERT INTO go_ret_iot VALUES (:1, :2, :3) RETURNING ROWID INTO :4",
			1, "ABC", date, oracle.Out{Dest: &rowid, Size: 4000})
		if err != nil {
			t.Fatal(err)
		}
		var s string
		var d time.Time
		if err := db.QueryRowContext(ctx, "SELECT s, d FROM go_ret_iot WHERE ROWID = :1", rowid).Scan(&s, &d); err != nil || s != "ABC" || !d.Equal(date) {
			t.Errorf("got %q and %v, %v", s, d, err)
		}
	})
	t.Run("2222 no spaces", func(t *testing.T) {
		var id int
		_, err := db.ExecContext(ctx, "INSERT INTO go_ret (id) VALUES (:in_val)RETURNING(id)INTO :out_val",
			sql.Named("in_val", 25), sql.Named("out_val", oracle.Out{Dest: &id}))
		if err != nil || id != 25 {
			t.Errorf("got %d, %v", id, err)
		}
	})
	t.Run("more than one row", func(t *testing.T) {
		db.ExecContext(ctx, "INSERT INTO go_ret (id, s) VALUES (900, 'x')")
		db.ExecContext(ctx, "INSERT INTO go_ret (id, s) VALUES (901, 'x')")
		var id int
		_, err := db.ExecContext(ctx, "UPDATE go_ret SET s = 'y' WHERE id >= 900 RETURNING id INTO :1", oracle.Out{Dest: &id})
		if err == nil {
			t.Errorf("got %d and no error", id)
		}
		var n int
		if err := db.QueryRowContext(ctx, "SELECT 1 FROM dual").Scan(&n); err != nil {
			t.Errorf("the connection after the error: %v", err)
		}
	})
}

// TestReturningRows reads a RETURNING of more than one row into OUTs of
// slices, one element for each row (D35). python-oracledb returns a list for
// each OUT. A RETURNING of more than one row into an OUT of one value fails,
// and a RETURNING of no row gives empty slices.
func TestReturningRows(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := t.Context()
	db.Exec("DROP TABLE go_ret_rows PURGE")
	if err := execCmd(db, "CREATE TABLE go_ret_rows (id NUMBER(9), name VARCHAR2(30), d DATE)"); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP TABLE go_ret_rows PURGE")
	date := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if _, err := db.ExecContext(ctx, "INSERT INTO go_ret_rows VALUES (:1, :2, :3)",
		[]int{1, 2, 3, 4}, []string{"one", "two", "", "four"}, []time.Time{date, date, date, date}); err != nil {
		t.Fatal(err)
	}
	var ids []int64
	var names []sql.Null[string]
	var dates []time.Time
	if _, err := db.ExecContext(ctx, "UPDATE go_ret_rows SET d = d + id WHERE id < 4 RETURNING id, name, d INTO :1, :2, :3",
		sql.Out{Dest: &ids}, oracle.Out{Dest: &names, Size: 30}, sql.Out{Dest: &dates}); err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 || len(names) != 3 || len(dates) != 3 {
		t.Fatalf("got %v, %v and %v", ids, names, dates)
	}
	for i, id := range ids {
		want := map[int64]string{1: "one", 2: "two", 3: ""}[id]
		if names[i].V != want || names[i].Valid != (want != "") || !dates[i].Equal(date.AddDate(0, 0, int(id))) {
			t.Errorf("row %d: got %d, %+v and %v", i, id, names[i], dates[i])
		}
	}
	// no row
	ids = []int64{9}
	if _, err := db.ExecContext(ctx, "DELETE FROM go_ret_rows WHERE id > 100 RETURNING id INTO :1", sql.Out{Dest: &ids}); err != nil || len(ids) != 0 {
		t.Errorf("no row: got %v, %v", ids, err)
	}
	// a DELETE of every row
	if _, err := db.ExecContext(ctx, "DELETE FROM go_ret_rows RETURNING id INTO :1", sql.Out{Dest: &ids}); err != nil || len(ids) != 4 {
		t.Errorf("a DELETE: got %v, %v", ids, err)
	}
	// an OUT of one value and more than one row
	if _, err := db.ExecContext(ctx, "INSERT INTO go_ret_rows VALUES (:1, :2, :3)", []int{1, 2}, []string{"a", "b"}, []time.Time{date, date}); err != nil {
		t.Fatal(err)
	}
	var one int64
	if _, err := db.ExecContext(ctx, "UPDATE go_ret_rows SET name = 'x' RETURNING id INTO :1", sql.Out{Dest: &one}); err == nil {
		t.Error("an OUT of one value: no error")
	}
	var n int
	if err := db.QueryRowContext(ctx, "SELECT 1 FROM dual").Scan(&n); err != nil {
		t.Errorf("after: %v", err)
	}
}

// TestReturningInsertID inserts a row with RETURNING ID INTO an oracle.Out
// and checks the ID (sijms/go-ora#307).
func TestReturningInsertID(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err = db.Close()
		if err != nil {
			t.Error(err)
		}
	}()
	err = createMainTable(db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err = dropMainTable(db)
		if err != nil {
			t.Error(err)
		}
	}()
	var id int
	_, err = db.Exec("INSERT INTO TTB_MAIN(ID, NAME, LDATE) VALUES(:1, :2, :3) RETURNING ID INTO :4",
		1, "TEST", time.Now(), oracle.Out{Dest: &id})
	if err != nil {
		t.Fatal(err)
	}
	if id != 1 {
		t.Errorf("expected: %d and got: %d", 1, id)
	}
}
