package codec

import (
	"github.com/xo/oracle/internal/tns"
	"github.com/xo/oracle/internal/types"
)

type NumberParameter struct {
	BasicParameter
}

func (param *NumberParameter) Copy() OracleParameterCoder {
	ret := new(NumberParameter)
	*ret = *param
	return ret
}

func (param *NumberParameter) Init() {
	param.SetDefault()
	if param.MaxLen == 0 {
		param.MaxLen = 0x16
	}
}

func (param *NumberParameter) Encode(input any, _ Conn) error {
	param.Init()
	encoder := types.Number{}
	// a coder of BINARY_FLOAT or BINARY_DOUBLE, as an attribute of an
	// object has, encodes in that form. go-ora encoded a NUMBER, labeled
	// it NUMBER, and the server read its bytes as a binary float
	if param.DataType == types.IBFLOAT || param.DataType == types.IBDOUBLE {
		encoder.SetDataType(param.DataType)
	}
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

func (param *NumberParameter) Decode(_ Conn) (any, error) {
	decoder := types.Number{}
	decoder.SetBytes(param.BValue)
	decoder.SetDataType(param.DataType)
	return decoder.Value()
}

func (param *NumberParameter) Read(session tns.SessionReader) error {
	var err error
	param.BValue, err = param.BasicRead(session)
	return err
}
