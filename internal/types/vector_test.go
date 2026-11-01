package types

import (
	"bytes"
	"math"
	"reflect"
	"testing"
)

// TestVectorImage checks the image of a vector against the bytes that
// python-oracledb writes for it.
func TestVectorImage(t *testing.T) {
	norm := make([]byte, 8)
	cat := func(parts ...[]byte) []byte { return bytes.Join(parts, nil) }
	tests := []struct {
		name string
		in   any
		want []byte
	}{
		{"float32", []float32{1, 0, -1}, cat([]byte{0xDB, 0, 0, 0x12, 2, 0, 0, 0, 3}, norm,
			[]byte{0xBF, 0x80, 0, 0}, []byte{0x80, 0, 0, 0}, []byte{0x40, 0x7F, 0xFF, 0xFF})},
		{"float64", []float64{0}, cat([]byte{0xDB, 0, 0, 0x12, 3, 0, 0, 0, 1}, norm, []byte{0x80, 0, 0, 0, 0, 0, 0, 0})},
		{"int8", []int8{-128, -1, 0, 127}, cat([]byte{0xDB, 0, 0, 0x12, 4, 0, 0, 0, 4}, norm, []byte{0x80, 0xFF, 0, 0x7F})},
		{"binary", []byte{0xF0, 0x01}, cat([]byte{0xDB, 1, 0, 0x10, 5, 0, 0, 0, 16}, norm, []byte{0xF0, 0x01})},
		{"sparse", SparseVector{Dimensions: 300, Indices: []uint32{2, 299}, Values: []int8{-5, 7}},
			cat([]byte{0xDB, 2, 0, 0x32, 4, 0, 0, 1, 44}, norm, []byte{0, 2, 0, 0, 0, 2, 0, 0, 1, 43, 0xFB, 7})},
	}
	for _, test := range tests {
		got, err := encodeVector(test.in)
		if err != nil {
			t.Errorf("%s: %v", test.name, err)
			continue
		}
		if !bytes.Equal(got, test.want) {
			t.Errorf("%s: got % x\nwant % x", test.name, got, test.want)
		}
		back, err := decodeVector(got)
		if err != nil || !reflect.DeepEqual(back, test.in) {
			t.Errorf("%s: read back %v, %v", test.name, back, err)
		}
	}
}

// TestVectorValues reads each kind of vector back through a Vector, twice,
// which go-ora got wrong for a value that it decoded in place.
func TestVectorValues(t *testing.T) {
	values := []any{
		[]float32{1.5, -2.25, 0, float32(math.Copysign(0, -1)), math.MaxFloat32},
		[]float64{1.5, -2.25, 0, math.SmallestNonzeroFloat64},
		[]int8{-128, -5, 0, 5, 127},
		[]byte{0, 0xFF, 0x55, 0xAA},
		SparseVector{Dimensions: 16, Indices: []uint32{0, 15}, Values: []float32{1.5, -2}},
		SparseVector{Dimensions: 16, Indices: []uint32{3}, Values: []float64{6.25}},
		SparseVector{Dimensions: 16, Indices: []uint32{}, Values: []int8{}},
	}
	for _, in := range values {
		v, err := CreateVector(in)
		if err != nil {
			t.Errorf("%v: %v", in, err)
			continue
		}
		for range 2 {
			if got, err := v.Value(); err != nil || !reflect.DeepEqual(got, in) {
				t.Errorf("%v: got %v, %v", in, got, err)
			}
		}
	}
	f := []float32{4.5}
	v, err := CreateVector(&f)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := v.Value(); err != nil || !reflect.DeepEqual(got, f) {
		t.Errorf("a pointer: got %v, %v", got, err)
	}
	if v, err := CreateVector(nil); err != nil || v.bValue != nil {
		t.Errorf("nil: got %v, %v", v.bValue, err)
	}
}

func TestVectorRefuses(t *testing.T) {
	for name, in := range map[string]any{
		"no dimensions":       []float32{},
		"sparse of none":      SparseVector{Values: []float32{}},
		"other type":          []int{1},
		"slice of slices":     [][]float64{{1}},
		"sparse of binary":    SparseVector{Dimensions: 8, Indices: []uint32{0}, Values: []byte{1}},
		"counts differ":       SparseVector{Dimensions: 8, Indices: []uint32{0, 1}, Values: []float32{1}},
		"index past the dims": SparseVector{Dimensions: 8, Indices: []uint32{8}, Values: []float32{1}},
	} {
		if _, err := CreateVector(in); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	for name, in := range map[string][]byte{
		"empty":       nil,
		"no magic":    {0xDA, 0, 0, 0, 2, 0, 0, 0, 1},
		"version 3":   {0xDB, 3, 0, 0, 2, 0, 0, 0, 1},
		"cut":         {0xDB, 0, 0, 0x12, 2, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0xBF, 0x80, 0, 0},
		"format 9":    {0xDB, 0, 0, 0, 9, 0, 0, 0, 0},
		"cut indices": {0xDB, 2, 0, 0x20, 2, 0, 0, 0, 9, 0, 3, 0, 0},
	} {
		if _, err := decodeVector(in); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}
