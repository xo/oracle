package integration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/cockroachdb/apd/v3"
)

// TestNumberIntegers creates a table with an integer column of each width
// and inserts 100 rows of random values. It reads the rows back and checks
// that each value is the same.
func TestNumberIntegers(t *testing.T) {
	db := numberDB(t)
	cols := []string{
		"id",
		"data_int", "data_integer", "data_decimal", "data_number", "data_numeric",
		"data_tinyint", "data_smallint", "data_mediumint", "data_bigint",
		"data_int1", "data_int2", "data_int4", "data_int8",
	}
	createNumberTable(t, db, "test_int", cols, []string{
		"NVARCHAR2(8)",
		"INT", "INTEGER", "NUMERIC(38,0)", "NUMBER(38,0)", "DECIMAL(38,0)",
		"NUMERIC(3,0)", "SMALLINT", "DECIMAL(19,0)", "DEC(38,0)",
		"DEC(4,0)", "NUMBER(8,0)", "DECIMAL(16,0)", "NUMERIC(32,0)",
	})
	query := insertQuery("test_int", cols)
	var want [][]any
	for i := 1; i <= 100; i++ {
		v := rand.Int64()
		row := []any{
			fmt.Sprintf("%03d", i),
			int(v%(1<<32)) + 1, int(v%(1<<32)) + 2, int(v%(1<<32)) + 3, int(v%(1<<32)) + 4, int(v%(1<<32)) + 5,
			int8(v%(1<<8)) + 6, int16(v%(1<<16)) + 7, int32(v%(1<<24)) + 8, v - 1,
			int8(v%(1<<8)) + 9, int16(v%(1<<16)) + 10, int32(v%(1<<24)) + 11, v - 2,
		}
		if _, err := db.ExecContext(t.Context(), query, row...); err != nil {
			t.Fatalf("inserting row %d: %v", i, err)
		}
		want = append(want, row)
	}
	got := queryMaps(t, db, "SELECT * FROM test_int ORDER BY id")
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d", len(got), len(want))
	}
	for i, row := range got {
		if row["id"] != want[i][0] {
			t.Fatalf("row %d: got id %#v, want %#v", i, row["id"], want[i][0])
		}
		for j, col := range cols[1:] {
			e := reflect.ValueOf(want[i][j+1]).Int()
			v, err := toInt64(row[col])
			if err != nil || v != e {
				t.Fatalf("row %s, column %s: got %#v (%T), want %d (error: %v)", row["id"], col, row[col], row[col], e, err)
			}
		}
	}
}

// TestNumberMoney creates a table with a column for each scale of money,
// from 2 to 8 decimal places, and inserts 100 rows of random values. It reads
// the rows back and checks each value to the scale of its column.
func TestNumberMoney(t *testing.T) {
	db := numberDB(t)
	cols := []string{"id", "data_money2", "data_money4", "data_money6", "data_money8"}
	createNumberTable(t, db, "test_money", cols, []string{
		"NVARCHAR2(8)", "NUMERIC(24,2)", "DECIMAL(28,4)", "DEC(32,6)", "NUMERIC(36,8)",
	})
	scales := []int{2, 4, 6, 8}
	query := insertQuery("test_money", cols)
	var want [][]any
	for i := 1; i <= 100; i++ {
		small := float64(rand.IntN(65536))
		large := float64(rand.Int32())
		row := []any{
			fmt.Sprintf("%03d", i),
			math.Round(small*1e2) / 1e2,
			math.Round(small*1e4) / 1e4,
			math.Round(large*1e6) / 1e6,
			math.Round(large*1e8) / 1e8,
		}
		if _, err := db.ExecContext(t.Context(), query, row...); err != nil {
			t.Fatalf("inserting row %d: %v", i, err)
		}
		want = append(want, row)
	}
	got := queryMaps(t, db, "SELECT * FROM test_money ORDER BY id")
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d", len(got), len(want))
	}
	for i, row := range got {
		if row["id"] != want[i][0] {
			t.Fatalf("row %d: got id %#v, want %#v", i, row["id"], want[i][0])
		}
		for j, col := range cols[1:] {
			e := want[i][j+1].(float64)
			v, err := toFloat64(row[col])
			es, vs := strconv.FormatFloat(e, 'f', scales[j], 64), strconv.FormatFloat(v, 'f', scales[j], 64)
			if err != nil || vs != es {
				t.Fatalf("row %s, column %s: got %s from %#v (%T), want %s (error: %v)", row["id"], col, vs, row[col], row[col], es, err)
			}
		}
	}
}

// TestNumberReals inserts 100 rows of a value with a fraction into a column
// of each real type and checks each value that it reads back.
func TestNumberReals(t *testing.T) {
	checkReals(t, numberDB(t), 0.123456789)
}

// TestNumberRealsNoFraction inserts 100 rows of a whole number into a column
// of each real type and checks each value that it reads back.
func TestNumberRealsNoFraction(t *testing.T) {
	checkReals(t, numberDB(t), 12345)
}

// checkReals creates a table with a column of each real type and inserts 100
// rows of x, rounded to the precision of each column. It reads the rows back
// and checks each value to the precision of its column.
func checkReals(t *testing.T, db *sql.DB, x float64) {
	t.Helper()
	cols := []string{
		"id",
		"data_float", "data_double", "data_real",
		"data_decimal", "data_number", "data_numeric",
		"data_float32", "data_float64",
		"data_float4", "data_float8",
	}
	createNumberTable(t, db, "test_real", cols, []string{
		"NVARCHAR2(8)",
		"FLOAT", "DOUBLE PRECISION", "REAL",
		"DECIMAL(38,6)", "NUMBER(38,6)", "NUMERIC(38,6)",
		"BINARY_FLOAT", "BINARY_DOUBLE",
		"BINARY_FLOAT", "BINARY_DOUBLE",
	})
	query := insertQuery("test_real", cols)
	var want [][]any
	for i := 1; i <= 100; i++ {
		row := []any{
			fmt.Sprintf("%03d", i),
			math.Round(x*1e6) / 1e6,
			math.Round(x*1e6) / 1e6,
			math.Round(x*1e6) / 1e6,
			math.Round(x*1e6) / 1e6,
			math.Round(x*1e6) / 1e6,
			math.Round(x*1e6) / 1e6,
			float32(math.Round(x*1e4) / 1e4),
			math.Round(x*1e8) / 1e8,
			float32(math.Round(x*1e4) / 1e4),
			math.Round(x*1e8) / 1e8,
		}
		if _, err := db.ExecContext(t.Context(), query, row...); err != nil {
			t.Fatalf("inserting row %d: %v", i, err)
		}
		want = append(want, row)
	}
	got := queryMaps(t, db, "SELECT * FROM test_real ORDER BY id")
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d", len(got), len(want))
	}
	for i, row := range got {
		if row["id"] != want[i][0] {
			t.Fatalf("row %d: got id %#v, want %#v", i, row["id"], want[i][0])
		}
		for j, col := range cols[1:] {
			e := reflect.ValueOf(want[i][j+1]).Float()
			v, err := toFloat64(row[col])
			same := math.Abs(v-e) <= 1e-12*max(1, math.Abs(e))
			if _, ok := want[i][j+1].(float32); ok {
				// a BINARY_FLOAT holds the precision of a float32
				same = float32(v) == float32(e)
			}
			if err != nil || !same {
				t.Fatalf("row %s, column %s: got %#v (%T), want %v (error: %v)", row["id"], col, row[col], row[col], e, err)
			}
		}
	}
}

// numberDB opens the database and closes it when the test ends.
func numberDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}

// createNumberTable drops the table if it exists, and creates it with the
// columns and types, with the first column as the primary key. It drops the
// table when the test ends.
func createNumberTable(t *testing.T, db *sql.DB, name string, cols, types []string) {
	t.Helper()
	_, _ = db.ExecContext(t.Context(), "DROP TABLE "+name)
	var b strings.Builder
	b.WriteString("CREATE TABLE " + name + " (")
	for i, col := range cols {
		b.WriteString(col + " " + types[i] + ",")
	}
	b.WriteString("PRIMARY KEY(" + cols[0] + "))")
	if _, err := db.ExecContext(t.Context(), b.String()); err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	t.Cleanup(func() {
		if _, err := db.ExecContext(context.Background(), "DROP TABLE "+name+" PURGE"); err != nil {
			t.Errorf("dropping %s: %v", name, err)
		}
	})
}

// insertQuery returns an INSERT into the columns of the table, with one
// placeholder for each column.
func insertQuery(table string, cols []string) string {
	params := make([]string, len(cols))
	for i := range params {
		params[i] = ":" + strconv.Itoa(i+1)
	}
	return "INSERT INTO " + table + " (" + strings.Join(cols, ",") + ") VALUES (" + strings.Join(params, ",") + ")"
}

// queryMaps runs the query and returns each row as a map from the lower case
// name of each column to the value that a scan into *any gives.
func queryMaps(t *testing.T, db *sql.DB, query string) []map[string]any {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var res []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		dest := make([]any, len(cols))
		for i := range vals {
			dest[i] = &vals[i]
		}
		if err := rows.Scan(dest...); err != nil {
			t.Fatal(err)
		}
		m := make(map[string]any, len(cols))
		for i, col := range cols {
			m[strings.ToLower(col)] = vals[i]
		}
		res = append(res, m)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return res
}

// toInt64 returns v as an int64 when v is an integer or a *apd.Decimal with
// no fraction.
func toInt64(v any) (int64, error) {
	if d, ok := v.(*apd.Decimal); ok {
		return d.Int64()
	}
	if v == nil {
		return 0, errors.New("value is nil")
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(rv.Uint()), nil
	}
	return 0, fmt.Errorf("value %#v (%T) is not an integer", v, v)
}

// toFloat64 returns v as a float64 when v is a float or a *apd.Decimal.
func toFloat64(v any) (float64, error) {
	if d, ok := v.(*apd.Decimal); ok {
		return d.Float64()
	}
	if v == nil {
		return 0, errors.New("value is nil")
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Float32, reflect.Float64:
		return rv.Float(), nil
	}
	return 0, fmt.Errorf("value %#v (%T) is not a real number", v, v)
}
