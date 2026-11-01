package oson

import (
	"errors"
	"fmt"

	"github.com/xo/oracle/internal/types"
)

type (
	FieldEncoder interface {
		Encode() ([]byte, error)
	}
	FieldDecoder any
)
type Field interface {
	Encode() ([]byte, error)
	OpCode() uint8
	KeyIndex() int
	SetKeyIndex(int)
	Offset() int
	SetOffset(int)
	Children() []Field
	Value() (any, error)
}

type basicField struct {
	opCode   uint8
	keyIndex int
	offset   int
	children []Field
}

func (field *basicField) OpCode() uint8 {
	return field.opCode
}
func (field *basicField) KeyIndex() int {
	return field.keyIndex
}
func (field *basicField) Children() []Field {
	return field.children
}
func (field *basicField) SetKeyIndex(index int) {
	field.keyIndex = index
}
func (field *basicField) Offset() int {
	return field.offset
}
func (field *basicField) SetOffset(offset int) {
	field.offset = offset
}

// valueField is a scalar of OSON that has no field of its own, such as a
// date. Its value is the text that JSON_SERIALIZE writes for it.
type valueField struct {
	value string
	basicField
}

func (field *valueField) Value() (any, error) {
	return field.value, nil
}

func (field *valueField) Encode() ([]byte, error) {
	return nil, errors.New("writing a JSON value of the node type " + fmt.Sprintf("%#x", field.opCode) + ": not supported")
}

// vectorField is a VECTOR in a JSON value. Its value is the elements of the
// vector.
type vectorField struct {
	value types.Vector
	basicField
}

func (field *vectorField) Value() (any, error) {
	return field.value.Value()
}

func (field *vectorField) Encode() ([]byte, error) {
	return nil, errors.New("writing a VECTOR in a JSON value: not supported")
}

type nullField struct {
	basicField
}

func (field *nullField) Value() (any, error) {
	return nil, nil
}

func (field *nullField) Encode() ([]byte, error) {
	field.opCode = 48
	return []byte{field.opCode}, nil
}

type booleanField struct {
	value bool
	basicField
}

func (field *booleanField) Value() (any, error) {
	return field.value, nil
}

func (field *booleanField) Encode() ([]byte, error) {
	if field.value {
		field.opCode = 49
	} else {
		field.opCode = 50
	}
	return []byte{field.opCode}, nil
}
