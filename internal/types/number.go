package types

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
)

type Number struct {
	Basic
}

func (number *Number) MaxLen() int64 {
	switch number.dataType {
	case IBFLOAT:
		return MaxLenBFloat
	case BDOUBLE:
		return MaxLenBDouble
	default:
		return MaxLenNumber
	}
}
func (number *Number) isZero() bool {
	return len(number.bValue) > 0 && number.bValue[0] == 0x80
}

func (number *Number) encode(mantissa []byte, exp int, negative bool) ([]byte, error) {
	trailingZeros := 0
	for i := len(mantissa) - 1; i >= 0 && mantissa[i] == '0'; i-- {
		trailingZeros++
	}
	mantissa = mantissa[:len(mantissa)-trailingZeros]
	if len(mantissa) == 0 {
		return []byte{0x80}, nil
	}
	if exp%2 == 0 {
		mantissa = append([]byte{'0'}, mantissa...)
	}
	mantissaLen := len(mantissa)
	size := 1 + (mantissaLen+1)/2
	if negative && mantissaLen < 21 {
		size++
	}
	bValue := make([]byte, size)

	for i := 0; i < mantissaLen; i += 2 {
		b := 10 * (mantissa[i] - '0')
		if i < mantissaLen-1 {
			b += mantissa[i+1] - '0'
		}
		if negative {
			b = 100 - b
		}
		bValue[1+i/2] = b + 1
	}

	if negative && mantissaLen < 21 {
		bValue[len(bValue)-1] = 0x66
	}

	if exp < 0 {
		exp--
	}
	exp = (exp / 2) + 1
	if negative {
		bValue[0] = byte(exp+64) ^ 0x7f
	} else {
		bValue[0] = byte(exp+64) | 0x80
	}
	return bValue, nil
}

func (number *Number) decode() (strNum string, exp int, negative bool, err error) {
	if len(number.bValue) == 0 {
		err = fmt.Errorf("invalid NUMBER")
		return
	}
	if number.isZero() {
		strNum = "0"
		return
	}
	negative = number.bValue[0]&0x80 == 0
	if negative {
		exp = int(number.bValue[0]^0x7F) - 64
	} else {
		exp = int(number.bValue[0]&0x7F) - 64
	}

	if _isPosInf(number.bValue) || _isNegInf(number.bValue) {
		strNum = "Infinity"
		exp = 0
		return
	}

	buf := number.bValue[1:]
	if len(buf) == 0 {
		err = fmt.Errorf("invalid NUMBER")
		return
	}
	if negative && buf[len(buf)-1] == 0x66 {
		buf = buf[:len(buf)-1]
	}
	var output []byte
	for _, digit := range buf {
		digit--
		if negative {
			digit = 100 - digit
		}
		output = append(output, (digit/10)+'0')
		output = append(output, (digit%10)+'0')
	}
	exp = exp*2 - len(output)
	strNum = string(output)
	return
}

func _isNegInf(b []byte) bool {
	return b[0] == 0 && len(b) == 1
}

func _isPosInf(b []byte) bool {
	// -1 =255
	return len(b) == 2 && b[0] == 255 && b[1] == 101
}

func (number *Number) Int64() (int64, error) {
	strNum, exp, negative, err := number.decode()
	if err != nil {
		return 0, err
	}
	mantissa, err := strconv.ParseInt(strNum, 10, 64)
	if err != nil {
		return 0, err
	}
	for exp > 0 {
		mantissa *= 10
		exp--
	}
	if negative && (mantissa>>63) == 0 {
		return -mantissa, nil
	}
	return mantissa, nil
}

func (number *Number) Uint64() (uint64, error) {
	strNum, exp, _, err := number.decode()
	if err != nil {
		return 0, err
	}
	mantissa, err := strconv.ParseUint(strNum, 10, 64)
	if err != nil {
		return 0, err
	}
	for exp > 0 {
		mantissa *= 10
		exp--
	}
	return mantissa, nil
}

func (number *Number) Float64() (float64, error) {
	strNum, exp, negative, err := number.decode()
	if err != nil {
		return 0, err
	}
	mantissa, err := strconv.ParseFloat(strNum, 64)
	if err != nil {
		return 0, err
	}
	absExponent := int(math.Abs(float64(exp)))
	if negative {
		return -math.Round(mantissa*math.Pow10(exp)*math.Pow10(absExponent)) / math.Pow10(absExponent), nil
	}
	return math.Round(mantissa*math.Pow10(exp)*math.Pow10(absExponent)) / math.Pow10(absExponent), nil
}

func (number *Number) String() (string, error) {
	strNum, exp, negative, err := number.decode()
	if err != nil {
		return "", err
	}
	// remove zeros from beginning
	if len(strNum) > 1 {
		strNum = strings.TrimLeft(strNum, "0")
	}

	if exp > 0 {
		strNum += strings.Repeat("0", exp)
	} else if exp < 0 {
		pos := len(strNum) + exp // exp is negative
		if pos < 0 {
			pos = -pos
			strNum = strings.Repeat("0", pos) + strNum
			pos = 0
		}
		strNum = strNum[:pos] + "." + strNum[pos:]
		// remove zeros at right
		strNum = strings.TrimRight(strNum, "0")
	}
	if strNum[0] == '.' {
		strNum = "0" + strNum
	}
	if negative {
		strNum = "-" + strNum
	}
	return strNum, nil
}

func NewNumber(n any) (*Number, error) {
	var err error
	ret := new(Number)
	ret.SetDataType(NUMBER)
	err = ret.SetValue(n)
	return ret, err
}

func (number *Number) decodeDouble() (float64, error) {
	if len(number.bValue) < int(MaxLenBDouble) {
		return 0, fmt.Errorf("error decoding binary double, supplied buffer length: %d and required length: %d", len(number.bValue), 8)
	}
	if number.bValue[0]&128 != 0 {
		number.bValue[0] = number.bValue[0] & 127
	} else {
		xorBuffer(number.bValue, int(MaxLenBDouble))
	}
	return math.Float64frombits(binary.BigEndian.Uint64(number.bValue)), nil
}

func (number *Number) decodeFloat() (float32, error) {
	if len(number.bValue) < int(MaxLenBFloat) {
		return 0, fmt.Errorf("error decoding binary float, supplied buffer length: %d and required length: %d", len(number.bValue), 4)
	}
	if number.bValue[0]&128 != 0 {
		number.bValue[0] = number.bValue[0] & 127
	} else {
		xorBuffer(number.bValue, int(MaxLenBFloat))
	}
	return math.Float32frombits(binary.BigEndian.Uint32(number.bValue)), nil
}

func (number *Number) Value() (any, error) {
	if len(number.bValue) == 0 {
		return nil, nil
	}
	switch number.dataType {
	case IBDOUBLE:
		return number.decodeDouble()
	case IBFLOAT:
		return number.decodeFloat()
	default:
		return number.String()
	}
}

func (number *Number) encodeInt(input int64) error {
	mantissa := []byte(strconv.FormatInt(input, 10))
	negative := mantissa[0] == '-'
	if negative {
		mantissa = mantissa[1:]
	}
	var err error
	exp := len(mantissa) - 1
	number.bValue, err = number.encode(mantissa, exp, negative)
	return err
}
func (number *Number) encodeUint(input uint64) error {
	mantissa := []byte(strconv.FormatUint(input, 10))
	exponent := len(mantissa) - 1
	var err error
	number.bValue, err = number.encode(mantissa, exponent, false)
	return err
}
func (number *Number) encodeFloat64(input float64) error {
	number.bValue = make([]byte, MaxLenBDouble)
	temp := math.Float64bits(input)
	binary.BigEndian.PutUint64(number.bValue, temp)
	if input > 0 {
		number.bValue[0] = number.bValue[0] | 128
	} else {
		xorBuffer(number.bValue, 8)
	}
	number.dataType = IBDOUBLE
	return nil
}
func (number *Number) encodeFloat32(input float32) error {
	number.bValue = make([]byte, MaxLenBFloat)
	temp := math.Float32bits(input)
	binary.BigEndian.PutUint32(number.bValue, temp)
	if input > 0 {
		number.bValue[0] = number.bValue[0] | 128
	} else {
		xorBuffer(number.bValue, 4)
	}
	number.dataType = IBFLOAT
	return nil
}
func (number *Number) encodeFloat(input float64) error {
	if input == 0.0 {
		number.bValue = []byte{128}
		return nil
	}
	var (
		exponent int
		err      error
	)
	mantissa := []byte(strconv.FormatFloat(input, 'e', -1, 64))
	if i := bytes.Index(mantissa, []byte{'e'}); i >= 0 {
		exponent, err = strconv.Atoi(string(mantissa[i+1:]))
		if err != nil {
			return err
		}
		mantissa = mantissa[:i]
	}

	negative := mantissa[0] == '-'
	if negative {
		mantissa = mantissa[1:]
	}

	if i := bytes.Index(mantissa, []byte{'.'}); i >= 0 {
		mantissa = append(mantissa[:i], mantissa[i+1:]...)
	}
	number.bValue, err = number.encode(mantissa, exponent, negative)
	return err
}
func (number *Number) encodeString(input string) error {
	mantissa := []byte(input)
	negative := mantissa[0] == '-'
	if negative {
		mantissa = mantissa[1:]
	}
	var (
		exp int
		err error
	)
	isFloat := false
	if i := bytes.Index(mantissa, []byte{'e'}); i >= 0 {
		exp, err = strconv.Atoi(string(mantissa[i+1:]))
		if err != nil {
			return err
		}
		mantissa = mantissa[:i]
	}
	if i := bytes.Index(mantissa, []byte{'.'}); i >= 0 {
		mantissa = append(mantissa[:i], mantissa[i+1:]...)
		exp += i - 1
		isFloat = true
	}
	if !isFloat {
		exp += len(mantissa) - 1
	}
	number.bValue, err = number.encode(mantissa, exp, negative)
	return err
}
func (number *Number) SetValue(input any) error {
	input, verr := ValueOf(input)
	if verr != nil {
		return verr
	}
	var err error
	defer func(input *Number) {
		if input.dataType == 0 {
			input.dataType = NUMBER
		}
	}(number)
	if input == nil {
		number.bValue = nil
		return nil
	}
	rType := reflect.TypeOf(input)
	rValue := reflect.ValueOf(input)
	if rType == reflect.TypeFor[Number]() {
		if num, ok := input.(Number); ok {
			*number = num
			if number.dataType == 0 {
				number.dataType = NUMBER
			}
		} else {
			return fmt.Errorf("conversion of unsupported type %T to number", input)
		}
		return nil
	}
	var tempValue any
	switch rValue.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		tempValue = rValue.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		tempValue = rValue.Uint()
	case reflect.Float32, reflect.Float64:
		tempValue = rValue.Float()
	case reflect.String:
		tempValue = rValue.String()
	default:
		return fmt.Errorf("conversion of unsupported type %T to number", input)
	}

	switch temp := tempValue.(type) {
	case int64:
		switch number.dataType {
		case IBFLOAT:
			err = number.encodeFloat32(float32(temp))
		case IBDOUBLE:
			err = number.encodeFloat64(float64(temp))
		default:
			err = number.encodeInt(temp)
		}
	case uint64:
		switch number.dataType {
		case IBFLOAT:
			err = number.encodeFloat32(float32(temp))
		case IBDOUBLE:
			err = number.encodeFloat64(float64(temp))
		default:
			err = number.encodeUint(temp)
		}
	case float64:
		switch number.dataType {
		case IBFLOAT:
			err = number.encodeFloat32(float32(temp))
		case IBDOUBLE:
			err = number.encodeFloat64(temp)
		default:
			err = number.encodeFloat(temp)
		}
	case string:
		switch number.dataType {
		case IBFLOAT:
			value, err := strconv.ParseFloat(rValue.String(), 64)
			if err != nil {
				return err
			}
			err = number.encodeFloat32(float32(value))
		case IBDOUBLE:
			value, err := strconv.ParseFloat(rValue.String(), 64)
			if err != nil {
				return err
			}
			err = number.encodeFloat64(value)
		default:
			err = number.encodeString(temp)
		}
	default:
		err = fmt.Errorf("conversion of unsupported type %T to number", temp)
	}
	return err

}

func (number *Number) Scan(value any) error {
	number.SetDataType(NUMBER)
	return number.SetValue(value)
}
