package oracle

import (
	"database/sql/driver"

	"github.com/xo/oracle/internal/ttc"
)

// The interface of database/sql that tx implements.
var _ driver.Tx = (*tx)(nil)

// tx is a transaction of database/sql. It holds the transaction of
// internal/ttc, and passes each call to it.
type tx struct {
	t *ttc.Tx
}

// wrapTx returns t as a tx.
func wrapTx(t driver.Tx, err error) (driver.Tx, error) {
	if err != nil {
		return nil, err
	}
	if tt, ok := t.(*ttc.Tx); ok {
		return &tx{t: tt}, nil
	}
	return t, nil
}

func (t *tx) Commit() error {
	return t.t.Commit()
}

func (t *tx) Rollback() error {
	return t.t.Rollback()
}
