package codec

import (
	"github.com/xo/oracle/internal/codec/oson"
	"github.com/xo/oracle/internal/tns"
	"github.com/xo/oracle/internal/types"
)

type JSONParameter struct {
	locator types.Locator
	lobParameter
	isQueueMessage bool
}

func (param *JSONParameter) Copy() OracleParameterCoder {
	ret := new(JSONParameter)
	*ret = *param
	return ret
}

func (param *JSONParameter) Init() {
	param.SetDefault()
	param.DataType = types.JSONType
}

func (param *JSONParameter) Encode(input any, _ Conn) (err error) {
	param.Init()
	encoder := &types.JSON{}
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

func (param *JSONParameter) Decode(_ Conn) (any, error) {
	decoder := &types.JSON{}
	decoder.Coder = &oson.Oson{}
	decoder.SetStreamer(param.streamer)
	decoder.SetBytes(param.BValue)
	return decoder, nil
}

func (param *JSONParameter) Write(session tns.SessionWriter) error {
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

func (param *JSONParameter) SetAQMessage() {
	param.isQueueMessage = true
}
