package types

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
)

type VectorDataType int
type VectorType int

const (
	VECTOR_SPARSE VectorType = iota
	VECTOR_DENSE
)

type Vector struct {
	Basic
	loc Locator
	lobBase
}

func (vector *Vector) GetLocator() Locator {
	if vector.lobBase.GetLocator() != nil {
		return vector.lobBase.GetLocator()
	}
	return vector.loc
}
func (vector *Vector) Upload() error {
	return vector.uploadData(vector.bValue, 0, 0)
}

func (vector *Vector) SetValue(input any) error {
	if input == nil {
		vector.bValue = nil
		return nil
	}
	var (
		header        = []byte{219, 0}
		flag   uint16 = 2 | 16
		format byte
		length uint32
		err    error
		buffer = &bytes.Buffer{}
		data   any
	)
	switch value := input.(type) {
	case Vector:
		*vector = value
		return nil
	case *Vector:
		*vector = *value
		return nil
	case []uint8:
		format = 4
		length = uint32(len(value))
		data = value
	case *[]uint8:
		format = 4
		length = uint32(len(*value))
		data = *value
	case []*uint8:
		temp := make([]byte, 0, len(value))
		for _, val := range value {
			temp = append(temp, *val)
		}
		format = 4
		length = uint32(len(value))
		data = temp
	case []float32:
		format = 2
		length = uint32(len(value))
		data = value
	case *[]float32:
		format = 2
		length = uint32(len(*value))
		data = *value
	case []*float32:
		temp := make([]float32, 0, len(value))
		for _, val := range value {
			temp = append(temp, *val)
		}
		format = 2
		length = uint32(len(value))
		data = temp
	case []float64:
		format = 3
		length = uint32(len(value))
		data = value
	case *[]float64:
		format = 3
		length = uint32(len(*value))
		data = *value
	case []*float64:
		temp := make([]float64, 0, len(value))
		for _, val := range value {
			temp = append(temp, *val)
		}
		format = 3
		length = uint32(len(value))
		data = temp
	default:
		return vectorTypeError
	}

	_, err = buffer.Write(header)
	if err != nil {
		return err
	}
	err = binary.Write(buffer, binary.BigEndian, flag)
	if err != nil {
		return err
	}
	err = buffer.WriteByte(format)
	if err != nil {
		return err
	}
	err = binary.Write(buffer, binary.BigEndian, length)
	if err != nil {
		return err
	}
	_, err = buffer.Write(bytes.Repeat([]byte{0}, 8))
	if err != nil {
		return err
	}
	switch value := data.(type) {
	case []uint8:
		_, err = buffer.Write(value)
		if err != nil {
			return err
		}
	case []float32:
		for _, val := range value {
			n := Number{}
			n.SetDataType(IBFLOAT)
			err = n.SetValue(val)
			if err != nil {
				return err
			}
			_, err = buffer.Write(n.Bytes())
			if err != nil {
				return err
			}
		}
	case []float64:
		for _, val := range value {
			n := Number{}
			n.SetDataType(IBDOUBLE)
			err = n.SetValue(val)
			if err != nil {
				return err
			}
			_, err = buffer.Write(n.Bytes())
			if err != nil {
				return err
			}
		}
	default:
		return vectorTypeError
	}
	vector.bValue = buffer.Bytes()
	dataLen := uint64(len(vector.bValue))
	if dataLen > 0 {
		vector.loc = NewQuasiLocator(dataLen)
	} else {
		vector.loc = nil
	}
	return nil
}

func (vector *Vector) Value() (any, error) {
	if len(vector.bValue) == 0 {
		return nil, nil
	}
	read := func(tempBuffer *bytes.Buffer, size int) (int, error) {
		var err error
		switch size {
		case 1:
			var temp uint8
			temp, err = tempBuffer.ReadByte()
			if err != nil {
				return 0, vectorTypeError
			}
			return int(temp), nil
		case 2:
			var temp uint16
			err = binary.Read(tempBuffer, binary.BigEndian, &temp)
			if err != nil {
				return 0, vectorTypeError
			}
			return int(temp), nil
		case 4:
			var temp uint32
			err = binary.Read(tempBuffer, binary.BigEndian, &temp)
			if err != nil {
				return 0, vectorTypeError
			}
			return int(temp), nil
		default:
			return 0, vectorTypeError
		}
	}
	buffer := bytes.NewBuffer(vector.bValue)
	magicNumber, err := read(buffer, 1)
	if err != nil {
		return nil, err
	}
	if magicNumber != 219 {
		return nil, vectorTypeError
	}
	var version, flag, format, count int
	version, err = read(buffer, 1)
	if err != nil {
		return nil, err
	}
	if version != 0 {
		return nil, fmt.Errorf("vector version (%d) not supported", version)
	}
	flag, err = read(buffer, 2)
	if err != nil {
		return nil, err
	}
	format, err = read(buffer, 1)
	if err != nil {
		return nil, err
	}
	if flag&1 > 0 {
		count, err = read(buffer, 1)
	} else if flag&2 > 0 {
		count, err = read(buffer, 4)
	} else {
		count, err = read(buffer, 2)
	}
	if err != nil {
		return nil, err
	}
	if flag&0x10 > 0 {
		_ = buffer.Next(8)
	}
	switch format {
	case 2:
		// float32 array
		vecData := make([]float32, count)
		for i := 0; i < count; i++ {
			n := Number{}
			n.SetBytes(buffer.Next(4))
			n.SetDataType(IBFLOAT)
			output, err := n.Value()
			if err != nil {
				return nil, err
			}
			if temp, ok := output.(float32); ok {
				vecData[i] = temp
			}
		}
		return vecData, nil
	case 3:
		// float64 array
		vecData := make([]float64, count)
		for i := 0; i < count; i++ {
			n := Number{}
			n.SetBytes(buffer.Next(8))
			n.SetDataType(IBDOUBLE)
			output, err := n.Value()
			if err != nil {
				return nil, err
			}
			if temp, ok := output.(float64); ok {
				vecData[i] = temp
			}
		}
		return vecData, nil
	case 4:
		// int8 array
		vecData := make([]uint8, count)
		for i := 0; i < count; i++ {
			vecData[i], err = buffer.ReadByte()
		}
		return vecData, nil
	default:
		return nil, fmt.Errorf("unsupported format (%d) for vector type", format)
	}
}

var vectorTypeError = errors.New("unexpected data for vector type")

// CreateVector : create vector from supported array type: uint8, float32 and float64
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

//func (v *vector) Read(ctx context.Context) error {
//	if v.Type != VECTOR_NIL || v.data != nil {
//		return nil
//	}
//	if v.IsQuasi() {
//		return nil
//	}
//	if v.decoder == nil {
//		return fmt.Errorf("no decoder defined for vector type")
//	}
//	temp, err := v.lobBase.ReadFromPos(ctx, 0)
//	if err != nil {
//		return err
//	}
//	if v.decoder == nil {
//		return fmt.Errorf("no decoder defined for vector type")
//	}
//	tempVector, err := v.decoder.DecodeVector(temp)
//	if err != nil {
//		return err
//	}
//	return v.Scan(tempVector)
//}
//func (v *vector) CopyFrom(source Vector) error {
//	if temp, ok := source.(*vector); ok {
//		err := v.lobBase.CopyFrom(&temp.lobBase)
//		if err != nil {
//			return err
//		}
//		//v.flag = temp.flag
//		v.Type = temp.Type
//		v.data = temp.data
//	}
//	return nil
//}
// load will retrieve data from Lob and then call decode
//func (vector *Vector) load(stream LobStreamer) error {
//	if len(vector.locator) > 0 {
//		bValue, err := stream.Read(context.Background(), 0, 0)
//		if err != nil {
//			return err
//		}
//		_, err = vector.Decode(bValue, VECTOR)
//		return err
//	}
//	return nil
//}

func (vector *Vector) Scan(input any) error {
	return vector.SetValue(input)
}
