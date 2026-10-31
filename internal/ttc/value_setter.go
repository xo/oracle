package ttc

import (
	"reflect"
)

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
