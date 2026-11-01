package types

import (
	"bytes"
	"strings"
	"testing"
)

// TestEncodeDecimal encodes decimals as NUMBER and reads them back. The
// bytes are those that Oracle stores for each value.
func TestEncodeDecimal(t *testing.T) {
	for _, test := range []struct {
		in, out string
		bytes   []byte
	}{
		{"0", "0", []byte{0x80}},
		{"1", "1", []byte{0xc1, 0x02}},
		{"-1", "-1", []byte{0x3e, 0x64, 0x66}},
		{"100", "100", []byte{0xc2, 0x02}},
		{"0.5", "0.5", []byte{0xc0, 0x33}},
		{"-24.5", "-24.5", []byte{0x3e, 0x4d, 0x33, 0x66}},
		{"1e-130", "0." + strings.Repeat("0", 129) + "1", []byte{0x80, 0x02}},
		{"-1E-130", "-0." + strings.Repeat("0", 129) + "1", nil},
		{"9.99999999999999E+125", "999999999999999" + strings.Repeat("0", 111), nil},
		{"18446744073709551615", "18446744073709551615", nil},
		{"12345678901234567890123456789012345678", "12345678901234567890123456789012345678", nil},
	} {
		b, err := encodeDecimal(test.in)
		if err != nil {
			t.Errorf("%s: %v", test.in, err)
			continue
		}
		if test.bytes != nil && !bytes.Equal(b, test.bytes) {
			t.Errorf("%s: got % x, want % x", test.in, b, test.bytes)
		}
		n := Number{}
		n.SetBytes(b)
		if got, err := n.String(); err != nil || got != test.out {
			t.Errorf("%s: read back %q, %v", test.in, got, err)
		}
	}
	for _, in := range []string{"1e126", "-1e126", "1e-131", "Infinity", "-Infinity", "NaN", "1" + strings.Repeat("1", 40), "x"} {
		if b, err := encodeDecimal(in); err == nil {
			t.Errorf("%s: got % x and no error", in, b)
		}
	}
}
