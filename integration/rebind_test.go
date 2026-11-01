package integration

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/xo/oracle"
)

// TestRebind runs one prepared INSERT many times while its arguments change
// size, type and NULL from one run to the next, which makes the driver send
// the definition of the binds again, and then reads the rows back. It holds
// the cases of tests/cursor/test_1000_execute.py (1017, 1022) and
// tests/data_types/test_2000_char.py (2002, 2027) of python-oracledb,
// written again here. A RETURNING statement that ran again with an argument
// changed from a value to NULL failed with ORA-03146, which was
// sijms/go-ora#697.
func TestRebind(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	db.Exec("DROP TABLE go_rebind PURGE")
	err = execCmd(db, `CREATE TABLE go_rebind (
	id NUMBER(10) PRIMARY KEY,
	s VARCHAR2(4000),
	n NUMBER,
	d DATE,
	c CLOB,
	r RAW(2000))`)
	if err != nil {
		t.Fatal(err)
	}
	defer execCmd(db, "DROP TABLE go_rebind PURGE")
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	stmt, err := conn.PrepareContext(ctx, "INSERT INTO go_rebind VALUES (:1, :2, :3, :4, :5, :6)")
	if err != nil {
		t.Fatal(err)
	}
	defer stmt.Close()
	date := time.Date(2024, 2, 3, 4, 5, 6, 0, time.UTC)
	long := strings.Repeat("long string ", 3000)
	type row struct {
		s, n, d, c, r any
	}
	rows := []row{
		{"short", 1, date, "c", []byte{1}},
		{strings.Repeat("ABC", 100), "2.5", nil, strings.Repeat("c", 3000), bytes.Repeat([]byte{2}, 1000)},
		{nil, nil, date.Add(time.Hour), nil, nil},
		{"short again", 3.25, date, long, []byte{3}},
		{strings.Repeat("é", 1500), int64(-4), nil, "c", bytes.Repeat([]byte{4}, 2000)},
		{"", 0, date, "", []byte{}},
		{"last", 5, date, "end", []byte{5}},
	}
	for i, r := range rows {
		if _, err := stmt.ExecContext(ctx, i+1, r.s, r.n, r.d, r.c, r.r); err != nil {
			t.Fatalf("row %d: %v", i+1, err)
		}
	}
	got, err := conn.QueryContext(ctx, "SELECT id, s, TO_CHAR(n), d, c, r FROM go_rebind ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer got.Close()
	text := func(v any) string {
		switch v := v.(type) {
		case nil:
			return ""
		case []byte:
			return fmt.Sprintf("%x", v)
		case time.Time:
			return v.Format(time.DateTime)
		case int:
			return fmt.Sprint(v)
		case float64:
			return fmt.Sprint(v)
		}
		return fmt.Sprint(v)
	}
	n := 0
	for got.Next() {
		var id int
		var s, num, c sql.Null[string]
		var d sql.Null[time.Time]
		var r []byte
		if err := got.Scan(&id, &s, &num, &d, &c, &r); err != nil {
			t.Fatal(err)
		}
		want := rows[id-1]
		// Oracle stores an empty string as NULL
		if s.V != text(want.s) || num.V != text(want.n) || text(d.V) != text(want.d) && want.d != nil ||
			c.V != text(want.c) || fmt.Sprintf("%x", r) != text(want.r) {
			t.Errorf("row %d: got %.20q %q %v %d characters %x", id, s.V, num.V, d.V, len(c.V), r)
		}
		n++
	}
	if err := got.Err(); err != nil || n != len(rows) {
		t.Fatalf("got %d rows, %v", n, err)
	}
	// RETURNING INTO, with inputs that change size and NULL, 1022
	ret, err := conn.PrepareContext(ctx, "INSERT INTO go_rebind (id, s, c) VALUES (:1, :2, :3) RETURNING id INTO :4")
	if err != nil {
		t.Fatal(err)
	}
	defer ret.Close()
	for i, args := range [][2]any{{`{"a", "b"}`, nil}, {nil, `{"a", "b"}`}, {`{"a"}`, nil}} {
		var id string
		if _, err := ret.ExecContext(ctx, 100+i, args[0], args[1], oracle.Out{Dest: &id, Size: 20}); err != nil {
			t.Fatalf("returning %d: %v", i, err)
		}
		if id != fmt.Sprint(100+i) {
			t.Errorf("returning %d: got %q", i, id)
		}
	}
	// an OUT bound to another variable on the second run, 2002
	call, err := conn.PrepareContext(ctx, "BEGIN :1 := 'Called'; END;")
	if err != nil {
		t.Fatal(err)
	}
	defer call.Close()
	for i := range 2 {
		var v string
		if _, err := call.ExecContext(ctx, oracle.Out{Dest: &v, Size: 30}); err != nil || v != "Called" {
			t.Errorf("call %d: got %q, %v", i, v, err)
		}
	}
}
