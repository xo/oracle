package codec

import (
	"github.com/xo/oracle/internal/tns"
	"github.com/xo/oracle/internal/types"
)

type CursorParameter struct {
	BasicParameter
}

func (param *CursorParameter) Copy() OracleParameterCoder {
	ret := new(CursorParameter)
	*ret = *param
	return ret
}

func (param *CursorParameter) Init() {
	param.SetDefault()
	param.DataType = types.REFCURSOR
}

func (param *CursorParameter) Encode(input any, _ Conn) error {
	param.Init()
	param.BValue = nil
	return nil
}

func (param *CursorParameter) Write(session tns.SessionWriter) error {
	session.PutBytes(1, 0)
	return nil
}

func (param *CursorParameter) Read(session tns.SessionReader) error {
	return nil
}

func (param *CursorParameter) Decode(_ Conn) (any, error) {
	return nil, nil
}
