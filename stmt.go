package oracle

import (
	"context"
	"database/sql/driver"

	"github.com/xo/oracle/internal/ttc"
)

// The interfaces of database/sql that stmt implements.
var (
	_ driver.Stmt              = (*stmt)(nil)
	_ driver.StmtExecContext   = (*stmt)(nil)
	_ driver.StmtQueryContext  = (*stmt)(nil)
	_ driver.NamedValueChecker = (*stmt)(nil)
)

// stmt is a prepared statement of database/sql. It holds the statement of
// internal/ttc, and passes each call to it.
type stmt struct {
	s *ttc.Stmt
}

// wrapStmt returns s as a stmt.
func wrapStmt(s driver.Stmt, err error) (driver.Stmt, error) {
	if err != nil {
		return nil, err
	}
	if ts, ok := s.(*ttc.Stmt); ok {
		return &stmt{s: ts}, nil
	}
	return s, nil
}

func (s *stmt) Close() error {
	return s.s.Close()
}

func (s *stmt) NumInput() int {
	return s.s.NumInput()
}

func (s *stmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.s.Exec(args)
}

func (s *stmt) Query(args []driver.Value) (driver.Rows, error) {
	return wrapRows(s.s.Query(args))
}

func (s *stmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	return s.s.ExecContext(ctx, args)
}

func (s *stmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	return wrapRows(s.s.QueryContext(ctx, args))
}

func (s *stmt) CheckNamedValue(nv *driver.NamedValue) error {
	return s.s.CheckNamedValue(nv)
}
