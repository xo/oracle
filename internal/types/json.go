package types

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	//"github.com/xo/oracle/oson"
)

type JSONCoder interface {
	JSONEncoder
	JSONDecoder
}

type JSONDecoder interface {
	DecodeJSON(data []byte) (any, error)
}
type JSONEncoder interface {
	EncodeJSON(input any) ([]byte, error)
}
type JSON struct {
	Basic
	Coder JSONCoder
	loc   Locator
	lobBase
}

var jsonTypeError = errors.New("unexpected data for json type")

func (js *JSON) GetLocator() Locator {
	if js.lobBase.GetLocator() != nil {
		return js.lobBase.GetLocator()
	}
	return js.loc
}
func (js *JSON) Upload() error {
	return js.uploadData(js.bValue, 0, 0)
}

func (js *JSON) SetValue(input any) error {
	if input == nil {
		js.bValue = nil
		return nil
	}
	var err error
	switch value := input.(type) {
	case JSON:
		currentCoder := js.Coder
		*js = value
		if currentCoder != nil && js.Coder == nil {
			js.Coder = currentCoder
		}
	case *JSON:
		currentCoder := js.Coder
		*js = *value
		if currentCoder != nil && js.Coder == nil {
			js.Coder = currentCoder
		}
	case string:
		value = strings.TrimSpace(value)
		if len(value) == 0 {
			js.bValue = nil
			return nil
		}
		if strings.HasPrefix(value, "[") {
			var temp []any
			err = json.Unmarshal([]byte(value), &temp)
			if err != nil {
				return err
			}
			return js.SetValue(temp)
		}

		var temp = make(map[string]any)
		err = json.Unmarshal([]byte(value), &temp)
		if err != nil {
			return err
		}
		return js.SetValue(temp)
	default:
		if js.Coder != nil {
			js.bValue, err = js.Coder.EncodeJSON(input)
		}

	}
	if err == nil {
		dataLen := uint64(len(js.bValue))
		if dataLen > 0 {
			js.loc = NewInlineLocator(dataLen)
		} else {
			js.loc = nil
		}
	}
	return err
}

func (js *JSON) Value() (any, error) {
	return js.Coder.DecodeJSON(js.bValue)
}

func (js *JSON) Scan(input any) error {
	return js.SetValue(input)
}

// Load reads the whole value and watches no context. See Lob.
func (js *JSON) Load() error {
	var err error
	js.bValue, err = js.load()
	return err
}

func (js *JSON) Read(ctx context.Context) error {
	var err error
	js.bValue, err = js.ReadFromPos(ctx, 0)
	return err
}
