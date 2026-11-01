package codec

import (
	"github.com/xo/oracle/internal/config"
	"github.com/xo/oracle/internal/tns"
	"github.com/xo/oracle/internal/types"
	"reflect"
)

type lobParameter struct {
	BasicParameter
	streamer types.LobStreamer
}

func (param *lobParameter) SetLobStreamer(streamer types.LobStreamer) {
	param.streamer = streamer
}

func (param *lobParameter) Write(session tns.SessionWriter) error {
	if param.IsUDTPar {
		// an attribute of an object is its locator with the length of the
		// image, as a RAW is, and a NULL is 0xFF, as python-oracledb writes
		// them. go-ora put a length of 4 bytes first and wrote a NULL as the
		// NULL of an object, and the server refused it with ORA-03101
		if len(param.BValue) == 0 {
			session.PutBytes(0xFF)
			return nil
		}
		session.PutFixedClr(param.BValue)
		return nil
	}
	if len(param.BValue) > 0 {
		if !param.IsArrayPar {
			session.PutUint(len(param.BValue), 4, true, true)
		}
	} else {
		if param.IsArrayPar {
			session.PutBytes(0xFF)
			return nil
		}
		if param.IsUDTPar {
			session.PutBytes(0xFD)
			return nil
		}
	}
	session.PutClr(param.BValue)
	return nil
}
func (param *lobParameter) Read(session tns.SessionReader) error {
	var locator types.Locator
	var err error
	if !(param.IsUDTPar || param.IsArrayPar) && param.streamer.LobStreamMode() == config.INLINE {
		// the following code is working when the lob is inline (default)
		var maxSize int64
		maxSize, err = session.ReadInt64(4, true, true)
		if err != nil {
			return err
		}
		if maxSize > 0 {
			/*size*/ _, err = session.ReadInt64(8, true, true)
			if err != nil {
				return err
			}
			/*chunkSize*/ _, err = session.ReadInt(4, true, true)
			if err != nil {
				return err
			}
			if param.DataType == types.OCIFileLocator {
				locator, err = session.ReadCLR()
			} else {
				param.BValue, err = session.ReadCLR()
				if err != nil {
					return err
				}
				locator, err = session.ReadCLR()
				if err != nil {
					return err
				}
			}
		} else {
			// set value nil
			locator = nil
			param.BValue = nil
		}
	} else {
		// the following code is working when the lob is not inline or part of UDT
		locator, err = param.BasicRead(session)
		if err != nil {
			return err
		}
		if len(locator) == 0 {
			locator = nil
			param.BValue = nil
		} else {
			if !(param.IsUDTPar || param.IsArrayPar) {
				locator, err = session.ReadCLR()
				if err != nil {
					return err
				}
			}
		}
	}
	param.streamer.SetLocator(locator)
	return nil
}

// isNilPointer reports whether v is a nil pointer of any type.
func isNilPointer(v any) bool {
	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Pointer && rv.IsNil()
}
