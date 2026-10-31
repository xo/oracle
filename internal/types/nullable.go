package types

import (
	"database/sql"
	"database/sql/driver"
	"reflect"
	"time"
)

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
	case sql.Null[TimestampTZ]:
		return nullOf(n), true
	case *sql.Null[TimestampTZ]:
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
	case OracleType, TimestampTZ, *TimestampTZ:
		return input, nil
	}
	if rv := reflect.ValueOf(input); rv.Kind() == reflect.Pointer && rv.IsNil() {
		return nil, nil
	}
	return valuer.Value()
}
