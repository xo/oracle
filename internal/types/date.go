package types

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Date struct {
	Basic
	AsUTC bool
	// DBTimeZone is the zone of the database, which the wall clock of a
	// TIMESTAMP WITH LOCAL TIME ZONE is in. Nil is UTC.
	DBTimeZone *time.Location
}

func (date *Date) MaxLen() int64 {
	switch date.dataType {
	case DATE:
		return MaxLenDate
	case TIMESTAMP:
		return MaxLenTimestamp
	default:
		return MaxLenTimestampTZ
	}
}
func (date *Date) Value() (any, error) {
	if len(date.bValue) == 0 {
		return nil, nil
	}
	return date.decode()
}
func (date *Date) encode(input time.Time) (bytes []byte, err error) {
	if date.dataType == 0 {
		// a DATE holds no fraction of a second, so a value with one binds
		// as a TIMESTAMP. A value with none binds as a DATE, so that a
		// DATE column that it meets keeps its index
		date.dataType = DATE
		if input.Nanosecond() != 0 {
			date.dataType = TIMESTAMP
		}
	}
	switch date.dataType {
	case DATE:
		bytes = make([]byte, MaxLenDate)
		putDate(bytes, &input)
	case TIMESTAMP:
		bytes = make([]byte, MaxLenTimestamp)
		putTimestamp(bytes, &input)
	case TIMESTAMPTZ, TimestampTZDTY, TimestampLTZ, TimestampLTZDTY:
		var val time.Time
		if date.AsUTC {
			val = input.UTC()
		} else {
			val = input
		}
		bytes = make([]byte, MaxLenTimestamp)
		putTimestamp(bytes, &val)
		zoneLoc := input.Location()
		zoneID := 0
		for key, val := range oracleZones {
			if strings.EqualFold(zoneLoc.String(), val) {
				zoneID = key
				break
			}
		}
		if zoneID > 0 {
			zone1 := uint8((zoneID&0x1FC0)>>6) | 0x80
			zone2 := uint8((zoneID & 0x3F) << 2)
			bytes = append(bytes, zone1, zone2)

		} else {
			_, offset := input.Zone()
			zone1 := uint8(offset/3600) + 20
			zone2 := uint8((offset/60)%60) + 60
			bytes = append(bytes, zone1, zone2)
		}
		if !date.AsUTC {
			if bytes[11]&0x80 != 0 {
				bytes[12] |= 1
				if input.IsDST() {
					bytes[12] |= 2
				}
			} else {
				bytes[11] |= 0x40
			}
		}
	}
	return
}

func (date *Date) SetValue(input any) (err error) {
	input, verr := ValueOf(input)
	if verr != nil {
		return verr
	}
	defer func(input *Date) {
		if input.dataType == 0 {
			input.dataType = DATE
		}
	}(date)
	if input == nil {
		date.bValue = nil
		return
	}
	switch data := input.(type) {
	case Date:
		if date.AsUTC == data.AsUTC {
			*date = data
		} else {
			temp, err := data.decode()
			if err != nil {
				return err
			}
			date.dataType = data.dataType
			return date.SetValue(temp)
		}
	case *Date:
		if date.AsUTC == data.AsUTC {
			*date = *data
		} else {
			temp, err := data.decode()
			if err != nil {
				return err
			}
			date.dataType = data.dataType
			return date.SetValue(temp)
		}
	case TimestampTZ:
		date.dataType = TimestampTZDTY
		date.bValue, err = date.encode(time.Time(data))
	case time.Time:
		date.bValue, err = date.encode(data)
	case *time.Time:
		date.bValue, err = date.encode(*data)
	default:
		err = fmt.Errorf("cannot set value of type %T into date/time", input)
	}

	return
}
func (date *Date) decode() (output time.Time, err error) {
	if len(date.bValue) < int(MaxLenDate) {
		err = errors.New("abnormal data representation for date/time")
		return
	}
	year := (int(date.bValue[0]) - 100) * 100
	year += int(date.bValue[1]) - 100
	nanoSec := 0
	tzHour := 0
	tzMin := 0
	if len(date.bValue) > 10 {
		nanoSec = int(binary.BigEndian.Uint32(date.bValue[7:11]))
	}
	if len(date.bValue) > 11 {
		tzHour = int(date.bValue[11]&0x3F) - 20
	}
	if len(date.bValue) > 12 {
		tzMin = int(date.bValue[12]) - 60
	}
	output = time.Date(year, time.Month(date.bValue[2]), int(date.bValue[3]),
		int(date.bValue[4]-1), int(date.bValue[5]-1), int(date.bValue[6]-1), nanoSec, time.UTC)
	if tzHour != 0 || tzMin != 0 {

		var zone *time.Location
		var timeInZone bool
		if date.bValue[11]&0x80 != 0 {
			zone, _ = regionZone(date.bValue[11], date.bValue[12])
			timeInZone = date.bValue[12]&0x1 == 1
		} else {
			timeInZone = date.bValue[11]&0x40 == 0x40
		}
		if zone == nil {
			zone = time.FixedZone(fmt.Sprintf("%+03d:%02d", tzHour, tzMin), tzHour*60*60+tzMin*60)
		}
		if timeInZone {
			output = time.Date(year, time.Month(date.bValue[2]), int(date.bValue[3]),
				int(date.bValue[4]-1), int(date.bValue[5]-1), int(date.bValue[6]-1), nanoSec, zone)
		} else {
			output = output.In(zone)
		}
	}
	// a DATE and a TIMESTAMP hold no zone, and stay in UTC
	switch date.dataType {
	case TimestampLTZ, TimestampLTZDTY:
		if date.DBTimeZone != nil {
			output = time.Date(output.Year(), output.Month(), output.Day(),
				output.Hour(), output.Minute(), output.Second(), output.Nanosecond(), date.DBTimeZone)
		}
	}
	return
}

// regionZone returns the zone of a region of Oracle, whose code is in the 2
// bytes of a zone with the high bit of hi set.
func regionZone(hi, lo byte) (*time.Location, error) {
	code := int(hi&0x7F)<<6 | int(lo&0xFC)>>2
	name, ok := oracleZones[code]
	if !ok {
		return nil, fmt.Errorf("the time zone region %d is not known", code)
	}
	return time.LoadLocation(name)
}

// DBTimeZone returns the zone of the database from the image of 11 bytes that
// the data type negotiation sends. It holds a region in bytes 2 and 3, as a
// TIMESTAMP WITH TIME ZONE does, or an offset in bytes 4 to 6. An offset of
// zero is UTC, sijms/go-ora#655.
func DBTimeZone(image []byte) (*time.Location, error) {
	if len(image) < 7 {
		return nil, errors.New("the image of the database time zone is short")
	}
	if image[2]&0x80 != 0 {
		return regionZone(image[2], image[3])
	}
	hours, mins, secs := int(image[4])-60, int(image[5])-60, int(image[6])-60
	offset := hours*60*60 + mins*60 + secs
	if offset == 0 {
		return time.UTC, nil
	}
	return time.FixedZone(fmt.Sprintf("%+03d:%02d", hours, mins), offset), nil
}
func (date *Date) Scan(value any) (err error) {
	return date.SetValue(value)
}
