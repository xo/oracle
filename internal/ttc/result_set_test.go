package ttc

import (
	"math"
	"reflect"
	"testing"

	"github.com/xo/oracle/internal/types"
)

// TestResultSetColumnsNilSafe verifies that Columns() does not panic when the
// ResultSet is uninitialized (cols == nil). This guards against the nil-pointer
// panic reported in issue #691, where a connection reset during _query() could
// yield a Rows with an uninitialized ResultSet.
func TestResultSetColumnsNilSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Columns() panicked on nil cols: %v", r)
		}
	}()
	rs := ResultSet{}
	if got := rs.Columns(); got != nil {
		t.Errorf("expected nil columns for uninitialized ResultSet, got %v", got)

	}
}

// TestColumnTypeScanTypeInlineLOB verifies that ColumnTypeScanType returns
// the correct Go type for columns whose DataType was mutated to LongVarChar
// or LongRaw by writeDefine() when LOB mode is INLINE. This is issue #719.
func TestColumnTypeScanTypeInlineLOB(t *testing.T) {
	cases := []struct {
		name     string
		dataType types.TypeID
		want     reflect.Type
	}{
		{"CLOB_inline", types.LongVarChar, reflect.TypeFor[string]()},
		{"BLOB_inline", types.LongRaw, reflect.TypeFor[[]byte]()},
		{"LongVarRaw", types.LongVarRaw, reflect.TypeFor[[]byte]()},
		{"CLOB_locator", types.OCIClobLocator, reflect.TypeFor[string]()},
		{"BLOB_locator", types.OCIBlobLocator, reflect.TypeFor[[]byte]()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			col := ParameterInfo{
				DataType: c.dataType}
			cols := &[]ParameterInfo{col}
			rs := ResultSet{cols: cols}
			got := rs.ColumnTypeScanType(0)
			if got != c.want {
				t.Errorf("ColumnTypeScanType(%v) = %v, want %v", c.dataType, got, c.want)
			}
		})
	}
}

// TestColumnTypeLength checks the length that each type gives. A CLOB and a
// BLOB of an inline define have the data type of a LONG, and still give the
// length of a LOB.
func TestColumnTypeLength(t *testing.T) {
	cases := []struct {
		name       string
		col        ParameterInfo
		want       int64
		wantLength bool
	}{
		{"VARCHAR2", ParameterInfo{DataType: types.NCHAR, databaseType: "VARCHAR2", MaxLen: 40, MaxCharLen: 10}, 10, true},
		{"NCHAR", ParameterInfo{DataType: types.CHAR, databaseType: "NCHAR", MaxLen: 10, MaxCharLen: 5}, 5, true},
		{"CHAR_no_char_length", ParameterInfo{DataType: types.CHAR, databaseType: "CHAR", MaxLen: 3}, 3, true},
		{"RAW", ParameterInfo{DataType: types.RAW, databaseType: "RAW", MaxLen: 16}, 16, true},
		{"CLOB_inline", ParameterInfo{DataType: types.LongVarChar, databaseType: "CLOB"}, math.MaxInt64, true},
		{"BLOB_inline", ParameterInfo{DataType: types.LongRaw, databaseType: "BLOB"}, math.MaxInt64, true},
		{"LONG", ParameterInfo{DataType: types.LONG, databaseType: "LONG"}, math.MaxInt64, true},
		{"NUMBER", ParameterInfo{DataType: types.NUMBER, databaseType: "NUMBER", MaxLen: 22}, 0, false},
		{"DATE", ParameterInfo{DataType: types.DATE, databaseType: "DATE", MaxLen: 11}, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rs := ResultSet{cols: &[]ParameterInfo{c.col}}
			got, ok := rs.ColumnTypeLength(0)
			if got != c.want || ok != c.wantLength {
				t.Errorf("got %d, %t, want %d, %t", got, ok, c.want, c.wantLength)
			}
		})
	}
}
