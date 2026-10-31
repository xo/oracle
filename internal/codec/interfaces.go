package codec

import (
	"time"

	"github.com/xo/oracle/internal/converters"
	"github.com/xo/oracle/internal/tns"
	"github.com/xo/oracle/internal/types"
)

type Conn interface {
	converters.StringCoder
	Session() tns.SessionReadWriter
	NewLobStreamer() types.LobStreamer
	ParameterCoder(input any) (OracleParameterCoder, error)
	SendTimeAsUTC() bool
	DBTimeZone() *time.Location
	DBServerTimeZone() *time.Location
	MaxRawLength() int64
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
	}
	OracleParameterEncoder interface {
		Encode(input any, conn Conn) error
		tns.ValueWriter
		ParameterInfo() BasicParameter
		UpdateParameterInfo(data BasicParameter)
	}

	OracleParameterDecoder interface {
		Read(session tns.SessionReader) error
		Decode(conn Conn) (any, error)
		SetLobStreamer(lobStreamer types.LobStreamer)
		SetParameterInfo(data BasicParameter)
		SetBytes(data []byte)
	}
)
