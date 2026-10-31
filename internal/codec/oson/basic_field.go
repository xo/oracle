package oson

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
