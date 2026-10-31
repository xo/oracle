package codec

import (
	"github.com/xo/oracle/internal/codec/oson"
	"github.com/xo/oracle/internal/tns"
	"github.com/xo/oracle/internal/types"
)

type JsonParameter struct {
	locator types.Locator
	lobParameter
	isQueueMessage bool
}

func (param *JsonParameter) Copy() OracleParameterCoder {
	ret := new(JsonParameter)
	*ret = *param
	return ret
}

func (param *JsonParameter) Init() {
	param.SetDefault()
	param.DataType = types.JSON
}

func (param *JsonParameter) Encode(input any, _ IConnection) (err error) {
	param.Init()
	encoder := &types.Json{}
	encoder.Coder = &oson.Oson{}
	encoder.SetDataType(param.DataType)
	err = encoder.SetValue(input)
	if err != nil {
		return
	}
	if dt := encoder.GetDataType(); dt != 0 {
		param.DataType = dt
	}
	if param.MaxLen < encoder.GetMaxLen() {
		param.MaxLen = encoder.GetMaxLen()
	}
	param.BValue = encoder.Bytes()
	param.locator = encoder.GetLocator()
	return
}

func (param *JsonParameter) Decode(_ IConnection) (any, error) {
	decoder := &types.Json{}
	decoder.Coder = &oson.Oson{}
	decoder.SetStreamer(param.streamer)
	decoder.SetBytes(param.BValue)
	return decoder, nil
}

func (param *JsonParameter) Write(session tns.SessionWriter) error {
	if param.locator != nil {
		session.PutUint(len(param.locator), 4, true, true)
		if param.isQueueMessage {
			session.PutBytes(param.locator...)
		} else {
			session.PutClr(param.locator)
		}
		session.PutClr(param.BValue)
	} else {
		session.PutClr(param.locator)
	}
	return nil
}

func (param *JsonParameter) SetAQMessage() {
	param.isQueueMessage = true
}
