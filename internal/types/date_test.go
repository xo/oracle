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
