package types

import (
	"context"
	"database/sql"

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
		return nil
	default:
		err = clob.String.SetValue(input)
		if err != nil {
			return err
		}
	}
	return clob.Upload()
}

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

func (clob *Clob) Scan(value any) error {
	return clob.SetValue(value)
}

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
