package oson

type Oson struct {
}

func (oson *Oson) EncodeJSON(input any) ([]byte, error) {
	return Encode(input)
}

func (oson *Oson) DecodeJSON(data []byte) (any, error) {
	return Decode(data)
}
