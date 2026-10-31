package codec

import (
	"github.com/xo/oracle/internal/tns"
	"github.com/xo/oracle/internal/types"
)

type BoolParameter struct {
	BasicParameter
}

func (param *BoolParameter) Copy() OracleParameterCoder {
	ret := new(BoolParameter)
	*ret = *param
	return ret
}

func (param *BoolParameter) Init() {
	param.SetDefault()
	param.DataType = types.BOOLEAN
}

func (param *BoolParameter) Encode(input any, conn Conn) error {
	param.Init()
	if conn != nil && conn.ServerRelease() < 23 {
		return param.encodeNumber(input, conn)
	}
	encoder := &types.Bool{}
	encoder.SetDataType(param.DataType)
	err := encoder.SetValue(input)
	if err != nil {
		return err
	}
	if dt := encoder.DataType(); dt != 0 {
		param.DataType = dt
	}
	if param.MaxLen < encoder.MaxLen() {
		param.MaxLen = encoder.MaxLen()
	}
	param.BValue = encoder.Bytes()
	return nil
}

// encodeNumber encodes input as the NUMBER 1 or 0, for a server before 23ai,
// whose SQL has no BOOLEAN.
func (param *BoolParameter) encodeNumber(input any, conn Conn) error {
	var b types.Bool
	if err := b.SetValue(input); err != nil {
		return err
	}
	v, err := b.Value()
	if err != nil {
		return err
	}
	var n any
	switch v {
	case true:
		n = int64(1)
	case false:
		n = int64(0)
	}
	num := &NumberParameter{}
	if err := num.Encode(n, conn); err != nil {
		return err
	}
	param.BasicParameter = num.BasicParameter
	return nil
}

func (param *BoolParameter) Decode(_ Conn) (any, error) {
	decoder := &types.Bool{}
	decoder.SetBytes(param.BValue)
	decoder.SetDataType(param.DataType)
	return decoder.Value()
}

func (param *BoolParameter) Read(session tns.SessionReader) error {
	var err error
	param.BValue, err = param.BasicRead(session)
	return err
}
