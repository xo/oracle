package codec

import (
	"time"

	"github.com/xo/oracle/internal/charset"
	"github.com/xo/oracle/internal/tns"
	"github.com/xo/oracle/internal/types"
)

type Conn interface {
	charset.StringCoder
	Session() tns.SessionReadWriter
	NewLobStreamer() types.LobStreamer
	ParameterCoder(input any) (OracleParameterCoder, error)
	SendTimeAsUTC() bool
	// DBTimeZone returns the zone of the database, or the error of a zone
	// that the driver could not load.
	DBTimeZone() (*time.Location, error)
	MaxRawLength() int64
	// ServerRelease returns the major release of the server, such as 11 or
	// 23.
	ServerRelease() int
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
