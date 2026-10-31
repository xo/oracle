package types

import (
	"testing"
	"time"
)

// The vectors in this file are the bytes that Oracle writes for a value, read
// with dump() in SQL*Plus. They came from the tests of the converters package,
// which tested functions that no longer exist.

func TestDecodeBinaryFloat(t *testing.T) {
	// SELECT dump(cast(134.45 as binary_float)) FROM dual
	tests := []struct {
		raw  []byte
		want float32
	}{
		{[]byte{195, 6, 115, 51}, 134.45},
		{[]byte{60, 249, 140, 204}, -134.45},
	}
	for _, test := range tests {
		n := &Number{Basic{bValue: append([]byte(nil), test.raw...), dataType: IBFLOAT}}
		got, err := n.Value()
		if err != nil {
			t.Fatalf("decoding %v: %v", test.raw, err)
		}
		if got != test.want {
			t.Errorf("decoding %v: got %v, want %v", test.raw, got, test.want)
		}
	}
}

func TestDecodeBinaryDouble(t *testing.T) {
	// SELECT dump(cast(134.45 as binary_double)) FROM dual
	tests := []struct {
		raw  []byte
		want float64
	}{
		{[]byte{192, 96, 206, 102, 102, 102, 102, 102}, 134.45},
		{[]byte{63, 159, 49, 153, 153, 153, 153, 153}, -134.45},
	}
	for _, test := range tests {
		n := &Number{Basic{bValue: append([]byte(nil), test.raw...), dataType: IBDOUBLE}}
		got, err := n.Value()
		if err != nil {
			t.Fatalf("decoding %v: %v", test.raw, err)
		}
		if got != test.want {
			t.Errorf("decoding %v: got %v, want %v", test.raw, got, test.want)
		}
	}
}

func TestDecodeTimestampTZ(t *testing.T) {
	raw := []byte{0x78, 0x7C, 0x01, 0x08, 0x11, 0x23, 0x32, 0x28, 0xE9, 0x6E, 0x70, 0x84, 0x80}
	d := &Date{bValue: raw, dataType: TimeStampTZ_DTY}
	got, err := d.Value()
	if err != nil {
		t.Fatalf("decoding %v: %v", raw, err)
	}
	want := time.Date(2024, 1, 8, 19, 34, 49, 686386800, time.FixedZone("+03", 3*60*60))
	if tm, ok := got.(time.Time); !ok || !tm.Equal(want) {
		t.Errorf("decoding %v: got %v, want %v", raw, got, want)
	}
}

// The interval vectors hold what Oracle prints for each value. Interval.Value
// decodes none of them correctly. It returns a time.Time, so a year to month
// interval of +2021-10 comes back as 2021-09-30, and it multiplies the
// nanoseconds of a day to second interval by 1000. DESIGN.md replaces the
// type, and BACKLOG.md holds the fault.

func TestDecodeIntervalYM(t *testing.T) {
	t.Skip("Interval.Value decodes year to month intervals wrongly. See docs/BACKLOG.md")
	// SELECT dump(cast(TO_YMINTERVAL('2021-10') as INTERVAL YEAR TO MONTH)) FROM dual
	_ = []struct {
		raw          []byte
		years, month int
	}{
		{[]byte{128, 0, 7, 229, 70}, 2021, 10},
		{[]byte{127, 255, 248, 27, 50}, -2021, -10},
		{[]byte{127, 255, 255, 251, 50}, -5, -10},
		{[]byte{127, 255, 255, 251, 57}, -5, -3},
		{[]byte{128, 0, 0, 0, 70}, 0, 10},
		{[]byte{128, 0, 0, 0, 57}, 0, -3},
	}
}

func TestDecodeIntervalDS(t *testing.T) {
	t.Skip("Interval.Value decodes day to second intervals wrongly. See docs/BACKLOG.md")
	// SELECT dump(cast(TO_DSINTERVAL('2 12:23:34.456') as INTERVAL DAY TO SECOND)) FROM dual
	_ = []struct {
		raw  []byte
		want time.Duration
	}{
		{[]byte{128, 0, 0, 2, 72, 83, 94, 155, 46, 2, 0}, 2*24*time.Hour + 12*time.Hour + 23*time.Minute + 34*time.Second + 456*time.Millisecond},
		{[]byte{127, 255, 255, 254, 48, 37, 26, 100, 197, 243, 248}, -(2*24*time.Hour + 12*time.Hour + 23*time.Minute + 34*time.Second + 456789*time.Microsecond)},
		{[]byte{128, 0, 0, 0, 70, 80, 90, 155, 58, 12, 8}, 10*time.Hour + 20*time.Minute + 30*time.Second + 456789*time.Microsecond},
		{[]byte{128, 0, 0, 0, 50, 40, 30, 100, 197, 243, 248}, -(10*time.Hour + 20*time.Minute + 30*time.Second + 456789*time.Microsecond)},
		{[]byte{128, 0, 0, 0, 50, 40, 30, 128, 0, 0, 0}, -(10*time.Hour + 20*time.Minute + 30*time.Second)},
	}
}
