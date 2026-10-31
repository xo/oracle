package ttc

import (
	"database/sql/driver"
	"errors"

	"github.com/xo/oracle/internal/tns"
)

type simpleObject struct {
	connection  *Connection
	operationID uint8
	data        []byte
	err         error
}

func (obj *simpleObject) write() *simpleObject {
	session := obj.connection.session
	session.PutTTCFunc(0x3, obj.operationID)
	if obj.data != nil {
		session.PutBytes(obj.data...)
	}
	obj.err = session.Write()
	return obj
}

func (obj *simpleObject) exec() error {
	conn := obj.connection
	logger := conn.logger
	obj.write()
	if obj.err != nil {
		return obj.err
	}
	err := conn.read()
	if errors.Is(err, tns.ErrConnReset) {
		err = conn.read()
	}
	if err != nil {
		if isBadConn(err) {
			obj.connection.setBad()
			logger.Info("bad connection", "error", err)
			return driver.ErrBadConn
		}
		return err
	}
	return nil
}
