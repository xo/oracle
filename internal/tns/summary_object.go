package tns

type BindError struct {
	errorCode int
	rowOffset int
	errorMsg  []byte
}
type SummaryObject struct {
	EndOfCallStatus      int // uint32
	EndToEndECIDSequence int // uint16
	CurRowNumber         int // uint32
	RetCode              int // uint16
	arrayElmWError       int // uint16
	arrayElmErrno        int // uint16
	CursorID             int // uint16
	errorPos             int // uint16
	sqlType              uint8
	oerFatal             uint8
	Flags                int // uint16
	userCursorOPT        int // uint16
	upiParam             uint8
	warningFlag          uint8
	rba                  int // uint32
	partitionID          int // uint16
	tableID              uint8
	blockNumber          int // uint32
	slotNumber           int // uint16
	osError              int // uint32
	stmtNumber           uint8
	callNumber           uint8
	pad1                 int // uint16
	successIter          int // uint16
	ErrorMessage         []byte
	bindErrors           []BindError
}

func NewSummary(session *Session) (*SummaryObject, error) {
	result := new(SummaryObject)
	var err error
	if session.HasEOSCapability {
		result.EndOfCallStatus, err = session.ReadInt(4, true, true)
		if err != nil {
			return nil, err
		}
	}
	if session.TTCVersion >= 3 {
		if session.HasFSAPCapability {
			result.EndToEndECIDSequence, err = session.ReadInt(2, true, true)
			if err != nil {
				return nil, err
			}

		}
	}
	result.CurRowNumber, err = session.ReadInt(4, true, true)
	if err != nil {
		return nil, err
	}
	result.RetCode, err = session.ReadInt(2, true, true)
	if err != nil {
		return nil, err
	}
	result.arrayElmWError, err = session.ReadInt(2, true, true)
	if err != nil {
		return nil, err
	}
	result.arrayElmErrno, err = session.ReadInt(2, true, true)
	if err != nil {
		return nil, err
	}
	result.CursorID, err = session.ReadInt(2, true, true)
	if err != nil {
		return nil, err
	}
	result.errorPos, err = session.ReadInt(2, true, true)
	if err != nil {
		return nil, err
	}
	result.sqlType, err = session.ReadByte()
	if err != nil {
		return nil, err
	}
	result.oerFatal, err = session.ReadByte()
	if err != nil {
		return nil, err
	}
	if session.TTCVersion >= 4 {
		result.Flags, err = session.ReadInt(2, true, true)
		if err != nil {
			return nil, err
		}
		result.userCursorOPT, err = session.ReadInt(2, true, true)
		if err != nil {
			return nil, err
		}
	} else {
		var temp uint8
		temp, err = session.ReadByte()
		if err != nil {
			return nil, err
		}
		result.Flags = int(temp)
		temp, err = session.ReadByte()
		if err != nil {
			return nil, err
		}
		result.userCursorOPT = int(temp)
	}
	result.upiParam, err = session.ReadByte()
	if err != nil {
		return nil, err
	}
	result.warningFlag, err = session.ReadByte()
	if err != nil {
		return nil, err
	}
	result.rba, err = session.ReadInt(4, true, true)
	if err != nil {
		return nil, err
	}
	result.partitionID, err = session.ReadInt(2, true, true)
	if err != nil {
		return nil, err
	}
	result.tableID, err = session.ReadByte()
	if err != nil {
		return nil, err
	}
	result.blockNumber, err = session.ReadInt(4, true, true)
	if err != nil {
		return nil, err
	}
	result.slotNumber, err = session.ReadInt(2, true, true)
	if err != nil {
		return nil, err
	}
	result.osError, err = session.ReadInt(4, true, true)
	if err != nil {
		return nil, err
	}
	result.stmtNumber, err = session.ReadByte()
	if err != nil {
		return nil, err
	}
	result.callNumber, err = session.ReadByte()
	if err != nil {
		return nil, err
	}
	result.pad1, err = session.ReadInt(2, true, true)
	if err != nil {
		return nil, err
	}
	result.successIter, err = session.ReadInt(4, true, true)
	if err != nil {
		return nil, err
	}
	_, _ = session.ReadDLC()
	// the errors of an array DML come as three arrays on every version: the
	// error codes, the row offsets and the messages. go-ora read them only
	// from TTC 7, and read three empty DLCs before it, which matched while
	// no row failed. On 11g a batch that failed waited for bytes that never
	// came, which was sijms/go-ora#698. Only the return code and the row
	// count after the arrays are new in TTC 7.
	length, err := session.ReadInt(2, true, true)
	if err != nil {
		return nil, err
	}
	if length > 0 {
		result.bindErrors = make([]BindError, length)
		num, err := session.ReadByte()
		if err != nil {
			return nil, err
		}
		flag := num == 0xFE
		for x := 0; x < length; x++ {
			if flag {
				if session.UseBigClrChunks {
					_, _ = session.ReadInt(4, true, true)
				} else {
					_, _ = session.ReadByte()
				}
			}
			result.bindErrors[x].errorCode, err = session.ReadInt(2, true, true)
			if err != nil {
				return nil, err
			}
		}
		if flag {
			_, _ = session.ReadByte()
		}
	}
	length, err = session.ReadInt(4, true, true)
	if err != nil {
		return nil, err
	}
	if length > 0 {
		if len(result.bindErrors) == 0 {
			result.bindErrors = make([]BindError, length)
		}
		num, err := session.ReadByte()
		if err != nil {
			return nil, err
		}
		flag := num == 0xFE
		for x := 0; x < length; x++ {
			if flag {
				if session.UseBigClrChunks {
					_, _ = session.ReadInt(4, true, true)
				} else {
					_, _ = session.ReadByte()
				}
			}
			result.bindErrors[x].rowOffset, err = session.ReadInt(4, true, true)
			if err != nil {
				return nil, err
			}
		}
		if flag {
			_, _ = session.ReadByte()
		}
	}
	length, err = session.ReadInt(2, true, true)
	if err != nil {
		return nil, err
	}
	if length > 0 {
		if len(result.bindErrors) == 0 {
			result.bindErrors = make([]BindError, length)
		}
		_, _ = session.ReadByte()
		for x := 0; x < length; x++ {
			_, err := session.ReadInt(2, true, true)
			if err != nil {
				return nil, err
			}
			result.bindErrors[x].errorMsg, err = session.ReadCLR()
			if err != nil {
				return nil, err
			}
			_, _ = session.ReadByte()
			_, _ = session.ReadByte()
		}
	}
	if session.TTCVersion >= 7 {
		result.RetCode, err = session.ReadInt(4, true, true)
		if err != nil {
			return nil, err
		}
		result.CurRowNumber, err = session.ReadInt(8, true, true)
		if err != nil {
			return nil, err
		}
	}
	if result.RetCode != 0 {
		result.ErrorMessage, err = session.ReadCLR()
		if err != nil {
			return nil, err
		}
	}
	if len(result.bindErrors) > 0 && result.RetCode == 24381 {
		result.RetCode = result.bindErrors[0].errorCode
		result.ErrorMessage = result.bindErrors[0].errorMsg
	}
	return result, nil
}

type WarningObject struct {
	retCode      int
	flag         int
	errorMessage string
}

func NewWarningObject(session *Session) (*WarningObject, error) {
	result := new(WarningObject)
	var err error
	result.retCode, err = session.ReadInt(2, true, true)
	if err != nil {
		return nil, err
	}
	length, err := session.ReadInt(2, true, true)
	if err != nil {
		return nil, err
	}
	result.flag, err = session.ReadInt(2, true, true)
	if err != nil {
		return nil, err
	}
	if result.retCode == 0 || length == 0 {
		return nil, nil
	} else {
		msg, err := session.ReadCLR()
		if err != nil {
			return nil, err
		}
		result.errorMessage = string(msg)
	}
	return result, nil
}
