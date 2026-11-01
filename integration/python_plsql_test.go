package integration

import (
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/xo/oracle"
)

// TestPLSQLArrays binds PL/SQL arrays of NUMBER, DATE, VARCHAR2 and BOOLEAN,
// IN, IN OUT and OUT. It holds 2702 to 2709 and 2713 to 2716 of
// tests/cursor/test_2700_features_12_1.py of python-oracledb, written again
// here with the packages of its schema.
func TestPLSQLArrays(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := execCmd(db, `CREATE OR REPLACE PACKAGE go_plsql_arrays AS
	TYPE numbers IS TABLE OF NUMBER INDEX BY BINARY_INTEGER;
	TYPE dates IS TABLE OF DATE INDEX BY BINARY_INTEGER;
	TYPE strings IS TABLE OF VARCHAR2(100) INDEX BY BINARY_INTEGER;
	FUNCTION number_total (a_start NUMBER, a_array numbers) RETURN NUMBER;
	PROCEDURE number_scale (a_count NUMBER, a_array IN OUT NOCOPY numbers);
	PROCEDURE number_fill (a_count NUMBER, a_array OUT NOCOPY numbers);
	FUNCTION date_total (a_start NUMBER, a_base DATE, a_array dates) RETURN NUMBER;
	PROCEDURE date_week (a_count NUMBER, a_array IN OUT NOCOPY dates);
	PROCEDURE date_fill (a_count NUMBER, a_array OUT NOCOPY dates);
	FUNCTION string_total (a_start NUMBER, a_array strings) RETURN NUMBER;
	PROCEDURE string_convert (a_count NUMBER, a_array IN OUT NOCOPY strings);
	PROCEDURE string_fill (a_count NUMBER, a_array OUT NOCOPY strings);
	PROCEDURE string_holes (a_array OUT NOCOPY strings);
END;`, `CREATE OR REPLACE PACKAGE BODY go_plsql_arrays AS
	FUNCTION number_total (a_start NUMBER, a_array numbers) RETURN NUMBER IS
		v NUMBER := a_start;
	BEGIN
		FOR i IN 1..a_array.COUNT LOOP
			v := v + a_array(i);
		END LOOP;
		RETURN v;
	END;
	PROCEDURE number_scale (a_count NUMBER, a_array IN OUT NOCOPY numbers) IS
	BEGIN
		FOR i IN 1..a_count LOOP
			a_array(i) := a_array(i) * 10;
		END LOOP;
	END;
	PROCEDURE number_fill (a_count NUMBER, a_array OUT NOCOPY numbers) IS
	BEGIN
		FOR i IN 1..a_count LOOP
			a_array(i) := i * 100;
		END LOOP;
	END;
	FUNCTION date_total (a_start NUMBER, a_base DATE, a_array dates) RETURN NUMBER IS
		v NUMBER := a_start;
	BEGIN
		FOR i IN 1..a_array.COUNT LOOP
			v := v + a_array(i) - a_base;
		END LOOP;
		RETURN v;
	END;
	PROCEDURE date_week (a_count NUMBER, a_array IN OUT NOCOPY dates) IS
	BEGIN
		FOR i IN 1..a_count LOOP
			a_array(i) := a_array(i) + 7;
		END LOOP;
	END;
	PROCEDURE date_fill (a_count NUMBER, a_array OUT NOCOPY dates) IS
	BEGIN
		FOR i IN 1..a_count LOOP
			a_array(i) := TO_DATE(20021212, 'YYYYMMDD') + i * 1.2;
		END LOOP;
	END;
	FUNCTION string_total (a_start NUMBER, a_array strings) RETURN NUMBER IS
		v NUMBER := a_start;
	BEGIN
		FOR i IN 1..a_array.COUNT LOOP
			v := v + LENGTH(a_array(i));
		END LOOP;
		RETURN v;
	END;
	PROCEDURE string_convert (a_count NUMBER, a_array IN OUT NOCOPY strings) IS
	BEGIN
		FOR i IN 1..a_count LOOP
			a_array(i) := 'Converted element # ' || TO_CHAR(i) ||
				' originally had length ' || TO_CHAR(LENGTH(a_array(i)));
		END LOOP;
	END;
	PROCEDURE string_fill (a_count NUMBER, a_array OUT NOCOPY strings) IS
	BEGIN
		FOR i IN 1..a_count LOOP
			a_array(i) := 'Test out element # ' || TO_CHAR(i);
		END LOOP;
	END;
	PROCEDURE string_holes (a_array OUT NOCOPY strings) IS
	BEGIN
		a_array(-1048576) := 'First element';
		a_array(-576) := 'Second element';
		a_array(284) := 'Third element';
		a_array(8388608) := 'Fourth element';
	END;
END;`); err != nil {
		t.Fatal(err)
	}
	defer execCmd(db, "DROP PACKAGE go_plsql_arrays")
	ctx := t.Context()
	date := func(y, m, d, h, mi, s int) time.Time {
		return time.Date(y, time.Month(m), d, h, mi, s, 0, time.UTC)
	}

	t.Run("NUMBER", func(t *testing.T) {
		var total float64
		// 2707
		if _, err := db.ExecContext(ctx, "BEGIN :1 := go_plsql_arrays.number_total(:2, :3); END;",
			sql.Out{Dest: &total}, 5, []float64{10, 20, 30, 40, 50}); err != nil || total != 155 {
			t.Errorf("IN: got %v, %v, want 155", total, err)
		}
		// 2708
		inOut := []float64{5, 8, 3, 2}
		if _, err := db.ExecContext(ctx, "BEGIN go_plsql_arrays.number_scale(:1, :2); END;",
			4, oracle.Out{Dest: &inOut, Size: 4, In: true}); err != nil || !slices.Equal(inOut, []float64{50, 80, 30, 20}) {
			t.Errorf("IN OUT: got %v, %v", inOut, err)
		}
		// 2709
		var out []int64
		if _, err := db.ExecContext(ctx, "BEGIN go_plsql_arrays.number_fill(:1, :2); END;",
			3, oracle.Out{Dest: &out, Size: 3}); err != nil || !slices.Equal(out, []int64{100, 200, 300}) {
			t.Errorf("OUT: got %v, %v", out, err)
		}
	})

	t.Run("DATE", func(t *testing.T) {
		var total float64
		// 2704
		in := []time.Time{date(2016, 2, 5, 0, 0, 0), date(2016, 2, 8, 12, 15, 30), date(2016, 2, 12, 5, 44, 30)}
		if _, err := db.ExecContext(ctx, "BEGIN :1 := go_plsql_arrays.date_total(:2, :3, :4); END;",
			sql.Out{Dest: &total}, 2, date(2016, 2, 1, 0, 0, 0), in); err != nil || total != 24.75 {
			t.Errorf("IN: got %v, %v, want 24.75", total, err)
		}
		// 2705
		inOut := []time.Time{date(2016, 1, 1, 0, 0, 0), date(2016, 1, 7, 0, 0, 0), date(2016, 1, 13, 0, 0, 0), date(2016, 1, 19, 0, 0, 0)}
		want := []time.Time{date(2016, 1, 8, 0, 0, 0), date(2016, 1, 14, 0, 0, 0), date(2016, 1, 20, 0, 0, 0), date(2016, 1, 26, 0, 0, 0)}
		if _, err := db.ExecContext(ctx, "BEGIN go_plsql_arrays.date_week(:1, :2); END;",
			4, oracle.Out{Dest: &inOut, Size: 4, In: true}); err != nil || !sameTimes(inOut, want) {
			t.Errorf("IN OUT: got %v, %v", inOut, err)
		}
		// 2706
		var out []time.Time
		want = []time.Time{date(2002, 12, 13, 4, 48, 0), date(2002, 12, 14, 9, 36, 0), date(2002, 12, 15, 14, 24, 0)}
		if _, err := db.ExecContext(ctx, "BEGIN go_plsql_arrays.date_fill(:1, :2); END;",
			3, oracle.Out{Dest: &out, Size: 3}); err != nil || !sameTimes(out, want) {
			t.Errorf("OUT: got %v, %v", out, err)
		}
	})

	t.Run("VARCHAR2", func(t *testing.T) {
		var total int
		// 2713
		if _, err := db.ExecContext(ctx, "BEGIN :1 := go_plsql_arrays.string_total(:2, :3); END;",
			sql.Out{Dest: &total}, 5, []string{"First element", "Second element", "Third element"}); err != nil || total != 45 {
			t.Errorf("IN: got %d, %v, want 45", total, err)
		}
		// 2714
		inOut := []string{"The first element", "The second element", "The third and final element"}
		want := []string{
			"Converted element # 1 originally had length 17",
			"Converted element # 2 originally had length 18",
			"Converted element # 3 originally had length 27",
		}
		if _, err := db.ExecContext(ctx, "BEGIN go_plsql_arrays.string_convert(:1, :2); END;",
			3, oracle.Out{Dest: &inOut, Size: 3, In: true}); err != nil || !slices.Equal(inOut, want) {
			t.Errorf("IN OUT: got %q, %v", inOut, err)
		}
		// 2715
		var out []string
		want = []string{"Test out element # 1", "Test out element # 2", "Test out element # 3", "Test out element # 4"}
		if _, err := db.ExecContext(ctx, "BEGIN go_plsql_arrays.string_fill(:1, :2); END;",
			4, oracle.Out{Dest: &out, Size: 4}); err != nil || !slices.Equal(out, want) {
			t.Errorf("OUT: got %q, %v", out, err)
		}
		// 2716 binds a collection of the package, which holds the holes. A
		// slice has no holes, and the server refuses them for it
		out = nil
		_, err := db.ExecContext(ctx, "BEGIN go_plsql_arrays.string_holes(:1); END;", oracle.Out{Dest: &out, Size: 4})
		requireErr(t, "holes", err, "ORA-06513")
	})
}

// TestPLSQLBooleanArrays binds a PL/SQL array of BOOLEAN, as 2702 and 2703
// of tests/cursor/test_2700_features_12_1.py of python-oracledb do with a
// collection type of the package. The server fails with an internal error on
// a host array of BOOLEAN, ORA-06544 on 26ai and ORA-00600 on 18c, so the
// driver refuses a []bool for PL/SQL with ErrNotSupported. A []bool of a
// batch of DML is not an array of PL/SQL, and binds.
func TestPLSQLBooleanArrays(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	requireRelease(t, db, 12)
	var n int
	_, err = db.ExecContext(t.Context(), `DECLARE
	TYPE booleans IS TABLE OF BOOLEAN INDEX BY BINARY_INTEGER;
	a booleans := :2;
BEGIN
	:1 := a.COUNT;
END;`, sql.Out{Dest: &n}, []bool{true, false})
	if !errors.Is(err, oracle.ErrNotSupported) {
		t.Errorf("IN: got %v, want ErrNotSupported", err)
	}
	var out []bool
	_, err = db.ExecContext(t.Context(), "BEGIN NULL; END;", oracle.Out{Dest: &out, Size: 2})
	if !errors.Is(err, oracle.ErrNotSupported) {
		t.Errorf("OUT: got %v, want ErrNotSupported", err)
	}
	// the connection still serves
	if err := db.QueryRowContext(t.Context(), "SELECT 1 FROM dual").Scan(&n); err != nil {
		t.Error(err)
	}
	t.Run("a batch of DML", func(t *testing.T) {
		requireRelease(t, db, 23)
		db.Exec("DROP TABLE go_plsql_booleans PURGE")
		if err := execCmd(db, "CREATE TABLE go_plsql_booleans (b BOOLEAN)"); err != nil {
			t.Fatal(err)
		}
		defer execCmd(db, "DROP TABLE go_plsql_booleans PURGE")
		if _, err := db.ExecContext(t.Context(), "INSERT INTO go_plsql_booleans VALUES (:1)", []bool{true, false, true}); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM go_plsql_booleans WHERE b").Scan(&n); err != nil || n != 2 {
			t.Errorf("got %d, %v, want 2", n, err)
		}
	})
}

// TestImplicitResults reads the cursors that a PL/SQL block returns with
// DBMS_SQL.RETURN_RESULT, through Rows.NextResultSet, as 2720 and 2725 of
// tests/cursor/test_2700_features_12_1.py of python-oracledb do.
func TestImplicitResults(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	requireRelease(t, db, 12)
	db.Exec("DROP TABLE go_implicit PURGE")
	if err := execCmd(db, "CREATE TABLE go_implicit (int_col NUMBER(9), nullable_col NUMBER(38), number_col NUMBER(9, 2))"); err != nil {
		t.Fatal(err)
	}
	defer execCmd(db, "DROP TABLE go_implicit PURGE")
	for i, v := range []string{"", "", "2924207", "", "59797108943", "", "1222791080775407", "", "25004854810776297743", ""} {
		var nullable any
		if v != "" {
			nullable = v
		}
		if _, err := db.Exec("INSERT INTO go_implicit VALUES (:1, TO_NUMBER(:2), :3)", i+1, nullable, float64(i+1)*1.25); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := db.QueryContext(t.Context(), `DECLARE
	c1 SYS_REFCURSOR;
	c2 SYS_REFCURSOR;
	c3 SYS_REFCURSOR;
BEGIN
	OPEN c1 FOR SELECT nullable_col FROM go_implicit WHERE int_col BETWEEN 3 AND 5 ORDER BY int_col;
	DBMS_SQL.RETURN_RESULT(c1);
	OPEN c2 FOR SELECT nullable_col FROM go_implicit WHERE int_col BETWEEN 7 AND 10 ORDER BY int_col;
	DBMS_SQL.RETURN_RESULT(c2);
	OPEN c3 FOR SELECT number_col, int_col FROM go_implicit WHERE int_col BETWEEN 3 AND 5 ORDER BY int_col;
	DBMS_SQL.RETURN_RESULT(c3);
END;`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var sets [][]string
	for {
		var set []string
		for rows.Next() {
			cols, _ := rows.Columns()
			values := make([]any, len(cols))
			var s sql.Null[string]
			values[0] = &s
			for i := 1; i < len(values); i++ {
				values[i] = new(any)
			}
			if err := rows.Scan(values...); err != nil {
				t.Fatal(err)
			}
			set = append(set, s.V)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		sets = append(sets, set)
		if !rows.NextResultSet() {
			break
		}
	}
	want := [][]string{
		{"2924207", "", "59797108943"},
		{"1222791080775407", "", "25004854810776297743", ""},
		{"3.75", "5", "6.25"},
	}
	if len(sets) != len(want) {
		t.Fatalf("got %d result sets, %q, want %d", len(sets), sets, len(want))
	}
	for i := range want {
		if !slices.Equal(sets[i], want[i]) {
			t.Errorf("result set %d: got %q, want %q", i+1, sets[i], want[i])
		}
	}
}

// sameTimes reports whether a and b hold the same instants.
func sameTimes(a, b []time.Time) bool {
	return slices.EqualFunc(a, b, func(x, y time.Time) bool {
		return x.Equal(y)
	})
}

// plsqlRecord is the record go_plsql_records.rec.
type plsqlRecord struct {
	Number        float64   `udt:"NUMBER_VALUE"`
	String        string    `udt:"STRING_VALUE"`
	Date          time.Time `udt:"DATE_VALUE"`
	Timestamp     time.Time `udt:"TIMESTAMP_VALUE"`
	Boolean       bool      `udt:"BOOLEAN_VALUE"`
	PLSInteger    int       `udt:"PLS_INTEGER_VALUE"`
	BinaryInteger int       `udt:"BINARY_INTEGER_VALUE"`
}

// plsqlInner and plsqlOuter are the records go_plsql_records.inner and
// go_plsql_records.outer, which holds two of the first.
type plsqlInner struct {
	Attr1 int `udt:"ATTR1"`
	Attr2 int `udt:"ATTR2"`
}

type plsqlOuter struct {
	Inner1 plsqlInner `udt:"INNER1"`
	Inner2 plsqlInner `udt:"INNER2"`
}

// plsqlString is the element of go_plsql_records.strings, a VARCHAR2.
type plsqlString string

// TestPLSQLRecords binds a record of a package, IN and OUT, a record that
// holds records, and an index-by table of records, and reads an index-by
// table with holes. It holds 2710 to
// 2712 and 2716 of tests/cursor/test_2700_features_12_1.py of
// python-oracledb, written again here. RegisterType takes a type of a package
// as PKG.TYPE. go-ora looked a type up in ALL_TYPES alone, which holds no
// type of a package.
func TestPLSQLRecords(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	requireRelease(t, db, 18)
	if err := execCmd(db, `CREATE OR REPLACE PACKAGE go_plsql_records AS
	TYPE rec IS RECORD (
		number_value NUMBER,
		string_value VARCHAR2(30),
		date_value DATE,
		timestamp_value TIMESTAMP,
		boolean_value BOOLEAN,
		pls_integer_value PLS_INTEGER,
		binary_integer_value BINARY_INTEGER
	);
	TYPE recs IS TABLE OF rec INDEX BY BINARY_INTEGER;
	TYPE strings IS TABLE OF VARCHAR2(100) INDEX BY BINARY_INTEGER;
	TYPE inner IS RECORD (attr1 NUMBER, attr2 NUMBER);
	TYPE outer IS RECORD (inner1 inner, inner2 inner);
	FUNCTION text (a rec) RETURN VARCHAR2;
	FUNCTION outer_text (a outer) RETURN VARCHAR2;
	PROCEDURE outer_fill (a OUT NOCOPY outer);
	FUNCTION texts (a recs) RETURN VARCHAR2;
	PROCEDURE fill (a OUT NOCOPY rec);
	PROCEDURE holes (a OUT NOCOPY strings);
END;`, `CREATE OR REPLACE PACKAGE BODY go_plsql_records AS
	FUNCTION text (a rec) RETURN VARCHAR2 IS
	BEGIN
		RETURN 'rec(' ||
			NVL(TO_CHAR(a.number_value), 'null') || ', ' ||
			CASE WHEN a.string_value IS NULL THEN 'null' ELSE '''' || a.string_value || '''' END || ', ' ||
			CASE WHEN a.date_value IS NULL THEN 'null' ELSE TO_CHAR(a.date_value, 'YYYY-MM-DD') END || ', ' ||
			CASE WHEN a.timestamp_value IS NULL THEN 'null' ELSE TO_CHAR(a.timestamp_value, 'YYYY-MM-DD HH24:MI:SS') END || ', ' ||
			CASE WHEN a.boolean_value IS NULL THEN 'null' WHEN a.boolean_value THEN 'true' ELSE 'false' END || ', ' ||
			NVL(TO_CHAR(a.pls_integer_value), 'null') || ', ' ||
			NVL(TO_CHAR(a.binary_integer_value), 'null') || ')';
	END;
	FUNCTION texts (a recs) RETURN VARCHAR2 IS
		v VARCHAR2(4000);
		i BINARY_INTEGER := a.FIRST;
	BEGIN
		WHILE i IS NOT NULL LOOP
			IF v IS NOT NULL THEN
				v := v || '; ';
			END IF;
			v := v || i || ': ' || text(a(i));
			i := a.NEXT(i);
		END LOOP;
		RETURN v;
	END;
	PROCEDURE fill (a OUT NOCOPY rec) IS
	BEGIN
		a.number_value := 25;
		a.string_value := 'String in record';
		a.date_value := TO_DATE(20160216, 'YYYYMMDD');
		a.timestamp_value := TO_TIMESTAMP('20160216 18:23:55', 'YYYYMMDD HH24:MI:SS');
		a.boolean_value := TRUE;
		a.pls_integer_value := 45;
		a.binary_integer_value := 10;
	END;
	FUNCTION outer_text (a outer) RETURN VARCHAR2 IS
	BEGIN
		RETURN a.inner1.attr1 || ' ' || a.inner1.attr2 || ' ' || a.inner2.attr1 || ' ' || a.inner2.attr2;
	END;
	PROCEDURE outer_fill (a OUT NOCOPY outer) IS
	BEGIN
		a.inner1.attr1 := 101;
		a.inner1.attr2 := 102;
		a.inner2.attr1 := 201;
		a.inner2.attr2 := 202;
	END;
	PROCEDURE holes (a OUT NOCOPY strings) IS
	BEGIN
		a(-1048576) := 'First element';
		a(-576) := 'Second element';
		a(284) := 'Third element';
		a(8388608) := 'Fourth element';
	END;
END;`); err != nil {
		t.Fatal(err)
	}
	defer execCmd(db, "DROP PACKAGE go_plsql_records")
	if err := oracle.RegisterType(db, "go_plsql_records.rec", "go_plsql_records.recs", plsqlRecord{}); err != nil {
		t.Fatal(err)
	}
	if err := oracle.RegisterType(db, "VARCHAR2", "go_plsql_records.strings", plsqlString("")); err != nil {
		t.Fatal(err)
	}
	if err := oracle.RegisterType(db, "go_plsql_records.inner", "", plsqlInner{}); err != nil {
		t.Fatal(err)
	}
	if err := oracle.RegisterType(db, "go_plsql_records.outer", "", plsqlOuter{}); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	date := func(y, m, d, h, mi, s int) time.Time {
		return time.Date(y, time.Month(m), d, h, mi, s, 0, time.UTC)
	}

	// 2711
	var text string
	in := plsqlRecord{18, "A string in a record", date(2016, 2, 15, 0, 0, 0), date(2016, 2, 12, 14, 25, 36), false, 21, 5}
	if _, err := db.ExecContext(ctx, "BEGIN :1 := go_plsql_records.text(:2); END;", oracle.Out{Dest: &text, Size: 4000}, in); err != nil {
		t.Errorf("IN: %v", err)
	} else if want := "rec(18, 'A string in a record', 2016-02-15, 2016-02-12 14:25:36, false, 21, 5)"; text != want {
		t.Errorf("IN: got %q, want %q", text, want)
	}

	// 2712
	out := plsqlRecord{5, "Test value", time.Now(), time.Now(), false, 23, 9}
	if _, err := db.ExecContext(ctx, "BEGIN go_plsql_records.fill(:1); END;", sql.Out{Dest: &out, In: true}); err != nil {
		t.Errorf("OUT: %v", err)
	} else if want := (plsqlRecord{25, "String in record", date(2016, 2, 16, 0, 0, 0), date(2016, 2, 16, 18, 23, 55), true, 45, 10}); !out.Date.Equal(want.Date) ||
		!out.Timestamp.Equal(want.Timestamp) || out.Number != want.Number || out.String != want.String ||
		out.Boolean != want.Boolean || out.PLSInteger != want.PLSInteger || out.BinaryInteger != want.BinaryInteger {
		t.Errorf("OUT: got %+v, want %+v", out, want)
	}

	// 2710, with keys from 1
	var recs []plsqlRecord
	for i := range 3 {
		recs = append(recs, plsqlRecord{float64(i + 1), fmt.Sprintf("String in record #%d", i+1),
			date(2017, i+1, 1, 0, 0, 0), date(2017, 1, i+1, 0, 0, 0), i%2 == 1, i * 5, i * 2})
	}
	if _, err := db.ExecContext(ctx, "BEGIN :1 := go_plsql_records.texts(:2); END;", oracle.Out{Dest: &text, Size: 4000}, recs); err != nil {
		t.Errorf("an array of records: %v", err)
	} else if want := "1: rec(1, 'String in record #1', 2017-01-01, 2017-01-01 00:00:00, false, 0, 0); " +
		"2: rec(2, 'String in record #2', 2017-02-01, 2017-01-02 00:00:00, true, 5, 2); " +
		"3: rec(3, 'String in record #3', 2017-03-01, 2017-01-03 00:00:00, false, 10, 4)"; text != want {
		t.Errorf("an array of records: got %q, want %q", text, want)
	}

	// a record that holds records of the package
	if _, err := db.ExecContext(ctx, "BEGIN :1 := go_plsql_records.outer_text(:2); END;", oracle.Out{Dest: &text, Size: 4000},
		plsqlOuter{plsqlInner{1, 2}, plsqlInner{3, 4}}); err != nil || text != "1 2 3 4" {
		t.Errorf("an outer record IN: got %q, %v", text, err)
	}
	var outer plsqlOuter
	if _, err := db.ExecContext(ctx, "BEGIN go_plsql_records.outer_fill(:1); END;", sql.Out{Dest: &outer}); err != nil ||
		outer != (plsqlOuter{plsqlInner{101, 102}, plsqlInner{201, 202}}) {
		t.Errorf("an outer record OUT: got %+v, %v", outer, err)
	}

	// a map binds an index-by table with its keys (D33)
	byKey := map[int32]plsqlRecord{-5: recs[0], 0: recs[1], 7: recs[2]}
	if _, err := db.ExecContext(ctx, "BEGIN :1 := go_plsql_records.texts(:2); END;", oracle.Out{Dest: &text, Size: 4000}, byKey); err != nil {
		t.Errorf("a map of records: %v", err)
	} else if want := "-5: rec(1, 'String in record #1', 2017-01-01, 2017-01-01 00:00:00, false, 0, 0); " +
		"0: rec(2, 'String in record #2', 2017-02-01, 2017-01-02 00:00:00, true, 5, 2); " +
		"7: rec(3, 'String in record #3', 2017-03-01, 2017-01-03 00:00:00, false, 10, 4)"; text != want {
		t.Errorf("a map of records: got %q, want %q", text, want)
	}

	// 2716 into a map, which keeps the keys and the holes (D33)
	var holesByKey map[int32]plsqlString
	if _, err := db.ExecContext(ctx, "BEGIN go_plsql_records.holes(:1); END;", sql.Out{Dest: &holesByKey}); err != nil {
		t.Errorf("holes in a map: %v", err)
	} else if want := map[int32]plsqlString{-1048576: "First element", -576: "Second element", 284: "Third element", 8388608: "Fourth element"}; !maps.Equal(holesByKey, want) {
		t.Errorf("holes in a map: got %v, want %v", holesByKey, want)
	}

	// 2716, whose holes a slice drops
	var holes []plsqlString
	if _, err := db.ExecContext(ctx, "BEGIN go_plsql_records.holes(:1); END;", sql.Out{Dest: &holes}); err != nil {
		t.Errorf("holes: %v", err)
	} else if want := []plsqlString{"First element", "Second element", "Third element", "Fourth element"}; !slices.Equal(holes, want) {
		t.Errorf("holes: got %q, want %q", holes, want)
	}
}
