package aq

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/xo/oracle/internal/codec"
	"github.com/xo/oracle/internal/tns"
	"github.com/xo/oracle/internal/types"
)

// Execer runs statements. *sql.DB, *sql.Conn and *sql.Tx are each one. A
// queue holds the connection of the Execer that made it, so a *sql.Conn or a
// *sql.Tx is the one to use. The connection of a *sql.DB goes back to its
// pool, which can give it to another caller while the queue uses it. go-ora
// asked for Exec and Prepare, which a *sql.Conn lacks.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type Queue interface {
	NewMessage(data any) (*Message, error)
	Enqueue(message *Message) error
	EnqueueMessages(messages []*Message) error
	Dequeue(options *DequeueOptions) (*Message, error)
	DequeueMessages(options *DequeueOptions, count int) ([]*Message, error)
	SetAutoCommit(autoCommit bool)
}
type queue struct {
	Name        string
	version     int
	conn        Conn
	messageType MessageType
	udtName     string
	toid        []byte
	AutoCommit  bool
}

func CreateQueue(db Execer, name string, messageType MessageType, udtName string) (Queue, error) {
	ret := &queue{
		Name:        name,
		messageType: messageType,
		udtName:     udtName,
		version:     1,
	}
	_, err := db.ExecContext(context.Background(), "--GET-CONNECTION-REF--", &ret.conn)
	if err != nil {
		return nil, err
	}

	switch messageType {
	case RAW:
		ret.toid = RawTOID
	case JSON:
		ret.toid = JSONTOID
	case XML:
		ret.toid = XMLTypeTOID
	default:
		if len(udtName) > 0 {
			coder, err := ret.conn.ParameterCoder(udtName)
			if err != nil {
				return nil, err
			}
			ret.toid = coder.ParameterInfo().ToID
		} else {
			return nil, fmt.Errorf("message type udt should have udtName")
		}
	}
	return ret, nil
}

func (qu *queue) SetAutoCommit(autoCommit bool) {
	qu.AutoCommit = autoCommit
}

func (qu *queue) NewMessage(data any) (*Message, error) {
	var err error
	message := &Message{
		messageID:        nil,
		Delay:            0,
		Expiration:       -1,
		correlation:      "",
		exceptionQueue:   "",
		deqAttempts:      0,
		DeliveryMode:     DeliveryModePersistent,
		VisibilityMode:   VisibilityOnCommit,
		recipients:       nil,
		sender:           OracleAgent{},
		EnqTime:          time.Time{},
		State:            0,
		Priority:         0,
		ID:               "",
		propModified:     false,
		shareNum:         0xFFFFFFFF,
		senderName:       "",
		senderAddress:    "",
		senderProtocol:   0,
		transactionGroup: "",
		Payload:          data,
		extensions:       nil,
	}
	var encoder codec.OracleParameterCoder
	switch qu.messageType {
	case RAW:
		encoder, err = qu.conn.ParameterCoder(types.RAW)
	case UDT:
		encoder, err = qu.conn.ParameterCoder(qu.udtName)
	case JSON:
		encoder, err = qu.conn.ParameterCoder(types.JSONType)
		if err != nil {
			return nil, err
		}
	default:
		err = fmt.Errorf("unsupported message type: %v", qu.messageType)
	}
	if err != nil {
		return nil, err
	}
	encoder.SetAQMessage()
	err = encoder.Encode(data, qu.conn)
	if err != nil {
		return nil, err
	}
	ms := tns.NewMemorySession(nil, nil, qu.conn.Session().Properties())
	err = encoder.Write(ms)
	message.bValue = ms.WriteBuffer()
	return message, err
}
func (qu *queue) Enqueue(message *Message) error {
	session := qu.conn.Session()
	session.ResetBuffer()
	session.PutTTCFunc(0x3, 0x79)

	queueNameBytes := qu.conn.ServerStringCoder().Encode(qu.Name)
	if len(queueNameBytes) > 0 {
		session.PutBytes(1)
		session.PutInt(len(queueNameBytes), 2, true, true)
	} else {
		session.PutBytes(0, 0)
	}
	message.write(qu.conn)
	if len(message.recipients) > 0 {
		session.PutBytes(1)
		session.PutInt(len(message.recipients)*3, 2, true, true)
	} else {
		session.PutBytes(0, 0)
	}
	session.PutInt(int(message.VisibilityMode), 4, true, true)
	if len(message.messageID) > 0 {
		session.PutBytes(1)
		session.PutInt(len(message.messageID), 2, true, true)
	} else {
		session.PutBytes(0, 0)
	}

	session.PutBytes(0, 1)
	session.PutInt(0x10, 2, true, true)
	session.PutUint(qu.version, 2, true, true)
	switch qu.messageType {
	case RAW:
		session.PutBytes(0, 1)
		session.PutUint(len(message.bValue), 4, true, true)
	case JSON:
		session.PutBytes(0, 0, 0)
	default:
		session.PutBytes(1, 0, 0)
	}
	// retrieve message id in response
	if len(message.messageID) == 0 {
		session.PutBytes(1)
		session.PutInt(0x10, 2, true, true)
	} else {
		session.PutBytes(0, 0)
	}

	num := 0
	if qu.AutoCommit {
		num = 32
	}
	if message.DeliveryMode == DeliveryModeBuffered {
		num |= 2
	} else if message.DeliveryMode == DeliveryModeAny {
		num |= 16
	}
	session.PutUint(num, 4, true, true)
	session.PutBytes(0, 0)
	if len(message.extensions) > 0 {
		session.PutBytes(1)
		session.PutInt(len(message.extensions), 2, true, true)
	} else {
		session.PutBytes(0, 0)
	}
	session.PutBytes(0, 0, 0, 0, 0)
	if qu.conn.TTCVersion() >= 4 {
		session.PutBytes(0, 0, 0, 0, 0, 0, 0, 0)
	}
	if qu.conn.TTCVersion() >= 14 {
		if qu.messageType == JSON {
			session.PutBytes(1)
		} else {
			session.PutBytes(0)
		}
	}
	if len(queueNameBytes) > 0 {
		session.PutCHR(queueNameBytes)
	}
	for _, recipient := range message.recipients {
		session.PutKeyValString(recipient.Key, recipient.Value, recipient.Num)
	}
	if len(message.messageID) > 0 {
		session.PutBytes(message.messageID...)
	}
	session.PutBytes(qu.toid...)
	switch qu.messageType {
	case JSON:
	case RAW:
		fallthrough
	case UDT:
		session.PutBytes(message.bValue...)
	default:
		return errors.New("unsupported message type")
	}
	for _, extension := range message.extensions {
		session.PutKeyVal(extension.key, extension.value, extension.num)
	}
	if qu.messageType == JSON {
		session.PutBytes(message.bValue...)
	}
	err := session.Write()
	if err != nil {
		return err
	}
	return qu.afterReset(qu.readEnqueueResponse(message))
}

func (qu *queue) Dequeue(options *DequeueOptions) (*Message, error) {
	outMsg := &Message{}
	session := qu.conn.Session()
	session.ResetBuffer()
	session.PutTTCFunc(0x3, 0x7A)
	queueNameBytes := qu.conn.ServerStringCoder().Encode(qu.Name)
	if len(queueNameBytes) > 0 {
		session.PutBytes(1)
		session.PutInt(len(queueNameBytes), 2, true, true)
	} else {
		session.PutBytes(0, 0)
	}
	session.PutBytes(1, 1, 1, 1)
	consumer := qu.conn.ServerStringCoder().Encode(options.Consumer)
	if len(consumer) > 0 {
		session.PutBytes(1)
		session.PutUint(len(consumer), 4, true, true)
	} else {
		session.PutBytes(0, 0)
	}
	session.PutInt(int(options.Mode), 4, true, true)
	session.PutInt(int(options.Navigation), 4, true, true)
	session.PutInt(int(options.Visibility), 4, true, true)
	session.PutInt(options.Wait, 4, true, true)
	if len(outMsg.messageID) > 0 {
		session.PutBytes(1)
		session.PutInt(len(outMsg.messageID), 2, true, true)
	} else {
		session.PutBytes(0, 0)
	}
	correlation := qu.conn.ServerStringCoder().Encode(options.Correlation)
	if len(correlation) > 0 {
		session.PutBytes(1)
		session.PutInt(len(correlation), 4, true, true)
	} else {
		session.PutBytes(0, 0)
	}
	session.PutBytes(1)
	session.PutInt(len(qu.toid), 4, true, true)
	session.PutUint(qu.version, 2, true, true)
	session.PutBytes(1)
	// retrieve message id in response
	if len(outMsg.messageID) == 0 {
		session.PutBytes(1)
		session.PutInt(0x10, 2, true, true)
	} else {
		session.PutBytes(0, 0)
	}
	num := 0
	if qu.AutoCommit {
		num = 32
	}
	// the delivery mode of the options. go-ora took it from the new message,
	// which has none, so a buffered message never dequeued
	switch options.Delivery {
	case DeliveryModeBuffered:
		num |= 2
	case DeliveryModeAny:
		num |= 16
	}
	session.PutUint(num, 4, true, true)
	condition := qu.conn.ServerStringCoder().Encode(options.Condition)
	if len(condition) > 0 {
		session.PutBytes(1)
		session.PutInt(len(condition), 4, true, true)
	} else {
		session.PutBytes(0, 0)
	}
	if len(outMsg.extensions) > 0 {
		session.PutBytes(1)
		session.PutInt(len(outMsg.extensions), 2, true, true)
	} else {
		session.PutBytes(0, 0)
	}
	if qu.conn.TTCVersion() >= 14 {
		session.PutBytes(0)
	}
	if qu.conn.TTCVersion() >= 16 {
		// the shard, -1, as python-oracledb writes it
		session.PutUint(uint32(0xFFFFFFFF), 4, true, true)
	}
	if len(queueNameBytes) > 0 {
		session.PutClr(queueNameBytes)
	}
	if len(consumer) > 0 {
		session.PutClr(consumer)
	}
	if len(outMsg.messageID) > 0 {
		session.PutBytes(outMsg.messageID...)
	}
	if len(correlation) > 0 {
		session.PutClr(correlation)
	}
	session.PutBytes(qu.toid...)
	if len(condition) > 0 {
		session.PutClr(condition)
	}
	for _, extension := range outMsg.extensions {
		session.PutKeyVal(extension.key, extension.value, extension.num)
	}
	err := session.Write()
	if err != nil {
		return nil, err
	}
	err = qu.afterReset(qu.readDequeueResponse(outMsg))
	if noMessage(err) {
		return nil, nil
	}
	return outMsg, err
}

// noMessage reports whether err is ORA-25228, which a dequeue that finds no
// message ends with. python-oracledb returns no message for it, and so does
// a dequeue here. go-ora returned the error and an empty message.
func noMessage(err error) bool {
	var oerr *tns.OracleError
	return errors.As(err, &oerr) && oerr.ErrCode == 25228
}

func (qu *queue) readEnqueueResponse(message *Message) error {
	session := qu.conn.Session()
	loop := true
	for loop {
		msg, err := session.ReadByte()
		if err != nil {
			return err
		}
		switch msg {
		case 8:
			message.messageID, err = session.ReadBytes(16)
			_, err = session.ReadInt(2, true, true)
		default:
			err = qu.conn.ProcessTCCResponse(msg)
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

func (qu *queue) readDequeueResponse(message *Message) error {
	session := qu.conn.Session()
	loop := true
	for loop {
		msg, err := session.ReadByte()
		if err != nil {
			return err
		}
		switch msg {
		case 8:
			var num int
			var err error
			num, err = session.ReadInt(4, true, true)
			if err != nil {
				return err
			}
			if num > 0 {
				err = message.read(qu.conn)
				if err != nil {
					return err
				}
			}
			_, err = session.ReadInt(4, true, true)
			if err != nil {
				return err
			}
			// unmarshal toc
			imageLen, err := qu.readTypeInformation()
			if err != nil {
				return err
			}
			if imageLen > 0 {
				err = message.readData(qu.conn, qu.messageType, qu.udtName)
				if err != nil {
					return err
				}
			}
			message.messageID, err = session.ReadBytes(16)
		default:
			err = qu.conn.ProcessTCCResponse(msg)
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

func (qu *queue) readTypeInformation() (dataLen int, err error) {
	session := qu.conn.Session()
	_ /*qu.toid*/, err = session.ReadDLC()
	if err != nil {
		return
	}
	_ /*oid*/, err = session.ReadDLC()
	if err != nil {
		return
	}
	_ /*snapshot*/, err = session.ReadDLC()
	if err != nil {
		return
	}
	_ /*version*/, err = session.ReadInt(2, true, true)
	if err != nil {
		return
	}
	dataLen, err = session.ReadInt(4, true, true)
	if err != nil {
		return
	}
	_ /*flag*/, err = session.ReadInt(2, true, true)
	return
}

// readArrayResponse reads the reply to an array of messages, and returns
// the count of messages that the server returned.
func (qu *queue) readArrayResponse(messages []*Message, isEnqueue bool) (int, error) {
	session := qu.conn.Session()
	count := 0
	for {
		msgCode, err := session.ReadByte()
		if err != nil {
			return count, qu.afterReset(err)
		}
		switch msgCode {
		case 8:
			count, err = qu.readArrayRPA(messages, isEnqueue)
			if err != nil {
				return count, err
			}
		default:
			err = qu.conn.ProcessTCCResponse(msgCode)
			if err != nil {
				return count, err
			}
			if msgCode == 4 || msgCode == 9 {
				return count, nil
			}
		}
	}
}

// afterReset reads the reply that follows a break of the server, which ends
// with the error of the call, as the statements of the driver do. The server
// breaks a dequeue of an array that finds no message, and sends ORA-25228
// after the reset. go-ora returned the reset as a timeout.
func (qu *queue) afterReset(err error) error {
	if !errors.Is(err, tns.ErrConnReset) {
		return err
	}
	session := qu.conn.Session()
	for {
		msg, err := session.ReadByte()
		if err != nil {
			return err
		}
		if err := qu.conn.ProcessTCCResponse(msg); err != nil {
			return err
		}
		if msg == 4 || msg == 9 {
			return nil
		}
	}
}

func (qu *queue) readArrayRPA(messages []*Message, isEnqueue bool) (int, error) {
	session := qu.conn.Session()

	numResp, err := session.ReadInt(4, true, true)
	if err != nil {
		return 0, err
	}

	for i := range numResp {
		if i >= len(messages) {
			break
		}
		var num int
		num, err = session.ReadInt(2, true, true)
		if err != nil {
			return numResp, err
		}
		if num > 0 {
			_, err = session.ReadByte()
			if err != nil {
				return numResp, err
			}
			err = messages[i].read(qu.conn)
			if err != nil {
				return numResp, err
			}
		}
		_, err = session.ReadInt(2, true, true)
		if err != nil {
			return numResp, err
		}
		var dataLen int
		dataLen, err = session.ReadInt(2, true, true)
		if err != nil {
			return numResp, err
		}
		if !isEnqueue {
			// get type information
			dataLen, err = qu.readTypeInformation()
			if err != nil {
				return numResp, err
			}
		}
		if dataLen > 0 {
			err = messages[i].readData(qu.conn, qu.messageType, qu.udtName)
			if err != nil {
				return numResp, err
			}
		}
		msgID, err := session.ReadDLC()
		if err != nil {
			return numResp, err
		}
		if len(msgID) == 0x10 {
			messages[i].messageID = make([]byte, 0x10)
			copy(messages[i].messageID, msgID)
		}
		// an enqueue returns the IDs of every message at once. go-ora gave
		// each message the ID of the first
		if len(msgID)/0x10 == len(messages) {
			for j := range messages {
				messages[j].messageID = make([]byte, 0x10)
				copy(messages[j].messageID, msgID[j*16:(j+1)*0x10])
			}
		}
		_, err = session.ReadInt(2, true, true)
		if err != nil {
			return numResp, err
		}
		_, err = session.ReadInt(2, true, true)
		if err != nil {
			return numResp, err
		}

	}
	if isEnqueue {
		_ /*count*/, err = session.ReadInt(4, true, true)
	}
	return numResp, err
}
