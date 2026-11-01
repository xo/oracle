package oson

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/xo/oracle/internal/types"
)

/*
	json operations:

flag = 0x80 = start object
flag = 0xC0 = start array
if object size or array size > 0xFFFF flag | 32
then write byte flag | 4

string:
if length < 31 op = length
if length < 0x100 op = 51
if length < 0x10000 op = 55
else: op = 56

double op = 54 followed by value
bool op = 49 if true and op=50 if false

decimal
convert to array
if array length <= 8 then op = (length - 1) | 0x60
if length < 0x100 op = 0x74 followed by length followed by bytes

int32
op = array length | 0x40

int64
op = length | 0x50

number as string
convert to string length should be <= 256 then op = 53

bytes
if length < 0x10000 op = 58 else op = 59
then length + value

Id
length should be <= 16
op = 0x7E

float
convert to bytes
op = 7F

null op = 48

date
op = 60 and length of buffer = 7

timestamp
op = 57 with fraction size = 11
op = 0x7D without fraction size = 7

timestamp TZ
op = 0x7C size = 13

interval ym
op = 61 and length = 5

interval DS
op = 62 and length = 11
*/

/*
	operation:

1- determine type of json input like string, dictionary and objects
2- parse input
*/

//var jsonDecoder map[int]FieldDecoder

// Encode returns the OSON image of mainObj, a map or a slice, as
// python-oracledb encodes one. Names of up to 255 bytes go in the segment of
// short names and longer ones in the segment of long names, in version 3,
// each sorted, and the field IDs count the short names first. The flags are
// set before the tree is encoded, since an object writes its field IDs in the
// width that they give. go-ora set them after, wrote long names with a
// length of one byte, and left the names in the order of a map.
func Encode(mainObj any) ([]byte, error) {
	switch mainObj.(type) {
	case nil, number, string, bool:
		return encodeScalar(mainObj)
	}
	header := &header{flags: 0x2106, keys: keyCollection{}}
	header.keys.extractKeys(mainObj)
	var short, long keyCollection
	for _, k := range header.keys {
		if k.size > 0xFFFF {
			return nil, fmt.Errorf("encoding JSON: the field name of %d bytes is longer than 65,535", k.size)
		}
		if k.size > 0xFF {
			long = append(long, k)
		} else {
			short = append(short, k)
		}
	}
	short.Sort()
	long.Sort()
	shortSeg, longSeg := short.encode(1), long.encode(2)
	header.keys = append(append(keyCollection{}, short...), long...)
	switch n := len(header.keys); {
	case n > 0xFFFF:
		header.flags |= 0x8
	case n > 0xFF:
		header.flags |= 0x400
	}
	if len(shortSeg) > 0xFFFF {
		header.flags |= 0x800
	}
	var objectData []byte
	var err error
	switch mainObj := mainObj.(type) {
	case map[string]any:
		obj, err := newObjectField(mainObj, header)
		if err != nil {
			return nil, err
		}
		objectData, err = obj.Encode()
		if err != nil {
			return nil, err
		}
	case []any:
		obj, err := newArrayField(mainObj, header)
		if err != nil {
			return nil, err
		}
		objectData, err = obj.Encode()
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("cannot set value of %T into JSON", mainObj)
	}
	if len(objectData) > 0xFFFF {
		header.flags |= 0x1000
	}
	put := func(b []byte, v int, wide bool) []byte {
		if wide {
			return binary.BigEndian.AppendUint32(b, uint32(v))
		}
		return binary.BigEndian.AppendUint16(b, uint16(v))
	}
	version := byte(1)
	if len(long) > 0 {
		version = 3
	}
	out := []byte{0xff, 0x4a, 0x5a, version}
	out = binary.BigEndian.AppendUint16(out, header.flags)
	switch {
	case header.flags&0x8 != 0:
		out = binary.BigEndian.AppendUint32(out, uint32(len(short)))
	case header.flags&0x400 != 0:
		out = binary.BigEndian.AppendUint16(out, uint16(len(short)))
	default:
		out = append(out, uint8(len(short)))
	}
	out = put(out, len(shortSeg), header.flags&0x800 != 0)
	longWide := len(longSeg) > 0xFFFF
	if len(long) > 0 {
		var flags2 uint16
		if !longWide {
			flags2 = 0x100
		}
		out = binary.BigEndian.AppendUint16(out, flags2)
		out = binary.BigEndian.AppendUint32(out, uint32(len(long)))
		out = binary.BigEndian.AppendUint32(out, uint32(len(longSeg)))
	}
	out = put(out, len(objectData), header.flags&0x1000 != 0)
	out = binary.BigEndian.AppendUint16(out, 0) // the tiny nodes
	for _, k := range short {
		out = append(out, k.hash)
	}
	for _, k := range short {
		out = put(out, k.offset, header.flags&0x800 != 0)
	}
	out = append(out, shortSeg...)
	for _, k := range long {
		out = binary.BigEndian.AppendUint16(out, uint16(k.hash32))
	}
	for _, k := range long {
		out = put(out, k.offset, longWide)
	}
	out = append(out, longSeg...)
	return append(out, objectData...), err
}

func isObject(opCode uint8) bool {
	return opCode&0xC0 == 0x80
}
func isArray(opCode uint8) bool {
	return opCode&0xC0 == 0xC0
}
func readObjectHeader(buffer *bytes.Reader, header *header, opCode uint8) (childCount int, keyIndex []int, err error) {
	var useWideOffset = opCode&0x20 != 0
	switch opCode & 0x18 {
	case 0x18:
		var offset int64
		if useWideOffset {
			var temp uint32
			err = binary.Read(buffer, binary.BigEndian, &temp)
			if err != nil {
				return
			}
			offset = int64(temp)
		} else {
			var temp uint16
			err = binary.Read(buffer, binary.BigEndian, &temp)
			if err != nil {
				return
			}
			offset = int64(temp)
		}
		offset = header.absoluteOffset(offset)
		var currentOffset int64
		currentOffset, err = buffer.Seek(0, io.SeekCurrent)
		if err != nil {
			return
		}
		_, err = buffer.Seek(offset, io.SeekStart)
		if err != nil {
			return
		}
		opCode, err = buffer.ReadByte()
		if err != nil {
			return
		}
		childCount, keyIndex, err = readObjectHeader(buffer, header, opCode)
		if err != nil {
			return
		}
		_, err = buffer.Seek(currentOffset, io.SeekStart)
		return
	case 0:
		var temp uint8
		temp, err = buffer.ReadByte()
		if err != nil {
			return
		}
		childCount = int(temp)
	case 8:
		var temp uint16
		err = binary.Read(buffer, binary.BigEndian, &temp)
		if err != nil {
			return
		}
		childCount = int(temp)
	case 0x10:
		var temp uint32
		err = binary.Read(buffer, binary.BigEndian, &temp)
		if err != nil {
			return
		}
		childCount = int(temp)
	}
	if childCount > 0 && isObject(opCode) {
		keyIndex = make([]int, childCount)
		for index := 0; index < childCount; index++ {
			if header.flags&0x8 > 0 {
				var temp uint32
				err = binary.Read(buffer, binary.BigEndian, &temp)
				keyIndex[index] = int(temp)
			} else if header.flags&0x400 > 0 {
				var temp uint16
				err = binary.Read(buffer, binary.BigEndian, &temp)
				keyIndex[index] = int(temp)
			} else {
				var temp uint8
				temp, err = buffer.ReadByte()
				keyIndex[index] = int(temp)
			}
			if err != nil {
				return
			}
		}
	}
	return
}

func decodeNode(buffer *bytes.Reader, header *header) (Field, error) {
	var opCode uint8
	var err error
	err = header.pushCurrentOffset(buffer)
	if err != nil {
		return nil, err
	}
	defer header.popBaseOffset()

	// object header contain
	// 1- opCode
	// 2- child count
	// 3- key index in case of object not array
	opCode, err = buffer.ReadByte()
	if err != nil {
		return nil, err
	}
	var keyIndex []int
	if isArray(opCode) || isObject(opCode) {
		var childCount int
		childCount, keyIndex, err = readObjectHeader(buffer, header, opCode)
		if err != nil {
			return nil, err
		}
		var useWideOffset = opCode&0x20 != 0
		var index int
		var children []Field
		offset := make([]int64, childCount)
		if childCount > 0 {
			if useWideOffset {
				for index = 0; index < childCount; index++ {
					var temp uint32
					err = binary.Read(buffer, binary.BigEndian, &temp)
					if err != nil {
						return nil, err
					}
					offset[index] = int64(temp)
				}
			} else {
				for index = 0; index < childCount; index++ {
					var temp uint16
					err = binary.Read(buffer, binary.BigEndian, &temp)
					if err != nil {
						return nil, err
					}
					offset[index] = int64(temp)
				}
			}

			children = make([]Field, childCount)

			for index = 0; index < childCount; index++ {
				_, err = buffer.Seek(header.absoluteOffset(offset[index]), io.SeekStart)
				if err != nil {
					return nil, err
				}
				children[index], err = decodeNode(buffer, header)
				if err != nil {
					return nil, err
				}
				if keyIndex != nil {
					children[index].SetKeyIndex(int(keyIndex[index]))
				}
			}
		}

		if isArray(opCode) {
			value := make([]any, childCount)
			if childCount > 0 {
				for index = 0; index < childCount; index++ {
					value[index], err = children[index].Value()
					if err != nil {
						return nil, err
					}
				}
			}
			return &arrayField{
				value:    value,
				opCode:   opCode,
				keyIndex: 0,
				offset:   0,
				children: children,
			}, nil
		}
		if isObject(opCode) {

			value := make(map[string]any)
			if childCount > 0 {
				for index = 0; index < childCount; index++ {
					key := header.keys[children[index].KeyIndex()-1]
					value[key.name], err = children[index].Value()
					if err != nil {
						return nil, err
					}
				}
			}
			return &objectField{
				value:    value,
				opCode:   opCode,
				keyIndex: 0,
				offset:   0,
				children: children,
			}, nil
		}
	}
	return decodeScalar(buffer, opCode)
}

// encodeScalar returns the OSON of a document that is a scalar: the header,
// whose flags say so, the size of the tree and its one node, as
// python-oracledb writes it. go-ora refused such a document.
func encodeScalar(v any) ([]byte, error) {
	var field Field
	switch v := v.(type) {
	case nil:
		field = &nullField{}
	case number:
		n, err := types.NewNumber(string(v))
		if err != nil {
			return nil, err
		}
		field = &numberField{value: *n}
	case string:
		field = newStringField(v)
	case bool:
		field = &booleanField{value: v}
	}
	node, err := field.Encode()
	if err != nil {
		return nil, err
	}
	// the flags of an inline leaf, a length in the node type and a scalar
	out := []byte{0xFF, 0x4A, 0x5A, 1, 0, 0x16}
	if len(node) > 0xFFFF {
		out[4] |= 0x10
		out = binary.BigEndian.AppendUint32(out, uint32(len(node)))
	} else {
		out = binary.BigEndian.AppendUint16(out, uint16(len(node)))
	}
	return append(out, node...), nil
}

// decodeScalar returns the scalar of OSON of the node type op, as
// python-oracledb reads it. A date, an interval and a RAW are the text that
// JSON_SERIALIZE writes for them. go-ora read none of them, read an interval
// as a NUMBER, and read no integer that holds its length in its node type.
func decodeScalar(buffer *bytes.Reader, op uint8) (Field, error) {
	read := func(n int) ([]byte, error) {
		b := make([]byte, n)
		_, err := io.ReadFull(buffer, b)
		return b, err
	}
	// readLen reads n bytes and the value of n bytes that precedes them
	readLen := func(size int) ([]byte, error) {
		b, err := read(size)
		if err != nil {
			return nil, err
		}
		n := 0
		for _, c := range b {
			n = n<<8 | int(c)
		}
		return read(n)
	}
	str := func(b []byte, err error) (Field, error) {
		return &stringField{value: string(b), bValue: b, opCode: op}, err
	}
	num := func(b []byte, err error) (Field, error) {
		field := &numberField{opCode: op}
		field.value.SetBytes(b)
		return field, err
	}
	// text returns the reader of a field of the text that format makes
	text := func(format func([]byte) (string, error)) func([]byte, error) (Field, error) {
		return func(b []byte, err error) (Field, error) {
			if err != nil {
				return nil, err
			}
			v, err := format(b)
			return &valueField{value: v, opCode: op}, err
		}
	}
	switch {
	case op == 0x30:
		return &nullField{opCode: op}, nil
	case op == 0x31, op == 0x32:
		return &booleanField{value: op == 0x31, opCode: op}, nil
	case op == 0x3C, op == 0x7D:
		// a DATE, and a TIMESTAMP of no fraction
		b, err := read(7)
		return text(dateText)(b, err)
	case op == 0x39:
		b, err := read(11)
		return text(dateText)(b, err)
	case op == 0x7C:
		b, err := read(13)
		return text(dateText)(b, err)
	case op == 0x7F:
		b, err := read(4)
		field := &binaryFloatField{opCode: op}
		field.value.SetBytes(b)
		return field, err
	case op == 0x36:
		b, err := read(8)
		field := &binaryDoubleField{opCode: op}
		field.value.SetBytes(b)
		return field, err
	case op == 0x3E:
		b, err := read(11)
		return text(dayText)(b, err)
	case op == 0x3D:
		b, err := read(5)
		return text(monthText)(b, err)
	case op == 0x33:
		return str(readLen(1))
	case op == 0x37:
		return str(readLen(2))
	case op == 0x38:
		return str(readLen(4))
	case op == 0x34, op == 0x74:
		return num(readLen(1))
	case op == 0x7E:
		// an ID, which python-oracledb reads as bytes
		return text(hexText)(readLen(1))
	case op == 0x3A:
		return text(hexText)(readLen(2))
	case op == 0x3B:
		return text(hexText)(readLen(4))
	case op == 0x7B:
		// an extended type, of which a VECTOR, 0x01, is the one there is
		head, err := read(5)
		if err != nil {
			return nil, err
		}
		if head[0] != 0x01 {
			return nil, fmt.Errorf("reading a JSON value of the extended type %#x: not supported", head[0])
		}
		image, err := read(int(binary.BigEndian.Uint32(head[1:])))
		field := &vectorField{opCode: op}
		field.value.SetBytes(image)
		return field, err
	case op&0xF0 == 0x20, op&0xF0 == 0x60:
		// a NUMBER, whose length less 1 is in the node type
		return num(read(int(op&0x0F) + 1))
	case op&0xF0 == 0x40, op&0xF0 == 0x50:
		// an integer, whose length is in the node type
		return num(read(int(op & 0x0F)))
	case op&0xE0 == 0:
		// a string, whose length is the node type
		return str(read(int(op)))
	}
	return nil, fmt.Errorf("reading a JSON value of the node type %#x: not supported", op)
}

// dateText returns the text of the image b of a DATE, a TIMESTAMP or a
// TIMESTAMP WITH TIME ZONE, as JSON_SERIALIZE writes it. A fraction has 6
// digits, or 9 when it needs them. A time with no zone shows no fraction of
// zero, and a time with a zone shows it, and Z for UTC.
func dateText(b []byte) (string, error) {
	var d types.Date
	d.SetBytes(b)
	v, err := d.Value()
	if err != nil {
		return "", err
	}
	t := v.(time.Time)
	zone := len(b) == 13
	s := t.Format("2006-01-02T15:04:05")
	switch ns := t.Nanosecond(); {
	case ns%1000 != 0:
		s += fmt.Sprintf(".%09d", ns)
	case ns != 0, zone:
		s += fmt.Sprintf(".%06d", ns/1000)
	}
	if !zone {
		return s, nil
	}
	if _, offset := t.Zone(); offset == 0 {
		return s + "Z", nil
	}
	return s + t.Format("-07:00"), nil
}

// fields returns the signed fields of an interval, each of 4 bytes, biased by
// 2^31, or of 1 byte, biased by 60, and whether one is negative.
func fields(b []byte, sizes ...int) ([]int64, bool) {
	var out []int64
	negative := false
	for _, size := range sizes {
		var v int64
		if size == 4 {
			v = int64(binary.BigEndian.Uint32(b)) - 0x80000000
		} else {
			v = int64(b[0]) - 60
		}
		b = b[size:]
		if v < 0 {
			negative, v = true, -v
		}
		out = append(out, v)
	}
	return out, negative
}

// dayText returns the text of the image b of an INTERVAL DAY TO SECOND, as
// JSON_SERIALIZE writes it, such as P1DT2H3M4.500000S or -PT8S.
func dayText(b []byte) (string, error) {
	f, negative := fields(b, 4, 1, 1, 1, 4)
	days, hours, minutes, seconds, nanos := f[0], f[1], f[2], f[3], f[4]
	if days == 0 && hours == 0 && minutes == 0 && seconds == 0 && nanos == 0 {
		return "P0D", nil
	}
	var s strings.Builder
	if negative {
		s.WriteByte('-')
	}
	s.WriteByte('P')
	if days != 0 {
		fmt.Fprintf(&s, "%dD", days)
	}
	if hours != 0 || minutes != 0 || seconds != 0 || nanos != 0 {
		s.WriteByte('T')
		if hours != 0 {
			fmt.Fprintf(&s, "%dH", hours)
		}
		if minutes != 0 {
			fmt.Fprintf(&s, "%dM", minutes)
		}
		switch {
		case nanos%1000 != 0:
			fmt.Fprintf(&s, "%d.%09dS", seconds, nanos)
		case nanos != 0:
			fmt.Fprintf(&s, "%d.%06dS", seconds, nanos/1000)
		case seconds != 0:
			fmt.Fprintf(&s, "%dS", seconds)
		}
	}
	return s.String(), nil
}

// monthText returns the text of the image b of an INTERVAL YEAR TO MONTH, as
// JSON_SERIALIZE writes it, such as P1Y2M or -P4M.
func monthText(b []byte) (string, error) {
	f, negative := fields(b, 4, 1)
	s := "P"
	if negative {
		s = "-P"
	}
	if f[0] != 0 {
		s += fmt.Sprintf("%dY", f[0])
	}
	if f[1] != 0 {
		s += fmt.Sprintf("%dM", f[1])
	}
	if f[0] == 0 && f[1] == 0 {
		s += "0Y"
	}
	return s, nil
}

// hexText returns the bytes b in upper case hex, as JSON_SERIALIZE writes a
// RAW.
func hexText(b []byte) (string, error) {
	return strings.ToUpper(hex.EncodeToString(b)), nil
}

func Decode(data []byte) (any, error) {
	buffer := bytes.NewReader(data)
	header := &header{}
	err := header.read(buffer)
	if err != nil {
		return nil, err
	}
	obj, err := decodeNode(buffer, header)
	if err != nil {
		return nil, err
	}
	return obj.Value()
}
