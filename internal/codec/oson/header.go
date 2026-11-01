package oson

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

type header struct {
	version              int
	flags                uint16
	flags2               uint16
	keys                 keyCollection
	keyDataLen           int
	nodeDataLen          int
	tinyNodeCount        uint16
	nodeOffset           int64
	currentObjBaseOffset []int64
}

// read reads the header of an OSON image and its field names, and leaves
// buffer at the tree of nodes. It follows the decoder of python-oracledb. A
// version 3 image holds field names longer than 255 bytes in a second
// segment, after the short ones. go-ora took the version byte for part of
// the magic, so each version 3 image was corrupted, and it read a count of
// 2 bytes where there are 4, an offset of 2 bytes where there can be 4, and
// put the long names in place of the short ones.
func (h *header) read(buffer *bytes.Reader) error {
	var magic [3]byte
	if _, err := io.ReadFull(buffer, magic[:]); err != nil {
		return err
	}
	if magic != [3]byte{0xff, 0x4a, 0x5a} {
		return errors.New("corrupted json data")
	}
	version, err := buffer.ReadByte()
	if err != nil {
		return err
	}
	h.version = int(version)
	if h.version != 1 && h.version != 3 {
		return fmt.Errorf("unsupported json version %d", h.version)
	}
	if err := binary.Read(buffer, binary.BigEndian, &h.flags); err != nil {
		return err
	}
	readUint := func(wide bool) (int, error) {
		if wide {
			var v uint32
			err := binary.Read(buffer, binary.BigEndian, &v)
			return int(v), err
		}
		var v uint16
		err := binary.Read(buffer, binary.BigEndian, &v)
		return int(v), err
	}
	if h.flags&0x10 != 0 {
		// a scalar, whose header is the size of its tree
		if _, err := readUint(h.flags&0x1000 != 0); err != nil {
			return err
		}
		h.nodeOffset, err = buffer.Seek(0, io.SeekCurrent)
		return err
	}
	shortCount, err := h.readKeyCount(buffer)
	if err != nil {
		return err
	}
	shortWide := h.flags&0x800 != 0
	if h.keyDataLen, err = readUint(shortWide); err != nil {
		return err
	}
	var longCount, longSegSize int
	longWide := true
	if h.version == 3 {
		if err := binary.Read(buffer, binary.BigEndian, &h.flags2); err != nil {
			return err
		}
		longWide = h.flags2&0x100 == 0
		if longCount, err = readUint(true); err != nil {
			return err
		}
		if longSegSize, err = readUint(true); err != nil {
			return err
		}
	}
	if h.nodeDataLen, err = readUint(h.flags&0x1000 != 0); err != nil {
		return err
	}
	if err := binary.Read(buffer, binary.BigEndian, &h.tinyNodeCount); err != nil {
		return err
	}
	h.keys = nil
	if err := h.readKeys(buffer, shortCount, 1, shortWide, h.keyDataLen, 1); err != nil {
		return err
	}
	if err := h.readKeys(buffer, longCount, 2, longWide, longSegSize, 2); err != nil {
		return err
	}
	h.nodeOffset, err = buffer.Seek(0, io.SeekCurrent)
	return err
}

// readKeys reads count field names: an array of hashes of hashSize bytes,
// an array of offsets of 2 bytes, or 4 when wide, and a segment of segSize
// bytes, which holds each name after its length of lenSize bytes. It leaves
// buffer after the segment.
func (h *header) readKeys(buffer *bytes.Reader, count, hashSize int, wide bool, segSize, lenSize int) error {
	if count == 0 {
		return nil
	}
	hashes := make([]byte, count*hashSize)
	if _, err := io.ReadFull(buffer, hashes); err != nil {
		return err
	}
	offsets := make([]int, count)
	for i := range offsets {
		var err error
		if wide {
			var v uint32
			err = binary.Read(buffer, binary.BigEndian, &v)
			offsets[i] = int(v)
		} else {
			var v uint16
			err = binary.Read(buffer, binary.BigEndian, &v)
			offsets[i] = int(v)
		}
		if err != nil {
			return err
		}
	}
	seg := make([]byte, segSize)
	if _, err := io.ReadFull(buffer, seg); err != nil {
		return err
	}
	for i, off := range offsets {
		if off+lenSize > len(seg) {
			return errors.New("corrupted json data")
		}
		n := int(seg[off])
		if lenSize == 2 {
			n = int(binary.BigEndian.Uint16(seg[off:]))
		}
		if off+lenSize+n > len(seg) {
			return errors.New("corrupted json data")
		}
		k := key{name: string(seg[off+lenSize : off+lenSize+n]), offset: off, size: n}
		if hashSize == 1 {
			k.hash = hashes[i]
		}
		h.keys = append(h.keys, k)
	}
	return nil
}

func (h *header) readKeyCount(buffer *bytes.Reader) (int, error) {
	if h.flags&0x8 != 0 {
		var keyCount uint32
		err := binary.Read(buffer, binary.BigEndian, &keyCount)
		return int(keyCount), err
	}
	if h.flags&0x400 != 0 {
		var keyCount uint16
		err := binary.Read(buffer, binary.BigEndian, &keyCount)
		return int(keyCount), err
	}
	keyCount, err := buffer.ReadByte()
	return int(keyCount), err
}
func (h *header) isRelativeOffsetUsed() bool {
	return h.flags&1 != 0
}
func (h *header) pushBaseOffset(offset int64) {
	h.currentObjBaseOffset = append(h.currentObjBaseOffset, offset)
}
func (h *header) pushCurrentOffset(buffer *bytes.Reader) error {
	var baseOffset int64
	var err error
	baseOffset, err = buffer.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	h.pushBaseOffset(baseOffset)
	return nil
}
func (h *header) popBaseOffset() int64 {
	length := len(h.currentObjBaseOffset)
	if length == 0 {
		return 0
	}
	result := h.currentObjBaseOffset[length-1]
	h.currentObjBaseOffset = h.currentObjBaseOffset[:length-1]
	return result
}
func (h *header) currentBaseOffset() int64 {
	length := len(h.currentObjBaseOffset)
	if length == 0 {
		return 0
	}
	return h.currentObjBaseOffset[length-1]
}
func (h *header) absoluteOffset(offset int64) int64 {
	if h.isRelativeOffsetUsed() {
		return h.currentBaseOffset() + offset
	} else {
		return h.nodeOffset + offset
	}
}
