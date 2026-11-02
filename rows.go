package oracle

import (
	"database/sql/driver"
	"reflect"

	"github.com/xo/oracle/internal/ttc"
)

// The interfaces of database/sql that rows implements.
var (
	_ driver.Rows                           = (*rows)(nil)
	_ driver.RowsColumnScanner              = (*rows)(nil)
	_ driver.RowsNextResultSet              = (*rows)(nil)
	_ driver.RowsColumnTypeDatabaseTypeName = (*rows)(nil)
	_ driver.RowsColumnTypeLength           = (*rows)(nil)
	_ driver.RowsColumnTypeNullable         = (*rows)(nil)
	_ driver.RowsColumnTypePrecisionScale   = (*rows)(nil)
	_ driver.RowsColumnTypeScanType         = (*rows)(nil)
)

// rows is the rows of a query of database/sql. It holds the rows of
// internal/ttc, and passes each call to them.
type rows struct {
	r *ttc.Rows
}

// wrapRows returns r as rows.
func wrapRows(r driver.Rows, err error) (driver.Rows, error) {
	if err != nil {
		return nil, err
	}
	if tr, ok := r.(*ttc.Rows); ok {
		return &rows{r: tr}, nil
	}
	return r, nil
}

func (r *rows) Columns() []string {
	return r.r.Columns()
}

func (r *rows) Close() error {
	return r.r.Close()
}

func (r *rows) Next(dest []driver.Value) error {
	return r.r.Next(dest)
}

func (r *rows) NextRow() error {
	return r.r.NextRow()
}

func (r *rows) ScanColumn(scanCtx driver.ScanContext, index int, dest any) error {
	return r.r.ScanColumn(scanCtx, index, dest)
}

func (r *rows) HasNextResultSet() bool {
	return r.r.HasNextResultSet()
}

func (r *rows) NextResultSet() error {
	return r.r.NextResultSet()
}

func (r *rows) ColumnTypeDatabaseTypeName(index int) string {
	return r.r.ColumnTypeDatabaseTypeName(index)
}

func (r *rows) ColumnTypeLength(index int) (int64, bool) {
	return r.r.ColumnTypeLength(index)
}

func (r *rows) ColumnTypeNullable(index int) (nullable, ok bool) {
	return r.r.ColumnTypeNullable(index)
}

func (r *rows) ColumnTypePrecisionScale(index int) (int64, int64, bool) {
	return r.r.ColumnTypePrecisionScale(index)
}

func (r *rows) ColumnTypeScanType(index int) reflect.Type {
	return r.r.ColumnTypeScanType(index)
}
