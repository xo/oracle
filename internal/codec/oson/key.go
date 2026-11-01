package oson

import (
	"encoding/binary"
	"sort"
)

type key struct {
	name   string
	size   int
	hash   uint8
	hash32 uint32
	offset int
}

type keyCollection []key

// NewKey returns the key of name, with the hash that OSON and the server
// use: FNV-1a of 32 bits over the UTF-8 of the name. go-ora hashed the
// decimal code point of a character above U+07FF, so the server found no such
// key in a document that the driver encoded.
func NewKey(name string) *key {
	var h uint32 = 0x811C9DC5
	for _, b := range []byte(name) {
		h = (h ^ uint32(b)) * 16777619
	}
	return &key{name: name, size: len(name), hash: uint8(h), hash32: h}
}

// encode returns the segment of the names of the keys, each after its
// length of lenSize bytes, and sets the offset of each key in it.
func (collection keyCollection) encode(lenSize int) []byte {
	var seg []byte
	for x, k := range collection {
		collection[x].offset = len(seg)
		if lenSize == 2 {
			seg = binary.BigEndian.AppendUint16(seg, uint16(len(k.name)))
		} else {
			seg = append(seg, uint8(len(k.name)))
		}
		seg = append(seg, k.name...)
	}
	return seg
}

// Sort sorts the keys as OSON wants them: by the low byte of the hash, then
// by length, then by bytes, as python-oracledb sorts them.
func (collection *keyCollection) Sort() {
	sort.Slice(*collection, func(i, j int) bool {
		a, b := (*collection)[i], (*collection)[j]
		if a.hash != b.hash {
			return a.hash < b.hash
		}
		if a.size != b.size {
			return a.size < b.size
		}
		return a.name < b.name
	})
}

func (collection *keyCollection) Index(keyName string) int {
	for i, key := range *collection {
		if key.name == keyName {
			return i
		}
	}
	return -1
}
func (collection *keyCollection) Add(input *key) {
	for _, key := range *collection {
		if key.name == input.name {
			return
		}
	}
	*collection = append(*collection, *input)
}
func (collection *keyCollection) extractKeys(obj any) {
	switch v := obj.(type) {
	case map[string]any:
		for key, value := range v {
			collection.Add(NewKey(key))
			collection.extractKeys(value)
		}
	case []any:
		for _, item := range v {
			collection.extractKeys(item)
		}
	}
}
