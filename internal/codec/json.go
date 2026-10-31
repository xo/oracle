package codec

import (
	"fmt"

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
	encoder.SetDataType(param.DataType)
	err = encoder.SetValue(input)
	if err != nil {
		return
	}
	if text := encoder.Text(); text != nil {
		b, err := oson.EncodeText(text)
		if err != nil {
			return err
		}
		encoder.SetOSON(b)
	}
	if dt := encoder.DataType(); dt != 0 {
		param.DataType = dt
	}
	if param.MaxLen < encoder.MaxLen() {
		param.MaxLen = encoder.MaxLen()
	}
	param.BValue = encoder.Bytes()
	param.locator = encoder.Locator()
	return
}

func (param *JSONParameter) Decode(_ Conn) (any, error) {
	decoder := &types.JSON{}
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

// DecodeOSON sets the text of js from the OSON bytes that it holds, when it
// holds them. A JSON that the server streams holds a locator until it is
// loaded, and the driver calls DecodeOSON again after the load.
func DecodeOSON(js *types.JSON) error {
	b := js.Bytes()
	if len(b) == 0 || js.Text() != nil {
		return nil
	}
	text, err := oson.DecodeText(b)
	if err != nil {
		return fmt.Errorf("decoding OSON: %w", err)
	}
	return js.SetValue(text)
}
