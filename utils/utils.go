package utils

import (
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type (
	ParameterDirection int
)

func Max(params ...int64) int64 {
	ret := params[0]
	for i := 1; i < len(params); i++ {
		if ret < params[i] {
			ret = params[i]
		}
	}
	return ret
}
func CreateQuasiLocator(dataLength uint64) []byte {
	ret := make([]byte, 40)
	ret[1] = 38
	ret[3] = 4
	ret[4] = 97
	ret[5] = 8
	ret[9] = 1
	binary.BigEndian.PutUint64(ret[10:], dataLength)
	return ret
}

var (
	TyFloat64 = reflect.TypeFor[float64]()
	TyFloat32 = reflect.TypeFor[float32]()
	TyInt64   = reflect.TypeFor[int64]()
	TyBool    = reflect.TypeFor[bool]()
	TyBytes   = reflect.TypeFor[[]byte]()
	TyString  = reflect.TypeFor[string]()
	//TyNVarChar        = reflect.TypeOf((*NVarChar)(nil)).Elem()
	TyTime = reflect.TypeFor[time.Time]()
	//TyTimeStamp       = reflect.TypeOf((*TimeStamp)(nil)).Elem()
	//TyTimeStampTZ     = reflect.TypeOf((*TimeStampTZ)(nil)).Elem()
	//TyClob            = reflect.TypeOf((*Clob)(nil)).Elem()
	//TyNClob           = reflect.TypeOf((*NClob)(nil)).Elem()
	//TyBlob            = reflect.TypeOf((*Blob)(nil)).Elem()
	//TyBFile           = reflect.TypeOf((*BFile)(nil)).Elem()
	//TyVector          = reflect.TypeOf((*Vector)(nil)).Elem()
	TyNullByte    = reflect.TypeFor[sql.NullByte]()
	TyNullInt16   = reflect.TypeFor[sql.NullInt16]()
	TyNullInt32   = reflect.TypeFor[sql.NullInt32]()
	TyNullInt64   = reflect.TypeFor[sql.NullInt64]()
	TyNullFloat64 = reflect.TypeFor[sql.NullFloat64]()
	TyNullBool    = reflect.TypeFor[sql.NullBool]()
	TyNullString  = reflect.TypeFor[sql.NullString]()
	//TyNullNVarChar    = reflect.TypeOf((*NullNVarChar)(nil)).Elem()
	TyNullTime = reflect.TypeFor[sql.NullTime]()
	//TyNullTimeStamp   = reflect.TypeOf((*NullTimeStamp)(nil)).Elem()
	//TyNullTimeStampTZ = reflect.TypeOf((*NullTimeStampTZ)(nil)).Elem()
	//TyRefCursor       = reflect.TypeOf((*RefCursor)(nil)).Elem()
	//TyPLBool          = reflect.TypeOf((*PLBool)(nil)).Elem()
	//TyObject          = reflect.TypeOf((*Object)(nil)).Elem()
	//TyNumber          = reflect.TypeOf((*Number)(nil)).Elem()
	TyFloat32Array = reflect.TypeFor[[]float32]()
	TyUint8Array   = reflect.TypeFor[[]uint8]()
	TyFloat64Array = reflect.TypeFor[[]float64]()
)

func IsSigned(input reflect.Type) bool {
	switch input.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	default:
		return false
	}
}

func IsUnsigned(input reflect.Type) bool {
	switch input.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	default:
		return false
	}
}

func IsInteger(input reflect.Type) bool {
	return IsSigned(input) || IsUnsigned(input)
}

func IsFloat(input reflect.Type) bool {
	return input.Kind() == reflect.Float32 || input.Kind() == reflect.Float64
}

func IsNumber(input reflect.Type) bool {
	return IsInteger(input) || IsFloat(input)
}
func IsNullNumber(input reflect.Type) bool {
	switch input {
	case TyNullBool, TyNullByte, TyNullInt16, TyNullInt32, TyNullInt64:
		fallthrough
	case TyNullFloat64:
		return true
	default:
		return false
	}
}
func GetValue(origVal driver.Value) (driver.Value, error) {
	if origVal == nil {
		return nil, nil
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

func GetType(value driver.Value) reflect.Type {
	valueType := reflect.TypeOf(value)
	if valueType != nil {
		for valueType.Kind() == reflect.Pointer {
			valueType = valueType.Elem()
		}
	}
	return valueType
}
func ExtractTag(tag string) (name, _type string, size int, direction int) {
	extractNameValue := func(input string, pos int) {
		parts := strings.Split(input, "=")
		var id, value string
		if len(parts) < 2 {
			switch pos {
			case 0:
				id = "name"
			case 1:
				id = "type"
			case 2:
				id = "size"
			case 3:
				id = "direction"
			}
			value = input
		} else {
			id = strings.TrimSpace(strings.ToLower(parts[0]))
			value = strings.TrimSpace(parts[1])
		}
		switch id {
		case "name":
			name = value
		case "type":
			_type = value
		case "size":
			tempSize, _ := strconv.ParseInt(value, 10, 32)
			size = int(tempSize)
		case "dir":
			fallthrough
		case "direction":
			switch value {
			case "in", "input":
				direction = 1
			case "out", "output":
				direction = 2
			case "inout":
				direction = 3
			}
		}
	}
	tag = strings.TrimSpace(tag)
	if len(tag) == 0 {
		return
	}
	tagFields := strings.Split(tag, ",")
	if len(tagFields) > 0 {
		extractNameValue(tagFields[0], 0)
	}
	if len(tagFields) > 1 {
		extractNameValue(tagFields[1], 1)
	}
	if len(tagFields) > 2 {
		extractNameValue(tagFields[2], 2)
	}
	if len(tagFields) > 3 {
		extractNameValue(tagFields[3], 3)
	}
	if direction == 0 {
		direction = 1
	}
	return
}
