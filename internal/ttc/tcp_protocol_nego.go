package ttc

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/xo/oracle/internal/charset"
)

type TCPNego struct {
	conn                  *Connection
	MessageCode           uint8
	ProtocolServerString  string
	OracleVersion         int
	ServerCharset         int
	ServerFlags           uint8
	ServerNCharset        int
	ServerCompileTimeCaps []byte
	ServerRuntimeCaps     []byte
}

func (nego *TCPNego) writeMessage() {
	session := nego.conn.session
	session.PutBytes(1, 6, 0)
	session.PutBytes([]byte("OracleClientGo\x00")...)
}

func (nego *TCPNego) write() error {
	session := nego.conn.session
	session.ResetBuffer()
	nego.writeMessage()
	return session.Write()
}

func (nego *TCPNego) readMessage() error {
	var err error
	session := nego.conn.session
	logger := nego.conn.logger
	nego.MessageCode, err = session.ReadByte()
	if err != nil {
		return err
	}
	if nego.MessageCode != 1 {
		return fmt.Errorf("message code error: received code %d and expected code is 1", nego.MessageCode)
	}
	var protocolServerVersion uint8
	protocolServerVersion, err = session.ReadByte()
	if err != nil {
		return err
	}
	switch protocolServerVersion {
	case 4:
		nego.OracleVersion = 7230
	case 5:
		nego.OracleVersion = 8030
	case 6:
		nego.OracleVersion = 8100
	default:
		return fmt.Errorf("unsupported protocol server version: %d", protocolServerVersion)
	}
	_, err = session.ReadByte()
	if err != nil {
		return err
	}
	nego.ProtocolServerString, err = session.ReadNullTermString()
	if err != nil {
		return err
	}
	nego.ServerCharset, err = session.ReadInt(2, false, false)
	if err != nil {
		return err
	}
	logger.Debug("server charset", "charset", nego.ServerCharset)
	// create string converter object
	if nego.conn.sStrConv == nil {
		nego.conn.sStrConv = charset.New(nego.ServerCharset)
		if nego.conn.sStrConv == nil {
			return fmt.Errorf("the server use charset with id: %d which is not supported by the driver", nego.ServerCharset)
		}
	}
	nego.conn.session.StrConv = nego.conn.sStrConv
	nego.ServerFlags, err = session.ReadByte()
	if err != nil {
		return err
	}
	var charsetElem int
	charsetElem, err = session.ReadInt(2, false, false)
	if err != nil {
		return err
	}
	if charsetElem > 0 {
		_, _ = session.ReadBytes(charsetElem * 5)
	}
	len1, err := session.ReadInt(2, false, true)
	if err != nil {
		return err
	}
	numArray, err := session.ReadBytes(len1)
	if err != nil {
		return err
	}
	num3 := int(6 + (numArray[5]) + (numArray[6]))
	nego.ServerNCharset = int(binary.BigEndian.Uint16(numArray[(num3 + 3):(num3 + 5)]))
	if nego.conn.nStrConv == nil {
		nego.conn.nStrConv = charset.New(nego.ServerNCharset)
		if nego.conn.nStrConv == nil {
			return fmt.Errorf("the server use ncharset with id: %d which is not supported by the driver", nego.ServerNCharset)
		}
	}
	logger.Debug("server national charset", "charset", nego.ServerNCharset)
	len2, err := session.ReadByte()
	if err != nil {
		return err
	}
	nego.ServerCompileTimeCaps, err = session.ReadBytes(int(len2))
	if err != nil {
		return err
	}
	len3, err := session.ReadByte()
	if err != nil {
		return err
	}
	nego.ServerRuntimeCaps, err = session.ReadBytes(int(len3))
	if err != nil {
		return err
	}
	if len(nego.ServerCompileTimeCaps) > 15 && nego.ServerCompileTimeCaps[15]&1 != 0 {
		session.HasEOSCapability = true
	}
	if len(nego.ServerCompileTimeCaps) > 16 && nego.ServerCompileTimeCaps[16]&1 != 0 {
		session.HasFSAPCapability = true
	}
	if nego.ServerCompileTimeCaps == nil || len(nego.ServerCompileTimeCaps) < 8 {
		return errors.New("server compile time caps length less than 8")
	}
	if len(nego.ServerCompileTimeCaps) > 37 && nego.ServerCompileTimeCaps[37]&32 != 0 {
		session.UseBigClrChunks = true
		session.ClrChunkSize = 0x7FFF
	}

	return nil
}
