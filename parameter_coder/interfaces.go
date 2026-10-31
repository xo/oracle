package parameter_coder

import (
	"time"

	"github.com/xo/oracle/converters"
	"github.com/xo/oracle/network"
	"github.com/xo/oracle/types"
)

type IConnection interface {
	converters.StringCoder
	GetSession() network.SessionReadWriter
	NewLobStreamer() types.LobStreamer
	//GetGoParameterCoder(goType reflect.Type) OracleParameterCoder
	//GetOracleParameterCoder(oracleType uint16) OracleParameterCoder
	//GetNameParameterCoder(nameType string) OracleParameterCoder
	GetParameterCoder(input interface{}) (OracleParameterCoder, error)
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
		SetParentSession(input network.SessionReadWriter)
		//SetChild(bool)
		//IsChild() bool
	}
	OracleParameterEncoder interface {
		Encode(input interface{}, conn IConnection) error
		network.ValueStreamWriter
		GetParameterInfo() BasicParameter
		UpdateParameterInfo(data BasicParameter)
	}

	OracleParameterDecoder interface {
		Read(session network.SessionReader) error
		Decode(conn IConnection) (interface{}, error)
		SetLobStreamer(lobStreamer types.LobStreamer)
		SetParameterInfo(data BasicParameter)
		SetBytes(data []byte)
	}
)
