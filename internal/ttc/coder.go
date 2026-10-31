package ttc

import (
	"database/sql"
	"time"

	"github.com/xo/oracle/internal/codec"
	"github.com/xo/oracle/internal/types"
)

// builtinCoder returns a new coder for v, the zero value of a Go type that
// the driver binds itself, or nil for any other type. A type that a caller
// registers with RegisterType has its coder in the registry instead. The
// nullable forms of the common types are listed, so that a NULL of one binds
// with the type of its value, which an OUT argument needs.
func builtinCoder(v any) codec.OracleParameterCoder {
	switch v.(type) {
	case string, types.String, sql.NullString, sql.Null[string]:
		return &codec.StringParameter{}
	case []byte, types.Raw:
		return &codec.RawParameter{}
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64,
		types.Number,
		sql.NullByte, sql.NullInt16, sql.NullInt32, sql.NullInt64, sql.NullFloat64,
		sql.Null[int], sql.Null[int32], sql.Null[int64], sql.Null[uint64], sql.Null[float32], sql.Null[float64]:
		return &codec.NumberParameter{}
	case bool, types.Bool, sql.NullBool, sql.Null[bool]:
		// a BOOLEAN on 23ai and later, and the NUMBER 1 or 0 before
		return &codec.BoolParameter{}
	case time.Time, types.Date, sql.NullTime, sql.Null[time.Time]:
		return &codec.DateParameter{}
	case types.TimestampTZ, sql.Null[types.TimestampTZ]:
		return &codec.DateParameter{BasicParameter: codec.BasicParameter{DataType: types.TimestampTZDTY, MaxLen: types.MaxLenTimestampTZ}}
	case time.Duration, sql.Null[time.Duration]:
		return &codec.IntervalParameter{BasicParameter: codec.BasicParameter{DataType: types.IntervalDSDTY, MaxLen: types.MaxLenIntervalDS}}
	case types.IntervalYM, sql.Null[types.IntervalYM]:
		return &codec.IntervalParameter{BasicParameter: codec.BasicParameter{DataType: types.IntervalYMDTY, MaxLen: types.MaxLenIntervalYM}}
	case types.Interval:
		return &codec.IntervalParameter{}
	case types.Vector:
		return &codec.VectorParameter{}
	case types.JSON:
		return &codec.JSONParameter{}
	case types.Clob:
		return &codec.ClobParameter{}
	case types.Blob:
		return &codec.BlobParameter{}
	case types.BFile:
		return &codec.BFileParameter{}
	case RefCursor:
		return &codec.CursorParameter{}
	}
	return nil
}
