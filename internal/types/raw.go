package types

import (
	"database/sql/driver"
	"fmt"
)

type Raw struct {
	Basic
}

func (raw *Raw) Value() (any, error) {
	return raw.bValue, nil
}
func (raw *Raw) SetValue(input any) error {
	input, verr := ValueOf(input)
	if verr != nil {
		return verr
	}
	if input == nil {
		raw.bValue = nil
		return nil
	}
	switch data := input.(type) {
	case []byte:
		raw.bValue = data
	case *[]byte:
		raw.bValue = *data
	case string:
		raw.bValue = []byte(data)
	case *string:
		raw.bValue = []byte(*data)
	default:
		return fmt.Errorf("cannot set value of type %T into Blob", input)
	}
	return nil
}

func (raw *Raw) Scan(input any) error {
	return raw.SetValue(input)
}

func (raw *Raw) CopyTo(dest driver.Value) error {
	if ok, err := ScanExternal(dest, raw.Value); ok {
		return err
	}
	switch dst := dest.(type) {
	case *[]byte:
		*dst = raw.bValue
	case *string:
		*dst = string(raw.bValue)
	case *Raw:
		*dst = *raw
	default:
		return fmt.Errorf("cannot copy Raw to variable of type %T", dest)
	}
	return nil
}
