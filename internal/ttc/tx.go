package ttc

import (
	"database/sql/driver"
)

// Tx is a transaction of one connection. It keeps no context,
// because database/sql calls Commit and Rollback with none. When the context
// of BeginTx ends, database/sql calls Rollback, and the rollback runs to the
// end under the timeout of the connection and is never cancelled. A rollback
// that a cancelled context broke left the transaction open, and a later
// commit committed it, which was sijms/go-ora#767 (D10).
type Tx struct {
	conn *Conn
}

func (tx *Tx) Commit() error {
	if !tx.conn.IsValid() {
		return driver.ErrBadConn
	}
	tx.conn.autoCommit = true
	tx.conn.session.ResetBuffer()
	return tx.end((&simpleObject{connection: tx.conn, operationID: 0xE}).exec())
}

func (tx *Tx) Rollback() error {
	if !tx.conn.IsValid() {
		return driver.ErrBadConn
	}
	tx.conn.autoCommit = true
	tx.conn.session.ResetBuffer()
	return tx.end((&simpleObject{connection: tx.conn, operationID: 0xF}).exec())
}

// end returns err of a commit or a rollback, and makes the connection bad
// when the call failed, so that database/sql drops it. A transaction can be
// open still, and the next statement, which commits on its own, would commit
// it. Close rolls back what is open (sijms/go-ora#778).
func (tx *Tx) end(err error) error {
	if err != nil {
		tx.conn.setBad()
	}
	return err
}
