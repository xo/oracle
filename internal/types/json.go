package types

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
)

// JSON is a JSON value, of the type JSON of Oracle 21c and later. It holds
// the JSON text, and the driver turns it into OSON, the binary JSON of Oracle,
// when it binds it, and back when it reads it. encoding/json/v2 maps a Go
// value to the text and back, so the driver maps no Go type itself.
//
// A JSON column scans as a jsontext.Value, and into any destination that
// encoding/json/v2 decodes into, such as a struct or a map.
type JSON struct {
	Basic
	text jsontext.Value
	loc  Locator
	lobBase
}

// NewJSON returns the JSON of v. A string, a []byte and a jsontext.Value
// are JSON text already, and any other value goes through json.Marshal.
func NewJSON(v any) (*JSON, error) {
	js := &JSON{}
	return js, js.SetValue(v)
}

// Text returns the JSON text.
func (js *JSON) Text() jsontext.Value {
	return js.text
}

// SetOSON sets the OSON bytes that the driver sends, and the locator that
// says they are inline.
func (js *JSON) SetOSON(b []byte) {
	js.bValue = b
	if len(b) > 0 {
		js.loc = NewInlineLocator(uint64(len(b)))
	} else {
		js.loc = nil
	}
}

func (js *JSON) Locator() Locator {
	if js.lobBase.Locator() != nil {
		return js.lobBase.Locator()
	}
	return js.loc
}

func (js *JSON) Upload() error {
	return js.uploadData(js.bValue, 0, 0)
}

// SetValue sets the JSON text from v, as NewJSON does.
func (js *JSON) SetValue(v any) error {
	var text []byte
	switch v := v.(type) {
	case nil:
		js.text, js.bValue = nil, nil
		return nil
	case JSON:
		*js = v
		return nil
	case *JSON:
		*js = *v
		return nil
	case jsontext.Value:
		text = v
	case []byte:
		text = v
	case string:
		text = []byte(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("marshaling %T as JSON: %w", v, err)
		}
		text = b
	}
	val := jsontext.Value(bytes.Clone(text))
	if err := val.Compact(); err != nil {
		return fmt.Errorf("reading JSON text: %w", err)
	}
	js.text = val
	return nil
}

// Value returns the JSON text, or nil for NULL.
func (js *JSON) Value() (any, error) {
	if js.text == nil {
		return nil, nil
	}
	return js.text, nil
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
