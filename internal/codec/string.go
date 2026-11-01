package codec

import (
	"github.com/xo/oracle/internal/tns"
	"github.com/xo/oracle/internal/types"
)

type StringParameter struct {
	BasicParameter
	// National makes the string an NVARCHAR2, in the national character
	// set, for an NVarChar and for each element of a slice of them
	National bool
	// clob binds the string when it goes as a temporary CLOB
	clob *ClobParameter
}

func (param *StringParameter) Copy() OracleParameterCoder {
	ret := new(StringParameter)
	*ret = *param
	ret.clob = nil
	return ret
}

func (param *StringParameter) Init() {
	param.SetDefault()
	param.clob = nil
	param.DataType = types.NCHAR
	param.ContFlag = 0x10
	if param.CharsetForm == 0 {
		param.CharsetForm = 1
	}
	if param.National {
		param.CharsetForm = 2
	}
}

func (param *StringParameter) Encode(input any, conn Conn) error {
	param.Init()
	switch temp := input.(type) {
	case types.String:
		if temp.UseNCharset {
			param.CharsetForm = 2
		}
	case *types.String:
		if temp.UseNCharset {
			param.CharsetForm = 2
		}
	}
	conv, err := conn.StringCoder(param.CharsetID, param.CharsetForm)
	if err != nil {
		return err
	}
	if param.CharsetID == 0 {
		param.CharsetID = conv.LangID
	}
	encoder := &types.String{
		Conv: conv,
	}
	encoder.SetDataType(param.DataType)
	err = encoder.SetValue(input)
	if err != nil {
		return err
	}
	if dt := encoder.DataType(); dt != 0 {
		param.DataType = dt
	}
	param.BValue = encoder.Bytes()
	length := int64(len(param.BValue))
	length = max(length, param.MaxLen, param.MaxCharLen)
	param.MaxLen = length
	param.MaxCharLen = length
	length = conn.MaxStringLength()
	if param.MaxLen > length {
		param.DataType = types.LongVarChar
		// 11g and 18c convert a long string to the UTF-16 of a CLOB one
		// packet at a time, and store U+FFFD for a character that the end
		// of a packet splits. 19.33, 21c and later do not. A string with no
		// such character binds as a LONG, and one with them as a temporary
		// CLOB, which a CLOB column stores whole and a LONG column refuses
		// with an error.
		if conn.ServerRelease() < 19 && !isASCII(param.BValue) {
			return param.encodeClob(input, conn)
		}
	} else {
		param.DataType = types.NCHAR
	}
	if param.MaxLen < encoder.MaxLen() {
		param.MaxLen = encoder.MaxLen()
	}
	return nil
}

// encodeClob encodes input as a temporary CLOB.
func (param *StringParameter) encodeClob(input any, conn Conn) error {
	clob := &ClobParameter{}
	clob.CharsetForm = param.CharsetForm
	if err := clob.Encode(input, conn); err != nil {
		return err
	}
	param.clob = clob
	param.BasicParameter = clob.BasicParameter
	return nil
}

// Write writes the locator of the temporary CLOB, when the string went as
// one, and the string otherwise.
func (param *StringParameter) Write(session tns.SessionWriter) error {
	if param.clob != nil {
		return param.clob.Write(session)
	}
	return param.BasicParameter.Write(session)
}

func isASCII(b []byte) bool {
	for _, c := range b {
		if c >= 0x80 {
			return false
		}
	}
	return true
}

func (param *StringParameter) Decode(conn Conn) (any, error) {
	conv, err := conn.StringCoder(param.CharsetID, param.CharsetForm)
	if err != nil {
		return nil, err
	}
	if param.CharsetID == 0 {
		param.CharsetID = conv.LangID
	}
	decoder := &types.String{
		Conv: conv,
	}
	decoder.SetBytes(param.BValue)
	decoder.SetDataType(param.DataType)
	return decoder.Value()
}

func (param *StringParameter) Read(session tns.SessionReader) error {
	var err error
	param.BValue, err = param.BasicRead(session)
	return err
}
