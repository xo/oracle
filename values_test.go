package oracle_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json/jsontext"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/apd/v3"

	"github.com/xo/oracle"
)

// TestValuesIntoAny checks the Go type of each Oracle type, scanned into
// *any, against the table in docs/DESIGN.md, under Values.
func TestValuesIntoAny(t *testing.T) {
	db := openLive(t)
	tests := []struct {
		expr string
		want string
	}{
		{"CAST(42 AS NUMBER(10))", "int64 42"},
		{"CAST(-7 AS NUMBER(18))", "int64 -7"},
		{"CAST(1.5 AS NUMBER(10,2))", "*apd.Decimal 1.5"},
		{"CAST(12345678901234567890 AS NUMBER(20))", "*apd.Decimal 12345678901234567890"},
		{"1", "*apd.Decimal 1"},
		{"0.1", "*apd.Decimal 0.1"},
		{"CAST(1.5 AS BINARY_FLOAT)", "float64 1.5"},
		{"CAST(134.45 AS BINARY_FLOAT)", "float64 134.45"},
		{"CAST(134.45 AS BINARY_DOUBLE)", "float64 134.45"},
		{"'abc'", "string abc"},
		{"CAST(NULL AS VARCHAR2(10))", "<nil> <nil>"},
		{"HEXTORAW('DEADBEEF')", "[]uint8 [222 173 190 239]"},
		{"INTERVAL '2-3' YEAR TO MONTH", "types.IntervalYM +02-03"},
		{"INTERVAL '-5-10' YEAR TO MONTH", "types.IntervalYM -05-10"},
		{"INTERVAL '2 12:23:34.456' DAY TO SECOND", "time.Duration 60h23m34.456s"},
		{"INTERVAL '-0 10:20:30.456789' DAY TO SECOND", "time.Duration -10h20m30.456789s"},
	}
	for _, test := range tests {
		var v any
		if err := db.QueryRowContext(t.Context(), "SELECT "+test.expr+" FROM dual").Scan(&v); err != nil {
			t.Errorf("%s: %v", test.expr, err)
			continue
		}
		if got := fmt.Sprintf("%T %v", v, v); got != test.want {
			t.Errorf("%s: got %s, want %s", test.expr, got, test.want)
		}
	}
}

// TestNumberIntoDestinations checks that a decimal scans into each kind of
// destination through RowsColumnScanner.
func TestNumberIntoDestinations(t *testing.T) {
	db := openLive(t)
	ctx := t.Context()
	const big = "12345678901234567890.123456789"
	q := "SELECT CAST(" + big + " AS NUMBER(38,9)) FROM dual"
	var s string
	if err := db.QueryRowContext(ctx, q).Scan(&s); err != nil || s != big {
		t.Errorf("*string: got %q, %v", s, err)
	}
	var d apd.Decimal
	if err := db.QueryRowContext(ctx, q).Scan(&d); err != nil || d.Text('f') != big {
		t.Errorf("*apd.Decimal: got %v, %v", &d, err)
	}
	var f float64
	if err := db.QueryRowContext(ctx, "SELECT 1.25 FROM dual").Scan(&f); err != nil || f != 1.25 {
		t.Errorf("*float64: got %v, %v", f, err)
	}
	var n int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM dual").Scan(&n); err != nil || n != 1 {
		t.Errorf("*int64 from COUNT(*): got %v, %v", n, err)
	}
	var nd sql.Null[apd.Decimal]
	if err := db.QueryRowContext(ctx, "SELECT CAST(NULL AS NUMBER) FROM dual").Scan(&nd); err != nil || nd.Valid {
		t.Errorf("sql.Null[apd.Decimal] of NULL: got %+v, %v", nd, err)
	}
	var ni sql.Null[int64]
	if err := db.QueryRowContext(ctx, "SELECT CAST(5 AS NUMBER(5)) FROM dual").Scan(&ni); err != nil || !ni.Valid || ni.V != 5 {
		t.Errorf("sql.Null[int64]: got %+v, %v", ni, err)
	}
}

// TestBindIntervals binds a time.Duration and an IntervalYM and reads them
// back.
func TestBindIntervals(t *testing.T) {
	db := openLive(t)
	ctx := t.Context()
	ds := -(2*24*time.Hour + 3*time.Hour + 4*time.Minute + 5*time.Second + 6*time.Microsecond)
	var got time.Duration
	if err := db.QueryRowContext(ctx, "SELECT CAST(:1 AS INTERVAL DAY(3) TO SECOND(6)) FROM dual", ds).Scan(&got); err != nil || got != ds {
		t.Errorf("day to second: got %v, %v, want %v", got, err, ds)
	}
	ym := oracle.IntervalYM{Years: 7, Months: 11}
	var gotYM oracle.IntervalYM
	if err := db.QueryRowContext(ctx, "SELECT CAST(:1 AS INTERVAL YEAR TO MONTH) FROM dual", ym).Scan(&gotYM); err != nil || gotYM != ym {
		t.Errorf("year to month: got %v, %v, want %v", gotYM, err, ym)
	}
	var text string
	// a bind has the largest precision, so the server prints nine digits of days
	if err := db.QueryRowContext(ctx, "SELECT TO_CHAR(:1) FROM dual", ds).Scan(&text); err != nil {
		t.Errorf("day to second as text: %v", err)
	} else if text != "-000000002 03:04:05.000006000" {
		t.Errorf("day to second as text: got %q", text)
	}
}

// TestNullArguments binds sql.Null[T], valid and not, and reads each back.
func TestNullArguments(t *testing.T) {
	db := openLive(t)
	ctx := t.Context()
	tests := []struct {
		arg  any
		cast string
		want string
	}{
		{sql.Null[int64]{V: 5, Valid: true}, "NUMBER(10)", "int64 5"},
		{sql.Null[int64]{}, "NUMBER(10)", "<nil> <nil>"},
		{sql.Null[string]{V: "x", Valid: true}, "VARCHAR2(10)", "string x"},
		{sql.Null[string]{}, "VARCHAR2(10)", "<nil> <nil>"},
		{sql.NullString{String: "y", Valid: true}, "VARCHAR2(10)", "string y"},
		{sql.Null[time.Duration]{V: 90 * time.Minute, Valid: true}, "INTERVAL DAY TO SECOND", "time.Duration 1h30m0s"},
		{sql.Null[time.Duration]{}, "INTERVAL DAY TO SECOND", "<nil> <nil>"},
		{sql.Null[oracle.IntervalYM]{V: oracle.IntervalYM{Years: 1, Months: 2}, Valid: true}, "INTERVAL YEAR TO MONTH", "types.IntervalYM +01-02"},
		{sql.Null[oracle.IntervalYM]{}, "INTERVAL YEAR TO MONTH", "<nil> <nil>"},
		{sql.Null[oracle.TimestampTZ]{}, "TIMESTAMP WITH TIME ZONE", "<nil> <nil>"},
	}
	for _, test := range tests {
		var v any
		if err := db.QueryRowContext(ctx, "SELECT CAST(:1 AS "+test.cast+") FROM dual", test.arg).Scan(&v); err != nil {
			t.Errorf("%#v: %v", test.arg, err)
			continue
		}
		if got := fmt.Sprintf("%T %v", v, v); got != test.want {
			t.Errorf("%#v: got %s, want %s", test.arg, got, test.want)
		}
	}
}

// TestTimestampTZBindsWithZone checks that a TimestampTZ, and a valid
// sql.Null of one, bind as TIMESTAMP WITH TIME ZONE, which DUMP shows as the
// type 181, and keep the zone.
func TestTimestampTZBindsWithZone(t *testing.T) {
	db := openLive(t)
	ctx := t.Context()
	ts := time.Date(2024, 1, 2, 3, 4, 5, 0, time.FixedZone("", 7*60*60))
	for _, arg := range []any{
		oracle.TimestampTZ(ts),
		sql.Null[oracle.TimestampTZ]{V: oracle.TimestampTZ(ts), Valid: true},
	} {
		var dump string
		if err := db.QueryRowContext(ctx, "SELECT DUMP(:1) FROM dual", arg).Scan(&dump); err != nil {
			t.Errorf("%#v: %v", arg, err)
			continue
		}
		if !strings.HasPrefix(dump, "Typ=181 ") {
			t.Errorf("%#v: bound as %s, want Typ=181", arg, dump)
		}
		var got time.Time
		if err := db.QueryRowContext(ctx, "SELECT :1 FROM dual", arg).Scan(&got); err != nil {
			t.Errorf("%#v: %v", arg, err)
			continue
		}
		if !got.Equal(ts) {
			t.Errorf("%#v: got %v, want %v", arg, got, ts)
		}
	}
}

// TestNullOutArguments reads OUT arguments of PL/SQL into sql.Null[T] and an
// sql.NullString.
func TestNullOutArguments(t *testing.T) {
	db := openLive(t)
	var (
		n   sql.Null[int64]
		nul sql.Null[int64]
		s   sql.Null[string]
		old sql.NullString
		d   sql.Null[time.Time]
	)
	_, err := db.ExecContext(t.Context(), "BEGIN :1 := 42; :2 := NULL; :3 := 'abc'; :4 := 'def'; :5 := DATE '2024-01-02'; END;",
		oracle.Out{Dest: &n}, oracle.Out{Dest: &nul}, oracle.Out{Dest: &s, Size: 10}, oracle.Out{Dest: &old, Size: 10}, oracle.Out{Dest: &d})
	if err != nil {
		t.Fatal(err)
	}
	if !n.Valid || n.V != 42 {
		t.Errorf("sql.Null[int64]: got %+v", n)
	}
	if nul.Valid {
		t.Errorf("sql.Null[int64] of NULL: got %+v", nul)
	}
	if !s.Valid || s.V != "abc" {
		t.Errorf("sql.Null[string]: got %+v", s)
	}
	if !old.Valid || old.String != "def" {
		t.Errorf("sql.NullString: got %+v", old)
	}
	if !d.Valid || !d.V.Equal(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("sql.Null[time.Time]: got %+v", d)
	}
}

// TestJSON reads and binds the JSON type of Oracle 21c and later, through
// OSON and encoding/json/v2.
func TestJSON(t *testing.T) {
	db := openLive(t)
	if serverRelease(t, db) < 21 {
		// 18c reads JSON('...') as a function that returns NULL
		t.Skip("the JSON type needs 21c")
	}
	ctx := t.Context()
	const doc = `{"a":1,"b":[1,2.5,"x"],"big":12345678901234567890.12,"n":null}`
	var v any
	if err := db.QueryRowContext(ctx, "SELECT JSON('"+doc+"') FROM dual").Scan(&v); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%T %s", v, v); got != "jsontext.Value "+doc {
		t.Errorf("*any: got %s", got)
	}
	var s string
	if err := db.QueryRowContext(ctx, "SELECT JSON('"+doc+"') FROM dual").Scan(&s); err != nil || s != doc {
		t.Errorf("*string: got %s, %v", s, err)
	}
	var m map[string]any
	if err := db.QueryRowContext(ctx, "SELECT JSON('"+doc+"') FROM dual").Scan(&m); err != nil || m["a"] != float64(1) {
		t.Errorf("map: got %v, %v", m, err)
	}
	type rec struct {
		Name  string   `json:"name"`
		Tags  []string `json:"tags"`
		Count int      `json:"count"`
	}
	in := rec{Name: "x", Tags: []string{"a", "b"}, Count: 3}
	js, err := oracle.NewJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	// a table and not SELECT CAST(:1 AS JSON) FROM dual, which fails inside
	// 21c with ORA-00600 and works on 26ai. A JSON column is a SecureFile,
	// which the tablespace SYSTEM of 21c XE cannot hold, so it goes in USERS.
	const table = "xo_oracle_json"
	if _, err := db.ExecContext(ctx, "BEGIN EXECUTE IMMEDIATE 'DROP TABLE "+table+" PURGE'; EXCEPTION WHEN OTHERS THEN NULL; END;"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE "+table+" (id NUMBER(10), j JSON) TABLESPACE users"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.WithoutCancel(ctx), "DROP TABLE "+table+" PURGE")
	})
	if _, err := db.ExecContext(ctx, "INSERT INTO "+table+" (id, j) VALUES (1, :1)", js); err != nil {
		t.Fatalf("binding a struct as JSON: %v", err)
	}
	var out rec
	if err := db.QueryRowContext(ctx, "SELECT j FROM "+table+" WHERE id = 1").Scan(&out); err != nil {
		t.Fatalf("reading the struct back: %v", err)
	}
	if out.Name != in.Name || out.Count != in.Count || strings.Join(out.Tags, ",") != "a,b" {
		t.Errorf("struct round trip: got %+v, want %+v", out, in)
	}
	if err := db.QueryRowContext(ctx, "SELECT JSON_VALUE(JSON('"+doc+"'), '$.big' RETURNING NUMBER) FROM dual").Scan(&s); err == nil && s != "12345678901234567890.12" {
		t.Errorf("a NUMBER in JSON lost digits: %s", s)
	}
	var raw jsontext.Value
	if err := db.QueryRowContext(ctx, "SELECT JSON('"+doc+"') FROM dual").Scan(&raw); err != nil || !strings.Contains(string(raw), "12345678901234567890.12") {
		t.Errorf("jsontext.Value: got %s, %v", raw, err)
	}
}

// TestTimeFraction checks that a time.Time keeps its fraction of a second in
// a TIMESTAMP, and that a DATE, which holds none, takes the whole seconds.
// go-ora bound every time.Time as a DATE.
func TestTimeFraction(t *testing.T) {
	db := openLive(t)
	ctx := t.Context()
	const table = "xo_oracle_time"
	if _, err := db.ExecContext(ctx, "BEGIN EXECUTE IMMEDIATE 'DROP TABLE "+table+" PURGE'; EXCEPTION WHEN OTHERS THEN NULL; END;"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE "+table+" (d DATE, ts TIMESTAMP(9))"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DROP TABLE " + table + " PURGE") })
	v := time.Date(2020, 1, 2, 3, 4, 5, 987_654_321, time.UTC)
	if _, err := db.ExecContext(ctx, "INSERT INTO "+table+" VALUES (:1, :2)", v, v); err != nil {
		t.Fatal(err)
	}
	var d, ts time.Time
	if err := db.QueryRowContext(ctx, "SELECT d, ts FROM "+table).Scan(&d, &ts); err != nil {
		t.Fatal(err)
	}
	if want := v.Truncate(time.Second); !d.Equal(want) {
		t.Errorf("DATE: got %v, want %v", d, want)
	}
	if !ts.Equal(v) {
		t.Errorf("TIMESTAMP(9): got %v, want %v", ts, v)
	}
	// a value with no fraction binds as a DATE, and finds the row
	var n int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table+" WHERE d = :1", v.Truncate(time.Second)).Scan(&n); err != nil || n != 1 {
		t.Errorf("a DATE bind finds %d rows, %v", n, err)
	}
}

// serverRelease returns the major release of the server, such as 11 or 23.
func serverRelease(t *testing.T, db *sql.DB) int {
	t.Helper()
	var version int
	err := db.QueryRowContext(t.Context(), `SELECT TO_NUMBER(REGEXP_SUBSTR(version, '^[0-9]+'))
		FROM product_component_version WHERE product LIKE 'Oracle%' AND ROWNUM = 1`).Scan(&version)
	if err != nil {
		t.Fatalf("reading the release: %v", err)
	}
	return version
}

// createTable drops the table name and creates it with the columns cols, in
// the tablespace USERS, and drops it when the test ends. The test user is
// SYSTEM, and its tablespace holds no SecureFile LOB on 21c and no VECTOR.
func createTable(t *testing.T, db *sql.DB, name, cols string) {
	t.Helper()
	ctx := t.Context()
	if _, err := db.ExecContext(ctx, "BEGIN EXECUTE IMMEDIATE 'DROP TABLE "+name+" PURGE'; EXCEPTION WHEN OTHERS THEN NULL; END;"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE "+name+" ("+cols+") TABLESPACE USERS"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DROP TABLE " + name + " PURGE") })
}

// TestStrings binds text with characters outside ASCII and outside the BMP,
// and text longer than a VARCHAR2, and reads it back. go-ora held these in
// emoji_test.go and long_input_test.go.
func TestStrings(t *testing.T) {
	db := openLive(t)
	ctx := t.Context()
	createTable(t, db, "xo_oracle_strings", "id NUMBER, v VARCHAR2(400 CHAR), nv NVARCHAR2(400), c CLOB, nc NCLOB")
	long := strings.Repeat("한😀a", 40_000)
	tests := []string{"", "ascii", "é ñ ü", "漢字 かな 한글", "😀 🎉 𝄞", strings.Repeat("😀", 100)}
	for i, s := range tests {
		if _, err := db.ExecContext(ctx, "INSERT INTO xo_oracle_strings VALUES (:1, :2, :3, :4, :5)", i, s, s, s+long, s+long); err != nil {
			t.Fatalf("%q: %v", s, err)
		}
	}
	for i, s := range tests {
		var v, nv, c, nc sql.NullString
		err := db.QueryRowContext(ctx, "SELECT v, nv, c, nc FROM xo_oracle_strings WHERE id = :1", i).Scan(&v, &nv, &c, &nc)
		if err != nil {
			t.Fatalf("%q: %v", s, err)
		}
		// Oracle stores an empty string as NULL
		if v.String != s || nv.String != s {
			t.Errorf("%q: got %q and %q", s, v.String, nv.String)
		}
		if c.String != s+long || nc.String != s+long {
			t.Errorf("%q: the CLOB has %d bytes and the NCLOB %d, want %d", s, len(c.String), len(nc.String), len(s+long))
		}
	}
}

// TestBlob binds a BLOB of a megabyte as a []byte and reads it back. go-ora
// held this in lob_test.go.
func TestBlob(t *testing.T) {
	db := openLive(t)
	ctx := t.Context()
	createTable(t, db, "xo_oracle_blob", "id NUMBER, b BLOB")
	want := make([]byte, 1<<20)
	for i := range want {
		want[i] = byte(i * 7)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO xo_oracle_blob VALUES (1, :1)", want); err != nil {
		t.Fatal(err)
	}
	var got []byte
	if err := db.QueryRowContext(ctx, "SELECT b FROM xo_oracle_blob").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("got %d bytes, want %d", len(got), len(want))
	}
}

// TestBoolean binds a bool into a BOOLEAN of 23ai and reads it back, and
// into a NUMBER before 23ai, as 1 or 0. go-ora held this in
// bool_type_test.go, and bound no bool.
func TestBoolean(t *testing.T) {
	db := openLive(t)
	ctx := t.Context()
	if serverRelease(t, db) < 23 {
		var n, m int
		if err := db.QueryRowContext(ctx, "SELECT :1, :2 FROM dual", true, false).Scan(&n, &m); err != nil || n != 1 || m != 0 {
			t.Errorf("true and false as NUMBER: got %d and %d, %v", n, m, err)
		}
		return
	}
	createTable(t, db, "xo_oracle_boolean", "id NUMBER, b BOOLEAN")
	for i, v := range []any{true, false, nil} {
		if _, err := db.ExecContext(ctx, "INSERT INTO xo_oracle_boolean VALUES (:1, :2)", i, v); err != nil {
			t.Fatalf("%v: %v", v, err)
		}
	}
	for i, want := range []sql.Null[bool]{{V: true, Valid: true}, {V: false, Valid: true}, {}} {
		var got sql.Null[bool]
		if err := db.QueryRowContext(ctx, "SELECT b FROM xo_oracle_boolean WHERE id = :1", i).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("row %d: got %v, want %v", i, got, want)
		}
	}
	var b any
	if err := db.QueryRowContext(ctx, "SELECT b FROM xo_oracle_boolean WHERE id = 0").Scan(&b); err != nil || b != true {
		t.Errorf("into *any: got %#v, %v", b, err)
	}
}

// TestVector binds a VECTOR of 23ai and reads it back. go-ora held this in
// vector_test.go.
func TestVector(t *testing.T) {
	db := openLive(t)
	if serverRelease(t, db) < 23 {
		t.Skip("VECTOR needs 23ai")
	}
	ctx := t.Context()
	createTable(t, db, "xo_oracle_vector", "id NUMBER, f32 VECTOR(3, FLOAT32), f64 VECTOR(3, FLOAT64), i8 VECTOR(3, INT8)")
	f32, f64, i8 := []float32{1.5, -2.25, 3}, []float64{1.5, -2.25, 3}, []uint8{1, 2, 3}
	args := []any{}
	for _, v := range []any{f32, f64, i8} {
		vec, err := oracle.CreateVector(v)
		if err != nil {
			t.Fatal(err)
		}
		args = append(args, vec)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO xo_oracle_vector VALUES (1, :1, :2, :3)", args...); err != nil {
		t.Fatal(err)
	}
	var a, b, c oracle.Vector
	if err := db.QueryRowContext(ctx, "SELECT f32, f64, i8 FROM xo_oracle_vector").Scan(&a, &b, &c); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		vec  *oracle.Vector
		want any
	}{{&a, f32}, {&b, f64}, {&c, i8}} {
		got, err := test.vec.Value()
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(got) != fmt.Sprint(test.want) {
			t.Errorf("got %v, want %v", got, test.want)
		}
	}
}

// TestLongColumn binds text longer than a VARCHAR2 into a LONG. Before 12c
// a string with characters outside ASCII binds as a temporary CLOB, so that
// a CLOB column stores it whole, and a LONG column refuses it.
func TestLongColumn(t *testing.T) {
	db := openLive(t)
	ctx := t.Context()
	createTable(t, db, "xo_oracle_long", "id NUMBER, l LONG")
	old := serverRelease(t, db) < 12
	for i, s := range []string{strings.Repeat("a", 0x5000), strings.Repeat("é", 0x3000)} {
		_, err := db.ExecContext(ctx, "INSERT INTO xo_oracle_long VALUES (:1, :2)", i, s)
		if old && i == 1 {
			if err == nil {
				t.Error("11g stores text outside ASCII in a LONG, which it garbled")
			}
			continue
		}
		if err != nil {
			t.Fatalf("%d: %v", i, err)
		}
		var got string
		if err := db.QueryRowContext(ctx, "SELECT l FROM xo_oracle_long WHERE id = :1", i).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != s {
			t.Errorf("%d: got %d bytes, want %d", i, len(got), len(s))
		}
	}
}
