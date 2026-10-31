package ttc

import (
	"database/sql"
	"reflect"

	"github.com/xo/oracle/internal/charset"
	"github.com/xo/oracle/internal/codec"
	"github.com/xo/oracle/internal/types"
)

func (par *ParameterInfo) init() {
	par.SetDefault()
	par.Flag = 0
	par.iPrimValue = nil
	par.oPrimValue = nil
}

// plsqlMaxLen is the longest string or RAW that a PL/SQL block takes as a
// VARCHAR2 or a RAW bind, on every release.
const plsqlMaxLen = 32767

// plsqlConn is a connection whose longest string is that of PL/SQL.
type plsqlConn struct {
	*Connection
}

func (plsqlConn) MaxStringLength() int64 {
	return plsqlMaxLen
}

func (par *ParameterInfo) encodeValue(size int64, stmt *Stmt) error {
	connection := stmt.connection
	par.init()
	var err error
	par.MaxLen = size
	if par.Value == nil {
		par.Value = sql.Null[string]{}
	}
	tempType := getType(par.Value)
	tempValue, err := getValue(par.Value)
	if err != nil {
		return err
	}

	if p, ok := tempValue.(*types.Object); ok && p != nil {
		tempValue = *p
	}
	if value, ok := tempValue.(types.Object); ok {
		par.encoder, err = connection.ParameterCoder(value.Name)
		if err != nil {
			return err
		}
		par.Value = value.Value
		tempValue, err = getValue(par.Value)
		if err != nil {
			return err
		}
	} else {
		if par.encoder, err = connection.ParameterCoder(tempType); err == nil {
		} else {
			connection.logger.Debug("parameter encoder", "type", tempType, "error", err)
			if reflect.TypeOf(tempValue) != nil && tempType != reflect.TypeOf(tempValue) {
				par.encoder, err = connection.ParameterCoder(reflect.TypeOf(tempValue))
				if err != nil {
					connection.logger.Debug("parameter encoder", "type", reflect.TypeOf(tempValue), "error", err)
				}
			}
			if par.encoder == nil && tempValue == nil {
				// a NULL of a type with no coder, such as an invalid
				// sql.Null[T] of an uncommon T, binds as a NULL VARCHAR2
				par.encoder, err = connection.ParameterCoder(reflect.TypeFor[string]())
			}
			if par.encoder == nil {
				if tempType.Kind() == reflect.Array || tempType.Kind() == reflect.Slice {
					// get array item and ensure it is registered in parameter coders
					itemType := tempType.Elem()
					if itemType != nil {
						for itemType.Kind() == reflect.Pointer {
							itemType = itemType.Elem()
						}
					}
					tempCoder, err := connection.ParameterCoder(itemType)
					if err != nil {
						return err
					}
					tempCoder.Init()
					// if item is of type xml so search for slice of item in the registered parameter coders
					if tempCoder.ParameterInfo().DataType == types.XMLType {
						par.encoder, err = connection.ParameterCoder(reflect.SliceOf(itemType))
					} else {
						// if max length > 1 (mean it is set with size) so transfer size to array size
						if par.MaxLen > 1 {
							par.ArraySize = int(par.MaxLen)
							par.MaxLen = 1
						}
						// if item is not xml type go for arrays
						par.encoder = &codec.ArrayParameter{}
					}
				} else {
					return err
				}
			}
		}
	}
	par.encoder.UpdateParameterInfo(par.ParameterInfo())
	var conn codec.Conn = connection
	if stmt.stmtType == PLSQL {
		conn = plsqlConn{connection}
	}
	err = par.encoder.Encode(tempValue, conn)
	if err != nil {
		return err
	}
	par.UpdateParameterInfo(par.encoder.ParameterInfo())
	// PL/SQL takes a string or a RAW of up to 32,767 bytes, and no LONG
	// past that, which failed with ORA-01483. A longer value, IN or IN OUT,
	// goes as a temporary LOB, as python-oracledb sends it (D24). An OUT of
	// a longer size stays a VARCHAR2, which a LONG column selects into
	if stmt.stmtType == PLSQL && par.Direction != Output {
		switch par.DataType {
		case types.LONG, types.LongVarChar, types.LongRaw:
			err = par.encodeLob(tempValue, connection)
			if err != nil {
				return err
			}
			size = 0
		}
	}
	if par.MaxLen < size {
		par.MaxLen = size
	}
	if par.Direction != Input && par.DataType == types.NCHAR {
		// the size of an OUT string is in characters, and the server sizes
		// its buffer from the length in bytes divided by the bytes of a
		// character, so the length must be that many bytes for each
		// character. go-ora left this unreachable, and a string OUT of size
		// 10 could not hold 'abc' on AL32UTF8, which was sijms/go-ora#703.
		conv, err := connection.StringCoder(par.CharsetID, par.CharsetForm)
		if err != nil {
			return err
		}
		perChar := int64(charset.MaxBytesPerChar(conv.LangID))
		if size <= 0 {
			// an OUT string with no size takes the longest string of the
			// server, where a buffer of the length of its input silently
			// cut a RETURNING value of 8 characters to 4
			size = connection.MaxStringLength() / perChar
		}
		if par.MaxCharLen < size {
			par.MaxCharLen = size
		}
		par.MaxLen = par.MaxCharLen * perChar
	}

	if par.DataType == types.OCIFileLocator {
		par.MaxLen = int64(size)
		if par.MaxLen == 0 {
			par.MaxLen = types.MaxLenBFile
		}
	}
	if par.Direction == Output && !(par.DataType == types.XMLType) {
		par.BValue = nil
	}
	return nil
}

// encodeLob encodes value, a string or a []byte, as a temporary CLOB or
// BLOB.
func (par *ParameterInfo) encodeLob(value any, connection *Connection) error {
	var lob codec.OracleParameterCoder
	if par.DataType == types.LongRaw {
		lob = &codec.BlobParameter{}
	} else {
		clob := &codec.ClobParameter{}
		clob.CharsetForm = par.CharsetForm
		lob = clob
	}
	if err := lob.Encode(value, connection); err != nil {
		return err
	}
	par.encoder = lob
	// UpdateParameterInfo keeps a type that is set, so the LOB replaces
	// the parameter whole
	par.BasicParameter = lob.ParameterInfo()
	return nil
}
