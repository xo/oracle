package tns

import (
	"errors"
	"slices"
	"strconv"
)

// Error is the type of a sentinel error. The root package names it Error too,
// so that a sentinel compares the same from either package.
type Error string

// Error satisfies the error interface.
func (err Error) Error() string {
	return string(err)
}

// ErrAuthentication is a server that refused the credential: a wrong user or
// password, a wrong key or a wrong token. It never matches a refusal for lack
// of a permission.
const ErrAuthentication Error = "authentication refused"

// ErrNotSupported is a feature that the driver or the server does not have.
const ErrNotSupported Error = "not supported"

// ErrInvalidValue is a value that the driver cannot use.
const ErrInvalidValue Error = "invalid value"

// ErrRowsClosed is a read of a LOB of a row after its rows closed (D22).
const ErrRowsClosed Error = "the rows of the LOB are closed"

// ErrCompilation is a PL/SQL unit, a view or a type that the server created
// but could not compile (D32).
const ErrCompilation Error = "created with compilation errors"

// compilationCode is the code of ORA-24344, success with compilation error.
const compilationCode = 24344

// NewCompilationError returns the error of a unit that the server created
// but could not compile. The server sends no error for it, only a flag of
// the summary, and OCI returns ORA-24344 (D32).
func NewCompilationError() *OracleError {
	return &OracleError{
		ErrCode: compilationCode,
		ErrMsg:  "ORA-24344: success with compilation error",
		errPos:  -1,
	}
}

// authCodes are the codes of the errors that refuse a credential.
var authCodes = []int{
	1005,  // null password given
	1017,  // invalid username or password
	28041, // authentication protocol internal error
}

// OracleError is an error that the server returned, with its ORA code.
type OracleError struct {
	ErrCode int
	ErrMsg  string
	errPos  int
}

func NewOracleError(errCode int) *OracleError {
	return &OracleError{ErrCode: errCode, errPos: -1}
}
func (err *OracleError) Error() string {
	if len(err.ErrMsg) == 0 {
		err.translate()
	}
	var output string
	if err.errPos >= 0 {
		output = err.ErrMsg + " error occur at position: " + strconv.Itoa(err.errPos)
	} else {
		output = err.ErrMsg
	}
	return output
}

// Is reports whether target is ErrAuthentication and the server refused the
// credential, or target is ErrCompilation and the error is ORA-24344.
func (err *OracleError) Is(target error) bool {
	switch target {
	case ErrAuthentication:
		return slices.Contains(authCodes, err.ErrCode)
	case ErrCompilation:
		return err.ErrCode == compilationCode
	}
	return false
}

// ErrPos return sql error position。
func (err *OracleError) ErrPos() int {
	return err.errPos
}

func (err *OracleError) translate() {
	switch err.ErrCode {
	case 1:
		err.ErrMsg = "ORA-00001: Unique constraint violation"
	case 900:
		err.ErrMsg = "ORA-00900: Invalid SQL statement"
	case 901:
		err.ErrMsg = "ORA-00901: Invalid CREATE command"
	case 902:
		err.ErrMsg = "ORA-00902: Invalid data type"
	case 903:
		err.ErrMsg = "ORA-00903: Invalid table name"
	case 904:
		err.ErrMsg = "ORA-00904: Invalid identifier"
	case 905:
		err.ErrMsg = "ORA-00905: Misspelled keyword"
	case 906:
		err.ErrMsg = "ORA-00906: Missing left parenthesis"
	case 907:
		err.ErrMsg = "ORA-00907: Missing right parenthesis"
	case 1001:
		err.ErrMsg = "ORA-01001: invalid cursor"
	case 1013:
		err.ErrMsg = "ORA-01013: user requested cancel of current operation"
	case 12631:
		err.ErrMsg = "ORA-12631: Username retrieval failed"
	case 12564:
		err.ErrMsg = "ORA-12564: TNS connection refused"
	case 12506:
		err.ErrMsg = "ORA-12506: TNS:listener rejected connection based on service ACL filtering"
	case 12514:
		err.ErrMsg = "ORA-12514: TNS:listener does not currently know of service requested in connect descriptor"
	case 12516:
		err.ErrMsg = "ORA-12516: TNS:listener could not find available handler with matching protocol stack"
	case 3135:
		err.ErrMsg = "ORA-03135: connection lost contact"
	case 28041:
		err.ErrMsg = "ORA-28041: Authentication protocol internal error"
	default:
		err.ErrMsg = "ORA-" + strconv.Itoa(err.ErrCode)
	}
}

// Bad reports whether the error ends the session, so that the connection is
// of no more use. It holds the codes that python-oracledb reads as a closed
// connection, and the codes that go-ora held.
func (err *OracleError) Bad() bool {
	switch err.ErrCode {
	case 22, 28, 31, 45, 378, 600, 602, 603, 609, 1001, 1012, 1033, 1034, 1041,
		1043, 1089, 1092, 2396, 3113, 3114, 3122, 3135, 12153, 12528, 12537,
		12547, 12570, 12583:
		return true
	default:
		return false
	}
}

var ErrConnReset = errors.New("connection break due to context timeout")
