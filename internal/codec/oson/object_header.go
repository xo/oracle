package oson

import (
	"bytes"
	"encoding/binary"
)

type objectHeader struct {
	flag      uint8
	keysIndex []int
	offset    int
}

func newObjectHeader(flag uint8, keyIndex []int, offset int) *objectHeader {
	return &objectHeader{flag: flag, keysIndex: keyIndex, offset: offset}
}

// write writes the header of the object: its node type, the number of its
// children and their field IDs. Each object writes its own field IDs, as
// python-oracledb does. go-ora pointed an object at the field IDs of an
// earlier one with the same names, and the server failed with ORA-40596 on a
// path into an array of such objects.
func (objHeader *objectHeader) write(buffer *bytes.Buffer, header *header) (offset int, err error) {
	offset = objHeader.offset
	err = buffer.WriteByte(objHeader.flag)
	if err != nil {
		return
	}
	offset += 1
	childLen := len(objHeader.keysIndex)
	// write child count
	switch objHeader.flag & 0x18 {
	case 0:
		err = buffer.WriteByte(uint8(childLen))
		offset += 1
	case 8:
		err = binary.Write(buffer, binary.BigEndian, uint16(childLen))
		offset += 2
	case 0x10:
		err = binary.Write(buffer, binary.BigEndian, uint32(childLen))
		offset += 4
	}
	if err != nil {
		return
	}
	if childLen == 0 {
		return
	}
	// write key index
	for _, keyIndex := range objHeader.keysIndex {
		if header.flags&0x8 > 0 {
			err = binary.Write(buffer, binary.BigEndian, uint32(keyIndex))
			offset += 4
		} else if header.flags&0x400 > 0 {
			err = binary.Write(buffer, binary.BigEndian, uint16(keyIndex))
			offset += 2
		} else {
			err = buffer.WriteByte(uint8(keyIndex))
			offset += 1
		}
		if err != nil {
			return
		}
	}
	return
}
