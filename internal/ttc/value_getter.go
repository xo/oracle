package ttc

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"github.com/cockroachdb/apd/v3"
	"reflect"
	"strconv"
	"time"

	"github.com/xo/oracle/internal/types"
)

// ======== get primitive data from original data types ========//

// get value to bypass pointer and sql.Null* values
func getValue(origVal driver.Value) (driver.Value, error) {
	if origVal == nil {
		return nil, nil
	}
	if v, ok := types.OwnNull(origVal); ok {
		return v, nil
	}
	rOriginal := reflect.ValueOf(origVal)
	if rOriginal.Kind() == reflect.Pointer && rOriginal.IsNil() {
		return nil, nil
	}
	proVal := reflect.Indirect(rOriginal)
	if valuer, ok := proVal.Interface().(driver.Valuer); ok {
		return valuer.Value()
	}
	return proVal.Interface(), nil
}
func getType(value driver.Value) reflect.Type {
	valueType := reflect.TypeOf(value)
	if valueType != nil {
		for valueType.Kind() == reflect.Pointer {
			valueType = valueType.Elem()
		}
	}
	return valueType
}

func getString(col any) string {
	switch val := col.(type) {
	case types.Clob:
		temp, _ := val.Value()
		if temp == nil {
			return ""
		}
		return temp.(string)
	case *types.Clob:
		temp, _ := val.Value()
		if temp == nil {
			return ""
		}
		return temp.(string)
	}
	col, _ = getValue(col)
	if col == nil {
		return ""
	}
	if temp, ok := col.(string); ok {
		return temp
	}

	return fmt.Sprintf("%v", col)
}

// get int64 value from supported types
func getInt(col any) (int64, error) {
	var err error
	col, err = getValue(col)
	if err != nil {
		return 0, err
	}
	if col == nil {
		return 0, nil
	}
	rType := reflect.TypeOf(col)
	rValue := reflect.ValueOf(col)
	if tInteger(rType) {
		return rValue.Int(), nil
	}
	if tFloat(rType) {
		return int64(rValue.Float()), nil
	}
	switch rType.Kind() {
	case reflect.String:
		tempInt, err := strconv.ParseInt(rValue.String(), 10, 64)
		if err != nil {
			return 0, err
		}
		return tempInt, nil
	case reflect.Bool:
		if rValue.Bool() {
			return 1, nil
		} else {
			return 0, nil
		}
	default:
		return 0, errors.New("conversion of unsupported type to int")
	}
}

// get time.Time from supported types
func getDate(col any) (time.Time, error) {
	var err error
	col, err = getValue(col)
	if err != nil {
		return time.Time{}, err
	}
	if col == nil {
		return time.Time{}, nil
	}
	switch val := col.(type) {
	case time.Time:
		return val, nil
	case types.TimestampTZ:
		return time.Time(val), nil
	case string:
		return time.Parse(time.RFC3339, val)
	default:
		return time.Time{}, errors.New("conversion of unsupported type to time.Time")
	}
}

// get []byte from supported types
func getBytes(col any) ([]byte, error) {
	var err error
	switch val := col.(type) {
	case types.Blob:
		var temp any
		temp, err = val.Value()
		if err != nil {
			return nil, err
		}
		if temp == nil {
			return nil, nil
		}
		return temp.([]byte), nil
	case *types.Blob:
		var temp any
		temp, err = val.Value()
		if err != nil {
			return nil, err
		}
		if temp == nil {
			return nil, nil
		}
		return temp.([]byte), nil
	}
	col, err = getValue(col)
	if err != nil {
		return nil, err
	}
	if col == nil {
		return nil, nil
	}
	switch val := col.(type) {
	case []byte:
		return val, nil
	case string:
		return []byte(val), nil
	case types.Blob:
		var temp any
		temp, err = val.Value()
		if err != nil {
			return nil, err
		}
		if temp == nil {
			return nil, nil
		}
		return temp.([]byte), nil
	default:
		return nil, errors.New("conversion of unsupported type to []byte")
	}
}

// namedBasic reports whether v is a value of a named type of a basic kind,
// such as type ID string, that the driver does not know. database/sql
// converts such a value to its basic kind, which was sijms/go-ora#699. A
// named type that the driver knows, such as time.Duration, keeps its coder.
func namedBasic(v any) bool {
	if v == nil || builtinCoder(v) != nil {
		return false
	}
	t := reflect.TypeOf(v)
	if t.PkgPath() == "" {
		return false
	}
	switch t.Kind() {
	case reflect.Bool, reflect.String, reflect.Float32, reflect.Float64,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	}
	return false
}

// basicValue returns v as its basic kind when it is a named type of one, such
// as type Name string, and as a time.Time or a []byte when it is a named type
// of one of those. database/sql converts a named basic value of a statement,
// and the driver converts one in an object or a collection.
func basicValue(v any) (any, error) {
	if namedBasic(v) {
		return driver.DefaultParameterConverter.ConvertValue(v)
	}
	if v == nil || builtinCoder(v) != nil {
		return v, nil
	}
	rv := reflect.ValueOf(v)
	t := rv.Type()
	if t.PkgPath() == "" {
		return v, nil
	}
	timeType := reflect.TypeFor[time.Time]()
	switch {
	case t.Kind() == reflect.Struct && t.ConvertibleTo(timeType):
		return rv.Convert(timeType).Interface(), nil
	case t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8:
		return rv.Bytes(), nil
	}
	return v, nil
}

// ownValue reports whether v is a value that the driver binds itself, which
// database/sql must not convert first.
func ownValue(v any) bool {
	switch v.(type) {
	case types.TimestampTZ, *types.TimestampTZ:
		return true
	case sql.Null[bool], sql.NullBool:
		// a NULL bool binds as a NULL BOOLEAN, which PL/SQL assigns to a
		// BOOLEAN. database/sql made a NULL of no type of it, which PL/SQL
		// refused with ORA-06550
		return true
	case *apd.Decimal, apd.Decimal:
		// a decimal binds as a NUMBER. As the text that database/sql made
		// of it, the server read it with the NLS_NUMERIC_CHARACTERS of the
		// session
		return true
	}
	_, ok := types.OwnNull(v)
	return ok
}
