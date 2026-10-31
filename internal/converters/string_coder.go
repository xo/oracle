package converters

type StringCoder interface {
	StringCoder(charsetID, charsetForm int) (*StringConverter, error)
	DefaultStringCoder() (*StringConverter, error)
	ServerStringCoder() *StringConverter
	ServerNStringCoder() *StringConverter
	MaxStringLength() int64
}
