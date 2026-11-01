package types

import "encoding/binary"

// Locator is a LOB locator: the token that names a LOB on the server, which a
// row holds in place of the value of a LOB column.
type Locator []byte

// NewInlineLocator returns a locator for a value of dataLen bytes that the
// driver holds itself, with no LOB on the server: a LOB value that came inline
// in a row, or a JSON or VECTOR value that the driver sends. Byte 3 marks it,
// and the length is at byte 10, so the code of LOBs reads the value with no
// round trip. Oracle calls it a quasi locator.
func NewInlineLocator(dataLen uint64) Locator {
	var ret Locator = make([]byte, 40)
	ret[1] = 38
	ret[3] = 4
	ret[4] = 97
	ret[5] = 8
	ret[9] = 1
	binary.BigEndian.PutUint64(ret[10:], dataLen)
	return ret
}

// RowLocator returns the locator to send with the value b of one row: loc,
// or an inline locator of the length of b when loc is inline or nil. A batch
// writes every row with the coder of its last row, and go-ora sent the
// locator of that row, which holds its length, so the server refused a longer
// row with ORA-51803 and read every row as NULL when the last was NULL.
func RowLocator(loc Locator, b []byte) Locator {
	switch {
	case loc != nil && !loc.IsInline():
		return loc
	case len(b) == 0:
		return nil
	}
	return NewInlineLocator(uint64(len(b)))
}

// IsInline reports whether loc is a locator of NewInlineLocator.
func (loc Locator) IsInline() bool {
	return len(loc) > 3 && loc[3] == 4
}
func (loc Locator) IsValueBased() bool {
	return len(loc) > 4 && loc[4]&0x20 == 0x20
}
func (loc Locator) IsTemporary() bool {
	return len(loc) > 7 && loc[7]&1 == 1 || len(loc) > 4 && loc[4]&0x40 == 0x40 || loc.IsValueBased()
}
func (loc Locator) IsVarWidthChar() bool {
	return len(loc) > 6 && loc[6]&0x80 == 0x80
}

func (loc Locator) IsLittleEndian() bool {
	return len(loc) > 7 && loc[7]&0x40 == 0x40
}

func (loc Locator) IsReadOnly() bool {
	return len(loc) > 6 && loc[6]&1 == 1
}
