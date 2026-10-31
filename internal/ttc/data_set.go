package ttc

import (
	"database/sql/driver"
	"io"
	"reflect"

	"github.com/xo/oracle/internal/tns"
	"github.com/xo/oracle/internal/trace"
)

// Compile time Sentinels for implemented Interfaces.
var (
	_ = driver.Rows((*DataSet)(nil))
	_ = driver.RowsColumnTypeDatabaseTypeName((*DataSet)(nil))
	_ = driver.RowsColumnTypeLength((*DataSet)(nil))
	_ = driver.RowsColumnTypeNullable((*DataSet)(nil))
	_ = driver.RowsColumnTypePrecisionScale((*DataSet)(nil))
)

type DataSet struct {
	resultSets []ResultSet
	index      int
	// row is the current row of NextRow, which ScanColumn reads
	row []driver.Value
	//columnCount     int
	//rowCount        int
	//uACBufferLength int
	//maxRowSize      int
	//cols            *[]ParameterInfo
	//rows            []Row
	//currentRow      Row

	//index           int
	//parent          parentStmt
}

func (dataSet *DataSet) currentResultSet() *ResultSet {
	if dataSet.resultSets == nil {
		dataSet.resultSets = make([]ResultSet, 0)
		dataSet.resultSets = append(dataSet.resultSets, ResultSet{})
		dataSet.index = 0
	}
	return &dataSet.resultSets[dataSet.index]
}

func (dataSet *DataSet) clear() {
	dataSet.resultSets = nil
	dataSet.index = 0
}

// load Loading dataset information from network session
func (dataSet *DataSet) load(session *tns.Session) error {
	return dataSet.currentResultSet().load(session)
}

// Close closes each result set. A nil DataSet has nothing to close, where it
// panicked, which was sijms/go-ora#762.
func (dataSet *DataSet) Close() error {
	if dataSet == nil {
		return nil
	}
	var err error
	for _, resultSet := range dataSet.resultSets {
		err = resultSet.Close()
		if err != nil {
			return err
		}
	}
	dataSet.clear()
	return nil
}

// Advance act like Next in sql package return false if no other rows in dataset
func (dataSet *DataSet) Advance() bool {
	return dataSet.currentResultSet().Advance()
}

// Scan act like scan in sql package return row values to dest variable pointers
func (dataSet *DataSet) Scan(dest ...any) error {
	return dataSet.currentResultSet().Scan(dest...)
}

// set object value using currentRow[colIndex] return true if succeed or false
// for non-supported type
// error means error occur during operation
func (dataSet *DataSet) setObjectValue(obj reflect.Value, colIndex int) error {
	return dataSet.currentResultSet().setObjectValue(obj, colIndex)
}

func (dataSet *DataSet) Err() error {
	return dataSet.currentResultSet().Err()
}

// Next implement method need for sql.Rows interface
func (dataSet *DataSet) Next(dest []driver.Value) error {
	return dataSet.currentResultSet().Next(dest)
}

// Columns return a string array that represent columns names
func (dataSet *DataSet) Columns() []string {
	return dataSet.currentResultSet().Columns()
}

func (dataSet *DataSet) Trace(t trace.Tracer) {
	dataSet.currentResultSet().Trace(t)
}

// ColumnTypeDatabaseTypeName return Col DataType name
func (dataSet *DataSet) ColumnTypeDatabaseTypeName(index int) string {
	return dataSet.currentResultSet().ColumnTypeDatabaseTypeName(index)
}

// ColumnTypeLength return length of column type
func (dataSet *DataSet) ColumnTypeLength(index int) (int64, bool) {
	return dataSet.currentResultSet().ColumnTypeLength(index)
}

// ColumnTypeNullable return if column allow null or not
func (dataSet *DataSet) ColumnTypeNullable(index int) (nullable, ok bool) {
	return dataSet.currentResultSet().ColumnTypeNullable(index)
}

// ColumnTypePrecisionScale return the precision and scale for numeric types
func (dataSet *DataSet) ColumnTypePrecisionScale(index int) (int64, int64, bool) {
	return dataSet.currentResultSet().ColumnTypePrecisionScale(index)
}

func (dataSet *DataSet) ColumnTypeScanType(index int) reflect.Type {
	return dataSet.currentResultSet().ColumnTypeScanType(index)
}

func (dataSet *DataSet) NextResultSet() error {
	if dataSet.HasNextResultSet() {
		dataSet.index++
		return nil
	}
	return io.EOF
}

func (dataSet *DataSet) HasNextResultSet() bool {
	return dataSet.index < len(dataSet.resultSets)-1
}
