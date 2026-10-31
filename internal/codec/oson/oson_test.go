package oson

import (
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
