package converters

type StringCoder interface {
	GetStringCoder(charsetID, charsetForm int) (*StringConverter, error)
	GetDefaultStringCoder() (*StringConverter, error)
	GetServerStringCoder() *StringConverter
	GetServerNStringCoder() *StringConverter
	GetMaxStringLength() int64
}
