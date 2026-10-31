package types

import (
	"context"
	"database/sql"

	"github.com/xo/oracle/internal/config"
	"github.com/xo/oracle/internal/converters"
	"github.com/xo/oracle/internal/trace"
)

type LobStreamer interface {
	StartContext(ctx context.Context) func()
	EndContext(end func())
	GetLocator() Locator
	SetLocator(locator Locator)
	DatabaseVersionNumber() int
	GetStringCoder() converters.StringCoder
	GetLobStreamMode() config.LobFetch
	GetLobReadMode() config.LobReadMode
	GetTracer() trace.Tracer
	GetSize() (int64, error)
	Exists() (bool, error)
	CreateTemporaryLocator(charsetID, charsetForm int) (Locator, error)
	FreeTemporaryLocator() error
	Open(mode, opID int) error
	Read(offset, count int64) ([]byte, error)
	Write(data []byte) error
	Close(opID int) error
}

type OracleType interface {
	SetValue(input any) error
	Value() (any, error)
	SetDataType(dt uint16)
	GetDataType() uint16
	GetMaxLen() int64
	sql.Scanner
}
