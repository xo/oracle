package oracle

import (
	"context"
	"database/sql/driver"

	"github.com/xo/oracle/internal/ttc"
)

// The interfaces of database/sql that conn implements.
var (
	_ driver.Conn               = (*conn)(nil)
	_ driver.ConnPrepareContext = (*conn)(nil)
	_ driver.ConnBeginTx        = (*conn)(nil)
	_ driver.ExecerContext      = (*conn)(nil)
	_ driver.QueryerContext     = (*conn)(nil)
	_ driver.Pinger             = (*conn)(nil)
	_ driver.SessionResetter    = (*conn)(nil)
	_ driver.Validator          = (*conn)(nil)
	_ driver.NamedValueChecker  = (*conn)(nil)
)

// conn is a connection of database/sql. It holds the connection of
// internal/ttc, which runs the protocol, and passes each call to it.
type conn struct {
	c *ttc.Conn
}

func (c *conn) Prepare(query string) (driver.Stmt, error) {
	return wrapStmt(c.c.Prepare(query))
}

func (c *conn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	return wrapStmt(c.c.PrepareContext(ctx, query))
}

func (c *conn) Close() error {
	return c.c.Close()
}

func (c *conn) Begin() (driver.Tx, error) {
	return wrapTx(c.c.Begin())
}

func (c *conn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return wrapTx(c.c.BeginTx(ctx, opts))
}

func (c *conn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return c.c.ExecContext(ctx, query, args)
}

func (c *conn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return wrapRows(c.c.QueryContext(ctx, query, args))
}

func (c *conn) Ping(ctx context.Context) error {
	return c.c.Ping(ctx)
}

func (c *conn) ResetSession(ctx context.Context) error {
	return c.c.ResetSession(ctx)
}

func (c *conn) IsValid() bool {
	return c.c.IsValid()
}

func (c *conn) CheckNamedValue(nv *driver.NamedValue) error {
	return c.c.CheckNamedValue(nv)
}
