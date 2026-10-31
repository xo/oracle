package oracle

import (
	"context"
	"database/sql"
	"database/sql/driver"

	"github.com/xo/oracle/internal/config"
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

// TimestampTZ is a time that binds as TIMESTAMP WITH TIME ZONE, where a
// time.Time binds as TIMESTAMP. A nullable one is sql.Null[TimestampTZ].
type TimestampTZ = types.TimestampTZ

// NTS makes the NTLM messages of NTS, the Windows authentication of Oracle
// Net, for Config.NTS.
type NTS = config.NTSAuthenticator

// WrapRefCursor returns the rows of cursor, read through q.
func WrapRefCursor(ctx context.Context, q Querier, cursor *RefCursor) (*sql.Rows, error) {
	return ttc.WrapRefCursor(ctx, q, cursor)
}

// NewBatch returns an argument that binds each element of array, a slice, as
// one row of an array bind, for a bulk insert.
func NewBatch(array driver.Value) driver.Value {
	return ttc.NewBatch(array)
}

// Object names the user defined type of a bind, as RegisterType registered
// it. Most values need none, because the Go type that RegisterType names
// binds as its type. A collection of a basic type, such as a TABLE OF DATE,
// needs one, since a []time.Time binds as a PL/SQL array. Value is the
// value, or a pointer to the destination in an Out.
type Object = types.Object

// Clob is a CLOB or an NCLOB. A column scans into it, and it binds as an
// argument.
type Clob = types.Clob

// Blob is a BLOB.
type Blob = types.Blob

// BFile is a BFILE, a file that the server reads from an Oracle directory.
type BFile = types.BFile

// LobReader reads the value of a Clob, a Blob or a BFile in pieces, through
// their NewReader. A LOB of a row reads until its rows close, and a read
// after that fails with ErrRowsClosed (D22).
type LobReader = types.LobReader

// DefaultPieceSize is the size in bytes of the piece that a LobReader asks
// the server for, unless NewReaderSize sets another.
const DefaultPieceSize = types.DefaultPieceSize

// Vector is a VECTOR of Oracle 23ai and later.
type Vector = types.Vector

// JSON is a JSON value of Oracle 21c and later. A JSON column scans as a
// jsontext.Value, and into any destination that encoding/json/v2 decodes
// into. NewJSON makes one to bind.
type JSON = types.JSON

// NewJSON returns the JSON of v. A string, a []byte and a jsontext.Value are
// JSON text already, and any other value goes through json.Marshal of
// encoding/json/v2.
func NewJSON(v any) (*JSON, error) {
	return types.NewJSON(v)
}

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
