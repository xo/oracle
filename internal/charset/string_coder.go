package charset

type StringCoder interface {
	StringCoder(charsetID, charsetForm int) (*Converter, error)
	DefaultStringCoder() (*Converter, error)
	ServerStringCoder() *Converter
	ServerNStringCoder() *Converter
	MaxStringLength() int64
}
