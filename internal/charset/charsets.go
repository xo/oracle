package charset

import (
	"bytes"
	"compress/flate"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
)

// charsetData holds the tables of the 236 Oracle charsets that the driver
// converts. They were a generated Go file of 11 MB, a switch of literals that
// made a build 10 MB larger. The blob holds the same tables, as varints, one
// deflate stream for each charset, and a charset is read the first time it
// is used.
//
// The form is uvarint n, then n entries of uvarint ID and uvarint size, then
// the n deflate streams in that order. Each stream holds uvarint CharWidth,
// eReplace and dReplace, then dBuffer and dBuffer2, each as uvarint count,
// with 0 for nil and len+1 otherwise, and varint values, then eBuffer and
// leading, each as uvarint count as above and pairs of varint key delta and
// varint value.
//
//go:embed charsets.bin
var charsetData []byte

// charsetEntry is one charset of the blob, read the first time it is used.
type charsetEntry struct {
	data []byte
	conv func() (*Converter, error)
}

// charsets is the index of the blob, which a sync.OnceValues reads once. It
// is a package value that is the same for every database, and nothing
// changes it.
var charsets = sync.OnceValues(func() (map[int]*charsetEntry, error) {
	r := bytes.NewReader(charsetData)
	n, err := binary.ReadUvarint(r)
	if err != nil {
		return nil, fmt.Errorf("reading the count of charsets: %w", err)
	}
	type span struct{ id, size uint64 }
	spans := make([]span, n)
	for i := range spans {
		if spans[i].id, err = binary.ReadUvarint(r); err != nil {
			return nil, err
		}
		if spans[i].size, err = binary.ReadUvarint(r); err != nil {
			return nil, err
		}
	}
	off := len(charsetData) - r.Len()
	index := make(map[int]*charsetEntry, n)
	for _, s := range spans {
		end := off + int(s.size)
		if end > len(charsetData) {
			return nil, errors.New("reading the charsets: the blob is short")
		}
		e := &charsetEntry{data: charsetData[off:end]}
		id := int(s.id)
		e.conv = sync.OnceValues(func() (*Converter, error) { return decodeCharset(id, e.data) })
		index[id] = e
		off = end
	}
	return index, nil
})

// New returns the converter of the Oracle charset langID, or
// nil when the driver does not know it. The tables are shared and never
// change, so each converter that it returns is a copy of one template.
func New(langID int) *Converter {
	index, err := charsets()
	if err != nil {
		panic(err)
	}
	e, ok := index[langID]
	if !ok {
		return nil
	}
	conv, err := e.conv()
	if err != nil {
		panic(err)
	}
	return conv.Clone()
}

// decodeCharset reads one charset from its deflate stream.
func decodeCharset(id int, data []byte) (*Converter, error) {
	raw, err := io.ReadAll(flate.NewReader(bytes.NewReader(data)))
	if err != nil {
		return nil, fmt.Errorf("inflating charset %d: %w", id, err)
	}
	r := bytes.NewReader(raw)
	c := &Converter{LangID: id}
	var u [3]uint64
	for i := range u {
		if u[i], err = binary.ReadUvarint(r); err != nil {
			return nil, fmt.Errorf("reading charset %d: %w", id, err)
		}
	}
	c.CharWidth, c.eReplace, c.dReplace = int(u[0]), int(u[1]), int(u[2])
	if c.dBuffer, err = readInts(r); err != nil {
		return nil, fmt.Errorf("reading charset %d: %w", id, err)
	}
	if c.dBuffer2, err = readInts(r); err != nil {
		return nil, fmt.Errorf("reading charset %d: %w", id, err)
	}
	if c.eBuffer, err = readMap(r); err != nil {
		return nil, fmt.Errorf("reading charset %d: %w", id, err)
	}
	if c.leading, err = readMap(r); err != nil {
		return nil, fmt.Errorf("reading charset %d: %w", id, err)
	}
	if r.Len() != 0 {
		return nil, fmt.Errorf("reading charset %d: %d bytes after the tables", id, r.Len())
	}
	return c, nil
}

func readInts(r *bytes.Reader) ([]int, error) {
	n, err := binary.ReadUvarint(r)
	if err != nil || n == 0 {
		return nil, err
	}
	v := make([]int, n-1)
	for i := range v {
		x, err := binary.ReadVarint(r)
		if err != nil {
			return nil, err
		}
		v[i] = int(x)
	}
	return v, nil
}

func readMap(r *bytes.Reader) (map[int]int, error) {
	n, err := binary.ReadUvarint(r)
	if err != nil || n == 0 {
		return nil, err
	}
	m := make(map[int]int, n-1)
	prev := 0
	for range n - 1 {
		dk, err := binary.ReadVarint(r)
		if err != nil {
			return nil, err
		}
		v, err := binary.ReadVarint(r)
		if err != nil {
			return nil, err
		}
		prev += int(dk)
		m[prev] = int(v)
	}
	return m, nil
}
