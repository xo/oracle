package ttc

import (
	"database/sql/driver"
)

// Transaction is a transaction of one connection. It keeps no context,
// because database/sql calls Commit and Rollback with none. When the context
// of BeginTx ends, database/sql calls Rollback, and the rollback runs to the
// end under the timeout of the connection and is never cancelled. A rollback
// that a cancelled context broke left the transaction open, and a later
// commit committed it, which was sijms/go-ora#767 (D10).
type Transaction struct {
	conn *Connection
}

func (tx *Transaction) Commit() error {
	if tx.conn.State != Opened {
		return driver.ErrBadConn
	}
	tx.conn.autoCommit = true
	tx.conn.session.ResetBuffer()
	return (&simpleObject{connection: tx.conn, operationID: 0xE}).exec()
}

func (tx *Transaction) Rollback() error {
	if tx.conn.State != Opened {
		return driver.ErrBadConn
	}
	tx.conn.autoCommit = true
	tx.conn.session.ResetBuffer()
	return (&simpleObject{connection: tx.conn, operationID: 0xF}).exec()
}
