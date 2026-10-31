package oracle

import (
	"context"
	"database/sql"
	"database/sql/driver"

	"github.com/xo/oracle/internal/auth"
	"github.com/xo/oracle/internal/ttc"
)

// Out is an OUT or IN OUT argument of a PL/SQL call. Dest receives the value,
// Size is the largest size in bytes that the value can have, and In sends the
// value of Dest too.
type Out = ttc.Out

// RefCursor is a REF CURSOR that a PL/SQL call returns. WrapRefCursor reads
// its rows as *sql.Rows.
type RefCursor = ttc.RefCursor

// Querier runs a query, as *sql.DB, *sql.Conn and *sql.Tx do.
type Querier = ttc.Querier

// TimeStampTZ is a time that binds as TIMESTAMP WITH TIME ZONE.
type TimeStampTZ = ttc.TimeStampTZ

// NullTimeStampTZ is a TimeStampTZ that can be NULL.
type NullTimeStampTZ = ttc.NullTimeStampTZ

// NTSAuth authenticates a Windows user with NTS.
type NTSAuth = auth.NTSAuthInterface

// WrapRefCursor returns the rows of cursor, read through q.
func WrapRefCursor(ctx context.Context, q Querier, cursor *RefCursor) (*sql.Rows, error) {
	return ttc.WrapRefCursor(ctx, q, cursor)
}

// NewBatch returns an argument that binds each element of array, a slice, as
// one row of an array bind, for a bulk insert.
func NewBatch(array driver.Value) driver.Value {
	return ttc.NewBatch(array)
}

// SetNTSAuth sets the NTS authenticator of every connection in the process.
// BACKLOG.md holds the work that moves it to the Connector.
func SetNTSAuth(m NTSAuth) {
	ttc.SetNTSAuth(m)
}
