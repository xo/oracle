package oracle

import (
	"context"
	"database/sql"
	"database/sql/driver"

	"github.com/xo/oracle/internal/auth"
	"github.com/xo/oracle/internal/ttc"
	"github.com/xo/oracle/internal/types"
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

// Clob is a CLOB or an NCLOB. A column scans into it, and it binds as an
// argument.
type Clob = types.Clob

// Blob is a BLOB.
type Blob = types.Blob

// BFile is a BFILE, a file that the server reads from an Oracle directory.
type BFile = types.BFile

// Vector is a VECTOR of Oracle 23ai and later.
type Vector = types.Vector

// JSON is a JSON value of Oracle 21c and later.
type JSON = types.Json

// Bool is a BOOLEAN of Oracle 23ai and later, or of PL/SQL.
type Bool = types.Bool

// RowID is a ROWID. A column of the type ROWID scans into it.
type RowID = types.RowID

// IntervalYM is an INTERVAL YEAR TO MONTH. An INTERVAL DAY TO SECOND is a
// time.Duration.
type IntervalYM = types.IntervalYM

// CreateClob returns a CLOB, or an NCLOB when nclob is true, that holds
// input, a string or a []byte. Its value goes to the server when it is bound
// to a statement.
func CreateClob(ctx context.Context, db *sql.DB, input any, nclob bool) (*Clob, error) {
	return types.CreateClob(ctx, db, input, nclob)
}

// CreateBlob returns a BLOB that holds input, a []byte.
func CreateBlob(ctx context.Context, db *sql.DB, input any) (*Blob, error) {
	return types.CreateBlob(ctx, db, input)
}

// CreateBFile returns the BFILE of the file name in the Oracle directory dir.
func CreateBFile(ctx context.Context, db *sql.DB, dir, name string) (*BFile, error) {
	return types.CreateBFile(ctx, db, dir, name)
}

// CreateVector returns a VECTOR that holds array: a []uint8, a []float32 or
// a []float64, dense or sparse.
func CreateVector(array any) (*Vector, error) {
	return types.CreateVector(array)
}
