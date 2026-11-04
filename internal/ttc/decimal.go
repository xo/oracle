package ttc

import (
	"fmt"
	"reflect"

	"github.com/cockroachdb/apd/v3"
)

// decomposer is the method that database/sql calls to read a decimal of any
// package as its parts: a form, a sign, a coefficient and an exponent.
// database/sql does not export it.
type decomposer interface {
	Decompose(buf []byte) (form byte, negative bool, coefficient []byte, exponent int32)
}

// composer is the method that database/sql calls to set a decimal of any
// package from its parts. database/sql does not export it.
type composer interface {
	Compose(form byte, negative bool, coefficient []byte, exponent int32) error
}

// decimalArg returns v as an *apd.Decimal, which binds as a NUMBER, when v
// is a decimal of another package that has Decompose. A nil pointer is a
// NULL. It reports false for any other v, and for an apd.Decimal, which
// binds as it is.
func decimalArg(v any) (any, bool, error) {
	switch v.(type) {
	case *apd.Decimal, apd.Decimal:
		return nil, false, nil
	}
	d, ok := v.(decomposer)
	if !ok {
		return nil, false, nil
	}
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Pointer && rv.IsNil() {
		return nil, true, nil
	}
	dec := new(apd.Decimal)
	if err := dec.Compose(d.Decompose(nil)); err != nil {
		return nil, true, fmt.Errorf("converting %T to a decimal: %w", v, err)
	}
	return dec, true, nil
}

// composeDecimal sets dest from src when src is a number: an *apd.Decimal,
// an int64, a float64 or the text of a number, which is how an OUT returns a
// NUMBER. It reports false for any other src.
func composeDecimal(dest composer, src any) (bool, error) {
	var dec *apd.Decimal
	switch s := src.(type) {
	case *apd.Decimal:
		dec = s
	case string:
		var err error
		if dec, _, err = apd.NewFromString(s); err != nil {
			return true, fmt.Errorf("converting %q to a decimal: %w", s, err)
		}
	case int64:
		dec = apd.New(s, 0)
	case float64:
		dec = new(apd.Decimal)
		if _, err := dec.SetFloat64(s); err != nil {
			return true, fmt.Errorf("converting %v to a decimal: %w", s, err)
		}
	default:
		return false, nil
	}
	if err := dest.Compose(dec.Decompose(nil)); err != nil {
		return true, fmt.Errorf("assigning %s to %T: %w", dec.Text('f'), dest, err)
	}
	return true, nil
}
