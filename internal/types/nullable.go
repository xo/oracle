package types

import (
	"database/sql"
	"database/sql/driver"
	"reflect"
	"time"
)

// ScanExternal stores the value that value returns in dest when dest is an
// sql.Scanner that is not a type of this package, such as an *sql.Null[T] or
// an *sql.NullString, and reports whether it did. The Value methods of the
// types of this package return the plain Go value, which Scan takes, so the
// driver needs no case for each nullable type.
func ScanExternal(dest any, value func() (any, error)) (bool, error) {
	s, ok := dest.(sql.Scanner)
	if !ok {
		return false, nil
	}
	if _, own := dest.(OracleType); own {
		return false, nil
	}
	v, err := value()
	if err != nil {
		return true, err
	}
	return true, s.Scan(v)
}

// OwnNull returns the value of a nullable type whose value the driver binds
// itself, and reports whether v is one. The Value method of sql.Null[T]
// converts a time.Duration to an int64 and refuses an IntervalYM, so the
// driver reads these few itself.
func OwnNull(v any) (any, bool) {
	switch n := v.(type) {
	case sql.Null[time.Duration]:
		return nullOf(n), true
	case *sql.Null[time.Duration]:
		return nullOf(*n), true
	case sql.Null[IntervalYM]:
		return nullOf(n), true
	case *sql.Null[IntervalYM]:
		return nullOf(*n), true
	case sql.Null[TimeStampTZ]:
		return nullOf(n), true
	case *sql.Null[TimeStampTZ]:
		return nullOf(*n), true
	}
	return nil, false
}

// nullOf returns the value of n, or nil when n is not valid.
func nullOf[T any](n sql.Null[T]) any {
	if !n.Valid {
		return nil
	}
	return n.V
}

// ValueOf returns the value of input when input is a driver.Valuer that is
// not a type of this package, such as an sql.NullString or an sql.Null[T],
// and input when not. A nil pointer is a NULL.
func ValueOf(input any) (any, error) {
	if v, ok := OwnNull(input); ok {
		return v, nil
	}
	valuer, ok := input.(driver.Valuer)
	if !ok {
		return input, nil
	}
	switch input.(type) {
	case OracleType, TimeStampTZ, *TimeStampTZ:
		return input, nil
	}
	if rv := reflect.ValueOf(input); rv.Kind() == reflect.Pointer && rv.IsNil() {
		return nil, nil
	}
	return valuer.Value()
}
