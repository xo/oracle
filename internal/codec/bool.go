package codec

import (
	"github.com/xo/oracle/internal/tns"
	"github.com/xo/oracle/internal/types"
)

type BoolParameter struct {
	BasicParameter
}

func (param *BoolParameter) Copy() OracleParameterCoder {
	ret := new(BoolParameter)
	*ret = *param
	return ret
}

func (param *BoolParameter) Init() {
	param.SetDefault()
	param.DataType = types.BOOLEAN
}

func (param *BoolParameter) Encode(input any, conn Conn) error {
	param.Init()
	if conn != nil && conn.ServerRelease() < 23 && !(inPLSQL(conn) && conn.ServerRelease() >= 12) {
		return param.encodeNumber(input, conn)
	}
	encoder := &types.Bool{}
	encoder.SetDataType(param.DataType)
	err := encoder.SetValue(input)
	if err != nil {
		return err
	}
	if dt := encoder.DataType(); dt != 0 {
		param.DataType = dt
	}
	// the size of a BOOLEAN, as python-oracledb binds one. With the size 1
	// that go-ora sent, 18c never answered a block with a BOOLEAN IN and a
	// BOOLEAN OUT
	param.MaxLen = max(param.MaxLen, encoder.MaxLen(), boolSize)
	param.BValue = encoder.Bytes()
	return nil
}

// boolSize is the size of a BOOLEAN bind.
const boolSize = 4

// Write writes a NULL BOOLEAN as 0xFD 1, as python-oracledb writes it. 18c
// read the empty value that go-ora wrote as a BOOLEAN that is not NULL.
func (param *BoolParameter) Write(session tns.SessionWriter) error {
	if param.DataType == types.BOOLEAN && param.BValue == nil && !param.IsUDTPar && !param.IsArrayPar {
		session.PutBytes(0xFD, 1)
		return nil
	}
	return param.BasicParameter.Write(session)
}

// inPLSQL reports whether conn binds for a PL/SQL statement. PL/SQL takes a
// BOOLEAN bind from 12.1, as python-oracledb sends it, and refused the NUMBER
// that go-ora sent there with PLS-00382.
func inPLSQL(conn Conn) bool {
	p, ok := conn.(interface{ PLSQL() bool })
	return ok && p.PLSQL()
}

// encodeNumber encodes input as the NUMBER 1 or 0, for SQL before 23ai,
// which has no BOOLEAN, and for PL/SQL before 12.1.
func (param *BoolParameter) encodeNumber(input any, conn Conn) error {
	var b types.Bool
	if err := b.SetValue(input); err != nil {
		return err
	}
	v, err := b.Value()
	if err != nil {
		return err
	}
	var n any
	switch v {
	case true:
		n = int64(1)
	case false:
		n = int64(0)
	}
	num := &NumberParameter{}
	if err := num.Encode(n, conn); err != nil {
		return err
	}
	param.BasicParameter = num.BasicParameter
	return nil
}

func (param *BoolParameter) Decode(_ Conn) (any, error) {
	decoder := &types.Bool{}
	decoder.SetBytes(param.BValue)
	decoder.SetDataType(param.DataType)
	return decoder.Value()
}

func (param *BoolParameter) Read(session tns.SessionReader) error {
	var err error
	param.BValue, err = param.BasicRead(session)
	return err
}
