package integration

import (
	"bytes"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/xo/oracle"
)

// echo sends in through a PL/SQL variable of the type kind, and reads it
// back as an OUT and as an IN OUT, into a T, and then reads a NULL of kind
// into a sql.Null[T]. size is the size of the OUT, or 0.
func echo[T any](t *testing.T, db *sql.DB, kind string, in any, size int, want T, same func(a, b T) bool) {
	t.Helper()
	ctx := t.Context()
	var out T
	q := fmt.Sprintf("DECLARE v %s; BEGIN v := :1; :2 := v; END;", kind)
	if _, err := db.ExecContext(ctx, q, in, oracle.Out{Dest: &out, Size: size}); err != nil {
		t.Errorf("%s OUT: %v", kind, err)
	} else if !same(out, want) {
		t.Errorf("%s OUT: got %v, want %v", kind, out, want)
	}
	inOut, ok := in.(T)
	if ok {
		q = fmt.Sprintf("DECLARE v %s; BEGIN v := :1; :1 := v; END;", kind)
		if _, err := db.ExecContext(ctx, q, oracle.Out{Dest: &inOut, Size: size, In: true}); err != nil {
			t.Errorf("%s IN OUT: %v", kind, err)
		} else if !same(inOut, want) {
			t.Errorf("%s IN OUT: got %v, want %v", kind, inOut, want)
		}
	}
	null := sql.Null[T]{Valid: true}
	q = fmt.Sprintf("DECLARE v %s; BEGIN v := NULL; :1 := v; END;", kind)
	if _, err := db.ExecContext(ctx, q, oracle.Out{Dest: &null, Size: size}); err != nil {
		t.Errorf("%s NULL: %v", kind, err)
	} else if null.Valid {
		t.Errorf("%s NULL: got %v", kind, null.V)
	}
}

// equal is == as a func for echo.
func equal[T comparable](a, b T) bool {
	return a == b
}

// TestVarTypes sends a value of each type through a PL/SQL variable of that
// type, as an OUT and as an IN OUT, and a NULL of each. It holds the types of
// tests/cursor/test_2000_var.py of python-oracledb (2000 to 2026), whose
// cases set and read a variable of python-oracledb itself, here as a round
// trip through the server.
func TestVarTypes(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var release int
	if err := db.QueryRowContext(t.Context(), `SELECT TO_NUMBER(REGEXP_SUBSTR(version, '^[0-9]+'))
		FROM product_component_version WHERE product LIKE 'Oracle%' AND ROWNUM = 1`).Scan(&release); err != nil {
		t.Fatal(err)
	}
	ts := time.Date(2017, 5, 9, 9, 41, 13, 123456000, time.UTC)
	sameTime := func(a, b time.Time) bool { return a.Equal(b) }
	// the longest LONG of PL/SQL is 32,760 bytes. 11g holds an OUT string
	// to 32,767 bytes divided by the bytes of a character, 8,191 on
	// AL32UTF8, and 18c and later to 32,767 characters
	long := strings.Repeat("A long value. ", 2300)
	if release < 12 {
		long = long[:8000]
	}

	t.Run("NUMBER", func(t *testing.T) {
		echo(t, db, "NUMBER", 5, 0, int64(5), equal)
		echo(t, db, "NUMBER", 3.5, 0, 3.5, equal)
		echo(t, db, "NUMBER", "24.8", 0, "24.8", equal)
	})
	t.Run("BINARY_INTEGER", func(t *testing.T) {
		echo(t, db, "BINARY_INTEGER", 5, 0, int64(5), equal)
		echo(t, db, "PLS_INTEGER", -7, 0, int64(-7), equal)
	})
	t.Run("VARCHAR2", func(t *testing.T) {
		echo(t, db, "VARCHAR2(100)", "A VARCHAR string", 100, "A VARCHAR string", equal)
	})
	t.Run("NVARCHAR2", func(t *testing.T) {
		echo(t, db, "NVARCHAR2(100)", oracle.NVarChar("A NVARCHAR string あ"), 100, oracle.NVarChar("A NVARCHAR string あ"), equal)
	})
	t.Run("CHAR", func(t *testing.T) {
		echo(t, db, "CHAR(20)", "A CHAR", 20, "A CHAR"+strings.Repeat(" ", 14), equal)
	})
	t.Run("NCHAR", func(t *testing.T) {
		echo(t, db, "NCHAR(20)", oracle.NVarChar("An NCHAR あ"), 20, oracle.NVarChar("An NCHAR あ"+strings.Repeat(" ", 10)), equal)
	})
	t.Run("LONG", func(t *testing.T) {
		echo(t, db, "LONG", long, 32767, long, equal)
	})
	t.Run("RAW", func(t *testing.T) {
		// a NULL RAW, LONG RAW or BLOB read as a valid empty []byte, OUT
		// and in a row, where go-ora returned a nil []byte in an any
		echo(t, db, "RAW(100)", []byte("A RAW value"), 100, []byte("A RAW value"), bytes.Equal)
		var null sql.Null[[]byte]
		if err := db.QueryRowContext(t.Context(), "SELECT CAST(NULL AS RAW(10)) FROM dual").Scan(&null); err != nil || null.Valid {
			t.Errorf("a NULL RAW column: got %+v, %v", null, err)
		}
	})
	t.Run("LONG RAW", func(t *testing.T) {
		echo(t, db, "LONG RAW", []byte(long), 32767, []byte(long), bytes.Equal)
	})
	t.Run("DATE", func(t *testing.T) {
		echo(t, db, "DATE", ts.Truncate(time.Second), 0, ts.Truncate(time.Second), sameTime)
	})
	t.Run("TIMESTAMP", func(t *testing.T) {
		echo(t, db, "TIMESTAMP", ts, 0, ts, sameTime)
	})
	t.Run("TIMESTAMP WITH TIME ZONE", func(t *testing.T) {
		// a time.Time binds with no zone, and a TimestampTZ with its zone
		zoned := oracle.TimestampTZ(ts.In(time.FixedZone("", 5*3600+1800)))
		echo(t, db, "TIMESTAMP WITH TIME ZONE", zoned, 0, zoned, func(a, b oracle.TimestampTZ) bool {
			_, ao := time.Time(a).Zone()
			_, bo := time.Time(b).Zone()
			return time.Time(a).Equal(time.Time(b)) && ao == bo
		})
	})
	t.Run("TIMESTAMP WITH LOCAL TIME ZONE", func(t *testing.T) {
		echo(t, db, "TIMESTAMP WITH LOCAL TIME ZONE", ts, 0, ts, sameTime)
	})
	t.Run("CLOB", func(t *testing.T) {
		echo(t, db, "CLOB", long, 32767, long, equal)
	})
	t.Run("NCLOB", func(t *testing.T) {
		// an NCLOB becomes a string through AL16UTF16, whose 32,767 bytes
		// hold 16,383 characters
		nclob := "An NCLOB あ " + long[:min(len(long), 16000)]
		echo(t, db, "NCLOB", nclob, 32767, nclob, equal)
	})
	t.Run("BLOB", func(t *testing.T) {
		echo(t, db, "BLOB", []byte(long), 32767, []byte(long), bytes.Equal)
	})
	t.Run("BINARY_FLOAT", func(t *testing.T) {
		echo(t, db, "BINARY_FLOAT", float32(-1.25), 0, float32(-1.25), equal)
	})
	t.Run("BINARY_DOUBLE", func(t *testing.T) {
		echo(t, db, "BINARY_DOUBLE", 1.0/3, 0, 1.0/3, equal)
	})
	t.Run("BOOLEAN", func(t *testing.T) {
		if release < 12 {
			t.Skipf("needs release 12, and the server is %d", release)
		}
		// 2400 to 2408 of tests/data_types/test_2400_boolean.py. A bool
		// binds as a BOOLEAN in PL/SQL from 12.1, where go-ora sent a NUMBER,
		// which PL/SQL refused with PLS-00382
		echo(t, db, "BOOLEAN", true, 0, true, equal)
		echo(t, db, "BOOLEAN", false, 0, false, equal)
		var text string
		for _, test := range []struct {
			in   any
			want string
		}{{true, "TRUE"}, {false, "FALSE"}, {sql.Null[bool]{}, "NULL"}} {
			if _, err := db.ExecContext(t.Context(), `DECLARE v BOOLEAN := :1; BEGIN
				:2 := CASE WHEN v IS NULL THEN 'NULL' WHEN v THEN 'TRUE' ELSE 'FALSE' END; END;`,
				test.in, oracle.Out{Dest: &text, Size: 10}); err != nil || text != test.want {
				t.Errorf("%v IN: got %q, %v", test.in, text, err)
			}
		}
	})
	t.Run("INTERVAL DAY TO SECOND", func(t *testing.T) {
		d := -(133*24*time.Hour + 2*time.Hour + 5*time.Minute + 8*time.Second + 123*time.Millisecond)
		echo(t, db, "INTERVAL DAY(3) TO SECOND(3)", d, 0, d, equal)
	})
	t.Run("INTERVAL YEAR TO MONTH", func(t *testing.T) {
		iv := oracle.IntervalYM{Years: -12, Months: -5}
		echo(t, db, "INTERVAL YEAR TO MONTH", iv, 0, iv, equal)
	})
	t.Run("ROWID", func(t *testing.T) {
		var rowid string
		if err := db.QueryRowContext(t.Context(), "SELECT ROWID FROM dual").Scan(&rowid); err != nil {
			t.Fatal(err)
		}
		echo(t, db, "ROWID", rowid, 18, rowid, equal)
		echo(t, db, "UROWID", rowid, 4000, rowid, equal)
	})
}

// TestArrayPastItsSize fills a PL/SQL array past the size of its OUT, as
// 2105 of tests/cursor/test_2100_arrayvar.py of python-oracledb does with a
// variable of python-oracledb. The server refuses it, and the call fails
// with an error and leaves the connection whole.
func TestArrayPastItsSize(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var out []int64
	_, err = db.ExecContext(t.Context(), `DECLARE
	TYPE numbers IS TABLE OF NUMBER INDEX BY BINARY_INTEGER;
	a numbers;
BEGIN
	FOR i IN 1..4 LOOP
		a(i) := i;
	END LOOP;
	:1 := a;
END;`, oracle.Out{Dest: &out, Size: 3})
	if err == nil {
		t.Errorf("got %v and no error", out)
	} else {
		t.Logf("the error: %v", err)
	}
	var n int
	if err := db.QueryRowContext(t.Context(), "SELECT 1 FROM dual").Scan(&n); err != nil {
		t.Errorf("after: %v", err)
	}
	// an array that fits
	_, err = db.ExecContext(t.Context(), `DECLARE
	TYPE numbers IS TABLE OF NUMBER INDEX BY BINARY_INTEGER;
	a numbers;
BEGIN
	FOR i IN 1..3 LOOP
		a(i) := i;
	END LOOP;
	:1 := a;
END;`, oracle.Out{Dest: &out, Size: 3})
	if err != nil || !slices.Equal(out, []int64{1, 2, 3}) {
		t.Errorf("an array that fits: got %v, %v", out, err)
	}
}
