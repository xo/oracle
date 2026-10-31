package aq

import (
	"time"

	"github.com/xo/oracle/internal/codec"
	"github.com/xo/oracle/internal/types"
	//"github.com/xo/oracle/type_coder"
)

var XMLTypeTOID = []byte{
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 2, 1, 0}
var AnyDataTOID = []byte{
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 2, 0, 17}
var JSONTOID = []byte{
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 71}
var RawTOID = []byte{
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 23}

type OracleAgent struct {
	Key   string
	Value string
	Num   uint8
}
type DeliveryMode int

const (
	DeliveryModePersistent DeliveryMode = 1
	DeliveryModeBuffered   DeliveryMode = 2
	DeliveryModeAny        DeliveryMode = 3
)

type MessageType int

const (
	RAW  MessageType = 1
	UDT  MessageType = 2
	XML  MessageType = 3
	JSON MessageType = 4
)

type MessageState int

const (
	MessageStateReady MessageState = iota
	MessageStateWaiting
	MessageStateProcessed
	MessageStateExpired
)

type Message struct {
	ID             string
	messageID      []byte
	Delay          int
	Expiration     int
	Priority       int
	correlation    string
	exceptionQueue string
	deqAttempts    int
	State          MessageState
	DeliveryMode   DeliveryMode
	VisibilityMode VisibilityMode
	recipients     []OracleAgent
	sender         OracleAgent
	EnqTime        time.Time

	propModified bool
	Payload      any
	bValue       []byte
	shareNum     int

	senderName       string
	senderAddress    string
	senderProtocol   uint8
	transactionGroup string

	extensions []QAExtension
}

func (message *Message) read(conn Conn) error {
	session := conn.Session()
	var err error
	message.Priority, err = session.ReadInt(4, true, true)
	if err != nil {
		return err
	}
	message.Delay, err = session.ReadInt(4, true, true)
	if err != nil {
		return err
	}
	message.Expiration, err = session.ReadInt(4, true, true)
	if err != nil {
		return err
	}
	var num int
	num, err = session.ReadInt(4, true, true)
	if num > 0 {
		var temp []byte
		temp, err = session.ReadCLR()
		if err != nil {
			return err
		}
		message.correlation = conn.ServerStringCoder().Decode(temp)
	} else {
		message.correlation = ""
	}
	message.deqAttempts, err = session.ReadInt(4, true, true)
	if err != nil {
		return err
	}
	num, err = session.ReadInt(4, true, true)
	if err != nil {
		return err
	}
	if num > 0 {
		var temp []byte
		temp, err = session.ReadCLR()
		if err != nil {
			return err
		}
		message.exceptionQueue = conn.ServerStringCoder().Decode(temp)
	} else {
		message.exceptionQueue = ""
	}
	num, err = session.ReadInt(4, true, true)
	if err != nil {
		return err
	}
	message.State = MessageState(num)
	num, err = session.ReadInt(4, true, true)
	if err != nil {
		return err
	}
	if num > 0 {
		var temp []byte
		temp, err = session.ReadCLR()
		if err != nil {
			return err
		}
		decoder := &types.Date{}
		decoder.SetBytes(temp)
		tempTime, err := decoder.Value()
		if err != nil {
			return err
		}
		if tempTime != nil {
			message.EnqTime = tempTime.(time.Time)
		}
	}
	if conn.TTCVersion() >= 3 {
		num, err = session.ReadInt(2, true, true)
		if err != nil {
			return err
		}
		if num > 0 {
			var temp []byte
			temp, err = session.ReadCLR()
			if err != nil {
				return err
			}
			message.transactionGroup = conn.ServerStringCoder().Decode(temp)
		} else {
			message.transactionGroup = ""
		}
	}
	var length int
	length, err = session.ReadInt(4, true, true)
	if err != nil {
		return err
	}
	_, err = session.ReadByte()
	if err != nil {
		return err
	}
	for i := 0; i < length; i++ {
		var key, val []byte
		key, val, num, err = session.ReadKeyVal()
		if err != nil {
			return err
		}
		switch num {
		case 64:
			if len(key) > 0 {
				message.senderName = conn.ServerStringCoder().Decode(key)
			}
		case 65:
			if len(key) > 0 {
				message.senderAddress = conn.ServerStringCoder().Decode(key)
			}
		case 66:
			if len(val) > 0 {
				message.senderProtocol = val[0]
			}
		case 69:
			if len(val) > 0 {
				message.ID = conn.ServerStringCoder().Decode(val)
			}
		}
	}
	if conn.TTCVersion() >= 3 {
		_, err = session.ReadInt(2, true, true)
		if err != nil {
			return err
		}
		/*csn*/ _, err = session.ReadInt(4, true, true)
		if err != nil {
			return err
		}
		/*dsn*/ _, err = session.ReadInt(4, true, true)
		if err != nil {
			return err
		}
	}
	if conn.TTCVersion() >= 4 {
		var flag int
		flag, err = session.ReadInt(4, true, true)
		if err != nil {
			return err
		}
		if flag == 0 {
			flag = 1
		}
		message.DeliveryMode = DeliveryMode(flag)
	}
	if conn.TTCVersion() >= 16 {
		message.shareNum, err = session.ReadInt(4, true, true)
		if err != nil {
			return err
		}
	}
	return nil
}
func (message *Message) write(conn Conn) {
	session := conn.Session()
	session.PutInt(message.Priority, 4, true, true)
	session.PutInt(message.Delay, 4, true, true)
	session.PutInt(message.Expiration, 4, true, true)
	if len(message.correlation) > 0 {
		temp := conn.ServerStringCoder().Encode(message.correlation)
		session.PutInt(len(temp), 2, true, true)
		session.PutClr(temp)
	} else {
		session.PutBytes(0)
	}
	session.PutBytes(0)
	if len(message.exceptionQueue) > 0 {
		temp := conn.ServerStringCoder().Encode(message.exceptionQueue)
		session.PutInt(len(message.exceptionQueue), 2, true, true)
		session.PutClr(temp)
	} else {
		session.PutBytes(0)
	}
	session.PutInt(int(message.State), 4, true, true)
	session.PutBytes(0)
	if conn.TTCVersion() >= 3 {
		temp := conn.ServerStringCoder().Encode(message.transactionGroup)
		if len(temp) > 0 {
			session.PutInt(len(temp), 2, true, true)
			session.PutClr(temp)
		} else {
			session.PutBytes(0)
		}
	}
	session.PutInt(4, 2, true, true)
	session.PutBytes(14)
	session.PutKeyValString(message.senderName, "", 64)
	session.PutKeyValString(message.senderAddress, "", 65)
	session.PutKeyVal(nil, []byte{message.senderProtocol}, 66)
	session.PutKeyValString("", message.ID, 69)
	if conn.TTCVersion() >= 3 {
		session.PutBytes(0, 0, 0)
	}
	if conn.TTCVersion() >= 4 {
		session.PutBytes(0)
	}
	if conn.TTCVersion() >= 16 {
		session.PutUint(message.shareNum, 2, true, true)
	}
}

func (message *Message) ServerMessageID() []byte {
	return message.messageID
}

func (message *Message) readData(conn Conn, messageType MessageType, udtName string) error {
	var err error
	session := conn.Session()
	message.bValue, err = session.ReadCLR()
	if err != nil {
		return err
	}
	var decoder codec.OracleParameterDecoder
	switch messageType {
	case RAW:
		decoder, err = conn.ParameterCoder(types.RAW)

	case JSON:
		decoder, err = conn.ParameterCoder(types.JSONType)
	default:
		decoder, err = conn.ParameterCoder(udtName)

	}
	if err != nil {
		return err
	}
	if (messageType == RAW || messageType == JSON) && len(message.bValue) > 4 {
		decoder.SetBytes(message.bValue[4:])
	} else {
		decoder.SetBytes(message.bValue)
	}
	message.Payload, err = decoder.Decode(conn)
	if err != nil {
		return err
	}
	if messageType == JSON {
		if temp, ok := message.Payload.(*types.JSON); ok {
			if err = codec.DecodeOSON(temp); err != nil {
				return err
			}
			message.Payload, err = temp.Value()
			if err != nil {
				return err
			}
		}
	}
	return nil
}
