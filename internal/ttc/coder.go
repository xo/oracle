package ttc

import (
	"database/sql"
	"time"

	"github.com/xo/oracle/internal/codec"
	oraTypes "github.com/xo/oracle/internal/types"
)

// builtinCoder returns a new coder for v, the zero value of a Go type that
// the driver binds itself, or nil for any other type. A type that a caller
// registers with RegisterType has its coder in the registry instead. The
// nullable forms of the common types are listed, so that a NULL of one binds
// with the type of its value, which an OUT argument needs.
func builtinCoder(v any) codec.OracleParameterCoder {
	switch v.(type) {
	case string, oraTypes.String, sql.NullString, sql.Null[string]:
		return &codec.StringParameter{}
	case []byte, oraTypes.Raw:
		return &codec.RawParameter{}
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64,
		oraTypes.Number,
		sql.NullByte, sql.NullInt16, sql.NullInt32, sql.NullInt64, sql.NullFloat64,
		sql.Null[int], sql.Null[int32], sql.Null[int64], sql.Null[uint64], sql.Null[float32], sql.Null[float64]:
		return &codec.NumberParameter{}
	case oraTypes.Bool:
		return &codec.BoolParameter{}
	case time.Time, oraTypes.Date, sql.NullTime, sql.Null[time.Time]:
		return &codec.DateParameter{}
	case oraTypes.TimestampTZ, sql.Null[oraTypes.TimestampTZ]:
		return &codec.DateParameter{BasicParameter: codec.BasicParameter{DataType: oraTypes.TimestampTZDTY, MaxLen: oraTypes.MaxLenTimestampTZ}}
	case time.Duration, sql.Null[time.Duration]:
		return &codec.IntervalParameter{BasicParameter: codec.BasicParameter{DataType: oraTypes.IntervalDSDTY, MaxLen: oraTypes.MaxLenIntervalDS}}
	case oraTypes.IntervalYM, sql.Null[oraTypes.IntervalYM]:
		return &codec.IntervalParameter{BasicParameter: codec.BasicParameter{DataType: oraTypes.IntervalYMDTY, MaxLen: oraTypes.MaxLenIntervalYM}}
	case oraTypes.Interval:
		return &codec.IntervalParameter{}
	case oraTypes.Vector:
		return &codec.VectorParameter{}
	case oraTypes.JSON:
		return &codec.JSONParameter{}
	case oraTypes.Clob:
		return &codec.ClobParameter{}
	case oraTypes.Blob:
		return &codec.BlobParameter{}
	case oraTypes.BFile:
		return &codec.BFileParameter{}
	case RefCursor:
		return &codec.CursorParameter{}
	}
	return nil
}
