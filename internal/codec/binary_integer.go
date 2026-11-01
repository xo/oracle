package codec

import (
	"encoding/binary"
	"fmt"
	"math"
	"reflect"

	"github.com/xo/oracle/internal/tns"
	"github.com/xo/oracle/internal/types"
)

// BinaryIntegerParameter codes a PLS_INTEGER, a BINARY_INTEGER or a BOOLEAN
// that is an attribute of a PL/SQL record or an element of a collection of a
// package. The image of an object holds each one as a length of 4 and four
// bytes, big endian, as python-oracledb writes it. A BOOLEAN is 1 or 0. A
// NULL is 0xFF.
type BinaryIntegerParameter struct {
	BasicParameter
	// Boolean is a BOOLEAN, which decodes as a bool.
	Boolean bool
}

func (param *BinaryIntegerParameter) Copy() OracleParameterCoder {
	ret := new(BinaryIntegerParameter)
	*ret = *param
	return ret
}

func (param *BinaryIntegerParameter) Init() {
	param.SetDefault()
	param.DataType = types.BInteger
	if param.Boolean {
		param.DataType = types.BOOLEAN
	}
	param.MaxLen = 4
}

func (param *BinaryIntegerParameter) Encode(input any, _ Conn) error {
	param.Init()
	input, err := types.ValueOf(input)
	if err != nil || input == nil {
		return err
	}
	var n int64
	v := reflect.ValueOf(input)
	switch {
	case v.Kind() == reflect.Bool:
		if v.Bool() {
			n = 1
		}
	case param.Boolean:
		return fmt.Errorf("encoding a PL/SQL BOOLEAN: the value is of the type %T, not bool: %w", input, tns.ErrInvalidValue)
	case v.CanInt():
		n = v.Int()
	case v.CanUint() && v.Uint() <= math.MaxInt32:
		n = int64(v.Uint())
	default:
		return fmt.Errorf("encoding a PL/SQL BINARY_INTEGER: the value is of the type %T, not an integer: %w", input, tns.ErrInvalidValue)
	}
	if n < math.MinInt32 || n > math.MaxInt32 {
		return fmt.Errorf("encoding a PL/SQL BINARY_INTEGER: %d is out of range: %w", n, tns.ErrInvalidValue)
	}
	param.BValue = binary.BigEndian.AppendUint32(nil, uint32(int32(n)))
	return nil
}

func (param *BinaryIntegerParameter) Write(session tns.SessionWriter) error {
	if param.BValue == nil {
		session.PutBytes(0xFF)
		return nil
	}
	session.PutFixedClr(param.BValue)
	return nil
}

func (param *BinaryIntegerParameter) Read(session tns.SessionReader) error {
	var err error
	param.BValue, err = session.ReadFixedCLR()
	return err
}

func (param *BinaryIntegerParameter) Decode(_ Conn) (any, error) {
	switch {
	case param.BValue == nil:
		return nil, nil
	case len(param.BValue) != 4:
		return nil, fmt.Errorf("decoding a PL/SQL BINARY_INTEGER of %d bytes, not 4: %w", len(param.BValue), tns.ErrInvalidValue)
	}
	n := int32(binary.BigEndian.Uint32(param.BValue))
	if param.Boolean {
		return n != 0, nil
	}
	return int64(n), nil
}
