package ttc

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/cockroachdb/apd/v3"

	"github.com/xo/oracle/internal/tns"
	"github.com/xo/oracle/internal/types"
)

type Row []driver.Value
type ResultSet struct {
	columnCount     int
	rowCount        int
	uACBufferLength int
	maxRowSize      int
	cols            *[]ParameterInfo
	rows            []Row
	currentRow      Row
	// lazyLobs is true when ScanColumn loads the CLOBs and BLOBs of a row
	lazyLobs bool
	index    int
	parent   parentStmt
	lastErr  error
}

func (resultSet *ResultSet) load(session *tns.Session) error {
	var err error
	_, err = session.ReadByte()
	if err != nil {
		return err
	}
	var columnCount, num int
	columnCount, err = session.ReadInt(2, true, true)
	if err != nil {
		return err
	}
	num, err = session.ReadInt(4, true, true)
	if err != nil {
		return err
	}
	columnCount += num * 0x100
	if resultSet.columnCount == 0 {
		resultSet.columnCount = columnCount
	}

	if len(resultSet.currentRow) != resultSet.columnCount {
		resultSet.currentRow = make(Row, resultSet.columnCount)
	}
	resultSet.rowCount, err = session.ReadInt(4, true, true)
	if err != nil {
		return err
	}
	resultSet.uACBufferLength, err = session.ReadInt(2, true, true)
	if err != nil {
		return err
	}
	bitVector, err := session.ReadDLC()
	if err != nil {
		return err
	}
	resultSet.setBitVector(bitVector)
	_, err = session.ReadDLC()
	return nil
}

// setBitVector bit vector is an array of bit that defines which column needs to be read
// from network session
func (resultSet *ResultSet) setBitVector(bitVector []byte) {
	index := resultSet.columnCount / 8
	if resultSet.columnCount%8 > 0 {
		index++
	}
	if len(bitVector) > 0 {
		for x := range bitVector {
			for i := range 8 {
				if (x*8)+i < resultSet.columnCount {
					(*resultSet.cols)[(x*8)+i].getDataFromServer = bitVector[x]&(1<<i) > 0
				}
			}
		}
	} else {
		if resultSet.cols != nil {
			for x := 0; x < len(*resultSet.cols); x++ {
				(*resultSet.cols)[x].getDataFromServer = true
			}
		}
	}
}

func (resultSet *ResultSet) Close() error {
	if resultSet.parent.CanAutoClose() {
		return resultSet.parent.Close()
	}
	return nil
}

func (resultSet *ResultSet) Columns() []string {
	if resultSet.cols == nil || len(*resultSet.cols) == 0 {
		return nil
	}
	ret := make([]string, len(*resultSet.cols))
	for x := 0; x < len(*resultSet.cols); x++ {
		ret[x] = (*resultSet.cols)[x].Name
	}
	return ret
}

// ColumnTypeDatabaseTypeName returns the name of the type of a column, as
// the data dictionary writes it, such as VARCHAR2 or TIMESTAMP WITH TIME ZONE.
func (resultSet *ResultSet) ColumnTypeDatabaseTypeName(index int) string {
	return (*resultSet.cols)[index].databaseType
}

// ColumnTypeLength return length of column type
func (resultSet *ResultSet) ColumnTypeLength(index int) (int64, bool) {
	switch (*resultSet.cols)[index].DataType {
	case types.NCHAR, types.CHAR:
		return int64((*resultSet.cols)[index].MaxCharLen), true
	}
	return int64(0), false
}

// ColumnTypeNullable return if column allow null or not
func (resultSet *ResultSet) ColumnTypeNullable(index int) (nullable, ok bool) {
	return (*resultSet.cols)[index].AllowNull, true
}

// ColumnTypePrecisionScale return the precision and scale for numeric types
func (resultSet *ResultSet) ColumnTypePrecisionScale(index int) (int64, int64, bool) {
	col := (*resultSet.cols)[index]
	switch col.DataType {
	case types.NUMBER:
		return int64(col.Precision), int64(col.Scale), true
	}
	return int64(0), int64(0), false
}

func (resultSet *ResultSet) ColumnTypeScanType(index int) reflect.Type {
	col := (*resultSet.cols)[index]
	switch col.DataType {
	case types.NUMBER:
		if col.Scale == 0 && col.Precision >= 1 && col.Precision <= 18 {
			return reflect.TypeFor[int64]()
		}
		return reflect.TypeFor[*apd.Decimal]()
	case types.ROWID, types.UROWID:
		fallthrough
	case types.CHAR, types.NCHAR:
		fallthrough
	case types.OCIClobLocator:
		fallthrough
	case types.LongVarChar:
		return reflect.TypeFor[string]()
	case types.RAW:
		fallthrough
	case types.OCIBlobLocator, types.OCIFileLocator:
		fallthrough
	case types.LongRaw, types.LongVarRaw:
		return reflect.TypeFor[[]byte]()
	case types.DATE, types.TIMESTAMP:
		fallthrough
	case types.TimestampDTY:
		fallthrough
	case types.TimestampLTZ, types.TimestampLTZDTY:
		fallthrough
	case types.TIMESTAMPTZ, types.TimestampTZDTY:
		return reflect.TypeFor[time.Time]()
	case types.IBFLOAT:
		return reflect.TypeFor[float64]()
	case types.IBDOUBLE:
		return reflect.TypeFor[float64]()
	case types.IntervalDSDTY, types.IntervalYMDTY:
		return reflect.TypeFor[string]()
	default:
		return nil
	}
}

func (resultSet *ResultSet) Err() error {
	return resultSet.lastErr
}

// set object value using currentRow[colIndex] return true if succeed or false
// for non-supported type
// error means error occur during operation
func (resultSet *ResultSet) setObjectValue(obj reflect.Value, colIndex int) error {
	return copyValue(obj.Addr().Interface(), resultSet.currentRow[colIndex])
}

// Scan act like scan in sql package return row values to dest variable pointers
func (resultSet *ResultSet) Scan(dest ...any) error {
	if resultSet.lastErr != nil {
		return resultSet.lastErr
	}
	for srcIndex, destIndex := 0, 0; srcIndex < len(resultSet.currentRow); srcIndex, destIndex = srcIndex+1, destIndex+1 {
		if destIndex >= len(dest) {
			return errors.New("go-ora: mismatching between Scan function input count and column count")
		}
		if dest[destIndex] == nil {
			return fmt.Errorf("go-ora: argument %d is nil", destIndex)
		}
		destTyp := reflect.TypeOf(dest[destIndex])
		if destTyp.Kind() != reflect.Pointer {
			return errors.New("go-ora: argument in scan should be passed as pointers")
		}
		destTyp = destTyp.Elem()

		// if struct and tag
		if destTyp.Kind() == reflect.Struct {
			processedFields := 0
			for x := 0; x < destTyp.NumField(); x++ {
				if srcIndex+processedFields >= len(resultSet.currentRow) {
					continue
				}
				field := destTyp.Field(x)
				name, _, _, _ := extractTag(field.Tag.Get("db"))
				if len(name) == 0 {
					continue
				}
				colInfo := (*resultSet.cols)[srcIndex+processedFields]
				if !strings.EqualFold(colInfo.Name, name) {
					continue
				}
				err := resultSet.setObjectValue(reflect.ValueOf(dest[destIndex]).Elem().Field(x), srcIndex+processedFields)
				if err != nil {
					return err
				}
				processedFields++
			}
			if processedFields != 0 {
				srcIndex = srcIndex + processedFields - 1
				continue
			}
		}
		// else
		err := resultSet.setObjectValue(reflect.ValueOf(dest[destIndex]).Elem(), srcIndex)
		if err != nil {
			return err
		}
	}
	return nil
}

// Advance act like Next in sql package return false if no other rows in dataset
func (resultSet *ResultSet) Advance() bool {
	err := resultSet.Next(resultSet.currentRow)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return false
		}
		resultSet.lastErr = err
		return false
	}
	return true
}

// Next implement method need for sql.Rows interface
func (resultSet *ResultSet) Next(dest []driver.Value) error {
	hasMoreRows := resultSet.parent.hasMoreRows()
	noOfRowsToFetch := len(resultSet.rows) // dataSet.parent.noOfRowsToFetch()
	hasBLOB := resultSet.parent.hasBLOB()
	hasLONG := resultSet.parent.hasLONG()
	// a statement that is not a query, which Query can run, has no columns
	// and no rows to fetch
	if !hasMoreRows && noOfRowsToFetch == 0 || len(resultSet.Columns()) == 0 {
		return io.EOF
	}
	// fetch at the first row when the execute brought none, as it does for a
	// LOB, a LONG, and a row whose columns are all untyped NULLs, which was
	// sijms/go-ora#776
	if hasMoreRows && resultSet.index == 0 && (hasBLOB || hasLONG || len(resultSet.rows) == 0) {
		if err := resultSet.parent.fetch(resultSet); err != nil {
			return err
		}
		noOfRowsToFetch = len(resultSet.rows)
		hasMoreRows = resultSet.parent.hasMoreRows()
		if !hasMoreRows && noOfRowsToFetch == 0 {
			return io.EOF
		}
	}
	if resultSet.index > 0 && resultSet.index%len(resultSet.rows) == 0 {
		if hasMoreRows {
			resultSet.rows = make([]Row, 0, resultSet.parent.noOfRowsToFetch())
			err := resultSet.parent.fetch(resultSet)
			if err != nil {
				return err
			}
			noOfRowsToFetch = len(resultSet.rows)
			hasMoreRows = resultSet.parent.hasMoreRows()
			resultSet.index = 0
			if !hasMoreRows && noOfRowsToFetch == 0 {
				return io.EOF
			}
		} else {
			return io.EOF
		}
	}

	if noOfRowsToFetch > 0 && resultSet.index%noOfRowsToFetch < len(resultSet.rows) {
		row := resultSet.rows[resultSet.index%noOfRowsToFetch]
		if err := resultSet.parent.decodeRow(row, resultSet.lazyLobs); err != nil {
			return err
		}
		length := min(len(dest), len(row))
		for x := 0; x < length; x++ {
			dest[x] = row[x]
		}
		// the row is not read again, and a loaded LOB is freed with it
		resultSet.rows[resultSet.index%noOfRowsToFetch] = nil
		resultSet.index++
		return nil
	}
	return io.EOF
}
