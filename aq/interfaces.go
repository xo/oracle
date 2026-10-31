package aq

import (
	"time"

	"github.com/xo/oracle/internal/codec"
	"github.com/xo/oracle/internal/converters"
	"github.com/xo/oracle/internal/tns"
	"github.com/xo/oracle/internal/types"
)

type (
	IConnection interface {
		converters.StringCoder
		GetSession() tns.SessionReadWriter
		NewLobStreamer() types.LobStreamer
		ProcessTCCResponse(msgCode uint8) error
		GetParameterCoder(input any) (codec.OracleParameterCoder, error)
		SendTimeAsUTC() bool
		GetDBTimeZone() *time.Location
		GetDBServerTimeZone() *time.Location
		TTCVersion() uint8
		GetMaxRawLength() int64
	}
)
