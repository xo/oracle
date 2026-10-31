package types

import (
	"testing"
	"time"
)

// TestDateBindType checks that a time.Time with a fraction of a second binds
// as a TIMESTAMP, which keeps it, and one with none as a DATE.
func TestDateBindType(t *testing.T) {
	tests := []struct {
		in   time.Time
		want uint16
		size int
	}{
		{time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC), DATE, int(MaxLenDate)},
		{time.Date(2020, 1, 2, 3, 4, 5, 900_000_000, time.UTC), TIMESTAMP, int(MaxLenTimestamp)},
		{time.Date(2020, 1, 2, 3, 4, 5, 1, time.UTC), TIMESTAMP, int(MaxLenTimestamp)},
	}
	for _, test := range tests {
		var d Date
		if err := d.SetValue(test.in); err != nil {
			t.Fatal(err)
		}
		if d.DataType() != test.want || len(d.Bytes()) != test.size {
			t.Errorf("%v: got type %d of %d bytes, want %d of %d", test.in, d.DataType(), len(d.Bytes()), test.want, test.size)
		}
		got, err := d.Value()
		if err != nil {
			t.Fatal(err)
		}
		if !got.(time.Time).Equal(test.in) {
			t.Errorf("%v: decodes as %v", test.in, got)
		}
	}
}

// TestDBTimeZone reads images of DBTIMEZONE: +00:00 and Europe/London as 11g
// and 26ai sent them, and -05:00 in the same form. A region was read as an
// offset of 121 hours, sijms/go-ora#655.
func TestDBTimeZone(t *testing.T) {
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Skip(err)
	}
	for _, test := range []struct {
		image []byte
		want  *time.Location
	}{
		{[]byte{0x80, 0x00, 0x00, 0x00, 0x3c, 0x3c, 0x3c, 0x80, 0x00, 0x00, 0x00}, time.UTC},
		{[]byte{0x80, 0x00, 0x85, 0xc4, 0xb5, 0x3c, 0x3c, 0x80, 0x00, 0x00, 0x00}, london},
		{[]byte{0x80, 0x00, 0x00, 0x00, 0x37, 0x3c, 0x3c, 0x80, 0x00, 0x00, 0x00}, time.FixedZone("-05:00", -5*60*60)},
	} {
		got, err := DBTimeZone(test.image)
		if err != nil {
			t.Fatal(err)
		}
		at := time.Date(2025, 7, 1, 12, 0, 0, 0, time.UTC)
		if got.String() != test.want.String() || at.In(got).Format(time.RFC3339) != at.In(test.want).Format(time.RFC3339) {
			t.Errorf("% x: got %v, want %v", test.image, got, test.want)
		}
	}
}
