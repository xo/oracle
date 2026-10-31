package types

import (
	"database/sql"
	"encoding/binary"
	"reflect"
	"time"
)

var (
	TyFloat64     = reflect.TypeFor[float64]()
	TyFloat32     = reflect.TypeFor[float32]()
	TyInt         = reflect.TypeFor[int]()
	TyInt8        = reflect.TypeFor[int8]()
	TyInt16       = reflect.TypeFor[int16]()
	TyInt32       = reflect.TypeFor[int32]()
	TyInt64       = reflect.TypeFor[int64]()
	TyUint        = reflect.TypeFor[uint]()
	TyUint8       = reflect.TypeFor[uint8]()
	TyUint16      = reflect.TypeFor[uint16]()
	TyUint32      = reflect.TypeFor[uint32]()
	TyUint64      = reflect.TypeFor[uint64]()
	TyBool        = reflect.TypeFor[bool]()
	TyBytes       = reflect.TypeFor[[]byte]()
	TyString      = reflect.TypeFor[string]()
	TyTime        = reflect.TypeFor[time.Time]()
	TyDuration    = reflect.TypeFor[time.Duration]()
	TyIntervalYM  = reflect.TypeFor[IntervalYM]()
	TyNullByte    = reflect.TypeFor[sql.NullByte]()
	TyNullInt16   = reflect.TypeFor[sql.NullInt16]()
	TyNullInt32   = reflect.TypeFor[sql.NullInt32]()
	TyNullInt64   = reflect.TypeFor[sql.NullInt64]()
	TyNullFloat64 = reflect.TypeFor[sql.NullFloat64]()
	TyNullBool    = reflect.TypeFor[sql.NullBool]()
	TyNullString  = reflect.TypeFor[sql.NullString]()
	TyNullTime    = reflect.TypeFor[sql.NullTime]()
	TyScanner     = reflect.TypeFor[sql.Scanner]()

	TyNumber   = reflect.TypeFor[Number]()
	TyBoolean  = reflect.TypeFor[Bool]()
	TyVarchar  = reflect.TypeFor[String]()
	TyDate     = reflect.TypeFor[Date]()
	TyInterval = reflect.TypeFor[Interval]()
	TyRaw      = reflect.TypeFor[Raw]()
	TyVector   = reflect.TypeFor[Vector]()
	TyJson     = reflect.TypeFor[Json]()
	TyClob     = reflect.TypeFor[Clob]()
	TyBlob     = reflect.TypeFor[Blob]()
	TyBFile    = reflect.TypeFor[BFile]()
	TyObject   = reflect.TypeFor[Object]()
)

const (
	MaxLenNumber      int64 = 0x16
	MaxLenBFloat      int64 = 0x4
	MaxLenBDouble     int64 = 0x8
	MaxLenDate        int64 = 0x7
	MaxLenTimeStamp   int64 = 0xB
	MaxLenTimeStampTZ int64 = 0xD
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
