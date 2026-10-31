package codec

import (
	"github.com/xo/oracle/internal/tns"
	"github.com/xo/oracle/internal/types"
)

type DateParameter struct {
	BasicParameter
}

func (param *DateParameter) Copy() OracleParameterCoder {
	ret := new(DateParameter)
	*ret = *param
	return ret
}

func (param *DateParameter) Init() {
	param.SetDefault()
}

func (param *DateParameter) Encode(input any, conn Conn) error {
	param.Init()
	encoder := types.Date{}
	encoder.AsUTC = conn.SendTimeAsUTC()
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

func (param *DateParameter) Decode(conn Conn) (any, error) {
	decoder := types.Date{}
	if param.DataType == types.TimestampLTZ || param.DataType == types.TimestampLTZDTY {
		zone, err := conn.DBTimeZone()
		if err != nil {
			return nil, err
		}
		decoder.DBTimeZone = zone
	}
	decoder.SetBytes(param.BValue)
	decoder.SetDataType(param.DataType)
	return decoder.Value()
}

func (param *DateParameter) Read(session tns.SessionReader) error {
	var err error
	param.BValue, err = param.BasicRead(session)
	return err
}
