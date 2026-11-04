package ttc

import (
	"database/sql/driver"
	"testing"

	"github.com/cockroachdb/apd/v3"
)

// partsDecimal is a decimal of another package. It has the Decompose and the
// Compose of database/sql, and no Scan or Value.
type partsDecimal struct {
	d apd.Decimal
}

func (p *partsDecimal) Decompose(buf []byte) (byte, bool, []byte, int32) {
	return p.d.Decompose(buf)
}

func (p *partsDecimal) Compose(form byte, negative bool, coefficient []byte, exponent int32) error {
	return p.d.Compose(form, negative, coefficient, exponent)
}

// TestDecimalArg checks that a decimal with Decompose binds as an
// *apd.Decimal with every digit, and that a nil one binds as a NULL.
func TestDecimalArg(t *testing.T) {
	in := new(partsDecimal)
	if _, _, err := in.d.SetString("-12345678901234567890.123456789"); err != nil {
		t.Fatal(err)
	}
	v, ok, err := decimalArg(in)
	if d, isDecimal := v.(*apd.Decimal); err != nil || !ok || !isDecimal || d.Cmp(&in.d) != 0 {
		t.Errorf("got %v, %t, %v, want %s", v, ok, err, &in.d)
	}
	v, ok, err = decimalArg((*partsDecimal)(nil))
	if err != nil || !ok || v != nil {
		t.Errorf("nil: got %v, %t, %v, want nil, true", v, ok, err)
	}
	for _, v := range []any{apd.New(1, 0), apd.Decimal{}, "1", 1} {
		if _, ok, err := decimalArg(v); ok || err != nil {
			t.Errorf("%T: got %t, %v, want false", v, ok, err)
		}
	}
	nv := driver.NamedValue{Ordinal: 1, Value: in}
	if err := (&Stmt{}).CheckNamedValue(&nv); err != nil {
		t.Fatal(err)
	}
	if _, ok := nv.Value.(*apd.Decimal); !ok {
		t.Errorf("CheckNamedValue: got %T, want *apd.Decimal", nv.Value)
	}
}

// TestAssignDecimal checks that a decimal with Compose takes a NUMBER, an
// integer, a BINARY_DOUBLE and the text of an OUT with every digit.
func TestAssignDecimal(t *testing.T) {
	big, _, _ := apd.NewFromString("12345678901234567890.123456789")
	tests := []struct {
		src  any
		want string
	}{
		{big, "12345678901234567890.123456789"},
		{int64(-42), "-42"},
		{float64(1.5), "1.5"},
		{"-0.000123", "-0.000123"},
	}
	for _, test := range tests {
		var got partsDecimal
		if err := assign(driver.ScanContext{}, &got, test.src); err != nil || got.d.Text('f') != test.want {
			t.Errorf("%T: got %s, %v, want %s", test.src, got.d.Text('f'), err, test.want)
		}
	}
	var got partsDecimal
	if err := assign(driver.ScanContext{}, &got, nil); err == nil {
		t.Error("a NULL gives no error, and database/sql gives one")
	}
	if err := assign(driver.ScanContext{}, &got, "abc"); err == nil {
		t.Error("text that is no number gives no error")
	}
}
