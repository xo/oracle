package aq

import (
	"time"

	"github.com/xo/oracle/internal/charset"
	"github.com/xo/oracle/internal/codec"
	"github.com/xo/oracle/internal/tns"
	"github.com/xo/oracle/internal/types"
)

type (
	Conn interface {
		charset.StringCoder
		Session() tns.SessionReadWriter
		NewLobStreamer() types.LobStreamer
		ProcessTCCResponse(msgCode uint8) error
		ParameterCoder(input any) (codec.OracleParameterCoder, error)
		SendTimeAsUTC() bool
		DBTimeZone() *time.Location
		DBServerTimeZone() *time.Location
		TTCVersion() uint8
		MaxRawLength() int64
	}
)
