package ttc

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"

	oraTypes "github.com/xo/oracle/internal/types"
)

// copyValue stores src in dest, which is a pointer, for an OUT argument, a
// field of a user defined type, an element of an array and a field of a
// struct that DataSet.Scan fills. Rows scan through ScanColumn and not here.
//
// A value type of the driver, such as a LOB, goes as its plain value, unless
// dest is a value type of the driver too, which takes it through Scan. A
// collection, a []any, fills a slice one element at a time. Every other pair
// goes to sql.ConvertAssign, through assign.
//
// A NULL in a destination that cannot hold one is its zero value, as it was
// in go-ora, because a field of a user defined type is often a plain string.
// sql.ConvertAssign would refuse it.
func copyValue(dest, src any) error {
	if o, ok := src.(oraTypes.OracleType); ok {
		if _, own := dest.(oraTypes.OracleType); !own {
			v, err := o.Value()
			if err != nil {
				return err
			}
			src = v
		}
	}
	if src == nil {
		if s, ok := dest.(sql.Scanner); ok {
			return s.Scan(nil)
		}
		v := reflect.ValueOf(dest)
		if v.Kind() != reflect.Pointer || v.IsNil() {
			return fmt.Errorf("copying NULL into %T, which is not a pointer", dest)
		}
		v.Elem().SetZero()
		return nil
	}
	if items, ok := src.([]any); ok {
		return copySlice(dest, items)
	}
	return assign(driver.ScanContext{}, dest, src)
}

// copySlice stores items in dest, a pointer to a slice, one element at a
// time. An array bind of a slice of any element type needs reflect, and this
// is the one place that reads a slice by it.
func copySlice(dest any, items []any) error {
	if _, ok := dest.(*any); ok {
		return assign(driver.ScanContext{}, dest, items)
	}
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return fmt.Errorf("copying a collection into %T, which is not a pointer", dest)
	}
	v = v.Elem()
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Slice {
		return fmt.Errorf("copying a collection into %s, which is not a slice", v.Type())
	}
	s := reflect.MakeSlice(v.Type(), len(items), len(items))
	for i, item := range items {
		if err := copyValue(s.Index(i).Addr().Interface(), item); err != nil {
			return fmt.Errorf("copying element %d of a collection: %w", i, err)
		}
	}
	v.Set(s)
	return nil
}
