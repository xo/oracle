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

func (param *BoolParameter) Encode(input any, _ Conn) error {
	param.Init()
	encoder := &types.Bool{}
	encoder.SetDataType(param.DataType)
	err := encoder.SetValue(input)
	if err != nil {
		return err
	}
	if dt := encoder.GetDataType(); dt != 0 {
		param.DataType = dt
	}
	if param.MaxLen < encoder.GetMaxLen() {
		param.MaxLen = encoder.GetMaxLen()
	}
	param.BValue = encoder.Bytes()
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
