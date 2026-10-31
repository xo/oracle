package aq

import (
	"time"

	"github.com/xo/oracle/converters"
	"github.com/xo/oracle/network"
	"github.com/xo/oracle/parameter_coder"
	"github.com/xo/oracle/types"
)

type (
	IConnection interface {
		converters.StringCoder
		GetSession() network.SessionReadWriter
		NewLobStreamer() types.LobStreamer
		ProcessTCCResponse(msgCode uint8) error
		//GetGoParameterCoder(goType reflect.Type) OracleParameterCoder
		//GetOracleParameterCoder(oracleType uint16) OracleParameterCoder
		//GetNameParameterCoder(nameType string) OracleParameterCoder
		GetParameterCoder(input interface{}) (parameter_coder.OracleParameterCoder, error)
		SendTimeAsUTC() bool
		GetDBTimeZone() *time.Location
		GetDBServerTimeZone() *time.Location
		TTCVersion() uint8
		GetMaxRawLength() int64
	}
)
