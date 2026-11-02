package oracle

import "github.com/xo/oracle/internal/tns"

// Error is the type of the sentinel errors of the driver. Compare an error
// with a sentinel with errors.Is.
type Error = tns.Error

// The sentinel errors.
const (
	// ErrScheme is a DSN whose scheme is not oracle.
	ErrScheme Error = "the scheme of the dsn is not oracle"
	// ErrUnknownKey is a key of a DSN that the driver does not know.
	ErrUnknownKey Error = "unknown key"
	// ErrRepeatedKey is a key that a DSN holds twice.
	ErrRepeatedKey Error = "repeated key"
	// ErrInvalidValue is a value that the driver cannot use.
	ErrInvalidValue = tns.ErrInvalidValue
	// ErrNotSupported is a feature that the driver or the server does not
	// have.
	ErrNotSupported = tns.ErrNotSupported
	// ErrRowsClosed is a read of a LOB of a row after its rows closed. The
	// connection of the rows can serve another caller by then.
	ErrRowsClosed = tns.ErrRowsClosed
	// ErrConnectorClosed is a Connect after Close.
	ErrConnectorClosed Error = "the connector is closed"
	// ErrAuthentication is a server that refused the credential. A
	// *ServerError with ORA-01017 matches it.
	ErrAuthentication = tns.ErrAuthentication
	// ErrCompilation is a PL/SQL unit, a view or a type that the server
	// created but could not compile. The statement returns a *ServerError
	// with ORA-24344, which matches it, and USER_ERRORS holds the errors of
	// the compilation (D32).
	ErrCompilation = tns.ErrCompilation
)

// ServerError is an error that the server returned, with its ORA code in
// ErrCode. Its method AuthenticationRefused reports whether the server
// refused the credential, as usql asks an error, and matches
// errors.Is(err, ErrAuthentication).
type ServerError = tns.OracleError

// BatchError is the error of a batch of DML that ran with WithBatchErrors:
// the rows that failed, each with the index of the row from 0 and the error
// of the server. The other rows ran. errors.As finds the error of each row
// (D34).
type BatchError = tns.BatchError

// RowError is the error of one row of a BatchError.
type RowError = tns.RowError
