package ttc

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/cockroachdb/apd/v3"

	oraTypes "github.com/xo/oracle/internal/types"
)

// numberValue returns the Go value of a NUMBER column, from the text that the
// decoder of NUMBER returns. The column type decides the Go type, never the
// value, so every row of a column has the same type (D16):
//
//   - NUMBER with a scale of 0 and a precision from 1 to 18 is an int64,
//     because 18 digits always fit.
//   - Any other NUMBER is an *apd.Decimal, which holds the 38 digits that
//     NUMBER can hold. That includes NUMBER with no precision, such as the
//     result of COUNT(*), which Oracle describes as precision 38 and scale
//     -127.
func numberValue(precision, scale uint8, v any) (any, error) {
	s, ok := v.(string)
	if !ok {
		return v, nil
	}
	s = strings.TrimSuffix(s, ".")
	if scale == 0 && precision >= 1 && precision <= 18 {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("reading the NUMBER(%d) %q as an int64: %w", precision, s, err)
		}
		return n, nil
	}
	d, _, err := apd.NewFromString(s)
	if err != nil {
		return nil, fmt.Errorf("reading the NUMBER %q as a decimal: %w", s, err)
	}
	return d, nil
}

// columnValue returns the Go value of a column from the value that its
// decoder returns.
func columnValue(col *ParameterInfo, v any) (any, error) {
	switch col.DataType {
	case oraTypes.NUMBER:
		return numberValue(col.Precision, col.Scale, v)
	case oraTypes.IBFLOAT:
		if f, ok := v.(float32); ok {
			// through the shortest text, so that 1.5 stays 1.5 and does not
			// become 1.5000000xxx
			return strconv.ParseFloat(strconv.FormatFloat(float64(f), 'g', -1, 32), 64)
		}
	}
	return v, nil
}

// NextRow advances to the next row. It satisfies driver.RowsColumnScanner,
// which database/sql calls in place of Next from Go 1.27.
func (dataSet *DataSet) NextRow() error {
	n := len(dataSet.Columns())
	if cap(dataSet.row) < n {
		dataSet.row = make([]driver.Value, n)
	}
	dataSet.row = dataSet.row[:n]
	clear(dataSet.row)
	err := dataSet.Next(dataSet.row)
	if err == io.EOF {
		return io.EOF
	}
	return err
}

// ScanColumn stores the column index of the current row in dest. It satisfies
// driver.RowsColumnScanner.
func (dataSet *DataSet) ScanColumn(scanCtx driver.ScanContext, index int, dest any) error {
	if index < 0 || index >= len(dataSet.row) {
		return fmt.Errorf("scanning column %d of a row of %d columns: %w", index, len(dataSet.row), io.ErrUnexpectedEOF)
	}
	return assign(scanCtx, dest, dataSet.row[index])
}

// assign stores src in dest. A *any receives the value as it is, and an
// *apd.Decimal receives a decimal. Every other pair goes to sql.ConvertAssign,
// and a decimal goes to it as text, which is the form that a *string, a
// *float64, a *int64 and the Scan method of apd.Decimal take. This is Assign
// of dbimp.
func assign(scanCtx driver.ScanContext, dest, src any) error {
	switch d := dest.(type) {
	case *any:
		*d = src
		return nil
	case *apd.Decimal:
		if s, ok := src.(*apd.Decimal); ok {
			d.Set(s)
			return nil
		}
	}
	if d, ok := src.(*apd.Decimal); ok {
		src = d.Text('f')
	}
	if err := sql.ConvertAssign(scanCtx, dest, src); err != nil {
		return fmt.Errorf("assigning %T to %T: %w", src, dest, err)
	}
	return nil
}
