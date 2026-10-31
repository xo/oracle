package converters

/*
BINARY_FLOAT and BINARY_DOUBLE encoding observed using
SELECT dump(cast(xxx as binary_yyy) FROM dual;
*/
//func EncodeBool(val bool) []byte {
//	if val {
//		return []byte{1, 1}
//	} else {
//		return []byte{1, 0}
//	}
//}
//
//func DecodeBool(data []byte) bool {
//	return bytes.Compare(data, []byte{1, 1}) == 0
//}
//
//func EncodeFloat32(number float32) []byte {
//	b := make([]byte, 4)
//	temp := math.Float32bits(number)
//	binary.BigEndian.PutUint32(b, uint32(temp))
//	if number > 0 {
//		b[0] = b[0] | 128
//	} else {
//		b[0] = ^b[0]
//		b[1] = ^b[1]
//		b[2] = ^b[2]
//		b[3] = ^b[3]
//	}
//	return b
//
//}
//func ConvertBinaryFloat(bytes []byte) float32 {
//	if bytes[0]&128 != 0 {
//		bytes[0] = bytes[0] & 127
//	} else {
//		bytes[0] = ^bytes[0]
//		bytes[1] = ^bytes[1]
//		bytes[2] = ^bytes[2]
//		bytes[3] = ^bytes[3]
//	}
//	u := binary.BigEndian.Uint32(bytes)
//	// user can cast to float64 on their side if necessary (pass float64 field as parameter for row.Scan method)
//	return math.Float32frombits(u)
//	//test2 := float64(test)
//	//return test2
//	//if u > (1 << 31) {
//	//	return -math.Float32frombits(u)
//	//}
//	//return math.Float32frombits(^u)
//}
//func EncodeFloat64(number float64) []byte {
//	b := make([]byte, 8)
//	temp := math.Float64bits(number)
//	binary.BigEndian.PutUint64(b, uint64(temp))
//	if number > 0 {
//		b[0] = b[0] | 128
//	} else {
//		b[0] = ^b[0]
//		b[1] = ^b[1]
//		b[2] = ^b[2]
//		b[3] = ^b[3]
//		b[4] = ^b[4]
//		b[5] = ^b[5]
//		b[6] = ^b[6]
//		b[7] = ^b[7]
//	}
//	return b
//}
//func ConvertBinaryDouble(bytes []byte) float64 {
//	if bytes[0]&128 != 0 {
//		bytes[0] = bytes[0] & 127
//	} else {
//		bytes[0] = ^bytes[0]
//		bytes[1] = ^bytes[1]
//		bytes[2] = ^bytes[2]
//		bytes[3] = ^bytes[3]
//		bytes[4] = ^bytes[4]
//		bytes[5] = ^bytes[5]
//		bytes[6] = ^bytes[6]
//		bytes[7] = ^bytes[7]
//	}
//	u := binary.BigEndian.Uint64(bytes)
//	return math.Float64frombits(u)
//}
/*
INTERVAL_xxx encoding described at https://www.orafaq.com/wiki/Interval
*/
