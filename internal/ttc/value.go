package ttc

import (
	"bytes"
	"database/sql"
	"database/sql/driver"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/cockroachdb/apd/v3"

	"github.com/xo/oracle/internal/config"
	"github.com/xo/oracle/internal/types"
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
	case types.NUMBER:
		return numberValue(col.Precision, col.Scale, v)
	case types.IBFLOAT:
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
func (dataSet *Rows) NextRow() error {
	n := len(dataSet.Columns())
	if cap(dataSet.row) < n {
		dataSet.row = make([]driver.Value, n)
	}
	dataSet.row = dataSet.row[:n]
	clear(dataSet.row)
	dataSet.currentResultSet().lazyLobs = true
	err := dataSet.Next(dataSet.row)
	if err == io.EOF {
		return io.EOF
	}
	return err
}

// ScanColumn stores the column index of the current row in dest. It satisfies
// driver.RowsColumnScanner.
func (dataSet *Rows) ScanColumn(scanCtx driver.ScanContext, index int, dest any) error {
	if index < 0 || index >= len(dataSet.row) {
		return fmt.Errorf("scanning column %d of a row of %d columns: %w", index, len(dataSet.row), io.ErrUnexpectedEOF)
	}
	src, err := dataSet.lobValue(index, dest)
	if err != nil {
		return err
	}
	return assign(scanCtx, dest, src)
}

// rowLob is a LOB of a row that ScanColumn can load.
type rowLob interface {
	Streamer() types.LobStreamer
	IsNil() bool
	IsInline() bool
	ReadMode() config.LobReadMode
	Load() error
	Value() (any, error)
}

// lobValue returns the value of column index for dest. A CLOB or a BLOB that
// dest takes as a LOB stays a locator, which reads until the rows close
// (D22). For another dest, it loads the value, as the read mode auto asks,
// and keeps it in the row for a second Scan.
func (dataSet *Rows) lobValue(index int, dest any) (any, error) {
	var lob rowLob
	switch v := dataSet.row[index].(type) {
	case *types.Clob:
		lob = v
	case *types.Blob:
		lob = v
	case *types.Vector:
		// a VECTOR of stream mode stays a locator until here, a NULL one
		// too, which no slice took
		lob = v
	case *types.BFile:
		dataSet.own(v.Streamer())
		return v, nil
	default:
		return v, nil
	}
	dataSet.own(lob.Streamer())
	switch dest.(type) {
	case *types.Clob, *types.Blob, *types.Vector:
		return lob, nil
	}
	switch {
	case lob.IsNil():
		// a NULL has no locator, which a NULL BLOB of stream mode gave to
		// dest as itself
		dataSet.row[index] = nil
		return nil, nil
	case lob.IsInline():
		// the locator holds the value
	case lob.ReadMode() != config.LobReadModeAuto:
		return lob, nil
	default:
		if err := lob.Load(); err != nil {
			return nil, err
		}
	}
	v, err := lob.Value()
	if err != nil {
		return nil, err
	}
	dataSet.row[index] = v
	return v, nil
}

// own makes the LOB of stream read only until the rows close (D22).
func (dataSet *Rows) own(stream types.LobStreamer) {
	if s, ok := stream.(*LobStream); ok {
		s.rows = &dataSet.guard
	}
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
	// a NULL VECTOR is a nil slice, as a NULL RAW is a nil []byte, which
	// database/sql refused for the other element types
	case *[]float32:
		if src == nil {
			*d = nil
			return nil
		}
	case *[]float64:
		if src == nil {
			*d = nil
			return nil
		}
	case *[]int8:
		if src == nil {
			*d = nil
			return nil
		}
	}
	if d, ok := src.(*apd.Decimal); ok {
		src = d.Text('f')
	}
	if v, ok := src.(jsontext.Value); ok {
		return assignJSON(dest, v)
	}
	if err := sql.ConvertAssign(scanCtx, dest, src); err != nil {
		return fmt.Errorf("assigning %T to %T: %w", src, dest, err)
	}
	return nil
}

// assignJSON stores the JSON text v in dest. A string, a []byte and a
// jsontext.Value take the text, a scanner takes it as a string, and any other
// destination, such as a struct or a map, takes it through json.Unmarshal of
// encoding/json/v2.
func assignJSON(dest any, v jsontext.Value) error {
	switch d := dest.(type) {
	case *jsontext.Value:
		*d = bytes.Clone(v)
	case *string:
		*d = string(v)
	case *[]byte:
		*d = bytes.Clone(v)
	case *types.JSON:
		return d.SetValue(v)
	case sql.Scanner:
		return d.Scan(string(v))
	default:
		if err := json.Unmarshal(v, dest); err != nil {
			return fmt.Errorf("unmarshaling JSON into %T: %w", dest, err)
		}
	}
	return nil
}
