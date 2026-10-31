package types

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"

	"github.com/xo/oracle/internal/converters"
)

type Clob struct {
	String
	lobBase
}

func (clob *Clob) Upload() error {
	if len(clob.bValue) == 0 {
		return nil
	}
	charsetForm := 1
	if clob.UseNCharset {
		charsetForm = 2
	}
	var err error
	if !clob.IsDataUploaded() && clob.stream != nil {
		// the upload runs inside the call that binds the CLOB, which
		// watches the context of that call
		_, err = clob.stream.CreateTemporaryLocator(clob.Conv.GetLangID(), charsetForm)
		if err != nil {
			return err
		}
		if clob.Conv != nil {
			locator := clob.GetLocator()
			if locator.IsVarWidthChar() {
				tempValue := clob.Conv.Decode(clob.bValue)
				if clob.stream.DatabaseVersionNumber() < 10200 && locator.IsLittleEndian() {
					clob.Conv, err = clob.stream.GetStringCoder().GetStringCoder(2002, 0)
				} else {
					clob.Conv, err = clob.stream.GetStringCoder().GetStringCoder(2000, 0)
				}
				clob.bValue = clob.Conv.Encode(tempValue)
			}
		}
		err = clob.stream.Write(clob.bValue)
	}
	return err
	//return clob.uploadData(clob.bValue, clob.Conv.GetLangID(), charsetForm)
}

// SetValue use stream to get charset converter
// call basic SetValue
// upload value to server
func (clob *Clob) SetValue(input any) error {
	var err error
	charsetForm := 1
	if clob.UseNCharset {
		charsetForm = 2
	}
	if clob.Conv == nil {
		if clob.stream != nil {
			clob.Conv, err = clob.stream.GetStringCoder().GetStringCoder(0, charsetForm)
			if err != nil {
				return err
			}
		} else {
			clob.Conv = converters.NewStringConverter(0x7D0)
		}
	}
	switch input := input.(type) {
	case Clob:
		if input.IsDataUploaded() {
			*clob = input
		} else {
			if clob.Conv != nil && input.Conv != nil && clob.Conv.GetLangID() == input.Conv.GetLangID() {
				*clob = input
			} else {
				temp, err := input.Value()
				if err != nil {
					return err
				}
				return clob.SetValue(temp)
			}
		}
		return nil
	case *Clob:
		if input.IsDataUploaded() {
			*clob = *input
		} else {
			if clob.Conv != nil && input.Conv != nil && clob.Conv.GetLangID() == input.Conv.GetLangID() {
				*clob = *input
			} else {
				temp, err := input.Value()
				if err != nil {
					return err
				}
				return clob.SetValue(temp)
			}
		}
		//*clob = *input
		return nil
	default:
		err = clob.String.SetValue(input)
		if err != nil {
			return err
		}
	}
	return clob.Upload()
}

//func (clob *Clob) Value(_ uint16) (interface{}, error) {
//	var err error
//	charsetForm := 1
//	if clob.UseNCharset {
//		charsetForm = 2
//	}
//	locator := clob.stream.GetLocator()
//	if locator.IsVarWidthChar() {
//		if clob.stream.DatabaseVersionNumber() < 10200 && locator.IsLittleEndian() {
//			clob.Conv, err = clob.stream.GetStringCoder().GetStringCoder(2002, 0)
//		} else {
//			clob.Conv, err = clob.stream.GetStringCoder().GetStringCoder(2000, 0)
//		}
//	} else {
//		clob.Conv, err = clob.stream.GetStringCoder().GetStringCoder(0, charsetForm)
//	}
//	if err != nil {
//		return nil, err
//	}
//	return clob.String.Value(0)
//}
//	type Clob interface {
//		Data() sql.NullString
//		Charset() (charsetID int, charsetForm int)
//		ReadFromPos(ctx context.Context, pos int64) (sql.NullString, error)
//		ReadBytesFromPos(ctx context.Context, pos, count int64) (sql.NullString, error)
//		Lob
//	}
//type ClobDecoder interface {
//	DecodeClob(data []byte) (Clob, error)
//}
//	func (l *Clob) createLocatorAndLoadData(ctx context.Context, useNClob bool) error {
//		var err error
//		if l.stream == nil {
//			return errNilStreamer
//		}
//		if l.stream.GetLocator() == nil {
//			if l.charsetForm == 0 {
//				l.charsetForm = 1
//				if useNClob {
//					l.charsetForm = 2
//				}
//			}
//			l.charsetConverter, err = l.stream.GetStringCoder().GetStringCoder(l.charsetID, l.charsetForm)
//			if err != nil {
//				return err
//			}
//			if l.charsetID == 0 {
//				l.charsetID = l.charsetConverter.GetLangID()
//			}
//			if !l.Valid || len(l.String) == 0 {
//				return nil
//			}
//			_, err = l.stream.CreateTemporaryLocator(l.charsetID, l.charsetForm)
//			if err != nil {
//				return err
//			}
//			if l.IsVarWidthChar() {
//				if l.IsLittleEndian() {
//					l.charsetConverter, err = l.stream.GetStringCoder().GetStringCoder(2002, 0)
//				} else {
//					l.charsetConverter, err = l.stream.GetStringCoder().GetStringCoder(2000, 0)
//				}
//			}
//			bytes := l.charsetConverter.Encode(l.String)
//			done := l.stream.StartContext(ctx)
//			defer l.stream.EndContext(done)
//			err = l.stream.Write(bytes)
//		}
//		return err
//	}

// CreateClob returns a CLOB, or an NCLOB when useNClob is true, that holds
// input, with a streamer from db. The value goes to the server when the CLOB
// is bound to a statement.
func CreateClob(ctx context.Context, db *sql.DB, input any, useNClob bool) (*Clob, error) {
	var err error
	ret := &Clob{}
	ret.UseNCharset = useNClob
	err = ret.createStreamer(ctx, db)
	if err != nil {
		return nil, err
	}
	return ret, ret.SetValue(input)
}

//func NewClob(stream LobStreamer, uploadCtx context.Context, input interface{}, useNClob bool) (*Clob, error) {
//	ret := &Clob{}
//	ret.UseNCharset = useNClob
//	ret.UploadCtx = uploadCtx
//	ret.stream = stream
//	return ret, ret.SetValue(input)
//}
//	func NewClob(streamer LobStreamer) *Clob {
//		ret := &Clob{}
//		ret.stream = streamer
//		return ret
//	}
//
//	func NewClob(streamer LobStreamer, charsetID, charsetForm int, data []byte) (Clob, error) {
//		ret := &clob{}
//		var err error
//		ret.stream = streamer
//		if ret.IsVarWidthChar() {
//			if ret.stream.DatabaseVersionNumber() < 10200 && ret.IsLittleEndian() {
//				ret.charsetConverter, err = streamer.GetStringCoder().GetStringCoder(2002, 0)
//			} else {
//				ret.charsetConverter, err = streamer.GetStringCoder().GetStringCoder(2000, 0)
//			}
//		} else {
//			ret.charsetConverter, err = streamer.GetStringCoder().GetStringCoder(charsetID, charsetForm)
//		}
//		if err != nil {
//			return nil, err
//		}
//		if len(data) == 0 {
//			ret.Valid = false
//		} else {
//			ret.Valid = true
//			ret.String = ret.charsetConverter.Decode(data)
//		}
//		return ret, nil
//	}
//
//	func (l *clob) Charset() (charsetID int, charsetForm int) {
//		return l.charsetID, l.charsetForm
//	}

func (clob *Clob) CopyTo(dest driver.Value) error {
	if ok, err := ScanExternal(dest, clob.Value); ok {
		return err
	}
	temp, err := clob.Value()
	if err != nil {
		return err
	}
	switch dst := dest.(type) {
	case *string:
		if temp == nil {
			*dst = ""
		} else {
			*dst = temp.(string)
		}
	case *[]byte:
		if temp == nil {
			*dst = nil
		} else {
			*dst = []byte(temp.(string))
		}
	default:
		return fmt.Errorf("cannot copy Clob to variable of type %T ", dest)
	}
	return nil
}
func (clob *Clob) Scan(value any) error {
	return clob.SetValue(value)
}

//func (l *clob) Data() sql.NullString {
//	//if l.IsNil() || !l.Valid {
//	//	return sql.NullString{String: l.String, Valid: l.Valid}, nil
//	//}
//	//err := l.Read(ctx)
//	//if err != nil {
//	//	return sql.NullString{}, err
//	//}
//	//if free {
//	//	err = l.Free()
//	//	if err != nil {
//	//		return sql.NullString{}, err
//	//	}
//	//}
//	return sql.NullString{String: l.String, Valid: l.Valid}
//}

// Load reads the whole value and watches no context. See Lob.
func (clob *Clob) Load() error {
	var err error
	clob.bValue, err = clob.load()
	return err
}

func (clob *Clob) Read(ctx context.Context) error {
	var err error
	clob.bValue, err = clob.ReadFromPos(ctx, 0)
	return err
}
