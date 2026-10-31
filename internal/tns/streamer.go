package tns

type TerminalReader interface {
	read(num int) ([]byte, error)
}
type SessionReader interface {
	Peek() (byte, error)
	GetByte() (uint8, error)
	GetBytes(length int) ([]byte, error)
	GetInt64(size int, compress, bigEndian bool) (int64, error)
	GetInt(size int, compress, bigEndian bool) (int, error)
	GetClr() ([]byte, error)
	GetDlc() ([]byte, error)
	GetFixedClr() ([]byte, error)
	GetString(length int) (string, error)
	GetNullTermString() (string, error)
	GetKeyVal() (key, val []byte, num int, err error)
}
type SessionWriter interface {
	PutInt(number any, size uint8, bigEndian, compress bool)
	PutUint(number any, size uint8, bigEndian, compress bool)
	PutBytes(data ...byte)
	PutClr(data []byte)
	PutCHR(data []byte)
	PutFixedClr(data []byte)
	PutString(data string)
	PutDlc(data []byte)
	PutKeyVal(key, val []byte, num uint8)
	PutKeyValString(key, val string, num uint8)
	PutTTCFunc(ttcCode, funcCode uint8)
	GetWriteBuffer() []byte
	Write() error
}
type SessionReadWriter interface {
	ResetBuffer()
	GetProperties() SessionProperties
	SessionReader
	SessionWriter
}

type ValueStreamReader interface {
	Read(session SessionReader) (any, error)
}
type ValueStreamWriter interface {
	Write(session SessionWriter) error
}
type ValueStreamer interface {
	// Read value from stream
	ValueStreamReader
	// Write value to stream
	ValueStreamWriter
}
