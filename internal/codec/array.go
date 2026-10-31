package codec

import (
	"database/sql/driver"
	"fmt"
	"reflect"

	"github.com/xo/oracle/internal/tns"
	"github.com/xo/oracle/internal/types"
)

type ArrayParameter struct {
	BasicParameter
	attrib OracleParameterCoder
	//items  []parameter_coder.OracleParameterCoder
}

func (param *ArrayParameter) Copy() OracleParameterCoder {
	ret := new(ArrayParameter)
	*ret = *param
	return ret
}

func (param *ArrayParameter) Init() {
	param.SetDefault()
	param.Flag = 0x43
}

func (param *ArrayParameter) FillAttrib(input any, conn Conn) error {
	var err error
	if param.attrib == nil {
		inputType := derefType(input)
		if inputType != nil {
			itemType := inputType.Elem()
			if itemType != nil {
				for itemType.Kind() == reflect.Pointer {
					itemType = itemType.Elem()
				}
			}
			param.attrib, err = conn.GetParameterCoder(itemType)
		} else if param.DataType != 0 {
			param.attrib, err = conn.GetParameterCoder(param.DataType)
		} else {
			err = fmt.Errorf("to set array item type either pass non-nil value or pass data type")
		}

	}
	return err
}

func (param *ArrayParameter) Encode(input any, conn Conn) (err error) {
	param.Init()
	inputType := derefType(input)
	if inputType.Kind() == reflect.Array || inputType.Kind() == reflect.Slice {
		var coder OracleParameterCoder
		err = param.FillAttrib(input, conn)
		if err != nil {
			return err
		}
		coder = param.attrib.Copy()
		coder.Init()
		param.UpdateParameterInfo(coder.GetParameterInfo())
		rValue := reflect.ValueOf(input)
		length := rValue.Len()
		if length > param.ArraySize {
			param.ArraySize = length
		}
		if param.ArraySize > 0 {
			session := tns.NewMemorySession(nil, nil, conn.GetSession().GetProperties())
			session.PutUint(param.ArraySize, 4, true, true)
			if length > 0 {
				for i := range length {
					var item driver.Value
					if rValue.Index(i).CanInterface() {
						item, err = derefValue(rValue.Index(i).Interface())
						if err != nil {
							return err
						}
						err = coder.Encode(item, conn)
					} else {
						err = coder.Encode(nil, conn)
					}
					if err != nil {
						return err
					}
					err = coder.Write(session)
					if err != nil {
						return err
					}

				}
			} else {
				// encode default value of the array parameter to fill parameter properties
				err = coder.Encode(nil, conn)
				if err != nil {
					return err
				}
			}
			param.UpdateParameterInfo(coder.GetParameterInfo())
			if param.DataType == types.NCHAR {
				param.MaxLen = conn.GetMaxStringLength()
				param.MaxCharLen = param.MaxLen // / converters.MaxBytePerChar(par.CharsetID)
			}
			if param.DataType == types.RAW {
				param.MaxLen = conn.GetMaxRawLength()
			}
			param.BValue = session.GetWriteBuffer()
		} else {
			param.BValue = nil
		}
		if param.ArraySize == 0 {
			param.ArraySize = 1
		}

	} else {
		err = fmt.Errorf("no encoder register for data type: %s", inputType.Name())
	}
	return
}

func (param *ArrayParameter) Decode(conn Conn) (value any, err error) {
	err = param.FillAttrib(nil, conn)
	if err != nil {
		return nil, err
	}
	session := conn.GetSession()
	if param.ArraySize > 0 {
		items := make([]any, param.ArraySize)
		for i := 0; i < param.ArraySize; i++ {
			decoder := param.attrib.Copy()
			decoder.UpdateParameterInfo(param.GetParameterInfo())
			err = decoder.Read(session)
			if err != nil {
				return nil, err
			}
			_, err = session.GetInt(2, true, true)
			if err != nil {
				return nil, err
			}
			items[i], err = decoder.Decode(conn)
			if err != nil {
				return
			}
		}
		return items, nil
	}
	return nil, nil
}

func (param *ArrayParameter) Read(session tns.SessionReader) error {
	// non-xml array
	// this code should be in basic_read
	var err error
	param.ArraySize, err = session.GetInt(4, true, true)
	if err != nil {
		return err
	}

	return nil
}

func (param *ArrayParameter) Write(session tns.SessionWriter) (err error) {
	if len(param.BValue) == 0 {
		session.PutBytes(0)
	} else {
		session.PutBytes(param.BValue...)
	}
	return
}

// derefValue returns the value that origVal points to, through its Value
// method when it is a driver.Valuer.
func derefValue(origVal driver.Value) (driver.Value, error) {
	if origVal == nil {
		return nil, nil
	}
	rOriginal := reflect.ValueOf(origVal)
	if rOriginal.Kind() == reflect.Pointer && rOriginal.IsNil() {
		return nil, nil
	}
	proVal := reflect.Indirect(rOriginal)
	if valuer, ok := proVal.Interface().(driver.Valuer); ok {
		return valuer.Value()
	}
	return proVal.Interface(), nil
}

// derefType returns the type of value, with every pointer removed.
func derefType(value driver.Value) reflect.Type {
	valueType := reflect.TypeOf(value)
	if valueType != nil {
		for valueType.Kind() == reflect.Pointer {
			valueType = valueType.Elem()
		}
	}
	return valueType
}
