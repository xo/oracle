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
	Locator() Locator
	SetLocator(locator Locator)
	DatabaseVersionNumber() int
	StringCoder() converters.StringCoder
	LobStreamMode() config.LobFetch
	LobReadMode() config.LobReadMode
	Tracer() trace.Tracer
	Size() (int64, error)
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
	DataType() uint16
	MaxLen() int64
	sql.Scanner
}
