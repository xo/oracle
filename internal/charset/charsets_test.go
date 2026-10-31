package charset

import (
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
