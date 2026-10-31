package types

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	//"github.com/xo/oracle/oson"
)

type JsonCoder interface {
	JsonEncoder
	JsonDecoder
}

type JsonDecoder interface {
	DecodeJson(data []byte) (any, error)
}
type JsonEncoder interface {
	EncodeJson(input any) ([]byte, error)
}
type Json struct {
	Basic
	Coder JsonCoder
	loc   Locator
	lobBase
}

var jsonTypeError = errors.New("unexpected data for json type")

func (js *Json) GetLocator() Locator {
	if js.lobBase.GetLocator() != nil {
		return js.lobBase.GetLocator()
	}
	return js.loc
}
func (js *Json) Upload() error {
	return js.uploadData(js.bValue, 0, 0)
}

func (js *Json) SetValue(input any) error {
	if input == nil {
		js.bValue = nil
		return nil
	}
	var err error
	switch value := input.(type) {
	case Json:
		currentCoder := js.Coder
		*js = value
		if currentCoder != nil && js.Coder == nil {
			js.Coder = currentCoder
		}
	case *Json:
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
			js.bValue, err = js.Coder.EncodeJson(input)
		}

	}
	if err == nil {
		dataLen := uint64(len(js.bValue))
		if dataLen > 0 {
			js.loc = NewQuasiLocator(dataLen)
		} else {
			js.loc = nil
		}
	}
	return err
}

func (js *Json) Value() (any, error) {
	return js.Coder.DecodeJson(js.bValue)
}

func (js *Json) Scan(input any) error {
	return js.SetValue(input)
}

// Load reads the whole value and watches no context. See Lob.
func (js *Json) Load() error {
	var err error
	js.bValue, err = js.load()
	return err
}

func (js *Json) Read(ctx context.Context) error {
	var err error
	js.bValue, err = js.ReadFromPos(ctx, 0)
	return err
}
