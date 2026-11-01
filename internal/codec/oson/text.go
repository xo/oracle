package oson

import (
	"bytes"
	"encoding/json/jsontext"
	"fmt"
	"io"
	"maps"
	"math"
	"slices"

	"github.com/xo/oracle/internal/types"
)

// number is a JSON number as its text, so that a NUMBER keeps every digit
// between OSON and JSON text, where a float64 would round it.
type number string

// EncodeText returns the OSON of the JSON text text. encoding/json/v2 turns
// a Go value into JSON text, so OSON reads text alone and maps no Go type.
func EncodeText(text []byte) ([]byte, error) {
	dec := jsontext.NewDecoder(bytes.NewReader(text))
	v, err := readValue(dec)
	if err != nil {
		return nil, fmt.Errorf("reading JSON text: %w", err)
	}
	if _, err := dec.ReadToken(); err != io.EOF {
		return nil, fmt.Errorf("reading JSON text: text after the value")
	}
	return Encode(v)
}

// readValue reads one JSON value from dec as a tree of map[string]any, []any,
// string, number, bool and nil.
func readValue(dec *jsontext.Decoder) (any, error) {
	tok, err := dec.ReadToken()
	if err != nil {
		return nil, err
	}
	switch tok.Kind() {
	case '{':
		obj := map[string]any{}
		for dec.PeekKind() != '}' {
			key, err := dec.ReadToken()
			if err != nil {
				return nil, err
			}
			// a token is void after the next call to the decoder, so the
			// name is read before the value
			name := key.String()
			v, err := readValue(dec)
			if err != nil {
				return nil, err
			}
			obj[name] = v
		}
		_, err := dec.ReadToken()
		return obj, err
	case '[':
		arr := []any{}
		for dec.PeekKind() != ']' {
			v, err := readValue(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		_, err := dec.ReadToken()
		return arr, err
	case '"':
		return tok.String(), nil
	case '0':
		return number(tok.String()), nil
	case 't', 'f':
		return tok.Bool(), nil
	case 'n':
		return nil, nil
	}
	return nil, fmt.Errorf("reading a token of kind %v", tok.Kind())
}

// DecodeText returns the JSON text of the OSON data. A date, an interval and
// a RAW of OSON are strings, and an infinity and a NaN too, as JSON_SERIALIZE
// writes them. A VECTOR is an array of numbers. The keys of an object are in
// sorted order.
func DecodeText(data []byte) (jsontext.Value, error) {
	v, err := Decode(data)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := jsontext.NewEncoder(&buf)
	if err := writeValue(enc, v); err != nil {
		return nil, err
	}
	return jsontext.Value(bytes.TrimSpace(buf.Bytes())), nil
}

// writeValue writes a value that Decode returned to enc.
func writeValue(enc *jsontext.Encoder, v any) error {
	switch v := v.(type) {
	case nil:
		return enc.WriteToken(jsontext.Null)
	case bool:
		return enc.WriteToken(jsontext.Bool(v))
	case string:
		return enc.WriteToken(jsontext.String(v))
	case number:
		return enc.WriteValue(jsontext.Value(v))
	case float64:
		return enc.WriteToken(float(v))
	case float32:
		return enc.WriteToken(float(float64(v)))
	case []float32:
		return writeArray(enc, v, func(f float32) jsontext.Token { return float(float64(f)) })
	case []float64:
		return writeArray(enc, v, float)
	case []int8:
		return writeArray(enc, v, func(i int8) jsontext.Token { return jsontext.Int(int64(i)) })
	case []byte:
		return writeArray(enc, v, func(b byte) jsontext.Token { return jsontext.Uint(uint64(b)) })
	case types.SparseVector:
		// the text of a sparse vector in Oracle: the number of its
		// dimensions, its indices and its values
		if err := enc.WriteToken(jsontext.BeginArray); err != nil {
			return err
		}
		if err := enc.WriteToken(jsontext.Uint(uint64(v.Dimensions))); err != nil {
			return err
		}
		if err := writeArray(enc, v.Indices, func(i uint32) jsontext.Token { return jsontext.Uint(uint64(i)) }); err != nil {
			return err
		}
		if err := writeValue(enc, v.Values); err != nil {
			return err
		}
		return enc.WriteToken(jsontext.EndArray)
	case map[string]any:
		if err := enc.WriteToken(jsontext.BeginObject); err != nil {
			return err
		}
		for _, k := range slices.Sorted(maps.Keys(v)) {
			if err := enc.WriteToken(jsontext.String(k)); err != nil {
				return err
			}
			if err := writeValue(enc, v[k]); err != nil {
				return err
			}
		}
		return enc.WriteToken(jsontext.EndObject)
	case []any:
		if err := enc.WriteToken(jsontext.BeginArray); err != nil {
			return err
		}
		for _, item := range v {
			if err := writeValue(enc, item); err != nil {
				return err
			}
		}
		return enc.WriteToken(jsontext.EndArray)
	}
	return fmt.Errorf("writing a value of OSON of the type %T as JSON", v)
}

// float returns the token of a BINARY_FLOAT or a BINARY_DOUBLE. JSON has no
// infinity and no NaN, so they are the strings that JSON_SERIALIZE writes.
func float(f float64) jsontext.Token {
	switch {
	case math.IsInf(f, 1):
		return jsontext.String("Inf")
	case math.IsInf(f, -1):
		return jsontext.String("-Inf")
	case math.IsNaN(f):
		return jsontext.String("Nan")
	}
	return jsontext.Float(f)
}

// writeArray writes the elements of v as a JSON array, each as the token that
// token makes of it. A VECTOR in a JSON value is an array of numbers.
func writeArray[E any](enc *jsontext.Encoder, v []E, token func(E) jsontext.Token) error {
	if err := enc.WriteToken(jsontext.BeginArray); err != nil {
		return err
	}
	for _, e := range v {
		if err := enc.WriteToken(token(e)); err != nil {
			return err
		}
	}
	return enc.WriteToken(jsontext.EndArray)
}
