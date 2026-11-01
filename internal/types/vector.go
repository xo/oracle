package types

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

type VectorType int

const (
	VectorSparse VectorType = iota
	VectorDense
)

// The image of a VECTOR, as python-oracledb writes it: a magic byte, a
// version, flags, the format of the elements, the number of dimensions, 8
// bytes for a norm that the server fills, and the elements. A sparse vector
// puts the number of its elements and their indices before them.
const (
	vectorMagic            = 0xDB
	vectorVersionBase      = 0
	vectorVersionBinary    = 1
	vectorVersionSparse    = 2
	vectorFlagNorm         = 0x0002
	vectorFlagNormReserved = 0x0010
	vectorFlagSparse       = 0x0020
	vectorFloat32          = 2
	vectorFloat64          = 3
	vectorInt8             = 4
	vectorBinary           = 5
	vectorHeaderLen        = 17
)

// SparseVector is a sparse VECTOR. Dimensions is the number of its
// dimensions, Indices the indices of the elements that are not zero, and
// Values the values of those elements: a []float32, a []float64 or a []int8
// as long as Indices.
type SparseVector struct {
	Dimensions uint32
	Indices    []uint32
	Values     any
}

type Vector struct {
	Basic
	loc Locator
	lobBase
}

func (vector *Vector) Locator() Locator {
	if vector.lobBase.Locator() != nil {
		return vector.lobBase.Locator()
	}
	return vector.loc
}
func (vector *Vector) Upload() error {
	return vector.uploadData(vector.bValue, 0, 0)
}

// SetValue sets the vector to input: a []float32, a []float64, a []int8, a
// []byte of BINARY, which holds 8 dimensions in each byte, a SparseVector, a
// pointer to one of them, or a Vector.
func (vector *Vector) SetValue(input any) error {
	switch v := input.(type) {
	case nil:
		vector.bValue, vector.loc = nil, nil
		return nil
	case Vector:
		*vector = v
		return nil
	case *Vector:
		*vector = *v
		return nil
	}
	image, err := encodeVector(input)
	if err != nil {
		return err
	}
	vector.bValue = image
	vector.loc = NewInlineLocator(uint64(len(image)))
	return nil
}

// Value returns the elements of the vector, as SetValue takes them: a
// []float32, a []float64, a []int8, a []byte or a SparseVector. A NULL is nil.
func (vector *Vector) Value() (any, error) {
	if len(vector.bValue) == 0 {
		return nil, nil
	}
	return decodeVector(vector.bValue)
}

// CreateVector returns a VECTOR that holds array, which is any value that
// SetValue takes.
func CreateVector(array any) (*Vector, error) {
	v := new(Vector)
	return v, v.SetValue(array)
}

// Load reads the whole value and watches no context. See Lob.
func (vector *Vector) Load() error {
	var err error
	vector.bValue, err = vector.load()
	return err
}

func (vector *Vector) Read(ctx context.Context) error {
	var err error
	vector.bValue, err = vector.ReadFromPos(ctx, 0)
	return err
}

func (vector *Vector) Scan(input any) error {
	return vector.SetValue(input)
}

// vectorFormat returns the format of the elements values and their number.
func vectorFormat(values any) (format byte, n int, err error) {
	switch v := values.(type) {
	case []float32:
		return vectorFloat32, len(v), nil
	case []float64:
		return vectorFloat64, len(v), nil
	case []int8:
		return vectorInt8, len(v), nil
	case []byte:
		return vectorBinary, len(v), nil
	}
	return 0, 0, fmt.Errorf("binding a vector of %T: %w", values, errVectorType)
}

var errVectorType = errors.New("a vector holds a []float32, a []float64, a []int8, a []byte or a SparseVector")

// encodeVector returns the image of the vector v.
func encodeVector(v any) ([]byte, error) {
	switch p := v.(type) {
	case *[]float32:
		v = *p
	case *[]float64:
		v = *p
	case *[]int8:
		v = *p
	case *[]byte:
		v = *p
	case *SparseVector:
		v = *p
	}
	var (
		version = byte(vectorVersionBase)
		flags   = uint16(vectorFlagNormReserved)
		values  = v
		dims    uint32
	)
	sparse, isSparse := v.(SparseVector)
	if isSparse {
		values = sparse.Values
	}
	format, n, err := vectorFormat(values)
	if err != nil {
		return nil, err
	}
	switch {
	case isSparse && sparse.Dimensions == 0, !isSparse && n == 0:
		// the server refuses a vector of no dimensions, as python-oracledb
		// does
		return nil, errors.New("binding a vector of no dimensions")
	case isSparse:
		if format == vectorBinary {
			return nil, errors.New("binding a sparse vector of BINARY: Oracle has none")
		}
		if n != len(sparse.Indices) {
			return nil, fmt.Errorf("binding a sparse vector of %d indices and %d values: the counts differ", len(sparse.Indices), n)
		}
		if n > math.MaxUint16 {
			return nil, fmt.Errorf("binding a sparse vector of %d values: the most is %d", n, math.MaxUint16)
		}
		for _, i := range sparse.Indices {
			if i >= sparse.Dimensions {
				return nil, fmt.Errorf("binding a sparse vector of %d dimensions: the index %d is past them", sparse.Dimensions, i)
			}
		}
		version, flags, dims = vectorVersionSparse, flags|vectorFlagSparse|vectorFlagNorm, sparse.Dimensions
	case format == vectorBinary:
		version, dims = vectorVersionBinary, uint32(n)*8
	default:
		flags, dims = flags|vectorFlagNorm, uint32(n)
	}
	image := make([]byte, vectorHeaderLen, vectorHeaderLen+n*8)
	image[0], image[1] = vectorMagic, version
	binary.BigEndian.PutUint16(image[2:], flags)
	image[4] = format
	binary.BigEndian.PutUint32(image[5:], dims)
	if isSparse {
		image = binary.BigEndian.AppendUint16(image, uint16(n))
		for _, i := range sparse.Indices {
			image = binary.BigEndian.AppendUint32(image, i)
		}
	}
	switch v := values.(type) {
	case []float32:
		for _, f := range v {
			image = append(image, 0, 0, 0, 0)
			putBinaryFloat(image[len(image)-4:], f)
		}
	case []float64:
		for _, f := range v {
			image = append(image, 0, 0, 0, 0, 0, 0, 0, 0)
			putBinaryDouble(image[len(image)-8:], f)
		}
	case []int8:
		for _, i := range v {
			image = append(image, byte(i))
		}
	case []byte:
		image = append(image, v...)
	}
	return image, nil
}

// decodeVector returns the elements of the vector image b. go-ora read the
// number of dimensions in a width that it took from the flags, which BINARY
// and sparse vectors do not follow, and read INT8 as unsigned.
func decodeVector(b []byte) (any, error) {
	if len(b) < 9 || b[0] != vectorMagic {
		return nil, errBadVector
	}
	if b[1] > vectorVersionSparse {
		return nil, fmt.Errorf("reading a vector of the version %d: not supported", b[1])
	}
	flags, format, n := binary.BigEndian.Uint16(b[2:]), b[4], binary.BigEndian.Uint32(b[5:])
	b = b[9:]
	if flags&(vectorFlagNorm|vectorFlagNormReserved) != 0 {
		if len(b) < 8 {
			return nil, errBadVector
		}
		b = b[8:]
	}
	if flags&vectorFlagSparse == 0 {
		if format == vectorBinary {
			n /= 8
		}
		return vectorValues(b, format, int(n))
	}
	if len(b) < 2 {
		return nil, errBadVector
	}
	sparse := SparseVector{Dimensions: n}
	count := int(binary.BigEndian.Uint16(b))
	b = b[2:]
	if len(b) < count*4 {
		return nil, errBadVector
	}
	sparse.Indices = make([]uint32, count)
	for i := range sparse.Indices {
		sparse.Indices[i] = binary.BigEndian.Uint32(b[i*4:])
	}
	var err error
	sparse.Values, err = vectorValues(b[count*4:], format, count)
	if err != nil {
		return nil, err
	}
	return sparse, nil
}

var errBadVector = errors.New("reading a vector: the image is cut or has no magic byte")

// vectorValues returns the n elements of the format in b.
func vectorValues(b []byte, format byte, n int) (any, error) {
	size := 1
	switch format {
	case vectorFloat32:
		size = 4
	case vectorFloat64:
		size = 8
	case vectorInt8, vectorBinary:
	default:
		return nil, fmt.Errorf("reading a vector of the format %d: not supported", format)
	}
	if len(b) < n*size {
		return nil, errBadVector
	}
	switch format {
	case vectorFloat32:
		v := make([]float32, n)
		for i := range v {
			v[i] = getBinaryFloat(b[i*4:])
		}
		return v, nil
	case vectorFloat64:
		v := make([]float64, n)
		for i := range v {
			v[i] = getBinaryDouble(b[i*8:])
		}
		return v, nil
	case vectorInt8:
		v := make([]int8, n)
		for i := range v {
			v[i] = int8(b[i])
		}
		return v, nil
	}
	return append([]byte(nil), b[:n]...), nil
}
