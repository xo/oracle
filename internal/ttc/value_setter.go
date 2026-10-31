package ttc

import (
	"reflect"
)

// set number value from supported types
//func setNumber(value reflect.Value, input *Number) error {
//	if value.Kind() == reflect.Ptr {
//		if value.IsNil() {
//			value.Set(reflect.New(value.Type().Elem()))
//		}
//		return setNumber(value.Elem(), input)
//	}
//	if tSigned(value.Type()) {
//		temp, err := input.Int64()
//		if err != nil {
//			return err
//		}
//		value.SetInt(temp)
//		return nil
//	}
//	if tUnsigned(value.Type()) {
//		temp, err := input.Uint64()
//		if err != nil {
//			return err
//		}
//		value.SetUint(temp)
//		return nil
//	}
//	if tFloat(value.Type()) {
//		temp, err := input.Float64()
//		if err != nil {
//			return err
//		}
//		value.SetFloat(temp)
//		return nil
//	}
//	switch value.Type() {
//	case oraTypes.TyBool:
//		value.SetBool(!input.isZero())
//	case oraTypes.TyString:
//		temp, err := input.String()
//		if err != nil {
//			return err
//		}
//		value.SetString(temp)
//	case oraTypes.TyNullString:
//		temp, err := input.String()
//		if err != nil {
//			return err
//		}
//		value.Set(reflect.ValueOf(sql.NullString{temp, true}))
//	case oraTypes.TyNullByte:
//		temp, err := input.Int64()
//		if err != nil {
//			return err
//		}
//		value.Set(reflect.ValueOf(sql.NullByte{uint8(temp), true}))
//	case oraTypes.TyNullInt16:
//		temp, err := input.Int64()
//		if err != nil {
//			return err
//		}
//		value.Set(reflect.ValueOf(sql.NullInt16{int16(temp), true}))
//	case oraTypes.TyNullInt32:
//		temp, err := input.Int64()
//		if err != nil {
//			return err
//		}
//		value.Set(reflect.ValueOf(sql.NullInt32{int32(temp), true}))
//	case oraTypes.TyNullInt64:
//		temp, err := input.Int64()
//		if err != nil {
//			return err
//		}
//		value.Set(reflect.ValueOf(sql.NullInt64{temp, true}))
//	case oraTypes.TyNullFloat64:
//		temp, err := input.Float64()
//		if err != nil {
//			return err
//		}
//		value.Set(reflect.ValueOf(sql.NullFloat64{temp, true}))
//	case oraTypes.TyNullBool:
//		value.Set(reflect.ValueOf(sql.NullBool{!input.isZero(), true}))
//	//case oraTypes.TyNullNVarChar:
//	//	temp, err := input.String()
//	//	if err != nil {
//	//		return err
//	//	}
//	//	value.Set(reflect.ValueOf(NullNVarChar{NVarChar(temp), true}))
//	default:
//		if temp, ok := value.Interface().(sql.Scanner); ok {
//			if temp != nil && !reflect.ValueOf(temp).IsNil() {
//				return temp.Scan(input)
//			}
//		}
//		if value.CanAddr() {
//			if temp, ok := value.Addr().Interface().(sql.Scanner); ok {
//				err := temp.Scan(input)
//				return err
//			}
//		}
//		return fmt.Errorf("can not assign number to type: %v", value.Type().Name())
//	}
//	return nil
//}

// set field value
//func setFieldValue(fieldValue reflect.Value, cust *Object, input interface{}) error {
//	// input should be one of primitive values
//	if input == nil {
//		return setNull(fieldValue)
//	}
//	if fieldValue.Kind() == reflect.Ptr && fieldValue.Elem().Kind() == reflect.Interface {
//		fieldValue.Elem().Set(reflect.ValueOf(input))
//		return nil
//	}
//	if fieldValue.Kind() == reflect.Interface {
//		fieldValue.Set(reflect.ValueOf(input))
//		return nil
//	}
//	//if fieldValue.CanAddr() {
//	//	if scan, ok := fieldValue.Addr().Interface().(sql.Scanner); ok {
//	//		return scan.Scan(input)
//	//	}
//	//} else {
//	//	if scan, ok := fieldValue.Interface().(sql.Scanner); ok {
//	//		return scan.Scan(input)
//	//	}
//	//}
//
//	switch val := input.(type) {
//	case int64, float64:
//		num, err := NewNumber(val)
//		if err != nil {
//			return err
//		}
//		return setNumber(fieldValue, num)
//	// case float64:
//	//	return setNumber(fieldValue, val)
//	case string:
//		return setString(fieldValue, val)
//	case time.Time:
//		return setTime(fieldValue, val)
//	case []byte:
//		return setBytes(fieldValue, val)
//	case LobStream:
//		return setLob(fieldValue, val)
//	case oraTypes.Clob:
//		return setClob(fieldValue, val)
//	case oraTypes.Blob:
//		return setBlob(fieldValue, val)
//	//case BFile:
//	//	return setBFile(fieldValue, val)
//	//case Vector:
//	//	return setVector(fieldValue, val)
//	case []ParameterInfo:
//		return setUDTObject(fieldValue, cust, val)
//	default:
//		return setWithScanner(fieldValue, input)
//		//if temp, ok := fieldValue.Interface().(sql.Scanner); ok {
//		//	if temp != nil && !reflect.ValueOf(temp).IsNil() {
//		//		return temp.Scan(input)
//		//	}
//		//}
//		//if fieldValue.CanAddr() {
//		//	if temp, ok := fieldValue.Addr().Interface().(sql.Scanner); ok {
//		//		err := temp.Scan(input)
//		//		return err
//		//	}
//		//}
//		//return fmt.Errorf("unsupported primitive type: %s", fieldValue.Type().Name())
//	}
//}

func setArray(value reflect.Value, input []ParameterInfo) error {
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			value.Set(reflect.New(value.Type().Elem()))
		}
		return setArray(value.Elem(), input)
	}
	tValue := value.Type()
	tempSlice := reflect.MakeSlice(tValue, 0, len(input))
	for _, par := range input {
		tempObj := reflect.New(tValue.Elem())
		err := copyValue(tempObj.Interface(), par.oPrimValue)
		if err != nil {
			return err
		}
		tempSlice = reflect.Append(tempSlice, tempObj.Elem())
	}
	value.Set(tempSlice)
	return nil
}
