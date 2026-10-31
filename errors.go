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
)

// ServerError is an error that the server returned, with its ORA code in
// ErrCode.
type ServerError = tns.OracleError
