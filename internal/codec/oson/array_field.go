package oson

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/xo/oracle/internal/types"
)

type arrayField struct {
	value []any
	structField
}

func (array *arrayField) Value() (any, error) {
	return array.value, nil
}

func newArrayField(value []any, header *header) (*arrayField, error) {
	ret := new(arrayField)
	ret.header = header
	var err error
	for i, item := range value {
		var field Field
		switch v := item.(type) {
		case nil:
			field = &nullField{}
		case number:
			temp, err := types.NewNumber(string(v))
			if err != nil {
				return nil, err
			}
			field = &numberField{value: *temp}
		case string:
			field = newStringField(v)
		case bool:
			field = &booleanField{value: v}
		case []any:
			field, err = newArrayField(v, header)
			if err != nil {
				return nil, err
			}
		case map[string]any:
			field, err = newObjectField(v, header)
			if err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("encoding a value of the type %T at %v", v, i)
		}
		if field == nil {
			return nil, fmt.Errorf("no value for index %d", i)
		}
		ret.children = append(ret.children, field)
	}

	return ret, nil
}

func (array *arrayField) Encode() ([]byte, error) {
	buffer := bytes.NewBuffer(nil)
	childBuffer := bytes.NewBuffer(nil)
	var err error
	childLen := len(array.children)
	var flag uint8 = 0xC0
	offset := array.offset
	flag, err = array.modifyFlag(flag)
	if err != nil {
		return nil, err
	}
	err = buffer.WriteByte(flag)
	if err != nil {
		return nil, err
	}
	offset += 1
	// write child count
	if childLen < 0x100 {
		err = buffer.WriteByte(uint8(childLen))
		offset += 1
	} else if childLen < 0x10000 {
		err = binary.Write(buffer, binary.BigEndian, uint16(childLen))
		offset += 2
	} else {
		err = binary.Write(buffer, binary.BigEndian, uint32(childLen))
		offset += 4
	}
	if err != nil {
		return nil, err
	}
	if childLen == 0 {
		return buffer.Bytes(), nil
	}
	if flag&0x20 > 0 {
		offset += childLen * 4
	} else {
		offset += childLen * 2
	}
	for i, _ := range array.children {
		array.children[i].SetOffset(offset + childBuffer.Len())
		if flag&0x20 > 0 {
			err = binary.Write(buffer, binary.BigEndian, uint32(offset+childBuffer.Len()))
		} else {
			err = binary.Write(buffer, binary.BigEndian, uint16(offset+childBuffer.Len()))
		}
		data, err := array.children[i].Encode()
		if err != nil {
			return nil, err
		}
		_, err = childBuffer.Write(data)
		if err != nil {
			return nil, err
		}
	}
	_, err = childBuffer.WriteTo(buffer)
	return buffer.Bytes(), err
}
