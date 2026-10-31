package ttc

import (
	"database/sql"
	"database/sql/driver"
	"testing"

	"github.com/cockroachdb/apd/v3"

	oraTypes "github.com/xo/oracle/internal/types"
)

// unconstrained is the precision and the scale that the driver keeps for a
// NUMBER with neither, such as the result of COUNT(*).
const unconstrainedPrecision, unconstrainedScale = 38, 0xFF

func TestNumberValue(t *testing.T) {
	tests := []struct {
		precision, scale uint8
		in               string
		want             any
	}{
		{10, 0, "42", int64(42)},
		{18, 0, "-999999999999999999", int64(-999999999999999999)},
		{1, 0, "7", int64(7)},
		{19, 0, "1234567890123456789", "1234567890123456789"},
		{10, 2, "1.5", "1.5"},
		{unconstrainedPrecision, unconstrainedScale, "1", "1"},
		{unconstrainedPrecision, unconstrainedScale, "0.1", "0.1"},
		{38, 0, "12345678901234567890123456789012345678", "12345678901234567890123456789012345678"},
		{unconstrainedPrecision, unconstrainedScale, "1.", "1"},
	}
	for _, test := range tests {
		got, err := numberValue(test.precision, test.scale, test.in)
		if err != nil {
			t.Errorf("NUMBER(%d,%d) %q: %v", test.precision, test.scale, test.in, err)
			continue
		}
		switch want := test.want.(type) {
		case int64:
			if got != want {
				t.Errorf("NUMBER(%d,%d) %q: got %T(%v), want int64(%d)", test.precision, test.scale, test.in, got, got, want)
			}
		case string:
			d, ok := got.(*apd.Decimal)
			if !ok || d.Text('f') != want {
				t.Errorf("NUMBER(%d,%d) %q: got %T(%v), want *apd.Decimal(%s)", test.precision, test.scale, test.in, got, got, want)
			}
		}
	}
	if v, err := numberValue(10, 0, nil); err != nil || v != nil {
		t.Errorf("a NULL gives %v, %v, want nil", v, err)
	}
}

func TestColumnValueBinaryFloat(t *testing.T) {
	col := &ParameterInfo{}
	col.DataType = oraTypes.IBFLOAT
	got, err := columnValue(col, float32(1.5))
	if err != nil || got != 1.5 {
		t.Errorf("got %T(%v), %v, want float64(1.5)", got, got, err)
	}
	got, err = columnValue(col, float32(134.45))
	if err != nil || got != 134.45 {
		t.Errorf("got %T(%v), %v, want float64(134.45)", got, got, err)
	}
}

func TestAssign(t *testing.T) {
	d, _, _ := apd.NewFromString("12345678901234567890.5")
	var a any
	if err := assign(driver.ScanContext{}, &a, d); err != nil || a != d {
		t.Errorf("*any: got %v, %v", a, err)
	}
	var dec apd.Decimal
	if err := assign(driver.ScanContext{}, &dec, d); err != nil || dec.Cmp(d) != 0 {
		t.Errorf("*apd.Decimal: got %v, %v", &dec, err)
	}
	var s string
	if err := assign(driver.ScanContext{}, &s, d); err != nil || s != "12345678901234567890.5" {
		t.Errorf("*string: got %q, %v", s, err)
	}
	var f float64
	small, _, _ := apd.NewFromString("1.5")
	if err := assign(driver.ScanContext{}, &f, small); err != nil || f != 1.5 {
		t.Errorf("*float64: got %v, %v", f, err)
	}
	var n int64
	one, _, _ := apd.NewFromString("1")
	if err := assign(driver.ScanContext{}, &n, one); err != nil || n != 1 {
		t.Errorf("*int64: got %v, %v", n, err)
	}
	var nd sql.Null[apd.Decimal]
	if err := assign(driver.ScanContext{}, &nd, d); err != nil || !nd.Valid || nd.V.Cmp(d) != 0 {
		t.Errorf("sql.Null[apd.Decimal]: got %+v, %v", nd, err)
	}
	if err := assign(driver.ScanContext{}, &nd, nil); err != nil || nd.Valid {
		t.Errorf("sql.Null[apd.Decimal] of NULL: got %+v, %v", nd, err)
	}
	var ns string
	if err := assign(driver.ScanContext{}, &ns, nil); err == nil {
		t.Error("a NULL in a *string gives no error, and database/sql gives one")
	}
}
