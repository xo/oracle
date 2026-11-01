package types

import (
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/cockroachdb/apd/v3"
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

// isZero reports whether the NUMBER is 0, the one byte 0x80. A NUMBER of two
// bytes that starts with 0x80 is near 1e-130, which go-ora read as 0.
func (number *Number) isZero() bool {
	return len(number.bValue) == 1 && number.bValue[0] == 0x80
}

// encodeDecimal returns the NUMBER of s, the text of a decimal such as -1.5,
// 2e10 or 9.9E+125. A NUMBER is a byte of exponent and up to 20 digits of
// base 100, and holds values from 1e-130 to below 1e126, as python-oracledb
// writes them. A value out of that range, an infinity and a NaN are errors,
// where go-ora sent bytes that the server read as another number, and read
// an E as a digit.
func encodeDecimal(s string) ([]byte, error) {
	d, _, err := apd.NewFromString(strings.TrimSpace(s))
	if err != nil {
		return nil, fmt.Errorf("reading %q as a NUMBER: %w", s, err)
	}
	if d.Form != apd.Finite {
		return nil, fmt.Errorf("binding %s: a NUMBER holds no infinity and no NaN", s)
	}
	if d.IsZero() {
		return []byte{0x80}, nil
	}
	digits := d.Coeff.String()
	exp := int(d.Exponent)
	trimmed := strings.TrimRight(digits, "0")
	exp += len(digits) - len(trimmed)
	digits = trimmed
	// p is the count of digits before the decimal point, which the digits
	// of base 100 align to
	p := len(digits) + exp
	if p%2 != 0 {
		digits, p = "0"+digits, p+1
	}
	if len(digits)%2 != 0 {
		digits += "0"
	}
	if len(digits) > 40 {
		return nil, fmt.Errorf("binding %s: a NUMBER holds 40 digits", s)
	}
	exp100 := p/2 - 1
	if exp100 > 62 || exp100 < -65 {
		return nil, fmt.Errorf("binding %s: a NUMBER holds values from 1e-130 to below 1e126", s)
	}
	n := len(digits) / 2
	b := make([]byte, 0, n+2)
	if d.Negative {
		b = append(b, byte(62-exp100))
	} else {
		b = append(b, byte(exp100+193))
	}
	for i := range n {
		v := (digits[2*i]-'0')*10 + digits[2*i+1] - '0'
		if d.Negative {
			b = append(b, 101-v)
		} else {
			b = append(b, v+1)
		}
	}
	if d.Negative && n < 20 {
		b = append(b, 102)
	}
	return b, nil
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
	return getBinaryDouble(number.bValue), nil
}

func (number *Number) decodeFloat() (float32, error) {
	if len(number.bValue) < int(MaxLenBFloat) {
		return 0, fmt.Errorf("error decoding binary float, supplied buffer length: %d and required length: %d", len(number.bValue), 4)
	}
	return getBinaryFloat(number.bValue), nil
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
	var err error
	number.bValue, err = encodeDecimal(strconv.FormatInt(input, 10))
	return err
}

func (number *Number) encodeUint(input uint64) error {
	var err error
	number.bValue, err = encodeDecimal(strconv.FormatUint(input, 10))
	return err
}

func (number *Number) encodeFloat64(input float64) error {
	number.bValue = make([]byte, MaxLenBDouble)
	putBinaryDouble(number.bValue, input)
	number.dataType = IBDOUBLE
	return nil
}
func (number *Number) encodeFloat32(input float32) error {
	number.bValue = make([]byte, MaxLenBFloat)
	putBinaryFloat(number.bValue, input)
	number.dataType = IBFLOAT
	return nil
}

// putBinaryFloat writes the image of the BINARY_FLOAT f to b: its bits, big
// endian, with the sign bit set when it is clear and every bit flipped when it
// is set, so that the images sort as the values do. go-ora flipped the bits of
// a zero, which read back as NaN.
func putBinaryFloat(b []byte, f float32) {
	u := math.Float32bits(f)
	if u&(1<<31) == 0 {
		u |= 1 << 31
	} else {
		u = ^u
	}
	binary.BigEndian.PutUint32(b, u)
}

// getBinaryFloat returns the BINARY_FLOAT of the image b. go-ora decoded in
// place, so a second read of the value was wrong.
func getBinaryFloat(b []byte) float32 {
	u := binary.BigEndian.Uint32(b)
	if u&(1<<31) != 0 {
		u &^= 1 << 31
	} else {
		u = ^u
	}
	return math.Float32frombits(u)
}

// putBinaryDouble writes the image of the BINARY_DOUBLE f to b, as
// putBinaryFloat does.
func putBinaryDouble(b []byte, f float64) {
	u := math.Float64bits(f)
	if u&(1<<63) == 0 {
		u |= 1 << 63
	} else {
		u = ^u
	}
	binary.BigEndian.PutUint64(b, u)
}

// getBinaryDouble returns the BINARY_DOUBLE of the image b.
func getBinaryDouble(b []byte) float64 {
	u := binary.BigEndian.Uint64(b)
	if u&(1<<63) != 0 {
		u &^= 1 << 63
	} else {
		u = ^u
	}
	return math.Float64frombits(u)
}

func (number *Number) encodeFloat(input float64) error {
	if math.IsNaN(input) || math.IsInf(input, 0) {
		return fmt.Errorf("binding %v: a NUMBER holds no infinity and no NaN", input)
	}
	var err error
	number.bValue, err = encodeDecimal(strconv.FormatFloat(input, 'g', -1, 64))
	return err
}

func (number *Number) encodeString(input string) error {
	var err error
	number.bValue, err = encodeDecimal(input)
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
