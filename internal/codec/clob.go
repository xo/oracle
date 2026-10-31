package codec

import (
	"github.com/xo/oracle/internal/types"
)

type ClobParameter struct {
	//UseNClob  bool
	lobParameter
}

func (param *ClobParameter) Copy() OracleParameterCoder {
	ret := new(ClobParameter)
	*ret = *param
	return ret
}

func (param *ClobParameter) Init() {
	param.SetDefault()
	param.DataType = types.OCIClobLocator
	if param.CharsetForm == 0 {
		param.CharsetForm = 1
	}

}

func (param *ClobParameter) Encode(input any, conn Conn) (err error) {
	param.Init()

	switch input := input.(type) {
	case types.String:
		if input.UseNCharset {
			param.CharsetForm = 2
		}
	case *types.String:
		if input.UseNCharset {
			param.CharsetForm = 2
		}
	case types.Clob:
		if input.UseNCharset {
			param.CharsetForm = 2
		}
	case *types.Clob:
		if input.UseNCharset {
			param.CharsetForm = 2
		}
	}
	encoder := &types.Clob{}
	encoder.Conv, err = conn.StringCoder(param.CharsetID, param.CharsetForm)
	if err != nil {
		return
	}
	param.CharsetID = encoder.Conv.LangID
	encoder.SetDataType(param.DataType)
	err = encoder.SetValue(input)
	if err != nil {
		return
	}
	if dt := encoder.DataType(); dt != 0 {
		param.DataType = dt
	}
	if param.MaxLen < encoder.MaxLen() {
		param.MaxLen = encoder.MaxLen()
	}
	if !encoder.IsDataUploaded() && len(encoder.Bytes()) > 0 {
		encoder.SetStreamer(conn.NewLobStreamer())
		err = encoder.Upload()
		if err != nil {
			return
		}
	}
	param.BValue = encoder.Locator()
	return
}

func (param *ClobParameter) Decode(conn Conn) (any, error) {
	decoder := &types.Clob{}
	decoder.SetStreamer(param.streamer)
	decoder.SetBytes(param.BValue)
	locator := param.streamer.Locator()
	var err error
	if locator.IsVarWidthChar() {
		if param.streamer.DatabaseVersionNumber() < 10200 && locator.IsLittleEndian() {
			decoder.Conv, err = conn.StringCoder(2002, 0)
		} else {
			decoder.Conv, err = conn.StringCoder(2000, 0)
		}
	} else {
		decoder.Conv, err = conn.StringCoder(param.CharsetID, param.CharsetForm)
	}
	return decoder, err
}
