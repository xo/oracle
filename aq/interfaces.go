package aq

import (
	"github.com/xo/oracle/internal/codec"
)

type (
	Conn interface {
		codec.Conn
		ProcessTCCResponse(msgCode uint8) error
		TTCVersion() uint8
	}
)
