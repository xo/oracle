package ttc

import (
	"bytes"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/xo/oracle/internal/codec"
	"github.com/xo/oracle/internal/tns"
	"github.com/xo/oracle/internal/types"
)

type ObjectParameter struct {
	codec.BasicParameter
	fields       []string
	attribs      map[string]codec.OracleParameterCoder
	isArray      bool
	activeFields map[string]int
	typ          reflect.Type
}

func (param *ObjectParameter) loadActiveFields() {
	for x := 0; x < param.typ.NumField(); x++ {
		f := param.typ.Field(x)
		fieldID, _, _, _ := extractTag(f.Tag.Get("udt"))
		if len(fieldID) == 0 {
			continue
		}
		fieldID = strings.ToUpper(fieldID)
		param.activeFields[fieldID] = x
	}
}

func (param *ObjectParameter) loadObjectTypeInfo(db *sql.DB, drv *Registry, owner, name string) (err error) {
	if param.typ == nil {
		err = errors.New("type object cannot be nil")
		return
	}
	if param.typ.Kind() != reflect.Struct {
		err = errors.New("type object should be of structure type")
		return
	}
	param.activeFields = make(map[string]int)
	param.ToID, err = getTOID2(db, owner, name)
	if err != nil {
		return
	}
	sqlText := `SELECT ATTR_NAME, ATTR_TYPE_NAME, LENGTH, ATTR_NO 
					FROM ALL_TYPE_ATTRS 
					WHERE UPPER(OWNER)=:1 AND UPPER(TYPE_NAME)=:2
					ORDER BY ATTR_NO`
	var rows *sql.Rows
	rows, err = db.Query(sqlText, strings.ToUpper(owner), strings.ToUpper(name))
	if err != nil {
		return
	}
	defer func() {
		_ = rows.Close()
	}()
	var (
		attName     sql.NullString
		attOrder    int64
		attTypeName sql.NullString
		length      sql.NullInt64
	)
	param.attribs = make(map[string]codec.OracleParameterCoder)
	for rows.Next() {
		err = rows.Scan(&attName, &attTypeName, &length, &attOrder)
		if err != nil {
			return
		}
		param.fields = append(param.fields, attName.String)
		par, ok := drv.nameTypeCoder[strings.ToUpper(attTypeName.String)]
		if ok {
			param.attribs[strings.ToUpper(attName.String)] = par.Copy()
			param.attribs[strings.ToUpper(attName.String)].SetAsUDTPar()
		} else {
			err = fmt.Errorf("unsupported attribute type: %s", attTypeName.String)
			return
		}
	}
	if len(param.attribs) == 0 {
		err = fmt.Errorf("unknown or empty type: %s", name)
	}
	param.loadActiveFields()
	return
}

func (param *ObjectParameter) Copy() codec.OracleParameterCoder {
	ret := new(ObjectParameter)
	*ret = *param
	return ret
}

func (param *ObjectParameter) Init() {
	param.SetDefault()
	param.DataType = types.XMLType
	if !param.isArray {
		param.Version = 1
	}
}

func (param *ObjectParameter) Encode(input any, conn codec.Conn) (err error) {
	param.Init()
	session := tns.NewMemorySession(nil, nil, conn.Session().Properties())
	inputType := getType(input)
	rValue := reflect.ValueOf(input)
	// a nil slice is a NULL collection, and not an empty one
	if input == nil || (rValue.Kind() == reflect.Slice && rValue.IsNil()) {
		param.BValue = nil
		return
	}
	if param.isArray {
		coder := param.attribs[""].Copy()
		coder.Init()
		param.UpdateParameterInfo(coder.ParameterInfo())
		size := rValue.Len()
		// the prefix segment, of length 1, the flags of the collection, and
		// its count, as the server writes a collection
		session.PutBytes(1, 1, 0)
		putImageLength(session, size)
		for i := range size {
			var item driver.Value
			if rValue.Index(i).CanInterface() {
				item, err = getValue(rValue.Index(i).Interface())
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
			coderPI := coder.ParameterInfo()
			param.UpdateParameterInfo(coderPI)
		}
	} else {
		// this code is work for object struct
		if inputType != param.typ {
			return fmt.Errorf("wrong parameter mapping for type %s", inputType.String())
		}
		param.MaxLen = 2000
		for _, field := range param.fields {
			attrib := param.attribs[field].Copy()
			attrib.SetParentSession(session)
			if idx, ok := param.activeFields[field]; ok {
				field := rValue.Field(idx)
				if field.CanInterface() {
					err = attrib.Encode(field.Interface(), conn)
				} else {
					err = attrib.Encode(nil, conn)
				}
			} else {
				err = attrib.Encode(nil, conn)
			}
			if err != nil {
				return
			}
			err = attrib.Write(session)
			if err != nil {
				return err
			}
		}
	}

	// save BValue to buffer
	param.BValue = session.WriteBuffer()
	return
}

// maxShortImageLength is the longest length that the image of an object
// writes in one byte. A longer one is 0xFE and four bytes, big endian.
const maxShortImageLength = 245

// putImageLength writes n as the image of an object writes a length.
func putImageLength(session tns.SessionWriter, n int) {
	if n <= maxShortImageLength {
		session.PutBytes(uint8(n))
		return
	}
	session.PutBytes(0xFE)
	session.PutUint(n, 4, true, false)
}

// encapsulate puts the header of an image before BValue: the flags, the
// version, and the length of the whole image.
func (param *ObjectParameter) encapsulate() (err error) {
	code := uint8(0x84)
	if param.isArray {
		code = uint8(0x88)
	}
	size := len(param.BValue)
	objectBuffer := &bytes.Buffer{}
	objectBuffer.Write([]byte{code, 0x1})
	if size+3 <= maxShortImageLength {
		objectBuffer.WriteByte(uint8(size + 3))
	} else {
		objectBuffer.WriteByte(0xfe)
		err = binary.Write(objectBuffer, binary.BigEndian, uint32(size+7))
		if err != nil {
			return
		}
	}
	// at the end save buffer
	objectBuffer.Write(param.BValue)
	param.BValue = objectBuffer.Bytes()
	return
}

func (param *ObjectParameter) Write(session tns.SessionWriter) error {
	if param.PSession == nil {
		if param.BValue == nil && !param.IsArrayPar {
			// a NULL is the envelope with an image of length 0 and no
			// image. go-ora wrote an image of 0xFD, which the server read
			// in a batch as the object of the row before, sijms/go-ora#721
			session.PutBytes(0, 0, 0, 0)
			session.PutUint(0, 4, true, true)
			session.PutBytes(1, 1)
			return nil
		}
		if param.BValue == nil {
			if param.isArray {
				param.BValue = []byte{0xFF}
			} else {
				param.BValue = []byte{0xFD}
			}
		} else {
			err := param.encapsulate()
			if err != nil {
				return err
			}
		}
		if !param.IsArrayPar {
			session.PutBytes(0, 0, 0, 0)
			session.PutUint(len(param.BValue), 4, true, true)
			session.PutBytes(1, 1)
			session.PutClr(param.BValue)
		} else {
			session.PutFixedClr(param.BValue)
		}

	} else {
		// an attribute of an object. A collection there is an image of its
		// own, with its header and its length, as the server writes one.
		// go-ora wrote its body alone, which the server read as a length
		// that it never stopped allocating for, sijms/go-ora#721
		switch {
		case param.isArray && param.BValue == nil:
			// a NULL collection is 0xFF, with no length before it
			session.PutBytes(0xFF)
		case param.isArray:
			if err := param.encapsulate(); err != nil {
				return err
			}
			putImageLength(session, len(param.BValue))
			session.PutBytes(param.BValue...)
		case param.BValue == nil:
			session.PutBytes(0xFD)
		default:
			session.PutBytes(param.BValue...)
		}
	}
	return nil
}

func (param *ObjectParameter) Decode(conn codec.Conn) (any, error) {
	// need main session to get its properties
	var objectType uint8
	var err error
	if param.PSession == nil && len(param.BValue) == 0 {
		// a NULL object, which has no image
		return nil, nil
	}
	if param.PSession == nil {
		param.PSession = tns.NewMemorySession(param.BValue, nil, conn.Session().Properties())
		objectType, err = param.PSession.ReadByte()
		if err != nil {
			return nil, err
		}
		ctl, err := param.PSession.ReadInt64(4, true, true)
		if err != nil {
			return nil, err
		}
		if ctl == 0xFE {
			_, err = param.PSession.ReadInt(4, false, true)
			if err != nil {
				return nil, err
			}
		}
	}
	if objectType == 0 {
		if param.isArray {
			objectType = 0x88
		} else {
			objectType = 0x84
		}
	}
	switch objectType {
	case 0x88:
		_ /*attribsLen*/, err := param.PSession.ReadInt(2, true, true)
		if err != nil {
			return nil, err
		}

		itemsLen, err := param.PSession.ReadInt(2, false, true)
		if err != nil {
			return nil, err
		}
		if itemsLen == 0xFE {
			itemsLen, err = param.PSession.ReadInt(4, false, true)
			if err != nil {
				return nil, err
			}
		}
		items := make([]any, itemsLen)
		for i := 0; i < itemsLen; i++ {
			decoder := param.attribs[""].Copy()
			decoder.UpdateParameterInfo(param.ParameterInfo())
			decoder.SetLobStreamer(conn.NewLobStreamer())
			err = decoder.Read(param.PSession)
			if err != nil {
				return nil, err
			}
			items[i], err = decoder.Decode(conn)
			if err != nil {
				return nil, err
			}
		}
		return items, nil
	case 0x85:
		return nil, nil
	case 0x84:
		retObj := reflect.New(param.typ)
		var value any
		for _, field := range param.fields {
			attrib := param.attribs[field].Copy()
			attrib.SetParentSession(param.PSession)
			err = attrib.Read(param.PSession)
			if err != nil {
				return nil, err
			}
			value, err = attrib.Decode(conn)
			if err != nil {
				return nil, err
			}
			if idx, ok := param.activeFields[field]; ok {
				err = copyValue(retObj.Elem().Field(idx).Addr().Interface(), value)
				if err != nil {
					return nil, err
				}
			}
		}
		return retObj.Elem().Interface(), nil
	case 0xFD, 0xFF:
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown object type: %v", objectType)
	}

}

func (param *ObjectParameter) Read(session tns.SessionReader) error {
	var err error
	if param.IsArrayPar {
		param.BValue, err = session.ReadFixedCLR()
		return err
	}
	if param.PSession != nil && param.isArray {
		param.BValue, err = session.ReadFixedCLR()
		param.PSession = nil
		return err
	}
	if param.PSession == nil {
		_, err = session.ReadDLC() // contain toid and some 0s
		if err != nil {
			return err
		}
		_, err = session.ReadBytes(3) // 3 0s
		if err != nil {
			return err
		}
		var size int
		size, err = session.ReadInt(4, true, true)
		if err != nil {
			return err
		}
		_, err = session.ReadBytes(2) // 0x1 0x1
		if err != nil {
			return err
		}
		if size == 0 {
			// a NULL object has no image. go-ora read two more bytes here,
			// which the server never sent, sijms/go-ora#721
			param.BValue = nil
			return nil
		}
		param.BValue, err = param.BasicRead(session)
	}
	return err
}
