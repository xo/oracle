package types

type Basic struct {
	bValue   []byte
	dataType TypeID
}

func (b *Basic) Bytes() []byte {
	return b.bValue
}

func (b *Basic) SetBytes(input []byte) {
	b.bValue = input
}

func (b *Basic) SetDataType(dt TypeID) {
	if b.dataType == 0 {
		b.dataType = dt
	}
}

func (b *Basic) DataType() TypeID {
	return b.dataType
}

func (b *Basic) MaxLen() int64 { return 1 }
