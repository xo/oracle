package ttc

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"

	"github.com/xo/oracle/internal/types"
)

// copyValue stores src in dest, which is a pointer, for an OUT argument, a
// field of a user defined type, an element of an array and a field of a
// struct that Rows.Scan fills. The rows of database/sql scan through
// ScanColumn and not here.
//
// A value type of the driver, such as a LOB, goes as its plain value, unless
// dest is a value type of the driver too, which takes it through Scan. A
// collection, a []any, fills a slice one element at a time, and an index-by
// table fills a map by its keys or a slice. Every other pair goes to
// sql.ConvertAssign, through assign.
//
// A NULL in a destination that cannot hold one is its zero value, as it was
// in go-ora, because a field of a user defined type is often a plain string.
// sql.ConvertAssign would refuse it.
func copyValue(dest, src any) error {
	if o, ok := src.(types.OracleType); ok {
		if _, own := dest.(types.OracleType); !own {
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
	switch items := src.(type) {
	case []any:
		return copySlice(dest, items)
	case *indexedItems:
		return copyIndexed(dest, items)
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

// copyIndexed stores an index-by table in dest: in a map, by its keys, and
// in anything else as copySlice stores its elements.
func copyIndexed(dest any, src *indexedItems) error {
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Map {
		return copySlice(dest, src.items)
	}
	v = v.Elem()
	m := reflect.MakeMapWithSize(v.Type(), len(src.keys))
	for i, item := range src.items {
		key := reflect.New(v.Type().Key()).Elem()
		if !key.CanInt() {
			return fmt.Errorf("copying an index-by table into %s, whose key is not an integer", v.Type())
		}
		key.SetInt(int64(src.keys[i]))
		elem := reflect.New(v.Type().Elem())
		if err := copyValue(elem.Interface(), item); err != nil {
			return fmt.Errorf("copying the element of key %d of an index-by table: %w", src.keys[i], err)
		}
		m.SetMapIndex(key, elem.Elem())
	}
	v.Set(m)
	return nil
}
