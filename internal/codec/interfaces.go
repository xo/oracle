package codec

import (
	"time"

	"github.com/xo/oracle/internal/converters"
	"github.com/xo/oracle/internal/tns"
	"github.com/xo/oracle/types"
)

type IConnection interface {
	converters.StringCoder
	GetSession() tns.SessionReadWriter
	NewLobStreamer() types.LobStreamer
	//GetGoParameterCoder(goType reflect.Type) OracleParameterCoder
	//GetOracleParameterCoder(oracleType uint16) OracleParameterCoder
	//GetNameParameterCoder(nameType string) OracleParameterCoder
	GetParameterCoder(input any) (OracleParameterCoder, error)
	SendTimeAsUTC() bool
	GetDBTimeZone() *time.Location
	GetDBServerTimeZone() *time.Location
	GetMaxRawLength() int64
}

type (
	OracleParameterCoder interface {
		OracleParameterEncoder
		OracleParameterDecoder
		Init()
		Bytes() []byte
		Copy() OracleParameterCoder
		SetAsUDTPar()
		SetAsArrayPar()
		SetAQMessage()
		SetParentSession(input tns.SessionReadWriter)
		//SetChild(bool)
		//IsChild() bool
	}
	OracleParameterEncoder interface {
		Encode(input any, conn IConnection) error
		tns.ValueStreamWriter
		GetParameterInfo() BasicParameter
		UpdateParameterInfo(data BasicParameter)
	}

	OracleParameterDecoder interface {
		Read(session tns.SessionReader) error
		Decode(conn IConnection) (any, error)
		SetLobStreamer(lobStreamer types.LobStreamer)
		SetParameterInfo(data BasicParameter)
		SetBytes(data []byte)
	}
)
