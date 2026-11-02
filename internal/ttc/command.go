package ttc

import (
	"cmp"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/xo/oracle/internal/charset"
	"github.com/xo/oracle/internal/codec"
	"github.com/xo/oracle/internal/types"

	"github.com/xo/oracle/internal/config"
	"github.com/xo/oracle/internal/tns"
)

type StmtType int

const (
	SELECT StmtType = 1
	DML    StmtType = 2
	PLSQL  StmtType = 3
	OTHERS StmtType = 4
)

type parentStmt interface {
	hasMoreRows() bool
	noOfRowsToFetch() int
	fetch(resultSet *ResultSet) error
	decodeRow(row Row, lazy bool) error
	hasBLOB() bool
	hasLONG() bool
	read(resultSet *ResultSet) error
	Close() error
	CanAutoClose() bool
}
type defaultStmt struct {
	connection *Connection
	text       string
	// disableCompression bool
	_hasLONG     bool
	_hasBLOB     bool
	_hasMoreRows bool
	// batchErrors and rowCounts are the options of D34. counts holds the
	// count of each row of the last batch
	batchErrors       bool
	rowCounts         *[]int64
	counts            []int64
	_hasReturnClause  bool
	_noOfRowsToFetch  int
	stmtType          StmtType
	cursorID          int
	queryID           uint64
	Pars              []ParameterInfo
	columns           []ParameterInfo
	scnForSnapshot    []int
	arrayBindCount    int
	containOutputPars bool
	autoClose         bool
	multiSet          []RefCursor
	// prefetch and lob are the options of the statement (D109). lobSet is
	// true when lob holds them, which a REF CURSOR does not
	prefetch int
	lob      config.LobFetch
	lobSet   bool
	// autoFetch is true when sizeFetch sizes each fetch. lobRowBytes is the
	// size of the LOB values of a row in the last fetch that loaded them,
	// and lobMeasured is true once one did.
	autoFetch   bool
	lobRowBytes int
	lobMeasured bool
}

func (stmt *defaultStmt) CanAutoClose() bool {
	return stmt.autoClose
}

func (stmt *defaultStmt) hasMoreRows() bool {
	return stmt._hasMoreRows
}

func (stmt *defaultStmt) noOfRowsToFetch() int {
	return stmt._noOfRowsToFetch
}

func (stmt *defaultStmt) hasLONG() bool {
	return stmt._hasLONG
}

func (stmt *defaultStmt) hasBLOB() bool {
	return stmt._hasBLOB
}

// basicWrite this is the default write procedure for the all type of stmt
// through it the stmt data will send to network stream
func (stmt *defaultStmt) basicWrite(exeOp int, parse, define bool) error {
	session := stmt.connection.session
	strConv := stmt.connection.ServerStringCoder()
	session.PutTTCFunc(0x3, 0x5E)
	session.PutUint(exeOp, 4, true, true)
	session.PutUint(stmt.cursorID, 2, true, true)
	// the pointer of the text, which a parse sends, as python-oracledb
	// writes it. go-ora wrote it only for a new cursor, so a statement that
	// parsed again into the cursor of a failed run, which the server gives
	// with the error, sent its text with no pointer, and the server failed
	// with ORA-03120
	if stmt.cursorID == 0 || parse {
		session.PutBytes(1)
	} else {
		session.PutBytes(0)
	}
	if parse {
		session.PutUint(len(strConv.Encode(stmt.text)), 4, true, true)
		session.PutBytes(1)
	} else {
		session.PutBytes(0, 1)
	}
	session.PutUint(13, 2, true, true)
	session.PutBytes(0, 0)
	if exeOp&0x40 == 0 && exeOp&0x20 != 0 && exeOp&0x1 != 0 && stmt.stmtType == SELECT {
		session.PutBytes(0)
		session.PutUint(stmt._noOfRowsToFetch, 4, true, true)
	} else {
		session.PutBytes(0, 0)
	}
	// switch (longFetchSize)
	// {
	// case -1:
	// 	this.m_marshallingEngine.MarshalUB4((long) int.MaxValue);
	// 	break;
	// case 0:
	// 	this.m_marshallingEngine.MarshalUB4(1L);
	// 	break;
	// default:
	// 	this.m_marshallingEngine.MarshalUB4((long) longFetchSize);
	// 	break;
	// }
	// we use here int.MaxValue
	if stmt.connection.connOption.Lob == config.INLINE {
		session.PutInt(0x3FFFFFFF, 4, true, true)
	} else {
		session.PutUint(0x7FFFFFFF, 4, true, true)
	}

	if len(stmt.Pars) > 0 && !define {
		session.PutBytes(1)
		session.PutUint(len(stmt.Pars), 2, true, true)
	} else {
		session.PutBytes(0, 0)
	}
	session.PutBytes(0, 0, 0, 0, 0)
	if define {
		session.PutBytes(1)
		session.PutUint(len(stmt.columns), 2, true, true)
	} else {
		session.PutBytes(0, 0)
	}
	if session.TTCVersion >= 4 {
		session.PutBytes(0, 0, 1)
	}
	if session.TTCVersion >= 5 {
		session.PutBytes(0, 0, 0, 0, 0)
	}
	if session.TTCVersion >= 7 {
		if stmt.stmtType == DML && stmt.arrayBindCount > 0 {
			session.PutBytes(1)
			session.PutInt(stmt.arrayBindCount, 4, true, true)
			session.PutBytes(1)
		} else {
			session.PutBytes(0, 0, 0)
		}
	}
	if session.TTCVersion >= 8 {
		session.PutBytes(0, 0, 0, 0, 0)
	}
	if session.TTCVersion >= 9 {
		session.PutBytes(0, 0)
	}
	if parse {
		session.PutClr(strConv.Encode(stmt.text))
	}
	al8i4 := make([]int, 13)
	if exeOp&1 <= 0 {
		al8i4[0] = 0
	} else {
		al8i4[0] = 1
	}
	switch stmt.stmtType {
	case DML:
		fallthrough
	case PLSQL:
		if stmt.arrayBindCount > 0 {
			al8i4[1] = stmt.arrayBindCount
			if stmt.stmtType == DML {
				al8i4[9] = 0x4000
			}
		} else {
			al8i4[1] = 1
		}
	case OTHERS:
		al8i4[1] = 1
	default:
		// this.m_al8i4[1] = !fetch ? 0L : noOfRowsToFetch;
		// al8i4[1] = stmt._noOfRowsToFetch
		if stmt.connection.connOption.Lob == config.INLINE {
			if parse {
				al8i4[1] = 0
			} else {
				al8i4[1] = stmt._noOfRowsToFetch
			}
		} else {
			al8i4[1] = stmt._noOfRowsToFetch
		}

	}
	if len(stmt.scnForSnapshot) == 2 {
		al8i4[5] = stmt.scnForSnapshot[0]
		al8i4[6] = stmt.scnForSnapshot[1]
	} else {
		al8i4[5] = 0
		al8i4[6] = 0
	}
	if stmt.stmtType == SELECT {
		al8i4[7] = 1
	} else {
		al8i4[7] = 0
	}
	if exeOp&32 != 0 {
		al8i4[9] |= 0x8000
	} else {
		al8i4[9] &= -0x8000
	}
	for x := range al8i4 {
		session.PutUint(al8i4[x], 4, true, true)
	}
	if define {
		err := stmt.writeDefine()
		if err != nil {
			return err
		}
	} else {
		for _, par := range stmt.Pars {
			_ = par.write(session)
		}
	}
	return nil
}

func (stmt *defaultStmt) writeDefine() error {
	session := stmt.connection.session
	num := 0x7FFFFFFF
	for index, col := range stmt.columns {
		col.oaccollid = 0
		col.Precision = 0
		col.Scale = 0
		col.MaxCharLen = 0
		if col.isLobType() {
			num = 0
			// temp.ContFlag |= 0x2000000
			if stmt.connection.connOption.Lob == config.INLINE && !col.IsJSON && col.DataType != types.OCIFileLocator {
				num = 0x3FFFFFFF
				if col.DataType == types.OCIBlobLocator || col.DataType == types.VECTOR || col.DataType == types.JSONType {
					col.DataType = types.LongRaw
					// change data type in the original array
					stmt.columns[index].DataType = types.LongRaw
				} else {
					col.DataType = types.LongVarChar
					// change data type in the original array
					stmt.columns[index].DataType = types.LongVarChar
					// a CLOB of a database in AL32UTF8 is stored as UTF-16.
					// Defined in the database charset, 11g to 19c convert it
					// piece by piece and send U+FFFD for a pair that a piece
					// splits, which was sijms/go-ora#769. Defined in the
					// national charset when that is AL16UTF16, it comes as
					// it is stored.
					if col.CharsetForm == 1 && stmt.connection.nStrConv != nil && stmt.connection.nStrConv.LangID == charset.AL16UTF16 {
						col.CharsetForm, col.CharsetID = 2, charset.AL16UTF16
						stmt.columns[index].CharsetForm, stmt.columns[index].CharsetID = 2, charset.AL16UTF16
					}
				}
			} else {
				col.ContFlag |= 0x2000000
				// the length that the server sends with the locator. A
				// JSON and a VECTOR come whole within it, up to their
				// largest, as python-oracledb asks: 32 MiB of OSON and
				// 1 MiB of VECTOR. go-ora asked for 32 KiB, and a JSON
				// past it came back cut, with no error
				switch {
				case col.IsJSON || col.DataType == types.JSONType:
					col.MaxCharLen = 32 << 20
				case col.DataType == types.VECTOR:
					col.MaxCharLen = 1 << 20
				default:
					col.MaxCharLen = 0x8000
				}
			}
		} else {
			col.ContFlag = 0
		}
		col.Flag = 3
		col.MaxLen = int64(num)

		err := col.write(session)
		if err != nil {
			return err
		}
	}
	return nil
}

type Stmt struct {
	defaultStmt
	// reExec           bool
	reSendParDef bool
	parse        bool // means parse the command in the server this occurs if the stmt is not cached
	execute      bool
	define       bool
	bulkExec     bool
	// noOfDefCols        int
}

type QueryResult struct {
	lastInsertedID int64
	rowsAffected   int64
}

func (rs *QueryResult) LastInsertId() (int64, error) {
	return rs.lastInsertedID, nil
}

func (rs *QueryResult) RowsAffected() (int64, error) {
	return rs.rowsAffected, nil
}

// NewStmt create new stmt and set its connection properties
func NewStmt(text string, conn *Connection) *Stmt {
	stmt := &Stmt{
		reSendParDef: false,
		parse:        true,
		execute:      true,
		define:       false,
	}
	stmt.connection = conn
	stmt.text = text
	if conn != nil && conn.connOption != nil {
		stmt.prefetch = conn.connOption.PrefetchRows
		stmt.lob, stmt.lobSet = conn.lobFetch, true
	}
	stmt._hasBLOB = false
	stmt._hasLONG = false
	stmt.arrayBindCount = 0
	stmt.scnForSnapshot = make([]int, 2)
	// the server reports a string with no closing quote, so the error waits
	// for a bind by name, which needs the names
	info, _ := parseSQL(text)
	stmt.stmtType = info.kind
	stmt.bulkExec = info.kind == DML
	stmt._hasReturnClause = info.returning
	return stmt
}

func (stmt *Stmt) writePars() error {
	session := stmt.connection.session
	ms := tns.NewMemorySession(nil, nil, session.SessionProperties)
	for _, par := range stmt.Pars {
		if par.Flag == 0x80 {
			continue
		}
		if !stmt.parse && !stmt.reSendParDef && par.Direction == Output {
			continue
		}
		if !par.isLongType() {
			if par.encoder != nil {
				par.encoder.UpdateParameterInfo(par.ParameterInfo())
				err := par.encoder.Write(ms)
				if err != nil {
					return err
				}
				continue
			}
			if (par.Direction == Input || par.Direction == InOut) && par.isLobType() {
				if par.DataType == types.VECTOR {
				} else {
					if len(par.BValue) > 0 {
						ms.PutUint(len(par.BValue), 4, true, true)
					}
					ms.PutClr(par.BValue)
				}
			}
		}
	}
	for _, par := range stmt.Pars {
		if par.isLongType() {
			ms.PutClr(par.BValue)
		}
	}
	buffer := ms.WriteBuffer()
	if len(buffer) > 0 {
		session.PutBytes(7)
		session.PutBytes(buffer...)
	}
	return nil
}

// write stmt data to network stream
func (stmt *Stmt) write() error {
	session := stmt.connection.session
	if !stmt.parse && !stmt.reSendParDef {
		exeOf := 0
		execFlag := 0
		count := 1
		if stmt.arrayBindCount > 0 {
			count = stmt.arrayBindCount
		}
		if stmt.stmtType == SELECT {
			session.PutTTCFunc(0x3, 0x4E)
			count = stmt._noOfRowsToFetch
			exeOf = 0x20
			if stmt._hasReturnClause || stmt.stmtType == PLSQL /*|| stmt.disableCompression*/ {
				exeOf |= 0x40000
			}
		} else {
			session.PutTTCFunc(3, 4)
		}
		if stmt.connection.autoCommit {
			execFlag = 1
		}

		session.PutUint(stmt.cursorID, 2, true, true)
		session.PutUint(count, 2, true, true)
		session.PutUint(exeOf, 2, true, true)
		session.PutUint(execFlag, 2, true, true)
		var err error
		if stmt.bulkExec {
			// take copy of parameter values
			arrayValues := make([][][]byte, len(stmt.Pars))
			for x := 0; x < len(stmt.Pars); x++ {
				if stmt.Pars[x].Flag == 0x80 {
					continue
				}
				if tempVal, ok := stmt.Pars[x].iPrimValue.([][]byte); ok {
					arrayValues[x] = tempVal
				} else {
					return errors.New("error in encoding bulk exec array parameter")
				}
			}
			for valueIndex := 0; valueIndex < stmt.arrayBindCount; valueIndex++ {
				for parIndex, arrayValue := range arrayValues {
					if stmt.Pars[parIndex].Flag == 0x80 {
						continue
					}
					stmt.Pars[parIndex].BValue = arrayValue[valueIndex]
				}
				err = stmt.writePars()
				if err != nil {
					return err
				}
			}

		} else {
			err = stmt.writePars()
			if err != nil {
				return err
			}
		}
	} else {
		err := stmt.basicWrite(stmt.getExeOption(), stmt.parse, stmt.define)
		if err != nil {
			return err
		}
		if stmt.bulkExec {

			arrayValues := make([][][]byte, len(stmt.Pars))
			for x := 0; x < len(stmt.Pars); x++ {
				if stmt.Pars[x].Flag == 0x80 {
					continue
				}
				if tempVal, ok := stmt.Pars[x].iPrimValue.([][]byte); ok {
					arrayValues[x] = tempVal
				} else {
					return errors.New("incorrect array type")
				}
			}
			for valueIndex := 0; valueIndex < stmt.arrayBindCount; valueIndex++ {
				for parIndex, arrayValue := range arrayValues {
					if stmt.Pars[parIndex].Flag == 0x80 {
						continue
					}
					stmt.Pars[parIndex].BValue = arrayValue[valueIndex]
				}
				err = stmt.writePars()
				if err != nil {
					return err
				}
			}
		} else {
			err = stmt.writePars()
			if err != nil {
				return err
			}
		}
	}
	return session.Write()
}

// getExeOption return an integer that act like a flag carry bit value set according
// to stmt properties
func (stmt *Stmt) getExeOption() int {
	op := 0
	if stmt.stmtType == PLSQL || stmt._hasReturnClause {
		op |= 0x40000
	}
	if stmt.connection.autoCommit && (stmt.stmtType == DML || stmt.stmtType == PLSQL) {
		op |= 0x100
	}
	// a batch of DML that runs past the rows that fail (D34)
	if stmt.batchErrors && stmt.stmtType == DML && stmt.arrayBindCount > 0 {
		op |= 0x80000
	}
	if stmt.parse {
		op |= 1
	}
	if stmt.execute {
		op |= 0x20
	}
	if !stmt.parse && !stmt.execute {
		op |= 0x40
	}
	if len(stmt.Pars) > 0 && !stmt.define {
		op |= 0x8
		// the binds of PL/SQL and of DML RETURNING, as python-oracledb
		// sends them. go-ora left it out for RETURNING when it sent the
		// binds of a statement again, and the server refused the
		// execute with ORA-03146, sijms/go-ora#697
		if stmt.stmtType == PLSQL || stmt._hasReturnClause {
			op |= 0x400
		}
	}
	if stmt.stmtType != PLSQL && !stmt._hasReturnClause {
		op |= 0x8000
	}
	if stmt.define {
		op |= 0x10
	}
	return op

	/* HasReturnClause
	if  stmt.PLSQL or cmdText == "" return false
	Regex.IsMatch(cmdText, "\\bRETURNING\\b"
	*/
}

// fetch get more rows from network stream
func (stmt *defaultStmt) fetch(resultSet *ResultSet) error {
	if stmt.lobSet {
		// another statement can have run on the connection since the
		// last fetch of this one, with another mode
		stmt.connection.connOption.Lob = stmt.lob
	}
	stmt.sizeFetch()
	logger := stmt.connection.logger
	start := len(resultSet.rows)
	err := stmt._fetch(resultSet)
	if err == nil {
		stmt.measureLobs(resultSet.rows[start:])
	}
	if errors.Is(err, tns.ErrConnReset) {
		err = stmt.connection.read()
		session := stmt.connection.session
		if session.Summary != nil {
			stmt.cursorID = session.Summary.CursorID
		}
	}
	if err != nil {
		if isBadConn(err) {
			stmt.connection.setBad()
			logger.Info("bad connection", "error", err)
			return driver.ErrBadConn
		}
		return err
	}
	return nil
}

func (stmt *defaultStmt) _fetch(resultSet *ResultSet) error {
	session := stmt.connection.session
	session.ResetBuffer()
	session.PutTTCFunc(3, 5)
	session.PutInt(stmt.cursorID, 2, true, true)
	session.PutInt(stmt._noOfRowsToFetch, 2, true, true)
	err := session.Write()
	if err != nil {
		return err
	}
	return stmt.read(resultSet)
}

// fetchBudget is the bytes of the rows of one fetch, about, when sizeFetch
// sizes it. It counts a LOB as its locator, and a LONG as 2 bytes.
const fetchBudget = 0x20000

// lobFetchBudget is the bytes of the LOB and LONG values of one fetch,
// about, when sizeFetch sizes a fetch that brings them.
const lobFetchBudget = 4 << 20

// minLobRowBytes is the least size that sizeFetch takes for the LOB and LONG
// values of a row, whatever the last fetch measured.
const minLobRowBytes = 4 << 10

// sizeFetch sets the rows that the next fetch asks for. It uses a prefetch
// that the caller set as it is. For the default prefetch, it asks for the
// rows that fill fetchBudget. When the fetch brings the values of LOB or
// LONG columns inside it, it asks for one row first, and then for the rows
// that fill lobFetchBudget with values the size of the last ones. A LOB
// fetched inline is a LONG. go-ora counted it as 2 bytes, and so asked for
// up to 65,537 rows and held every value at once, sijms/go-ora#664. A LOB
// fetched as a locator is 86 bytes, because decodeRow loads its value one
// row at a time.
func (stmt *defaultStmt) sizeFetch() {
	if !stmt.autoFetch {
		return
	}
	inline := stmt.connection.connOption.Lob == config.INLINE
	rowSize, large := 0, false
	for _, col := range stmt.columns {
		switch {
		case col.isLobType():
			rowSize += 86
			large = large || inline
		case col.isLongType():
			rowSize += 2
			large = true
		default:
			rowSize += int(col.MaxLen)
		}
	}
	rows := config.DefaultPrefetch
	if rowSize > 0 {
		rows = fetchBudget/rowSize + 1
	}
	switch {
	case large && !stmt.lobMeasured:
		rows = 1
	case large:
		// a fetch of NULLs and short values does not let the next one ask
		// for every row
		rows = max(1, min(rows, lobFetchBudget/max(stmt.lobRowBytes, minLobRowBytes)))
	}
	stmt._noOfRowsToFetch = rows
	stmt.connection.logger.Debug("fetch size", "rows", rows)
}

// measureLobs records the size of the LOB and LONG values of a row of rows,
// which a fetch brought.
func (stmt *defaultStmt) measureLobs(rows []Row) {
	if len(rows) == 0 {
		return
	}
	n := 0
	for _, row := range rows {
		for i, col := range stmt.columns {
			if !(col.isLobType() || col.isLongType()) || i >= len(row) {
				continue
			}
			switch v := row[i].(type) {
			case string:
				n += len(v)
			case []byte:
				n += len(v)
			}
		}
	}
	stmt.lobRowBytes, stmt.lobMeasured = n/len(rows), true
}

func (stmt *defaultStmt) queryLobPrefetch(exeOp int, resultSet *ResultSet) error {
	stmt.sizeFetch()
	stmt.connection.session.ResetBuffer()
	err := stmt.basicWrite(exeOp, false, true)
	if err != nil {
		return err
	}
	err = stmt.connection.session.Write()
	if err != nil {
		return err
	}
	return stmt.read(resultSet)
}

// adjustFetchType gives col, a column of a cursor that the server
// described again, the type in which the server sends it. A cursor keeps the
// type in which it fetched a column, and the server converts a new type to
// it. A CLOB that was a string or a LONG comes as a LONG, in the charset of
// the old column, and a BLOB that was a RAW comes as a LONG RAW, as
// python-oracledb reads them. A string or a RAW that was an inline LOB comes
// as a LONG or a LONG RAW, which the define of the LOB asked for. A type the server cannot convert fails with
// ORA-00932, and the query parses again.
func adjustFetchType(col, previous *ParameterInfo) {
	switch {
	case col.DataType == types.OCIClobLocator && slices.Contains([]uint16{types.CHAR, types.NCHAR, types.LONG, types.LongVarChar}, previous.DataType):
		col.DataType = types.LONG
		col.CharsetForm, col.CharsetID = previous.CharsetForm, previous.CharsetID
	case col.DataType == types.OCIBlobLocator && slices.Contains([]uint16{types.RAW, types.LongRaw, types.LongVarRaw}, previous.DataType):
		col.DataType = types.LongRaw
	// the define of an inline LOB, which fetched it as a LONG, holds for a
	// string that the column becomes, in the charset of the string
	case slices.Contains([]uint16{types.CHAR, types.NCHAR}, col.DataType) && previous.DataType == types.LongVarChar:
		col.DataType = types.LONG
	case col.DataType == types.RAW && previous.DataType == types.LongRaw:
		col.DataType = types.LongRaw
	}
}

// read this is common read for stmt it read much information related to
// columns, dataset information, output parameter information, rows values
// and at the end summary object about this operation
func (stmt *defaultStmt) read(resultSet *ResultSet) (err error) {
	loop := true
	after7 := false
	resultSet.parent = stmt
	resultSet.cols = &stmt.columns
	session := stmt.connection.session
	defer func() {
		if session.Summary != nil {
			stmt.cursorID = session.Summary.CursorID
			if session.Summary.RetCode == 1403 {
				stmt._hasMoreRows = false
			}
		}
	}()
	for loop {
		msg, err := session.ReadByte()
		if err != nil {
			return err
		}
		switch msg {
		case 6:
			err = resultSet.load(session)
			if err != nil {
				return err
			}
			if !after7 {
				if stmt.stmtType == SELECT {
				}
			}
		case 7:
			after7 = true
			if stmt._hasReturnClause && stmt.containOutputPars {
				for x := 0; x < len(stmt.Pars); x++ {
					if stmt.Pars[x].Direction == Output {
						num, err := session.ReadInt(4, true, true)
						if err != nil {
							return err
						}
						// an OUT of a slice takes a value for each row (D35)
						if stmt.Pars[x].returnRows {
							values := make([]any, 0, num)
							for range num {
								if err = stmt.calculateParameterValue(&stmt.Pars[x]); err != nil {
									return err
								}
								values = append(values, stmt.Pars[x].oPrimValue)
							}
							stmt.Pars[x].oPrimValue = values
							continue
						}
						if num > 1 {
							return errors.New("a RETURNING of more than one row needs an OUT of a slice (D35)")
						}
						if num == 0 {
							// no row, so the value is NULL. go-ora cleared the
							// destination, and the copy after it failed
							stmt.Pars[x].BValue = nil
							stmt.Pars[x].oPrimValue = nil
						} else {
							err = stmt.calculateParameterValue(&stmt.Pars[x])
							if err != nil {
								return err
							}
						}
					}
				}
			} else {
				if stmt.containOutputPars {
					for x := 0; x < len(stmt.Pars); x++ {
						if stmt.Pars[x].DataType == types.REFCURSOR {
							typ := reflect.TypeOf(stmt.Pars[x].Value)
							if typ.Kind() == reflect.Pointer {
								if cursor, ok := stmt.Pars[x].Value.(*RefCursor); ok {
									cursor.connection = stmt.connection
									cursor.parent = stmt
									cursor.autoClose = true
									err = cursor.load()
									if err != nil {
										return err
									}
									if stmt.stmtType == PLSQL {
										_, err = session.ReadInt(2, true, true)
										if err != nil {
											return err
										}
									}
								} else {
									return errors.New("RefCursor parameter should contain pointer to  RefCursor struct")
								}
							} else {
								return errors.New("RefCursor parameter should contain pointer to  RefCursor struct")
							}
						} else {
							if stmt.Pars[x].Direction != Input {
								err = stmt.calculateParameterValue(&stmt.Pars[x])
								if err != nil {
									return err
								}
							} else {
							}
						}
					}
				} else {
					newRow := make(Row, resultSet.columnCount)
					for index, col := range stmt.columns {
						if col.getDataFromServer {
							err = stmt.calculateColumnValue(&col, false)
							if err != nil {
								return err
							}
							if col.isLongType() {
								_, err = session.ReadInt(4, true, true)
								if err != nil {
									return err
								}
								_, err = session.ReadInt(4, true, true)
								if err != nil {
									return err
								}
							}
							stmt.columns[index] = col
						}
						newRow[index] = col.oPrimValue
					}
					resultSet.rows = append(resultSet.rows, newRow)
				}
			}
		case 8:
			size, err := session.ReadInt(2, true, true)
			if err != nil {
				return err
			}
			for x := range 2 {
				stmt.scnForSnapshot[x], err = session.ReadInt(4, true, true)
				if err != nil {
					return err
				}
			}
			for x := 2; x < size; x++ {
				_, err = session.ReadInt(4, true, true)
				if err != nil {
					return err
				}
			}
			_, err = session.ReadInt(2, true, true)
			if err != nil {
				return err
			}
			size, err = session.ReadInt(2, true, true)
			for x := 0; x < size; x++ {
				_, val, num, err := session.ReadKeyVal()
				if err != nil {
					return err
				}
				if num == 163 {
					session.TimeZone = val
				}
			}
			if session.TTCVersion >= 4 {
				// get queryID
				size, err = session.ReadInt(4, true, true)
				if err != nil {
					return err
				}
				if size > 0 {
					bty, err := session.ReadBytes(size)
					if err != nil {
						return err
					}
					if len(bty) >= 8 {
						stmt.queryID = binary.LittleEndian.Uint64(bty[size-8:])
					}
				}
			}
			if session.TTCVersion >= 7 && stmt.stmtType == DML && stmt.arrayBindCount > 0 {
				length, err := session.ReadInt(4, true, true)
				if err != nil {
					return err
				}
				// for (int index = 0; index < length3; ++index)
				//	rowsAffectedByArrayBind[index] = this.m_marshallingEngine.UnmarshalSB8();
				stmt.counts = make([]int64, 0, length)
				for range length {
					n, err := session.ReadInt64(8, true, true)
					if err != nil {
						return err
					}
					stmt.counts = append(stmt.counts, n)
				}
			}
		case 11:
			err = resultSet.load(session)
			if err != nil {
				return err
			}
			places := stmt.ioVectorPlaces(resultSet.columnCount)
			for _, x := range places {
				direction, err := session.ReadByte()
				if err != nil {
					return err
				}
				switch direction {
				case 32:
					stmt.Pars[x].Direction = Input
				case 16:
					stmt.Pars[x].Direction = Output
					stmt.containOutputPars = true
				case 48:
					stmt.Pars[x].Direction = InOut
					stmt.containOutputPars = true
				}
			}
		case 16:
			size, err := session.ReadByte()
			if err != nil {
				return err
			}
			_, err = session.ReadBytes(int(size))
			if err != nil {
				return err
			}
			resultSet.maxRowSize, err = session.ReadInt(4, true, true)
			if err != nil {
				return err
			}
			resultSet.columnCount, err = session.ReadInt(4, true, true)
			if err != nil {
				return err
			}
			if resultSet.columnCount > 0 {
				_, err = session.ReadByte() // session.ReadInt(1, false, false)
			}
			// a describe comes with a parse, and with an execute after which
			// the server parsed again, as when a view changed the type of a
			// column. The flags hold what the new columns hold
			previous := stmt.columns
			stmt.columns = make([]ParameterInfo, resultSet.columnCount)
			stmt._hasLONG, stmt._hasBLOB = false, false
			for x := 0; x < resultSet.columnCount; x++ {
				err = stmt.columns[x].load(stmt.connection)
				if err != nil {
					return err
				}
				if x < len(previous) {
					adjustFetchType(&stmt.columns[x], &previous[x])
				}
				if stmt.columns[x].isLongType() {
					stmt._hasLONG = true
				}
				if stmt.columns[x].isLobType() {
					stmt._hasBLOB = true
				}
			}
			_, err = session.ReadDLC()
			if session.TTCVersion >= 3 {
				_, err = session.ReadInt(4, true, true)
				_, err = session.ReadInt(4, true, true)
			}
			if session.TTCVersion >= 4 {
				_, err = session.ReadInt(4, true, true)
				_, err = session.ReadInt(4, true, true)
			}
			if session.TTCVersion >= 5 {
				_, err = session.ReadDLC()
			}
		case 19:
			session.ResetBuffer()
			session.PutBytes(19)
			err = session.Write()
			if err != nil {
				return err
			}
			continue
		case 21:
			_, err := session.ReadInt(2, true, true) // noOfColumnSent
			if err != nil {
				return err
			}
			bitVectorLen := resultSet.columnCount / 8
			if resultSet.columnCount%8 > 0 {
				bitVectorLen++
			}
			bitVector := make([]byte, bitVectorLen)
			for x := 0; x < bitVectorLen; x++ {
				bitVector[x], err = session.ReadByte()
				if err != nil {
					return err
				}
			}
			resultSet.setBitVector(bitVector)
		case 27:
			count, err := session.ReadInt(4, true, true)
			if err != nil {
				return err
			}
			stmt.multiSet = make([]RefCursor, 0, count)
			for range count {
				// this function is equal to load cursor so each item is a cursor
				cursor := RefCursor{
					connection: stmt.connection,
					parent:     stmt,
					autoClose:  true}
				err = cursor.load()
				if err != nil {
					return err
				}
				stmt.multiSet = append(stmt.multiSet, cursor)
			}
		default:
			err = stmt.connection.ProcessTCCResponse(msg)
			if err != nil {
				return err
			}
			if msg == 4 || msg == 9 {
				loop = false
			}
		}
	}
	return nil
}

func (stmt *defaultStmt) calculateColumnValue(col *ParameterInfo, udt bool) error {
	session := stmt.connection.session
	if col.DataType == types.REFCURSOR {
		cursor := new(RefCursor)
		cursor.connection = stmt.connection
		cursor.parent = stmt
		cursor.autoClose = true
		err := cursor.load()
		if err != nil {
			return err
		}
		if stmt.stmtType == PLSQL {
			_, err = session.ReadInt(2, true, true)
			if err != nil {
				return err
			}
		}
		col.oPrimValue = cursor
		return nil
	}

	return col.decodeColumnValue(stmt.connection, udt)
}

// get values of rows and output parameter according to DataType and binary value (bValue)
func (stmt *defaultStmt) calculateParameterValue(param *ParameterInfo) error {
	if param.isLobType() {
		stmt._hasBLOB = true
	}
	err := param.decodeParameterValue(stmt.connection)
	if err != nil {
		return err
	}
	if param.DataType == types.XMLType && param.IsNull {
		return nil
	}
	if param.DataType != types.XMLType && param.ArraySize > 0 {
		return nil
	}
	// the length of the value before the server cut it to the size of the
	// OUT, or 0. go-ora read it and returned the cut value, as
	// python-oracledb does not
	length, err := stmt.connection.session.ReadInt(4, true, true)
	if err != nil {
		return err
	}
	// a NULL BOOLEAN of PL/SQL before 23ai is a byte of 0 with the length
	// -1, which python-oracledb reads as NULL. go-ora read it as false
	if length < 0 && param.DataType == types.BOOLEAN {
		param.BValue, param.oPrimValue = nil, nil
		return nil
	}
	// only a string or a RAW is cut. After an object, 11g sends the length
	// of its image there
	if length > 0 && param.oPrimValue != nil && slices.Contains([]uint16{types.NCHAR, types.CHAR, types.RAW}, param.DataType) {
		return fmt.Errorf("reading an OUT value: the server cut a value of length %d to the %d bytes of the OUT: %w", length, param.MaxLen, tns.ErrInvalidValue)
	}
	return nil
}

// Close stmt cursor in the server
func (stmt *defaultStmt) Close() error {
	if !stmt.connection.IsValid() {
		stmt.connection.setBad()
		return driver.ErrBadConn
	}
	var err error
	if stmt.cursorID != 0 {
		session := stmt.connection.session
		session.ResetBuffer()
		session.PutTTCFunc(0x11, 0x69)
		session.PutBytes(1, 1, 1)
		session.PutInt(stmt.cursorID, 4, true, true)
		return (&simpleObject{
			connection:  stmt.connection,
			operationID: 0x93,
			data:        nil,
			err:         nil,
		}).exec()
	}
	return err
}

func (stmt *Stmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if !stmt.connection.IsValid() {
		stmt.connection.setBad()
		return nil, driver.ErrBadConn
	}
	logger := stmt.connection.logger

	logger.Debug("exec", "query", stmt.text)
	stmt.arrayBindCount = 0
	opts, args, err := resolve(ctx, stmt.connection, args)
	if err != nil {
		return nil, err
	}
	if (opts.BatchErrors || opts.RowCounts != nil) && stmt.stmtType != DML {
		return nil, fmt.Errorf("applying the options WithBatchErrors and WithRowCounts: the statement is not DML: %w", tns.ErrNotSupported)
	}
	// the server sends the count of each row from TTC 7, 12.1
	if opts.RowCounts != nil && stmt.connection.session.TTCVersion < 7 {
		return nil, fmt.Errorf("applying the option WithRowCounts: the server sends no count of each row before 12.1: %w", tns.ErrNotSupported)
	}
	ctx, cancel := stmt.apply(ctx, opts)
	defer cancel()
	defer stmt.connection.restoreLobFetch()

	done := stmt.connection.session.StartContext(ctx)
	defer stmt.connection.session.EndContext(done)

	// the call runs here, and the watch of StartContext breaks it when ctx
	// ends, so no goroutine waits for it (D10)
	stmt.counts = nil
	result, err := stmt._exec(args)
	if stmt.rowCounts != nil {
		*stmt.rowCounts = stmt.rowCountsOf(result)
	}
	if errors.Is(err, tns.ErrConnReset) {
		// RETURNING and a batch of DML read the reply that follows the reset
		// as a statement, so that the counts of the rows of a batch that
		// failed reach WithRowCounts (D34)
		if stmt._hasReturnClause || stmt.stmtType == DML && stmt.arrayBindCount > 0 {
			dataSet := &DataSet{}
			err = stmt.read(dataSet.currentResultSet())
			if !errors.Is(err, tns.ErrConnReset) {
				if stmt.rowCounts != nil {
					*stmt.rowCounts = stmt.rowCountsOf(nil)
				}
				return nil, stmt.execError(ctx, err)
			}
		}
		err = stmt.connection.read()
		session := stmt.connection.session
		if session.Summary != nil {
			stmt.cursorID = session.Summary.CursorID
		}
	}
	if err != nil {
		return nil, stmt.execError(ctx, err)
	}
	return result, nil
}

func (stmt *Stmt) fillStructPar(parValue driver.Value) error {
	structType := reflect.TypeOf(parValue)
	structVal := reflect.ValueOf(parValue)
	if parValue != nil && structType.Kind() == reflect.Pointer && structVal.Elem().Kind() == reflect.Struct {
		structType = structType.Elem()
		structVal = structVal.Elem()
		structFieldCount := structType.NumField()
		for i := range structFieldCount {
			name, _, _, dir := extractTag(structType.Field(i).Tag.Get("db"))
			var err error
			if len(name) > 0 && ParameterDirection(dir) != Input {
				for _, par := range stmt.Pars {
					if par.Name == name {
						err = copyValue(structVal.Field(i).Addr().Interface(), par.oPrimValue)
						if err != nil {
							return err
						}

						break
					}
				}
			}
		}
	}
	return nil
}

func (stmt *Stmt) structPar(parValue driver.Value, parIndex int) (processedPars int, err error) {
	tempType := reflect.TypeOf(parValue)
	structValue := reflect.ValueOf(parValue)
	addOutputField := func(name, _type string, size int, dir ParameterDirection, fieldIndex int) (tempPar *ParameterInfo, err error) {
		field := structValue.Field(fieldIndex)
		fieldValue := field.Interface()
		fieldType := field.Type()
		hasNullValue := false
		if fieldType.Kind() == reflect.Pointer {
			if structValue.Field(fieldIndex).IsNil() {
				hasNullValue = true
				fieldType = fieldType.Elem()
			}
		}

		// if type mentioned so driver should create a temporary type and then update the current value
		typeErr := fmt.Errorf("error passing filed %s as type %s", tempType.Field(fieldIndex).Name, _type)
		switch _type {
		case "number":
			var fieldVal *types.Number
			if hasNullValue {
				fieldVal = &types.Number{}
			} else {
				fieldVal, err = types.NewNumber(fieldValue)
				if err != nil {
					err = typeErr
					return
				}
			}
			tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		case "varchar":
			fieldVal := &sql.Null[string]{}
			if !hasNullValue {
				fieldVal.V, fieldVal.Valid = getString(fieldValue), true
			}
			tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		case "nvarchar":
			fieldVal := &types.String{}
			fieldVal.UseNCharset = true
			err = fieldVal.SetValue(fieldValue)
			if err != nil {
				return
			}
			tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		case "date":
			fieldVal := &sql.Null[time.Time]{}
			if !hasNullValue {
				fieldVal.V, err = getDate(fieldValue)
				if err != nil {
					err = typeErr
					return
				}
				fieldVal.Valid = true
			}
			tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		case "timestamptz":
			fieldVal := &sql.Null[types.TimestampTZ]{}
			if !hasNullValue {
				var tempDate time.Time
				tempDate, err = getDate(fieldValue)
				if err != nil {
					err = typeErr
					return
				}
				fieldVal.V = types.TimestampTZ(tempDate)
				fieldVal.Valid = true
			}
			tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		case "raw":
			var fieldVal []byte
			if !hasNullValue {
				fieldVal, err = getBytes(fieldValue)
				if err != nil {
					err = typeErr
					return
				}
			}
			tempPar, err = stmt.NewParam(name, &fieldVal, size, dir)
		case "clob":
			fieldVal := &types.Clob{}
			err = fieldVal.SetValue(fieldValue)
			if err != nil {
				return
			}
			tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		case "nclob":
			fieldVal := &types.Clob{}
			fieldVal.UseNCharset = true
			err = fieldVal.SetValue(fieldValue)
			if err != nil {
				return
			}
			tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		case "blob":
			fieldVal := &types.Blob{}
			err = fieldVal.SetValue(fieldValue)
			tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		case "":
			if field.Kind() == reflect.Pointer {
				if field.IsNil() {
					field.Set(reflect.New(fieldType))
				}
				tempPar, err = stmt.NewParam(name, field.Interface(), size, dir)
			} else {
				if field.CanAddr() {
					tempPar, err = stmt.NewParam(name, field.Addr().Interface(), size, dir)
				} else {
					err = fmt.Errorf("can't take address for field: %s", name)
				}
			}
		default:
			err = fmt.Errorf("unknown type: %s for parameter: %s", _type, name)
		}
		return
	}
	if parValue != nil && tempType.Kind() == reflect.Struct {
		structFieldCount := tempType.NumField()

		for i := range structFieldCount {
			name, _type, _, _ := extractTag(tempType.Field(i).Tag.Get("db"))
			if name != "" {
				var tempPar *ParameterInfo
				tempPar, err = parseInputField(structValue, name, _type, i)
				if err != nil {
					return
				}
				err = tempPar.encodeValue(0, stmt)
				if err != nil {
					return
				}
				stmt.setParam(parIndex, *tempPar)
				processedPars++
				parIndex++
			}
		}
	}

	// deal with Ptr struct types
	if parValue != nil && tempType.Kind() == reflect.Pointer && structValue.Elem().Kind() == reflect.Struct {
		tempType = tempType.Elem()
		structValue = structValue.Elem()
		structFieldCount := tempType.NumField()
		for i := range structFieldCount {
			name, _type, size, dir := extractTag(tempType.Field(i).Tag.Get("db"))
			if dir == 0 {
				dir = 1
			}
			if name != "" {
				var tempPar *ParameterInfo
				if ParameterDirection(dir) == Input {
					tempPar, err = parseInputField(structValue, name, _type, i)
					if err != nil {
						return
					}
					err = tempPar.encodeValue(0, stmt)
				} else {
					tempPar, err = addOutputField(name, _type, size, ParameterDirection(dir), i)
				}
				if err != nil {
					return
				}
				stmt.setParam(parIndex, *tempPar)
				processedPars++
				parIndex++
			}
		}
	}
	return
}

func (stmt *Stmt) _exec(args []driver.NamedValue) (*QueryResult, error) {
	var err error
	useNamedPars := len(args) > 0
	parIndex := 0
	structPars := make([]driver.Value, 0, 2)
	for x := range args {
		var par *ParameterInfo
		switch tempOut := args[x].Value.(type) {
		case sql.Out:
			stmt.bulkExec = false
			direction := Output
			if tempOut.In {
				direction = InOut
			}
			par, err = stmt.NewParam(args[x].Name, tempOut.Dest, 0, direction)
			if err != nil {
				return nil, err
			}
		case *sql.Out:
			stmt.bulkExec = false
			direction := Output
			if tempOut.In {
				direction = InOut
			}
			par, err = stmt.NewParam(args[x].Name, tempOut.Dest, 0, direction)
			if err != nil {
				return nil, err
			}
		case Out:
			stmt.bulkExec = false
			direction := Output
			if tempOut.In {
				direction = InOut
			}
			par, err = stmt.NewParam(args[x].Name, tempOut.Dest, tempOut.Size, direction)
			if err != nil {
				return nil, err
			}
		case *Out:
			stmt.bulkExec = false
			direction := Output
			if tempOut.In {
				direction = InOut
			}
			par, err = stmt.NewParam(args[x].Name, tempOut.Dest, tempOut.Size, direction)
			if err != nil {
				return nil, err
			}
		case *batch:
			stmt.bulkExec = true
			args[x].Value = tempOut.array
		default:
			processedPars := 0
			processedPars, err = stmt.structPar(args[x].Value, parIndex)
			if err != nil {
				return nil, err
			}
			if processedPars > 0 {
				stmt.bulkExec = false
				stmt.connection.logger.Debug("bind", "index", x, "value", args[x])
				parIndex += processedPars
				structPars = append(structPars, args[x].Value)
				continue
			}
		}
		if stmt.bulkExec {
			tempType := reflect.TypeOf(args[x].Value)
			tempVal := reflect.ValueOf(args[x].Value)
			if args[x].Value != nil && tempType != reflect.TypeFor[[]byte]() && (tempType.Kind() == reflect.Array || tempType.Kind() == reflect.Slice) {
				// every array of a batch holds one value for each row. An
				// empty array panicked, and a shorter one cut the others
				// short, which was sijms/go-ora#695
				switch n := tempVal.Len(); {
				case n == 0:
					return nil, fmt.Errorf("batch parameter %d is empty", x+1)
				case stmt.arrayBindCount == 0:
					stmt.arrayBindCount = n
				case n != stmt.arrayBindCount:
					return nil, fmt.Errorf("batch parameter %d has %d rows, where the first has %d", x+1, n, stmt.arrayBindCount)
				}
				// see if first item is struct
				firstItem := tempVal.Index(0)
				if firstItem.Kind() == reflect.Struct {
					fieldCount := firstItem.NumField()
					structArrayAsNamedPars := make([]driver.NamedValue, 0, fieldCount)
					for fieldIndex := range fieldCount {
						name, _type, _, _ := extractTag(firstItem.Type().Field(fieldIndex).Tag.Get("db"))
						if name != "" {
							arrayValues := make([]driver.Value, stmt.arrayBindCount)
							for arrayIndex := 0; arrayIndex < stmt.arrayBindCount; arrayIndex++ {
								var tempPar *ParameterInfo
								tempPar, err = parseInputField(tempVal.Index(arrayIndex), name, _type, fieldIndex)
								if err != nil {
									return nil, err
								}
								arrayValues[arrayIndex] = tempPar.Value
							}
							structArrayAsNamedPars = append(structArrayAsNamedPars, driver.NamedValue{Name: name, Value: arrayValues})
						}
					}
					if len(structArrayAsNamedPars) > 0 {
						return stmt._exec(structArrayAsNamedPars)
					}
				}
				par = &ParameterInfo{
					Name:      args[x].Name,
					Direction: Input,
				}
				maxLen := int64(0)
				maxCharLen := int64(0)
				dataType := uint16(0)
				seen := map[uint16]bool{}
				nullType := uint16(0)
				var encoder codec.OracleParameterEncoder
				arrayValues := make([][]byte, stmt.arrayBindCount)
				for y := 0; y < stmt.arrayBindCount; y++ {
					par.Value = tempVal.Index(y).Interface()
					// each row takes the type of its own value. go-ora kept
					// the type of the row before, and a decimal went as
					// text labeled NUMBER, which the server stored as NULL
					par.DataType = 0
					err = par.encodeValue(0, stmt)
					if err != nil {
						return nil, err
					}
					if maxLen < par.MaxLen {
						maxLen = par.MaxLen
					}
					if maxCharLen < par.MaxCharLen {
						maxCharLen = par.MaxCharLen
					}
					// here I can take the binary value and store it into array
					arrayValues[y] = par.BValue
					// a NULL takes the type of the other rows, and a batch of
					// NULLs alone the type of its NULLs
					if len(par.BValue) == 0 {
						nullType = par.DataType
						continue
					}
					dataType = par.DataType
					seen[dataType] = true
					encoder = par.encoder
				}
				// every row is written with one coder, which must be that of
				// a value. go-ora kept the coder of the last row, and when
				// it was NULL, a VECTOR or a JSON went as a NULL VARCHAR2,
				// and the server waited for the rest of it
				if encoder != nil {
					par.encoder = encoder
				}
				// a batch binds one type for each argument
				switch {
				case len(seen) <= 1:
				case onlyTypes(seen, types.NCHAR, types.LONG, types.LongVarChar):
					// short and long strings go as LONG
					dataType = types.LongVarChar
				case onlyTypes(seen, types.RAW, types.LongRaw):
					dataType = types.LongRaw
				case onlyTypes(seen, types.DATE, types.TIMESTAMP):
					// a time with a fraction is a TIMESTAMP, and one with
					// none a DATE, which a TIMESTAMP holds
					dataType = types.TIMESTAMP
				default:
					return nil, fmt.Errorf("binding the batch argument %d: its values have different types: %w", x+1, tns.ErrInvalidValue)
				}
				// save arrayValues into primitive
				par.iPrimValue = arrayValues
				par.MaxLen = maxLen
				par.MaxCharLen = maxCharLen
				if dataType == 0 {
					dataType = cmp.Or(nullType, types.NCHAR)
				}
				par.DataType = dataType
			} else {
				if stmt.arrayBindCount > 0 {
					return nil, errors.New("to activate bulk insert/merge all parameters should be arrays")
				}
				stmt.bulkExec = false
			}
		}
		if par == nil {
			par, err = stmt.NewParam(args[x].Name, args[x].Value, 0, Input)
			if err != nil {
				return nil, err
			}
		}
		if len(par.Name) == 0 && useNamedPars {
			useNamedPars = false
		}
		stmt.setParam(parIndex, *par)
		parIndex++
		stmt.connection.logger.Debug("bind", "index", x, "value", args[x])
	}
	if useNamedPars {
		err = stmt.useNamedParameters()
		if err != nil {
			return nil, err
		}
	}
	session := stmt.connection.session
	session.ResetBuffer()
	err = stmt.write()
	if err != nil {
		stmt.connection.setBad()
		return nil, err
	}
	dataSet := new(DataSet)
	err = stmt.read(dataSet.currentResultSet())
	if err != nil {
		return nil, err
	}
	// before release results decode parameters
	for _, par := range stmt.Pars {
		if par.Direction != Input && par.DataType != types.REFCURSOR {
			fieldValue := reflect.ValueOf(par.Value)
			if fieldValue.Kind() != reflect.Pointer {
				return nil, errors.New("output parameter should be pointer type")
			}
			fieldValue = fieldValue.Elem()
			if par.ArraySize > 0 {
			} else {
				// check if LobReadMode is auto so auto read all lobs
				auto := stmt.connection.connOption.LobReadMode == config.LobReadModeAuto
				par.oPrimValue, err = resolveObjects(par.oPrimValue, auto)
				if err != nil {
					return nil, err
				}
				if auto {
					err = loadLobs(par.oPrimValue)
					if err != nil {
						return nil, err
					}
				}
				// an OUT JSON holds OSON, and the caller reads its text,
				// which go-ora left empty
				if js, ok := par.oPrimValue.(*types.JSON); ok {
					if err := codec.DecodeOSON(js); err != nil {
						return nil, err
					}
				}
			}
			err = copyValue(par.Value, par.oPrimValue)
			if err != nil {
				return nil, err
			}
		}
	}
	result := new(QueryResult)
	if session.Summary != nil {
		result.rowsAffected = int64(session.Summary.CurRowNumber)
	}
	for _, par := range structPars {
		err = stmt.fillStructPar(par)
		if err != nil {
			return nil, err
		}
	}
	stmt.parse = false
	stmt.define = false
	stmt.reSendParDef = false
	return result, nil
}

// ioVectorPlaces returns the index of the bind of each direction of an IO
// vector, which holds count directions. It is each bind, in order, but for a
// CALL in which a name appears more than once. The server takes a bind for
// each place of the name in a CALL, and sends one direction for each name.
// 26ai sends the count of the names, and 11g the count of the binds. The
// direction goes to the first place of the name, and a later place is an
// IN, which reads no value back. go-ora gave the directions to the binds in
// order, so it read the OUT of the next name into the second place of the
// first, and failed on the rest of the response.
func (stmt *defaultStmt) ioVectorPlaces(count int) []int {
	places := make([]int, 0, len(stmt.Pars))
	var info sqlInfo
	if stmt.stmtType == PLSQL {
		info, _ = parseSQL(stmt.text)
	}
	if info.call && len(info.names) == len(stmt.Pars) {
		seen := make(map[string]bool, len(info.names))
		for x, name := range info.names {
			if seen[name] {
				stmt.Pars[x].Direction = Input
				continue
			}
			seen[name] = true
			places = append(places, x)
		}
		return places
	}
	for x := 0; x < count && x < len(stmt.Pars); x++ {
		places = append(places, x)
	}
	return places
}

// sameName reports whether the name of an argument is the name of a bind. A
// name in quotes keeps its case, and any other name matches in any case.
// go-ora matched a quoted name in any case, so :"test" took the argument TEST.
func sameName(arg, bind string, quoted bool) bool {
	if quoted {
		return arg == bind
	}
	return strings.EqualFold(arg, bind)
}

// useNamedParameters: re-arrange parameters according parameter defined in sql text
func (stmt *Stmt) useNamedParameters() error {
	info, err := parseSQL(stmt.text)
	if err != nil {
		return err
	}
	names := info.names
	parCollection := make([]ParameterInfo, 0, len(names))
	// SQL and CALL take a bind for each place a name appears, and PL/SQL
	// one for each name. go-ora bound CALL once for each name, and the
	// server failed with ORA-01008
	if info.kind != PLSQL || info.call {
		for x := range names {
			found := false
			for _, par := range stmt.Pars {
				if sameName(par.Name, names[x], info.quoted[x]) {
					parCollection = append(parCollection, par)
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("parameter %s is not defined in parameter list", names[x])
			}
			for y := x - 1; y >= 0 && !info.call; y-- {
				if names[y] == names[x] {
					parCollection[x].Flag = 0x80
					break
				}
			}
		}
	} else {
		for x := range names {
			// search if name is repeated
			repeated := false
			for y := x - 1; y >= 0; y-- {
				if names[y] == names[x] {
					repeated = true
					break
				}
			}
			found := false
			for _, par := range stmt.Pars {
				if sameName(par.Name, names[x], info.quoted[x]) {
					if !repeated {
						parCollection = append(parCollection, par)
					}
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("parameter %s is not defined in parameter list", names[x])
			}

		}
	}

	stmt.Pars = parCollection
	return nil
}

// Exec execute stmt (INSERT, UPDATE, DELETE, DML, PLSQL) and return driver.Result object
func (stmt *Stmt) Exec(args []driver.Value) (driver.Result, error) {
	if !stmt.connection.IsValid() {
		stmt.connection.setBad()
		return nil, driver.ErrBadConn
	}
	logger := stmt.connection.logger
	logger.Debug("exec", "query", stmt.text)
	var result *QueryResult
	var err error
	stmt.arrayBindCount = 0
	if len(args) == 0 {
		result, err = stmt._exec(nil)
	} else {
		namedArgs := make([]driver.NamedValue, len(args))
		for x := range args {
			namedArgs[x].Value = args[x]
		}
		result, err = stmt._exec(namedArgs)
	}
	if errors.Is(err, tns.ErrConnReset) {
		err = stmt.connection.read()
		session := stmt.connection.session
		if session.Summary != nil {
			stmt.cursorID = session.Summary.CursorID
		}
	}
	if err != nil {
		if isBadConn(err) {
			stmt.connection.setBad()
			logger.Info("bad connection", "error", err)
			return nil, driver.ErrBadConn
		}
		return nil, err
	}
	return result, err
}

func (stmt *Stmt) CheckNamedValue(nv *driver.NamedValue) error {
	// an option goes to the statement as it is (D109)
	if IsOption(nv.Value) {
		return nil
	}
	// database/sql converts a driver.Valuer, such as sql.Null[T], to its
	// value, except the few that the driver binds itself
	if _, ok := nv.Value.(driver.Valuer); ok && !ownValue(nv.Value) {
		return driver.ErrSkip
	}
	if namedBasic(nv.Value) {
		return driver.ErrSkip
	}
	return nil
}

func (stmt *Stmt) NewParam(name string, val driver.Value, size int, direction ParameterDirection) (*ParameterInfo, error) {
	if !stmt.connection.IsValid() {
		stmt.connection.setBad()
		return nil, driver.ErrBadConn
	}
	param := &ParameterInfo{
		Name:      name,
		Direction: direction,
		Value:     val,
	}
	// an OUT of a slice of a DML RETURNING binds as its element, and takes
	// one for each row that the statement changes (D35)
	elem, rows := stmt.returningRows(val, direction)
	if rows {
		param.Value, param.returnRows = elem, true
	}
	err := param.encodeValue(int64(size), stmt)
	if err != nil {
		return nil, err
	}
	if rows {
		param.Value = val
	}
	return param, err
}

// returningRows returns the zero value of the element of val, and true, when
// val is an OUT of a DML RETURNING that is a pointer to a slice: not a
// []byte, which is one RAW, and not a collection that RegisterType
// registered, which is one value.
func (stmt *Stmt) returningRows(val driver.Value, direction ParameterDirection) (any, bool) {
	if direction != Output || stmt.stmtType != DML || !stmt._hasReturnClause {
		return nil, false
	}
	t := reflect.TypeOf(val)
	if t == nil || t.Kind() != reflect.Pointer || t.Elem().Kind() != reflect.Slice || t.Elem().Elem().Kind() == reflect.Uint8 {
		return nil, false
	}
	if _, err := stmt.connection.ParameterCoder(t.Elem()); err == nil {
		return nil, false
	}
	return reflect.New(t.Elem().Elem()).Elem().Interface(), true
}

func (stmt *Stmt) setParam(pos int, par ParameterInfo) {
	if pos >= 0 && pos < len(stmt.Pars) {
		originalPar := stmt.Pars[pos]
		// the server holds the type of each bind, so one that changes goes
		// again with its definition
		if par.MaxLen > originalPar.MaxLen || par.DataType != originalPar.DataType {
			stmt.reSendParDef = true
		}
		// resend parameter definition when string/[]byte length change from long to short and vice versa
		switch par.DataType {
		case types.NCHAR:
			if originalPar.DataType == types.LONG || originalPar.DataType == types.LongVarChar {
				stmt.reSendParDef = true
			}
		case types.RAW:
			if originalPar.DataType == types.LongRaw {
				stmt.reSendParDef = true
			}
		case types.LONG:
			fallthrough
		case types.LongVarChar:
			if originalPar.DataType == types.NCHAR {
				stmt.reSendParDef = true
			}
		case types.LongVarRaw:
			if originalPar.DataType == types.RAW {
				stmt.reSendParDef = true
			}
		}
		stmt.Pars[pos] = par
	} else {
		stmt.Pars = append(stmt.Pars, par)
	}
}

// RunQuery execute a query command and return oracle dataset object
//
// args is an array of values that corresponding to parameters in sql
func (stmt *Stmt) RunQuery(namedArgs []driver.NamedValue) (*DataSet, error) {
	if !stmt.connection.IsValid() {
		stmt.connection.setBad()
		return nil, driver.ErrBadConn
	}
	logger := stmt.connection.logger
	stmt._noOfRowsToFetch = stmt.prefetch
	stmt.autoFetch = stmt.prefetch == config.DefaultPrefetch
	stmt.lobMeasured = false
	stmt._hasMoreRows = true
	useNamedPars := len(namedArgs) > 0
	for x := range namedArgs {
		par, err := stmt.NewParam(namedArgs[x].Name, namedArgs[x].Value, 0, Input)
		if err != nil {
			return nil, err
		}
		if len(par.Name) == 0 && useNamedPars {
			useNamedPars = false
		}
		stmt.setParam(x, *par)
		logger.Debug("bind", "index", x, "value", namedArgs[x])
	}

	if useNamedPars {
		err := stmt.useNamedParameters()
		if err != nil {
			return nil, err
		}
	}

	parsed := stmt.parse
	dataSet, err := stmt._query()
	if errors.Is(err, tns.ErrConnReset) {
		err = stmt.connection.read()
		session := stmt.connection.session
		if session.Summary != nil {
			stmt.cursorID = session.Summary.CursorID
		}
	}
	// the server refuses to run a cursor again when a column changed type
	// under it, as when a view changed. The cursor closes, and the query
	// parses again, once, as python-oracledb does
	var oerr *tns.OracleError
	if !parsed && stmt.stmtType == SELECT && errors.As(err, &oerr) && (oerr.ErrCode == 932 || oerr.ErrCode == 1007) {
		logger.Debug("parse the query again", "error", err)
		if err = stmt.Close(); err != nil {
			return nil, err
		}
		stmt.cursorID, stmt.columns = 0, nil
		stmt.parse, stmt.execute, stmt.define = true, true, false
		stmt._hasLONG, stmt._hasBLOB = false, false
		dataSet, err = stmt._query()
	}
	if err != nil {
		if isBadConn(err) {
			stmt.connection.setBad()
			logger.Info("bad connection", "error", err)
			return nil, driver.ErrBadConn
		}
		return nil, err
	}
	if dataSet == nil {
		return nil, fmt.Errorf("go-ora: query returned no result set (connection reset)")
	}
	return dataSet, nil
}

// rowCountsOf returns the count of each row of the last statement: those of
// a batch, as the server sent them, or that of the one row of the result.
func (stmt *Stmt) rowCountsOf(result *QueryResult) []int64 {
	if stmt.counts != nil {
		return slices.Clone(stmt.counts)
	}
	if result == nil {
		return nil
	}
	return []int64{result.rowsAffected}
}

// execError returns the error of a call that failed. When ctx ended, it is
// the error of ctx, because the server answered the break with ORA-01013.
// Otherwise it is the error of the server, when there is one, and it marks
// the connection bad when the error says so.
func (stmt *Stmt) execError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	session := stmt.connection.session
	if be := session.BatchError(); be != nil && stmt.batchErrors {
		return be
	}
	if session.HasError() {
		err = session.ServerError()
	}
	if isBadConn(err) {
		stmt.connection.setBad()
	}
	return err
}

func (stmt *Stmt) QueryContext(ctx context.Context, namedArgs []driver.NamedValue) (driver.Rows, error) {
	if !stmt.connection.IsValid() {
		stmt.connection.setBad()
		return nil, driver.ErrBadConn
	}
	logger := stmt.connection.logger
	logger.Debug("query", "query", stmt.text)
	opts, namedArgs, err := resolve(ctx, stmt.connection, namedArgs)
	if err != nil {
		return nil, err
	}
	ctx, cancel := stmt.apply(ctx, opts)
	defer cancel()
	defer stmt.connection.restoreLobFetch()

	done := stmt.connection.session.StartContext(ctx)
	defer stmt.connection.session.EndContext(done)

	rows, err := stmt.RunQuery(namedArgs)
	if err != nil {
		return nil, stmt.execError(ctx, err)
	}
	return rows, nil
}

func (stmt *Stmt) _query() (*DataSet, error) {
	var err error
	var dataSet *DataSet
	stmt.connection.session.ResetBuffer()
	err = stmt.write()
	if err != nil {
		return nil, err
	}
	dataSet = new(DataSet)
	err = stmt.read(dataSet.currentResultSet())
	if err != nil {
		return nil, err
	}
	// deal with multiSet
	if len(stmt.multiSet) > 0 {
		for index, _ := range stmt.multiSet {
			tempSet, err := stmt.multiSet[index].Query()
			if err != nil {
				return nil, err
			}
			dataSet.resultSets = append(dataSet.resultSets, tempSet.resultSets[0])
		}
		// bypass first resultset
		dataSet.index++
	}
	// deal with lobs
	if stmt.parse && (stmt._hasBLOB || stmt._hasLONG) && stmt.connection.connOption.Lob == config.INLINE {
		stmt.define = true
		stmt.execute = false
		stmt.parse = false
		stmt.reSendParDef = false
		err = stmt.queryLobPrefetch(stmt.getExeOption(), dataSet.currentResultSet())
		if err != nil {
			return nil, err
		}
	}
	stmt.measureLobs(dataSet.currentResultSet().rows)
	// the next run executes again. go-ora left execute false after the
	// define, so each later run sent a fetch with the binds and no execute,
	// which the server refused with ORA-01002 once a bind changed type
	stmt.execute = true
	stmt.parse = false
	stmt.define = false
	stmt.reSendParDef = false
	return dataSet, err
}

// decodeRow converts the values of row that the driver holds into the
// values that it returns, such as a LOB locator into its value. Next calls
// it on each row as it returns the row, and not on each fetch, so that the
// driver holds the loaded LOB values of one row and not of a whole fetch,
// sijms/go-ora#664. When lazy is true, it leaves a CLOB and a BLOB to
// ScanColumn, which loads one only for a destination that is not a LOB.
func (stmt *defaultStmt) decodeRow(row Row, lazy bool) error {
	var err error
	for colIndex := range stmt.columns {
		if colIndex >= len(row) {
			break
		}
		switch val := row[colIndex].(type) {
		case *pendingObject, []any, *indexedItems:
			// an object with a LOB attribute, alone or in a collection
			auto := stmt.connection.connOption.LobReadMode == config.LobReadModeAuto
			row[colIndex], err = resolveObjects(val, auto)
			if err != nil {
				return err
			}
		case *RefCursor:
			row[colIndex], err = val.Query()
			if err != nil {
				return err
			}
		case *types.Vector:
			if stmt.connection.connOption.Lob == config.INLINE {
				row[colIndex], err = val.Value()
				if err != nil {
					return err
				}
			} else if val.ReadMode() == config.LobReadModeAuto {
				if err = val.Load(); err != nil {
					return err
				}
				row[colIndex], err = val.Value()
				if err != nil {
					return err
				}
			}
		case *types.JSON:
			// a JSON row is its JSON text. Inline, the row holds the OSON,
			// and streamed, it holds a locator that a load reads
			if stmt.connection.connOption.Lob != config.INLINE {
				if val.IsNil() {
					// a NULL has no locator, and stayed a JSON, which no
					// destination of text took
					row[colIndex] = nil
					continue
				}
				if val.ReadMode() != config.LobReadModeAuto {
					continue
				}
				if err = val.Load(); err != nil {
					return err
				}
			}
			if err = codec.DecodeOSON(val); err != nil {
				return err
			}
			row[colIndex], err = val.Value()
			if err != nil {
				return err
			}
		case *types.Clob:
			if !lazy && val.ReadMode() == config.LobReadModeAuto {
				if err = val.Load(); err != nil {
					return err
				}
				row[colIndex], err = val.Value()
				if err != nil {
					return err
				}
			}
		case *types.Blob:
			if !lazy && val.ReadMode() == config.LobReadModeAuto {
				if err = val.Load(); err != nil {
					return err
				}
				row[colIndex], err = val.Value()
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// onlyTypes reports whether each type of seen is one of allowed.
func onlyTypes(seen map[uint16]bool, allowed ...uint16) bool {
	for t := range seen {
		if !slices.Contains(allowed, t) {
			return false
		}
	}
	return true
}

// loadLobs loads v when it is a LOB, and each LOB of v when it is the
// elements of a collection. The values of a call are decoded while the reply
// is read, when no LOB can load, so an OUT collection of CLOBs held the
// locators of its elements, and a string read one as its text. The LOBs of
// an object load as resolveObjects fills it.
func loadLobs(v any) error {
	switch v := v.(type) {
	case types.Lob:
		return v.Load()
	case []any:
		for _, item := range v {
			if err := loadLobs(item); err != nil {
				return err
			}
		}
	case *indexedItems:
		return loadLobs(v.items)
	}
	return nil
}

// Query execute a query command and return dataset object in form of driver.Rows interface
//
// args is an array of values that corresponding to parameters in sql
func (stmt *Stmt) Query(args []driver.Value) (driver.Rows, error) {
	if !stmt.connection.IsValid() {
		stmt.connection.setBad()
		return nil, driver.ErrBadConn
	}
	logger := stmt.connection.logger
	logger.Debug("query", "query", stmt.text)
	var dataSet *DataSet
	var err error
	if len(args) == 0 {
		dataSet, err = stmt.RunQuery(nil)
	} else {
		namedArgs := make([]driver.NamedValue, len(args))
		for x := range args {
			namedArgs[x].Value = args[x]
		}
		dataSet, err = stmt.RunQuery(namedArgs)
	}
	return dataSet, err
}

func (stmt *Stmt) NumInput() int {
	return -1
}

/*
parse = true
execute = true
fetch = true if hasReturn or PLSQL
define = false
*/
