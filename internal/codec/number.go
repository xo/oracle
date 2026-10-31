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

func (param *NumberParameter) Encode(input any, _ IConnection) error {
	param.Init()
	encoder := types.Number{}
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

func (param *NumberParameter) Decode(_ IConnection) (any, error) {
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
