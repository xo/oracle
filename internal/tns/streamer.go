package tns

type terminalReader interface {
	read(num int) ([]byte, error)
}
type SessionReader interface {
	Peek() (byte, error)
	ReadByte() (byte, error)
	ReadBytes(length int) ([]byte, error)
	ReadInt64(size int, compress, bigEndian bool) (int64, error)
	ReadInt(size int, compress, bigEndian bool) (int, error)
	ReadCLR() ([]byte, error)
	ReadDLC() ([]byte, error)
	ReadFixedCLR() ([]byte, error)
	ReadString(length int) (string, error)
	ReadNullTermString() (string, error)
	ReadKeyVal() (key, val []byte, num int, err error)
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
	WriteBuffer() []byte
	Write() error
}
type SessionReadWriter interface {
	ResetBuffer()
	Properties() SessionProperties
	SessionReader
	SessionWriter
}

type ValueWriter interface {
	Write(session SessionWriter) error
}
