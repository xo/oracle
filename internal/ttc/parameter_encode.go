package ttc

import (
	"database/sql"
	"reflect"

	"github.com/xo/oracle/internal/charset"
	"github.com/xo/oracle/internal/codec"
	oraTypes "github.com/xo/oracle/internal/types"
)

func (par *ParameterInfo) init() {
	par.SetDefault()
	par.Flag = 0
	par.iPrimValue = nil
	par.oPrimValue = nil
}

func (par *ParameterInfo) encodeValue(size int64, connection *Connection) error {
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

	if value, ok := tempValue.(oraTypes.Object); ok {
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
					if tempCoder.ParameterInfo().DataType == oraTypes.XMLType {
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
	err = par.encoder.Encode(tempValue, connection)
	if err != nil {
		return err
	}
	par.UpdateParameterInfo(par.encoder.ParameterInfo())
	if par.MaxLen < size {
		par.MaxLen = size
	}
	if par.Direction != Input && par.DataType == oraTypes.NCHAR {
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

	if par.DataType == oraTypes.OCIFileLocator {
		par.MaxLen = int64(size)
		if par.MaxLen == 0 {
			par.MaxLen = oraTypes.MaxLenBFile
		}
	}
	if par.Direction == Output && !(par.DataType == oraTypes.XMLType) {
		par.BValue = nil
	}
	return nil
}
