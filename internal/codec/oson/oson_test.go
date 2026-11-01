package oson

import (
	"fmt"
	"strings"
	"testing"
)

func TestTextRoundTrip(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{`{"id":5,"name":"this is a test"}`, `{"id":5,"name":"this is a test"}`},
		// keys come back sorted, and a number keeps every digit
		{`{"z":1,"a":12345678901234567890.123456789}`, `{"a":12345678901234567890.123456789,"z":1}`},
		{`[1,"two",true,false,null,{"k":[1.5,-2]}]`, `[1,"two",true,false,null,{"k":[1.5,-2]}]`},
		{`{"nested":{"deep":{"deeper":[{"x":0.001}]}}}`, `{"nested":{"deep":{"deeper":[{"x":0.001}]}}}`},
		{`{"unicode":"日本語 ✓","escape":"a\"b\\c"}`, `{"escape":"a\"b\\c","unicode":"日本語 ✓"}`},
		// a document that is a scalar
		{`true`, `true`},
		{`false`, `false`},
		{`null`, `null`},
		{`"String"`, `"String"`},
		{`-9999999999999999999`, `-9999999999999999999`},
		{`319438950232418390.273596`, `319438950232418390.273596`},
	}
	for _, test := range tests {
		data, err := EncodeText([]byte(test.in))
		if err != nil {
			t.Errorf("EncodeText(%s): %v", test.in, err)
			continue
		}
		got, err := DecodeText(data)
		if err != nil {
			t.Errorf("DecodeText of %s: %v", test.in, err)
			continue
		}
		if string(got) != test.want {
			t.Errorf("round trip of %s\n got %s\nwant %s", test.in, got, test.want)
		}
	}
}

func TestEncodeTextRefusesBadText(t *testing.T) {
	for _, in := range []string{`{"a":}`, `{"a":1} 2`, ``} {
		if _, err := EncodeText([]byte(in)); err == nil {
			t.Errorf("EncodeText(%q): no error", in)
		}
	}
}

// TestNames checks the names that need the wider fields of the header: a
// name past 255 bytes, which only version 3 holds, more than 255 names, and
// a segment of short names past 64 KiB.
func TestNames(t *testing.T) {
	many := func(n, size int) map[string]any {
		m := make(map[string]any, n)
		for i := range n {
			name := fmt.Sprintf("%0*d", size, i)
			m[name] = name[:1]
		}
		return m
	}
	tests := []struct {
		name string
		in   map[string]any
	}{
		{"one long", map[string]any{strings.Repeat("a", 256): "v"}},
		{"short and long", map[string]any{"a": "v", strings.Repeat("b", 1000): "x", strings.Repeat("c", 255): true}},
		{"longest", map[string]any{strings.Repeat("d", 65535): "v"}},
		{"many", many(300, 3)},
		{"many long", many(300, 300)},
		{"wide segment", many(300, 250)},
	}
	for _, test := range tests {
		data, err := Encode(test.in)
		if err != nil {
			t.Errorf("%s: Encode: %v", test.name, err)
			continue
		}
		got, err := Decode(data)
		if err != nil {
			t.Errorf("%s: Decode: %v", test.name, err)
			continue
		}
		m, ok := got.(map[string]any)
		if !ok || len(m) != len(test.in) {
			t.Errorf("%s: got %T of %d names, want %d", test.name, got, len(m), len(test.in))
			continue
		}
		for name := range test.in {
			if _, ok := m[name]; !ok {
				t.Errorf("%s: name of %d bytes is missing", test.name, len(name))
			}
		}
	}
	if _, err := Encode(map[string]any{strings.Repeat("e", 65536): "v"}); err == nil {
		t.Error("a name of 65536 bytes: no error")
	}
}

// TestDecodeRefusesBadOSON reads OSON with a byte changed, as the case 3116
// of tests/data_types/test_3100_json.py of python-oracledb does, and every
// part of a document cut short. Each read must fail and not panic.
func TestDecodeRefusesBadOSON(t *testing.T) {
	bad, err := EncodeText([]byte(`{"key_1":"test_3116a","key_2":"test_3116b"}`))
	if err != nil {
		t.Fatal(err)
	}
	// the offset of the first name, past the end of the names
	bad[15] = 0xFF
	if got, err := DecodeText(bad); err == nil {
		t.Errorf("a byte changed: got %s, no error", got)
	}
	data, err := EncodeText([]byte(`{"key_1":"test_3116a","key_2":"test_3116b","list":[1,{"a":true}]}`))
	if err != nil {
		t.Fatal(err)
	}
	for n := range len(data) {
		if got, err := DecodeText(data[:n]); err == nil {
			t.Errorf("%d bytes of %d: got %s, no error", n, len(data), got)
		}
	}
}
