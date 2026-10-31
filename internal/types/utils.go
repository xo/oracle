package types

import (
	"encoding/binary"
	"time"
)

const (
	MaxLenNumber      int64 = 0x16
	MaxLenBFloat      int64 = 0x4
	MaxLenBDouble     int64 = 0x8
	MaxLenDate        int64 = 0x7
	MaxLenTimestamp   int64 = 0xB
	MaxLenTimestampTZ int64 = 0xD
	MaxLenIntervalYM  int64 = 0x5
	MaxLenIntervalDS  int64 = 0xB
	MaxLenBFile       int64 = 4000
)

func xorBuffer(buffer []byte, length int) {
	if len(buffer) < length {
		length = len(buffer)
	}
	for i := 0; i < length; i++ {
		buffer[i] = ^buffer[i]
	}
}

func putDate(buffer []byte, ti *time.Time) {
	buffer[0] = uint8(ti.Year()/100 + 100)
	buffer[1] = uint8(ti.Year()%100 + 100)
	buffer[2] = uint8(ti.Month())
	buffer[3] = uint8(ti.Day())
	buffer[4] = uint8(ti.Hour() + 1)
	buffer[5] = uint8(ti.Minute() + 1)
	buffer[6] = uint8(ti.Second() + 1)
}

func putTimestamp(buffer []byte, ti *time.Time) {
	putDate(buffer, ti)
	binary.BigEndian.PutUint32(buffer[7:11], uint32(ti.Nanosecond()))
}

func isEqualLoc(zone1, zone2 *time.Location) bool {
	t := time.Now()
	t1 := t.In(zone1)
	t2 := t.In(zone2)
	name1, offset1 := t1.Zone()
	name2, offset2 := t2.Zone()
	return name1 == name2 && offset1 == offset2
}
