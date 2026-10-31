package types

import (
	"encoding/binary"
	"testing"
	"time"
)

// TestIntervalOffset checks the bias of 2^31 in the fields of four bytes,
// which overflowed on 32 bit platforms in sijms/go-ora#738.
func TestIntervalOffset(t *testing.T) {
	ym, err := NewYearMonthInterval(2026, 5)
	if err != nil {
		t.Fatal(err)
	}
	b := ym.Bytes()
	if y := int64(binary.BigEndian.Uint32(b)) - 0x80000000; y != 2026 || int(b[4])-60 != 5 {
		t.Errorf("year to month: got %d years and %d months", y, int(b[4])-60)
	}
	ds, err := NewDaySecondInterval(4, 2, 30, 15, 0)
	if err != nil {
		t.Fatal(err)
	}
	v, err := ds.Value()
	if err != nil {
		t.Fatal(err)
	}
	if want := 4*24*time.Hour + 2*time.Hour + 30*time.Minute + 15*time.Second; v != want {
		t.Errorf("day to second: got %v, want %v", v, want)
	}
}

// TestTimeZoneBytes checks the interval of the time zone that the client
// sends at logon, which go-ora sent with 30 days, because it built it from
// time.Date(0, 0, 0, ...).
func TestTimeZoneBytes(t *testing.T) {
	iv, err := NewDaySecondInterval(0, 7, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{128, 0, 0, 0, 67, 60, 60, 128, 0, 0, 0}
	if got := iv.Bytes(); string(got) != string(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
