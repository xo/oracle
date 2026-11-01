package codec

import (
	"github.com/xo/oracle/internal/types"
)

type BlobParameter struct {
	lobParameter
}

func (param *BlobParameter) Copy() OracleParameterCoder {
	ret := new(BlobParameter)
	*ret = *param
	return ret
}
func (param *BlobParameter) Init() {
	param.SetDefault()
	param.DataType = types.OCIBlobLocator
}
func (param *BlobParameter) Encode(input any, conn Conn) (err error) {
	param.Init()
	if isNilPointer(input) {
		return nil
	}
	encoder := &types.Blob{}
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
func (param *BlobParameter) Decode(_ Conn) (any, error) {
	decoder := &types.Blob{}
	decoder.SetStreamer(param.streamer)
	decoder.SetBytes(param.BValue)
	return decoder, nil
}
