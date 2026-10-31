package charset

import (
	"bytes"
	"testing"
)

func TestCharsetsIndex(t *testing.T) {
	index, err := charsets()
	if err != nil {
		t.Fatal(err)
	}
	if len(index) != 236 {
		t.Errorf("the blob holds %d charsets, want 236", len(index))
	}
	for id := range index {
		if c := New(id); c == nil || c.LangID != id {
			t.Errorf("charset %d: got %+v", id, c)
		}
	}
	if c := New(99999); c != nil {
		t.Errorf("an unknown charset gives %+v, want nil", c)
	}
}

func TestCharsetsRoundTrip(t *testing.T) {
	// WE8ISO8859P1, US7ASCII, AL32UTF8, AL16UTF16 and UTF8
	for _, id := range []int{31, 1, 873, 2000, 871} {
		c := New(id)
		if c == nil {
			t.Fatalf("charset %d is missing", id)
		}
		in := "abc é"
		if id == 1 {
			in = "abc"
		}
		if got := c.Decode(c.Encode(in)); got != in {
			t.Errorf("charset %d: round trip of %q gives %q", id, in, got)
		}
	}
}

// TestCloneKeepsLeading checks the leading table of the EUC charsets, which
// Clone did not copy, so a clone of one decoded wrongly.
func TestCloneKeepsLeading(t *testing.T) {
	for _, id := range []int{0x35C, 0x35F} {
		c := New(id)
		if c == nil || len(c.leading) == 0 {
			t.Fatalf("charset %d has no leading table", id)
		}
		if got := c.Clone(); len(got.leading) != len(c.leading) || len(got.dBuffer) != len(c.dBuffer) {
			t.Errorf("charset %d: the clone has %d leading entries, want %d", id, len(got.leading), len(c.leading))
		}
	}
}

func BenchmarkFirstUse(b *testing.B) {
	index, err := charsets()
	if err != nil {
		b.Fatal(err)
	}
	e := index[0x354] // ZHS16GBK, one of the largest
	for b.Loop() {
		if _, err := decodeCharset(0x354, e.data); err != nil {
			b.Fatal(err)
		}
	}
}

// TestEncodeReplacement is sijms/go-ora#773: a character that a charset
// lacks was written as 0xFD, the low byte of U+FFFD, which is half a
// character of JA16EUC and ý in WE8ISO8859P1. It is the '?' of the charset.
func TestEncodeReplacement(t *testing.T) {
	tests := []struct {
		id   int
		in   string
		want []byte
	}{
		{830, "aⅠb", []byte("a?b")}, // JA16EUC
		{831, "Ⅱ", []byte("?")},     // JA16EUCYEN
		{837, "aⅠ", []byte("a?")},   // JA16EUCTILDE
		{31, "a€b", []byte("a?b")},  // WE8ISO8859P1
	}
	for _, test := range tests {
		c := New(test.id)
		if c == nil {
			t.Fatalf("no charset %d", test.id)
		}
		if got := c.Encode(test.in); !bytes.Equal(got, test.want) {
			t.Errorf("%d: %q encodes as % x, want % x", test.id, test.in, got, test.want)
		}
	}
	// a charset whose '?' is not 0x3F, such as EBCDIC, replaces with its own
	m, err := charsets()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for id := range m {
		c := New(id)
		if c == nil || c.eBuffer == nil {
			continue
		}
		q, ok := c.eBuffer['?']
		if !ok || q == '?' {
			continue
		}
		n++
		if _, has := c.eBuffer[0x2160]; has {
			continue
		}
		if got, want := c.Encode("Ⅰ"), appendChar(nil, q); !bytes.Equal(got, want) {
			t.Errorf("%d: Ⅰ encodes as % x, want its '?', % x", id, got, want)
		}
	}
	if n == 0 {
		t.Error("no charset has a '?' other than 0x3F")
	}
}
