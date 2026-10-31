package types

import (
	"fmt"

	"github.com/xo/oracle/internal/converters"
)

type String struct {
	Basic
	Conv        converters.IStringConverter
	UseNCharset bool
}

func (str *String) Value() (any, error) {
	if str.bValue == nil {
		return nil, nil
	}
	if str.Conv == nil {
		str.Conv = converters.NewStringConverter(0x7D0)
	}
	return str.Conv.Decode(str.bValue), nil
}

func (str *String) SetValue(input any) error {
	input, verr := ValueOf(input)
	if verr != nil {
		return verr
	}
	if input == nil {
		str.bValue = nil
		return nil
	}
	if str.Conv == nil {
		str.Conv = converters.NewStringConverter(0x7D0)
	}
	switch data := input.(type) {
	case String:
		if str.Conv.GetLangID() == data.Conv.GetLangID() {
			*str = data
		} else {
			temp, err := data.Value()
			if err != nil {
				return err
			}
			return str.SetValue(temp)
		}

	case *String:
		if str.Conv.GetLangID() == data.Conv.GetLangID() {
			*str = *data
		} else {
			temp, err := data.Value()
			if err != nil {
				return err
			}
			return str.SetValue(temp)
		}

	case string:
		str.bValue = str.Conv.Encode(data)
	case *string:
		str.bValue = str.Conv.Encode(*data)
	default:
		return fmt.Errorf("cannot set value of type %T into string", input)
	}
	return nil
}
func (str *String) Scan(input any) error {
	return str.SetValue(input)
}
