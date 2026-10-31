package types

import (
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
