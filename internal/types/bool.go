package types

import (
	"bytes"
	"fmt"
)

type Bool struct {
	Basic
}

func (bl *Bool) SetValue(input any) error {
	input, verr := ValueOf(input)
	if verr != nil {
		return verr
	}
	if input == nil {
		bl.bValue = nil
		return nil
	}
	switch value := input.(type) {
	case Bool:
		*bl = value
	case *Bool:
		*bl = *value
	case bool:
		if value {
			bl.bValue = []byte{1, 1}
		} else {
			bl.bValue = []byte{1, 0}
		}
	case *bool:
		if *value {
			bl.bValue = []byte{1, 1}
		} else {
			bl.bValue = []byte{1, 0}
		}
	default:
		return fmt.Errorf("cannot set value of type %T into Bool", input)
	}
	return nil
}

func (bl *Bool) Value() (any, error) {
	if bl.bValue == nil {
		return nil, nil
	}
	if bytes.Equal(bl.bValue, []byte{1, 1}) {
		return true, nil
	}
	return false, nil
}

func (bl *Bool) Scan(input any) error {
	return bl.SetValue(input)
}
