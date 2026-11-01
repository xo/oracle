package types

import (
	"encoding/binary"
	"fmt"
)

type RowID struct {
	Basic
	Rba         int64
	PartitionID int64
	filter      byte
	BlockNumber int64
	SlotNumber  int64
}

func (rowid RowID) SetValue(_ any) error {
	return fmt.Errorf("row id is returned from server only")
}

func (rowid RowID) Value() (any, error) {
	switch rowid.dataType {
	case ROWID:
		return string(rowid.decode()), nil
	case UROWID:
		if len(rowid.bValue) == 0 {
			return nil, nil
		}
		if rowid.bValue[0] == 1 {
			return string(rowid.physicalRawIDToByteArray(rowid.bValue)), nil
		}
		return string(rowid.logicalRawIDToByteArray(rowid.bValue)), nil
	}
	return nil, fmt.Errorf("row id is returned from server only")
}

func (rowid *RowID) decode() []byte {
	var convertRowIDToByte = func(number int64, size int) []byte {
		buffer := []byte{
			65, 66, 67, 68, 69, 70, 71, 72,
			73, 74, 75, 76, 77, 78, 79, 80,
			81, 82, 83, 84, 85, 86, 87, 88,
			89, 90, 97, 98, 99, 100, 101, 102,
			103, 104, 105, 106, 107, 108, 109, 110,
			111, 112, 113, 114, 115, 116, 117, 118,
			119, 120, 121, 122, 48, 49, 50, 51,
			52, 53, 54, 55, 56, 57, 43, 47,
		}
		output := make([]byte, size)
		for x := size; x > 0; x-- {
			output[x-1] = buffer[number&0x3F]
			if number >= 0 {
				number = number >> 6
			} else {
				number = (number >> 6) + (2 << (32 + ^6))
			}
		}
		return output
	}
	output := make([]byte, 0, 18)
	output = append(output, convertRowIDToByte(rowid.Rba, 6)...)
	output = append(output, convertRowIDToByte(rowid.PartitionID, 3)...)
	output = append(output, convertRowIDToByte(rowid.BlockNumber, 6)...)
	output = append(output, convertRowIDToByte(rowid.SlotNumber, 3)...)
	return output
}

func (rowid *RowID) physicalRawIDToByteArray(data []byte) []byte {
	// physical
	temp32 := binary.BigEndian.Uint32(data[1:5])
	rowid.Rba = int64(temp32)
	temp16 := binary.BigEndian.Uint16(data[5:7])
	rowid.PartitionID = int64(temp16)
	temp32 = binary.BigEndian.Uint32(data[7:11])
	rowid.BlockNumber = int64(temp32)
	temp16 = binary.BigEndian.Uint16(data[11:13])
	rowid.SlotNumber = int64(temp16)
	if rowid.Rba == 0 {
		return []byte(fmt.Sprintf("%08X.%04X.%04X", rowid.BlockNumber, rowid.SlotNumber, rowid.PartitionID))
	}
	return rowid.decode()
}

// logicalRawIDToByteArray returns the text of a logical UROWID, which an
// index organized table has: a * and the bytes after the first in base 64,
// with no padding, as python-oracledb writes it. go-ora left the first byte
// zero and wrote each group of four over the last, so the server refused
// the text with ORA-01410.
func (rowid *RowID) logicalRawIDToByteArray(data []byte) []byte {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	if len(data) == 0 {
		return nil
	}
	output := []byte{'*'}
	in := data[1:]
	for len(in) > 0 {
		output = append(output, alphabet[in[0]>>2])
		if len(in) == 1 {
			output = append(output, alphabet[(in[0]&0x3)<<4])
			break
		}
		output = append(output, alphabet[(in[0]&0x3)<<4|in[1]>>4])
		if len(in) == 2 {
			output = append(output, alphabet[(in[1]&0xf)<<2])
			break
		}
		output = append(output, alphabet[(in[1]&0xf)<<2|in[2]>>6], alphabet[in[2]&0x3f])
		in = in[3:]
	}
	return output
}
