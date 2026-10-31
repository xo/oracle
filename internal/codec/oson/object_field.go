package oson

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/xo/oracle/internal/types"
)

type objectField struct {
	value map[string]any
	structField
}

func (obj *objectField) Value() (any, error) {
	return obj.value, nil
}

func newObjectField(objData map[string]any, header *header) (*objectField, error) {
	ret := new(objectField)
	ret.header = header
	var err error
	for keyName, value := range objData {
		var field Field
		switch v := value.(type) {
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
			return nil, fmt.Errorf("encoding a value of the type %T at %v", v, keyName)
		}
		if field == nil {
			return nil, fmt.Errorf("no value for key %s", keyName)
		}
		field.SetKeyIndex(header.keys.Index(keyName) + 1)
		ret.children = append(ret.children, field)
	}
	return ret, nil
}

func (obj *objectField) Encode() ([]byte, error) {
	buffer := bytes.NewBuffer(nil)
	childBuffer := bytes.NewBuffer(nil)
	var err error
	childLen := len(obj.children)
	var flag uint8 = 0x84
	offset := obj.offset
	flag, err = obj.modifyFlag(flag)
	if err != nil {
		return nil, err
	}
	// get key information
	keys := make([]int, childLen)
	for index, child := range obj.children {
		keys[index] = child.KeyIndex()
	}
	// create object header
	objHeader := newObjectHeader(flag, keys, offset)
	var selectedHeader *objectHeader
	offset, selectedHeader, err = objHeader.write(buffer, obj.header)
	if childLen == 0 {
		return buffer.Bytes(), nil
	}
	// encode and calculate offsets
	if flag&0x20 > 0 {
		offset += childLen * 4
	} else {
		offset += childLen * 2
	}
	if selectedHeader != nil {
		for _, index := range selectedHeader.keysIndex {
			for i, child := range obj.children {
				if index == child.KeyIndex() {
					// calculate offset
					obj.children[i].SetOffset(offset + childBuffer.Len())
					child.SetOffset(offset + childBuffer.Len())
					if flag&0x20 > 0 {
						err = binary.Write(buffer, binary.BigEndian, uint32(offset+childBuffer.Len()))
					} else {
						err = binary.Write(buffer, binary.BigEndian, uint16(offset+childBuffer.Len()))
					}
					data, err := child.Encode()
					if err != nil {
						return nil, err
					}
					_, err = childBuffer.Write(data)
					if err != nil {
						return nil, err
					}
					break
				}
			}
		}
	} else {
		for i, child := range obj.children {
			// calculate offset
			obj.children[i].SetOffset(offset + childBuffer.Len())
			if flag&0x20 > 0 {
				err = binary.Write(buffer, binary.BigEndian, uint32(offset+childBuffer.Len()))
			} else {
				err = binary.Write(buffer, binary.BigEndian, uint16(offset+childBuffer.Len()))
			}
			data, err := child.Encode()
			if err != nil {
				return nil, err
			}
			_, err = childBuffer.Write(data)
			if err != nil {
				return nil, err
			}
		}
	}

	_, err = childBuffer.WriteTo(buffer)
	return buffer.Bytes(), err
}
