package types

import (
	"encoding/binary"
	"fmt"
	"time"
)

// IntervalYM is an INTERVAL YEAR TO MONTH. For a negative interval both
// fields are zero or negative, as Oracle stores them.
type IntervalYM struct {
	Years  int
	Months int
}

// String returns the interval as Oracle prints it, such as +02-03 or -00-03.
func (iv IntervalYM) String() string {
	sign := "+"
	y, m := iv.Years, iv.Months
	if y < 0 || m < 0 {
		sign = "-"
		y, m = -y, -m
	}
	return fmt.Sprintf("%s%02d-%02d", sign, y, m)
}

// Interval is the wire form of INTERVAL YEAR TO MONTH and INTERVAL DAY TO
// SECOND. Value returns an IntervalYM for the first and a time.Duration for
// the second.
type Interval struct {
	Basic
}

// NewYearMonthInterval returns an INTERVAL YEAR TO MONTH.
func NewYearMonthInterval(years, months int) (*Interval, error) {
	ret := &Interval{}
	ret.dataType = IntervalYMDTY
	return ret, ret.SetValue(IntervalYM{Years: years, Months: months})
}

// NewDaySecondInterval returns an INTERVAL DAY TO SECOND.
func NewDaySecondInterval(days, hours, minutes, seconds, nanoseconds int) (*Interval, error) {
	ret := &Interval{}
	ret.dataType = IntervalDSDTY
	d := time.Duration(days)*24*time.Hour + time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second +
		time.Duration(nanoseconds)
	return ret, ret.SetValue(d)
}

func (interval *Interval) GetMaxLen() int64 {
	switch interval.dataType {
	case IntervalYMDTY:
		return MaxLenIntervalYM
	default:
		return MaxLenIntervalDS
	}
}

// offset is the bias of each field of four bytes, which Oracle stores as
// the value plus 2^31. A field of one byte has a bias of 60.
const offset = 0x80000000

// Value returns an IntervalYM or a time.Duration, or nil for NULL. Each
// field of the bytes is signed, so a negative interval has every field zero
// or negative.
func (interval *Interval) Value() (any, error) {
	b := interval.bValue
	if len(b) == 0 {
		return nil, nil
	}
	typeID := interval.dataType
	if typeID == 0 {
		switch {
		case len(b) >= int(MaxLenIntervalDS):
			typeID = IntervalDSDTY
		case len(b) >= int(MaxLenIntervalYM):
			typeID = IntervalYMDTY
		}
	}
	switch typeID {
	case IntervalYMDTY:
		if len(b) < int(MaxLenIntervalYM) {
			return nil, fmt.Errorf("decoding an interval year to month of %d bytes", len(b))
		}
		return IntervalYM{
			Years:  int(int64(binary.BigEndian.Uint32(b)) - offset),
			Months: int(b[4]) - 60,
		}, nil
	case IntervalDSDTY:
		if len(b) < int(MaxLenIntervalDS) {
			return nil, fmt.Errorf("decoding an interval day to second of %d bytes", len(b))
		}
		days := int64(binary.BigEndian.Uint32(b)) - offset
		nanos := int64(binary.BigEndian.Uint32(b[7:])) - offset
		return time.Duration(days)*24*time.Hour +
			time.Duration(int(b[4])-60)*time.Hour +
			time.Duration(int(b[5])-60)*time.Minute +
			time.Duration(int(b[6])-60)*time.Second +
			time.Duration(nanos), nil
	default:
		return nil, fmt.Errorf("decoding an interval of the type %d", typeID)
	}
}

// encodeYM writes iv as an INTERVAL YEAR TO MONTH.
func (interval *Interval) encodeYM(iv IntervalYM) {
	b := make([]byte, 5)
	binary.BigEndian.PutUint32(b, uint32(int64(iv.Years)+offset))
	b[4] = uint8(iv.Months + 60)
	interval.dataType = IntervalYMDTY
	interval.bValue = b
}

// encodeDS writes d as an INTERVAL DAY TO SECOND. Each field takes the sign
// of d.
func (interval *Interval) encodeDS(d time.Duration) {
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	minutes := d / time.Minute
	d -= minutes * time.Minute
	seconds := d / time.Second
	d -= seconds * time.Second
	b := make([]byte, 11)
	binary.BigEndian.PutUint32(b, uint32(int64(days)+offset))
	b[4] = uint8(int(hours) + 60)
	b[5] = uint8(int(minutes) + 60)
	b[6] = uint8(int(seconds) + 60)
	binary.BigEndian.PutUint32(b[7:], uint32(int64(d)+offset))
	interval.dataType = IntervalDSDTY
	interval.bValue = b
}

// SetValue sets the interval from an IntervalYM, a time.Duration or another
// Interval.
func (interval *Interval) SetValue(input any) error {
	input, verr := ValueOf(input)
	if verr != nil {
		return verr
	}
	switch data := input.(type) {
	case nil:
		interval.bValue = nil
	case Interval:
		*interval = data
	case *Interval:
		*interval = *data
	case IntervalYM:
		interval.encodeYM(data)
	case *IntervalYM:
		interval.encodeYM(*data)
	case time.Duration:
		interval.encodeDS(data)
	case *time.Duration:
		interval.encodeDS(*data)
	default:
		return fmt.Errorf("cannot set value of type %T into interval", input)
	}
	return nil
}

func (interval *Interval) Scan(value any) error {
	return interval.SetValue(value)
}
