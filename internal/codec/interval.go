package codec

import (
	"github.com/xo/oracle/internal/tns"
	"github.com/xo/oracle/internal/types"
)

type IntervalParameter struct {
	BasicParameter
}

func (param *IntervalParameter) Copy() OracleParameterCoder {
	ret := new(IntervalParameter)
	*ret = *param
	return ret
}

func (param *IntervalParameter) Init() {
	param.SetDefault()
}

func (param *IntervalParameter) Encode(input any, _ Conn) error {
	param.Init()
	encoder := &types.Interval{}
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

func (param *IntervalParameter) Decode(_ Conn) (any, error) {
	decoder := &types.Interval{}
	decoder.SetBytes(param.BValue)
	return decoder.Value()
}

func (param *IntervalParameter) Read(session tns.SessionReader) error {
	var err error
	param.BValue, err = param.BasicRead(session)
	return err
}
