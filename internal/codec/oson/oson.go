package oson

type Oson struct {
}

func (oson *Oson) EncodeJson(input any) ([]byte, error) {
	return Encode(input)
}

func (oson *Oson) DecodeJson(data []byte) (any, error) {
	return Decode(data)
}
