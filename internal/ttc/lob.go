package ttc

import (
	"bytes"
	"context"
	"errors"

	"github.com/xo/oracle/internal/charset"
	"github.com/xo/oracle/internal/config"
	"github.com/xo/oracle/internal/trace"
	ora_types "github.com/xo/oracle/internal/types"
)

//type NClob Clob

var errEmptyLocator = errors.New("call lob operation on an empty locator")

type LobStream struct {
	conn          *Connection
	sourceLocator ora_types.Locator
	destLocator   ora_types.Locator
	scn           []byte
	sourceOffset  int64
	destOffset    int64
	destLen       int
	charsetID     int
	size          int64
	data          bytes.Buffer
	bNullO2U      bool
	isNull        bool
	sendSize      bool
}

func (lob *LobStream) initialize() {
	lob.bNullO2U = false
	lob.sendSize = false
	lob.size = 0
	lob.sourceOffset = 0
	lob.destOffset = 0
}

// variableWidthChar if lob has variable width char or not
//func (lob *LobStream) IsVarWidthChar() bool {
//	return len(lob.sourceLocator) > 6 && lob.sourceLocator[6]&0x80 == 0x80
//}
// IsLittleEndian if Lob is littleEndian or not
//func (lob *LobStream) IsLittleEndian() bool {
//	return len(lob.sourceLocator) > 7 && lob.sourceLocator[7]&0x40 == 0x40
//}

func (lob *LobStream) Locator() ora_types.Locator {
	return lob.sourceLocator
}
func (lob *LobStream) SetLocator(locator ora_types.Locator) {
	lob.sourceLocator = locator
	if !locator.IsInline() && locator.IsTemporary() {
		lob.conn.appendTemporaryLoc(locator)
	}

}
func (lob *LobStream) Tracer() trace.Tracer {
	return lob.conn.tracer
}

func (lob *LobStream) DatabaseVersionNumber() int {
	return int(lob.conn.dBVersion.Number)
}

func (lob *LobStream) StringCoder() charset.StringCoder {
	return lob.conn
}

// getSize return lob size
func (lob *LobStream) Size() (size int64, err error) {
	if lob.sourceLocator == nil {
		return 0, errEmptyLocator
	}
	lob.initialize()
	lob.sendSize = true
	session := lob.conn.session
	lob.conn.tracer.Print("Read Lob Size")
	session.ResetBuffer()
	lob.writeOp(1)
	err = session.Write()
	if err != nil {
		return
	}
	err = lob.read()
	err = processReset(err, lob.conn)
	if err != nil {
		return
	}
	size = lob.size
	lob.conn.tracer.Print("Lob Size: ", size)
	return
}

func (lob *LobStream) Read(offset, count int64) (data []byte, err error) {
	if lob.sourceLocator == nil {
		return nil, errEmptyLocator
	}
	if offset == 0 && count == 0 {
		lob.conn.tracer.Print("Read Lob Data:")
	} else {
		lob.conn.tracer.Printf("Read Lob Data Position: %d, Count: %d\n", offset, count)
	}
	lob.initialize()
	lob.size = count
	lob.sourceOffset = offset + 1
	lob.sendSize = true
	lob.data.Reset()
	session := lob.conn.session
	session.ResetBuffer()
	lob.writeOp(2)
	err = session.Write()
	if err != nil {
		return
	}
	err = lob.read()
	err = processReset(err, lob.conn)
	if err != nil {
		return
	}
	data = lob.data.Bytes()
	return
}
func (lob *LobStream) Write(data []byte) error {
	if lob.sourceLocator == nil {
		return errEmptyLocator
	}
	lob.conn.tracer.Printf("Write Lob Data: %d bytes", len(data))
	lob.initialize()
	lob.sourceOffset = 1
	lob.conn.session.ResetBuffer()
	lob.writeOp(0x40)
	lob.conn.session.PutBytes(0xE)
	lob.conn.session.PutClr(data)
	err := lob.conn.session.Write()
	if err != nil {
		return err
	}
	err = lob.read()
	return processReset(err, lob.conn)
}

func (lob *LobStream) Exists() (bool, error) {
	if lob.sourceLocator == nil {
		return false, errEmptyLocator
	}
	lob.initialize()
	lob.bNullO2U = true
	session := lob.conn.session
	session.ResetBuffer()
	lob.writeOp(0x800)
	err := session.Write()
	if err != nil {
		return false, err
	}
	err = lob.read()
	err = processReset(err, lob.conn)
	if err != nil {
		return false, err
	}
	return lob.isNull, nil
}

func (lob *LobStream) LobStreamMode() config.LobFetch {
	return lob.conn.connOption.Lob
}
func (lob *LobStream) LobReadMode() config.LobReadMode {
	return lob.conn.connOption.LobReadMode
}

// isTemporary: return true if the lob is temporary
//
//	func (lob *LobStream) isTemporary() bool {
//		if len(lob.sourceLocator) > 7 {
//			if lob.sourceLocator[7]&1 == 1 || lob.sourceLocator[4]&0x40 == 0x40 || lob.isValueBasedLocator() {
//				return true
//			}
//		}
//		return false
//	}
//
//	func (lob *LobStream) isQuasiLocator() bool {
//		return lob.sourceLocator.IsInline()
//	}
//
//	func (lob *LobStream) isValueBasedLocator() bool {
//		return lob.sourceLocator[4]&0x20 > 0
//	}
func (lob *LobStream) StartContext(ctx context.Context) func() {
	return lob.conn.session.StartContext(ctx)
}
func (lob *LobStream) EndContext(end func()) {
	lob.conn.session.EndContext(end)
}
func (lob *LobStream) FreeTemporaryLocator() error {
	if lob.sourceLocator == nil {
		return nil
	}
	lob.initialize()
	lob.conn.session.ResetBuffer()
	lob.writeOp(0x111)
	err := lob.conn.session.Write()
	if err != nil {
		return err
	}
	err = lob.read()
	err = processReset(err, lob.conn)
	if err != nil {
		return err
	}
	lob.sourceLocator = nil
	return nil
}
func (lob *LobStream) CreateTemporaryLocator(charsetID, charsetForm int) (ora_types.Locator, error) {
	lob.initialize()
	lob.conn.tracer.Print("Create Temporary Lob:")
	lob.sourceLocator = make([]byte, 0x28)
	lob.sourceLocator[1] = 0x54
	lob.bNullO2U = true
	lob.scn = make([]byte, 1)
	// lob.scn[0] = 1 if you need to cache otherwise 0
	// 0xA is equal to duration
	lob.destLen = 0xA
	lob.size = 0xA
	lob.sendSize = true
	if charsetForm == 0 {
		lob.destOffset = 0x71
		lob.charsetID = 1
	} else {
		lob.destOffset = 0x70
		lob.charsetID = charsetID
		lob.sourceOffset = int64(charsetForm)
	}
	session := lob.conn.session
	session.ResetBuffer()
	lob.writeOp(0x110)
	err := session.Write()
	if err != nil {
		return nil, err
	}
	err = lob.read()
	err = processReset(err, lob.conn)
	if err != nil {
		return nil, err
	}
	lob.conn.appendTemporaryLoc(lob.sourceLocator)
	return lob.sourceLocator, nil
}

func (lob *LobStream) Open(mode, opID int) error {
	if lob.sourceLocator == nil {
		return errEmptyLocator
	}
	lob.conn.tracer.Printf("Open Lob: Mode= %d   Operation ID= %d", mode, opID)
	if lob.sourceLocator.IsTemporary() {
		if lob.sourceLocator[7]&8 == 8 {
			return errors.New("TTC Error")
		}
		if mode == 2 {
			lob.sourceLocator[7] |= 0x10
		}
		return nil
	}

	lob.initialize()
	lob.size = int64(mode)
	lob.sendSize = true
	lob.conn.session.ResetBuffer()
	lob.writeOp(opID)
	err := lob.conn.session.Write()
	if err != nil {
		return err
	}
	err = lob.read()
	return processReset(err, lob.conn)
}

func (lob *LobStream) Close(opID int) error {
	if lob.sourceLocator == nil {
		return errEmptyLocator
	}
	lob.conn.tracer.Print("Close Lob: ")
	lob.initialize()
	lob.conn.session.ResetBuffer()
	lob.writeOp(opID)
	err := lob.conn.session.Write()
	if err != nil {
		return err
	}
	err = lob.read()
	return processReset(err, lob.conn)
}

func (lob *LobStream) writeOp(operationID int) {
	session := lob.conn.session
	session.PutTTCFunc(0x3, 0x60)
	if len(lob.sourceLocator) == 0 {
		session.PutBytes(0)
	} else {
		session.PutBytes(1)
	}
	session.PutUint(len(lob.sourceLocator), 4, true, true)

	if len(lob.destLocator) == 0 {
		session.PutBytes(0)
	} else {
		lob.destLen = len(lob.destLocator)
		session.PutBytes(1)
	}
	session.PutUint(lob.destLen, 4, true, true)

	// put offsets
	if session.TTCVersion < 3 {
		session.PutUint(lob.sourceOffset, 4, true, true)
		session.PutUint(lob.destOffset, 4, true, true)
	} else {
		session.PutBytes(0, 0)
	}

	if lob.charsetID != 0 {
		session.PutBytes(1)
	} else {
		session.PutBytes(0)
	}

	if lob.sendSize && session.TTCVersion < 3 {
		session.PutBytes(1)
	} else {
		session.PutBytes(0)
	}

	if lob.bNullO2U {
		session.PutBytes(1)
	} else {
		session.PutBytes(0)
	}

	session.PutInt(operationID, 4, true, true)
	if len(lob.scn) == 0 {
		session.PutBytes(0)
	} else {
		session.PutBytes(1)
	}
	session.PutUint(len(lob.scn), 4, true, true)

	if session.TTCVersion >= 3 {
		session.PutUint(lob.sourceOffset, 8, true, true)
		session.PutInt(lob.destOffset, 8, true, true)
		if lob.sendSize {
			session.PutBytes(1)
		} else {
			session.PutBytes(0)
		}
	}
	if session.TTCVersion >= 4 {
		session.PutBytes(0, 0, 0, 0, 0, 0)
	}

	if len(lob.sourceLocator) > 0 {
		session.PutBytes(lob.sourceLocator...)
	}

	if len(lob.destLocator) > 0 {
		session.PutBytes(lob.destLocator...)
	}

	if lob.charsetID != 0 {
		session.PutUint(lob.charsetID, 2, true, true)
	}
	if session.TTCVersion < 3 && lob.sendSize {
		session.PutUint(lob.size, 4, true, true)
	}
	for x := 0; x < len(lob.scn); x++ {
		session.PutUint(lob.scn[x], 4, true, true)
	}
	if session.TTCVersion >= 3 && lob.sendSize {
		session.PutUint(lob.size, 8, true, true)
	}
}

// read lob response from network session
func (lob *LobStream) read() error {
	loop := true
	session := lob.conn.session
	for loop {
		msg, err := session.ReadByte()
		if err != nil {
			return err
		}
		switch msg {
		case 8:
			// read rpa message
			if len(lob.sourceLocator) > 0 {
				lob.sourceLocator, err = session.ReadBytes(len(lob.sourceLocator))
				if err != nil {
					return err
				}
			}
			if len(lob.destLocator) != 0 {
				lob.destLocator, err = session.ReadBytes(len(lob.destLocator))
				if err != nil {
					return err
				}
				lob.destLen = len(lob.destLocator)
			} else {
				lob.destLen = 0
			}
			if lob.charsetID != 0 {
				lob.charsetID, err = session.ReadInt(2, true, true)
				if err != nil {
					return err
				}
			}
			if lob.sendSize {
				// get data size
				if session.TTCVersion < 3 {
					lob.size, err = session.ReadInt64(4, true, true)
					if err != nil {
						return err
					}
				} else {
					lob.size, err = session.ReadInt64(8, true, true)
					if err != nil {
						return err
					}
				}
			}
			if lob.bNullO2U {
				temp, err := session.ReadInt(2, true, true)
				if err != nil {
					return err
				}
				if temp != 0 {
					lob.isNull = true
				}
			}
		case 14:
			// get the data
			err = lob.readData()
			if err != nil {
				return err
			}
		default:
			err = lob.conn.ProcessTCCResponse(msg)
			if err != nil {
				return err
			}
			if msg == 4 || msg == 9 {
				loop = false
			}
		}
	}
	return nil
}

// read lob data chunks from network session
func (lob *LobStream) readData() error {
	session := lob.conn.session
	tempBytes, err := session.ReadCLR()
	if err != nil {
		return err
	}
	lob.data.Write(tempBytes)
	return nil
}

func (lob *LobStream) LobID() []byte {
	if lob.sourceLocator == nil {
		return nil
	}
	return lob.sourceLocator[10 : 10+10]
}

func (lob *LobStream) append(dest []byte) error {
	if lob.sourceLocator == nil {
		return errEmptyLocator
	}
	lob.initialize()
	lob.destLocator = dest
	lob.destLen = len(dest)
	lob.conn.session.ResetBuffer()
	lob.writeOp(0x80)
	err := lob.conn.session.Write()
	if err != nil {
		return err
	}
	err = lob.read()
	return processReset(err, lob.conn)
}

func (lob *LobStream) copy(srcLocator, dstLocator []byte, srcOffset, dstOffset, length int64) error {
	lob.initialize()
	lob.sourceLocator = srcLocator
	lob.destLocator = dstLocator
	lob.destLen = len(dstLocator)
	lob.sourceOffset = srcOffset
	lob.destOffset = dstOffset
	lob.size = length
	lob.sendSize = true
	lob.conn.session.ResetBuffer()
	lob.writeOp(4)
	err := lob.conn.session.Write()
	if err != nil {
		return err
	}
	err = lob.read()
	return processReset(err, lob.conn)
}
