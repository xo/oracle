package ttc

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/xo/oracle/internal/codec"
	oraTypes "github.com/xo/oracle/internal/types"

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
	_hasLONG          bool
	_hasBLOB          bool
	_hasMoreRows      bool
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
	if stmt.cursorID == 0 {
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
			if stmt.connection.connOption.Lob == config.INLINE && !col.IsJSON && col.DataType != oraTypes.OCIFileLocator {
				num = 0x3FFFFFFF
				if col.DataType == oraTypes.OCIBlobLocator || col.DataType == oraTypes.VECTOR || col.DataType == oraTypes.JSONType {
					col.DataType = oraTypes.LongRaw
					// change data type in the original array
					stmt.columns[index].DataType = oraTypes.LongRaw
				} else {
					col.DataType = oraTypes.LongVarChar
					// change data type in the original array
					stmt.columns[index].DataType = oraTypes.LongVarChar
				}
			} else {
				col.ContFlag |= 0x2000000
				col.MaxCharLen = 0x8000
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

var sqlQueryReturnStatementRegexp = regexp.MustCompile(`(\bRETURNING\b|\bRETURN\b)\s+.*\s+\bINTO\b`)

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
	stmt._hasBLOB = false
	stmt._hasLONG = false
	stmt.arrayBindCount = 0
	stmt.scnForSnapshot = make([]int, 2)
	// get stmt type
	uCmdText := strings.ToUpper(refineSqlText(text))
	if strings.HasPrefix(uCmdText, "(") {
		uCmdText = uCmdText[1:]
	}
	if strings.HasPrefix(uCmdText, "SELECT") || strings.HasPrefix(uCmdText, "WITH") {
		stmt.stmtType = SELECT
	} else if strings.HasPrefix(uCmdText, "INSERT") ||
		strings.HasPrefix(uCmdText, "MERGE") ||
		strings.HasPrefix(uCmdText, "UPDATE") ||
		strings.HasPrefix(uCmdText, "DELETE") {
		stmt.stmtType = DML
		stmt.bulkExec = true
	} else if strings.HasPrefix(uCmdText, "DECLARE") || strings.HasPrefix(uCmdText, "BEGIN") {
		stmt.stmtType = PLSQL
	} else {
		stmt.stmtType = OTHERS
	}

	if stmt.stmtType != PLSQL {
		stmt._hasReturnClause = sqlQueryReturnStatementRegexp.MatchString(uCmdText)
	}
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
			//if par.DataType == oraTypes.REFCURSOR {
			//	ms.PutBytes(1, 0)
			//	//session.WriteBytes(&buffer, 1, 0)
			//} else
			if (par.Direction == Input || par.Direction == InOut) && par.isLobType() {
				if par.DataType == oraTypes.VECTOR {
				} else {
					if len(par.BValue) > 0 {
						ms.PutUint(len(par.BValue), 4, true, true)
					}
					ms.PutClr(par.BValue)
				}
			} else {
				if par.cusType != nil {
					ms.PutBytes(0, 0, 0, 0)
					ms.PutUint(len(par.BValue), 4, true, true)
					ms.PutBytes(1, 1)
					ms.PutClr(par.BValue)
				} else {
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

			// for valueIndex, values := range arrayValue {
			// 	stmt.Pars[parIndex].BValue = values[valueIndex]
			// }
			//  valueIndex := 0; valueIndex < stmt.arrayBindCount; valueIndex++ {
			//  each value represented an array of []byte
			// }
			// for valueIndex := 0; valueIndex < stmt.arrayBindCount; valueIndex++ {
			// 	for parIndex, arrayValue := range arrayValues {
			// 		tempVal := reflect.ValueOf(arrayValue)
			// 		err = stmt.Pars[parIndex].encodeValue(tempVal.Index(valueIndex).Interface(), 0, stmt.connection)
			// 		if err != nil {
			// 			return err
			// 		}
			// 	}
			// 	err = stmt.writePars()
			// 	if err != nil {
			// 		return err
			// 	}
			// }
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
	if stmt.arrayBindCount > 1 {
		op |= 0x80000
	}
	if stmt.connection.autoCommit && (stmt.stmtType == DML || stmt.stmtType == PLSQL) {
		op |= 0x100
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
		if stmt.stmtType == PLSQL || (stmt._hasReturnClause && !stmt.reSendParDef) {
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
	if stmt._noOfRowsToFetch == 25 {
		// m_maxRowSize = m_maxRowSize + m_numOfLOBColumns * Math.Max(86, 86 + (int) lobSize) + m_numOfLONGColumns * Math.Max(2, longSize) + m_numOfBFileColumns * 86;
		maxRowSize := int64(0)
		for _, col := range stmt.columns {
			if col.isLobType() {
				maxRowSize += 86
			} else if col.isLongType() {
				maxRowSize += 2
			} else {
				maxRowSize += col.MaxLen
			}
		}
		if maxRowSize > 0 {
			stmt._noOfRowsToFetch = (0x20000 / int(maxRowSize)) + 1
		}
		stmt.connection.tracer.Printf("Fetch Size Calculated: %d", stmt._noOfRowsToFetch)
	}

	tracer := stmt.connection.tracer
	err := stmt._fetch(resultSet)
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
			tracer.Print("Error: ", err)
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
	err = stmt.read(resultSet)
	if err != nil {
		return err
	}
	return stmt.decodePrim(resultSet)
}

func (stmt *defaultStmt) queryLobPrefetch(exeOp int, resultSet *ResultSet) error {
	if stmt._noOfRowsToFetch == 25 {
		// m_maxRowSize = m_maxRowSize + m_numOfLOBColumns * Math.Max(86, 86 + (int) lobSize) + m_numOfLONGColumns * Math.Max(2, longSize) + m_numOfBFileColumns * 86;
		maxRowSize := 0
		for _, col := range stmt.columns {
			if col.isLobType() {
				maxRowSize += 86
			} else if col.isLongType() {
				maxRowSize += 2
			} else {
				maxRowSize += int(col.MaxLen)
			}
		}
		if maxRowSize > 0 {
			stmt._noOfRowsToFetch = (0x20000 / maxRowSize) + 1
		}
		stmt.connection.tracer.Printf("Fetch Size Calculated: %d", stmt._noOfRowsToFetch)
	}
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
						if num > 1 {
							return errors.New("more than one row affected with return clause")
						}
						if num == 0 {
							stmt.Pars[x].BValue = nil
							stmt.Pars[x].Value = nil
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
						if stmt.Pars[x].DataType == oraTypes.REFCURSOR {
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
						fmt.Println("query ID: ", stmt.queryID)
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
				for range length {
					_, err = session.ReadInt(8, true, true)
					if err != nil {
						return err
					}
				}
			}
		case 11:
			err = resultSet.load(session)
			if err != nil {
				return err
			}
			for x := 0; x < resultSet.columnCount; x++ {
				direction, err := session.ReadByte()
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
				if err != nil {
					return err
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
			stmt.columns = make([]ParameterInfo, resultSet.columnCount)
			for x := 0; x < resultSet.columnCount; x++ {
				err = stmt.columns[x].load(stmt.connection)
				if err != nil {
					return err
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
	if stmt.connection.tracer.IsOn() {
		resultSet.Trace(stmt.connection.tracer)
	}
	return nil
}

// requestCustomTypeInfo an experimental function to ask for UDT information
func (stmt *defaultStmt) requestCustomTypeInfo(typeName string) error {
	session := stmt.connection.session
	session.SaveState(nil)
	session.PutBytes(0x3, 0x5c, 0)
	session.PutInt(3, 4, true, true)

	session.PutBytes(bytes.Repeat([]byte{0}, 19)...)
	session.PutInt(2, 4, true, true)
	session.PutInt(len(stmt.connection.connOption.UserID), 4, true, true)
	session.PutClr(stmt.connection.sStrConv.Encode(stmt.connection.connOption.UserID))
	session.PutInt(len(typeName), 4, true, true)
	session.PutClr(stmt.connection.sStrConv.Encode(typeName))
	session.PutBytes(0, 0, 0, 0, 0, 1, 0, 0, 0, 0)
	session.PutBytes(bytes.Repeat([]byte{0}, 50)...)
	err := session.Write()
	if err != nil {
		return err
	}
	data, err := session.ReadBytes(0x10)
	if err != nil {
		return err
	}
	fmt.Printf("%#v\n", data)
	session.LoadState()
	return nil
}

func (stmt *defaultStmt) calculateColumnValue(col *ParameterInfo, udt bool) error {
	session := stmt.connection.session
	if col.DataType == oraTypes.REFCURSOR {
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
	if param.DataType == oraTypes.XMLType && param.IsNull {
		return nil
	}
	if param.DataType != oraTypes.XMLType && param.ArraySize > 0 {
		return nil
	}
	_, err = stmt.connection.session.ReadInt(2, true, true)
	if err != nil {
		return err
	}
	return nil
}

// Close stmt cursor in the server
func (stmt *defaultStmt) Close() error {
	if stmt.connection.State != Opened {
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
	if stmt.connection.State != Opened {
		stmt.connection.setBad()
		return nil, driver.ErrBadConn
	}
	tracer := stmt.connection.tracer
	tracer.Printf("Exec With Context:")

	tracer.Printf("Exec:\n%s", stmt.text)
	stmt.arrayBindCount = 0

	done := stmt.connection.session.StartContext(ctx)
	defer stmt.connection.session.EndContext(done)

	// the call runs here, and the watch of StartContext breaks it when ctx
	// ends, so no goroutine waits for it (D10)
	result, err := stmt._exec(args)
	if errors.Is(err, tns.ErrConnReset) {
		if stmt._hasReturnClause {
			dataSet := &DataSet{}
			err = stmt.read(dataSet.currentResultSet())
			if !errors.Is(err, tns.ErrConnReset) {
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
			var fieldVal *oraTypes.Number
			if hasNullValue {
				fieldVal = &oraTypes.Number{}
			} else {
				fieldVal, err = oraTypes.NewNumber(fieldValue)
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
			fieldVal := &oraTypes.String{}
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
			fieldVal := &sql.Null[oraTypes.TimestampTZ]{}
			if !hasNullValue {
				var tempDate time.Time
				tempDate, err = getDate(fieldValue)
				if err != nil {
					err = typeErr
					return
				}
				fieldVal.V = oraTypes.TimestampTZ(tempDate)
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
			fieldVal := &oraTypes.Clob{}
			err = fieldVal.SetValue(fieldValue)
			if err != nil {
				return
			}
			//fieldVal, err := oraTypes.NewClob(, nil, temp, false)
			//if err != nil {
			//	return nil, err
			//}
			//if !hasNullValue {
			//	fieldVal.String, fieldVal.Valid = getString(fieldValue), true
			//}
			tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		case "nclob":
			fieldVal := &oraTypes.Clob{}
			fieldVal.UseNCharset = true
			err = fieldVal.SetValue(fieldValue)
			if err != nil {
				return
			}
			tempPar, err = stmt.NewParam(name, fieldVal, size, dir)
		case "blob":
			fieldVal := &oraTypes.Blob{}
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
				err = tempPar.encodeValue(0, stmt.connection)
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
					err = tempPar.encodeValue(0, stmt.connection)
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
				stmt.connection.tracer.Printf("    %d:\n%v", x, args[x])
				parIndex += processedPars
				structPars = append(structPars, args[x].Value)
				continue
			}
		}
		if stmt.bulkExec {
			tempType := reflect.TypeOf(args[x].Value)
			tempVal := reflect.ValueOf(args[x].Value)
			if args[x].Value != nil && tempType != reflect.TypeFor[[]byte]() && (tempType.Kind() == reflect.Array || tempType.Kind() == reflect.Slice) {
				// setup array count
				if stmt.arrayBindCount == 0 {
					stmt.arrayBindCount = tempVal.Len()
				} else {
					if stmt.arrayBindCount > tempVal.Len() {
						stmt.arrayBindCount = tempVal.Len()
					}
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
				arrayValues := make([][]byte, stmt.arrayBindCount)
				for y := 0; y < stmt.arrayBindCount; y++ {
					par.Value = tempVal.Index(y).Interface()
					err = par.encodeValue(0, stmt.connection)
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
					if len(par.BValue) == 0 && par.DataType == oraTypes.NCHAR {
						continue
					}
					dataType = par.DataType
				}
				// save arrayValues into primitive
				par.iPrimValue = arrayValues
				par.MaxLen = maxLen
				par.MaxCharLen = maxCharLen
				if int(dataType) == 0 {
					dataType = oraTypes.NCHAR
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
		stmt.connection.tracer.Printf("    %d:\n%v", x, args[x])
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
		if par.Direction != Input && par.DataType != oraTypes.REFCURSOR {
			fieldValue := reflect.ValueOf(par.Value)
			if fieldValue.Kind() != reflect.Pointer {
				return nil, errors.New("output parameter should be pointer type")
			}
			fieldValue = fieldValue.Elem()
			if par.ArraySize > 0 {
			} else {
				// check if LobReadMode is auto so auto read all lobs
				if stmt.connection.connOption.LobReadMode == config.LobReadModeAuto {
					if lob, ok := par.oPrimValue.(oraTypes.Lob); ok {
						err = lob.Load()
						if err != nil {
							return nil, err
						}
					}
				}
				// reading of blob
				//err = copy_(par.Value, par.oPrimValue)
				//if err != nil {
				//	return nil, err
				//}
				//continue
				//err = setFieldValue(fieldValue, par.cusType, par.oPrimValue)
				//if err != nil {
				//	return nil, err
				//}

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

// useNamedParameters: re-arrange parameters according parameter defined in sql text
func (stmt *Stmt) useNamedParameters() error {
	names, err := parseQueryParametersNames(stmt.text)
	if err != nil {
		return err
	}
	parCollection := make([]ParameterInfo, 0, len(names))
	if stmt.stmtType == SELECT || stmt.stmtType == DML {
		for x := range names {
			found := false
			for _, par := range stmt.Pars {
				if strings.ToLower(par.Name) == strings.ToLower(names[x]) {
					parCollection = append(parCollection, par)
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("parameter %s is not defined in parameter list", names[x])
			}
			for y := x - 1; y >= 0; y-- {
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
				if strings.ToLower(par.Name) == strings.ToLower(names[x]) {
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
	if stmt.connection.State != Opened {
		stmt.connection.setBad()
		return nil, driver.ErrBadConn
	}
	tracer := stmt.connection.tracer
	tracer.Printf("Exec:\n%s", stmt.text)
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
			tracer.Print("Error: ", err)
			return nil, driver.ErrBadConn
		}
		return nil, err
	}
	return result, err
}

func (stmt *Stmt) CheckNamedValue(nv *driver.NamedValue) error {
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
	if stmt.connection.State != Opened {
		stmt.connection.setBad()
		return nil, driver.ErrBadConn
	}
	param := &ParameterInfo{
		Name:      name,
		Direction: direction,
		Value:     val,
	}
	// initialize bfile
	//if file, ok := val.(*BFile); ok {
	//	if !file.isInit() {
	//		err := file.init(stmt.connection)
	//		if err != nil {
	//			return nil, err
	//		}
	//	}
	//}
	err := param.encodeValue(int64(size), stmt.connection)
	if err != nil {
		return nil, err
	}
	return param, err
}

func (stmt *Stmt) setParam(pos int, par ParameterInfo) {
	if pos >= 0 && pos < len(stmt.Pars) {
		originalPar := stmt.Pars[pos]
		if par.MaxLen > originalPar.MaxLen {
			stmt.reSendParDef = true
		}
		// resend parameter definition when string/[]byte length change from long to short and vice versa
		switch par.DataType {
		case oraTypes.NCHAR:
			if originalPar.DataType == oraTypes.LONG || originalPar.DataType == oraTypes.LongVarChar {
				stmt.reSendParDef = true
			}
		case oraTypes.RAW:
			if originalPar.DataType == oraTypes.LongRaw {
				stmt.reSendParDef = true
			}
		case oraTypes.LONG:
			fallthrough
		case oraTypes.LongVarChar:
			if originalPar.DataType == oraTypes.NCHAR {
				stmt.reSendParDef = true
			}
		case oraTypes.LongVarRaw:
			if originalPar.DataType == oraTypes.RAW {
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
	if stmt.connection.State != Opened {
		stmt.connection.setBad()
		return nil, driver.ErrBadConn
	}
	tracer := stmt.connection.tracer
	stmt._noOfRowsToFetch = stmt.connection.connOption.PrefetchRows
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
		tracer.Printf("    %d:\n%v", x, namedArgs[x])
	}

	if useNamedPars {
		err := stmt.useNamedParameters()
		if err != nil {
			return nil, err
		}
	}

	dataSet, err := stmt._query()
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
			tracer.Print("Error: ", err)
			return nil, driver.ErrBadConn
		}
		return nil, err
	}
	if dataSet == nil {
		return nil, fmt.Errorf("go-ora: query returned no result set (connection reset)")
	}
	return dataSet, nil
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
	if session.HasError() {
		err = session.ServerError()
	}
	if isBadConn(err) {
		stmt.connection.setBad()
	}
	return err
}

func (stmt *Stmt) QueryContext(ctx context.Context, namedArgs []driver.NamedValue) (driver.Rows, error) {
	if stmt.connection.State != Opened {
		stmt.connection.setBad()
		return nil, driver.ErrBadConn
	}
	tracer := stmt.connection.tracer
	tracer.Print("Query With Context:", stmt.text)

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
	err = stmt.decodePrim(dataSet.currentResultSet())
	if err != nil {
		return nil, err
	}
	stmt.parse = false
	stmt.define = false
	stmt.reSendParDef = false
	return dataSet, err
}

func (stmt *defaultStmt) decodePrim(resultSet *ResultSet) error {
	var err error
	// convert from go-ora primitives to sql primitives
	for rowIndex, row := range resultSet.rows {
		for colIndex, _ := range stmt.columns {
			if row == nil {
				continue
			}
			switch val := row[colIndex].(type) {
			case *RefCursor:
				resultSet.rows[rowIndex][colIndex], err = val.Query()
				if err != nil {
					return err
				}
			case *oraTypes.Vector:
				if stmt.connection.connOption.Lob == config.INLINE {
					resultSet.rows[rowIndex][colIndex], err = val.Value()
					if err != nil {
						return err
					}
				} else {
					if val.ReadMode() == config.LobReadModeAuto {
						err = val.Load()
						if err != nil {
							return err
						}
						resultSet.rows[rowIndex][colIndex], err = val.Value()
						if err != nil {
							return err
						}
					}
				}
			case *oraTypes.JSON:
				// a JSON row is its JSON text. Inline, the row holds the OSON,
				// and streamed, it holds a locator that a load reads
				if stmt.connection.connOption.Lob != config.INLINE {
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
				resultSet.rows[rowIndex][colIndex], err = val.Value()
				if err != nil {
					return err
				}
			case *oraTypes.Clob:
				if val.ReadMode() == config.LobReadModeAuto {
					err = val.Load()
					if err != nil {
						return err
					}
					resultSet.rows[rowIndex][colIndex], err = val.Value()
					if err != nil {
						return err
					}
				}
			case *oraTypes.Blob:
				if val.ReadMode() == config.LobReadModeAuto {
					err = val.Load()
					if err != nil {
						return err
					}
					resultSet.rows[rowIndex][colIndex], err = val.Value()
					if err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// Query execute a query command and return dataset object in form of driver.Rows interface
//
// args is an array of values that corresponding to parameters in sql
func (stmt *Stmt) Query(args []driver.Value) (driver.Rows, error) {
	if stmt.connection.State != Opened {
		stmt.connection.setBad()
		return nil, driver.ErrBadConn
	}
	tracer := stmt.connection.tracer
	tracer.Printf("Query:\n%s", stmt.text)
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
