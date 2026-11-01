package types

import (
	"fmt"
)

type Raw struct {
	Basic
}

// Value returns the bytes of the RAW, or nil for a NULL. go-ora returned a
// nil []byte in an any, which is not nil, so a NULL scanned into a
// sql.Null[[]byte] as a valid value.
func (raw *Raw) Value() (any, error) {
	if raw.bValue == nil {
		return nil, nil
	}
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
