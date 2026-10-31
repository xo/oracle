package ttc

import (
	"database/sql/driver"
	"math"
	"strings"

	"github.com/xo/oracle/internal/codec"
	"github.com/xo/oracle/internal/config"
	"github.com/xo/oracle/internal/tns"
	oraTypes "github.com/xo/oracle/internal/types"
)

type (
	ParameterDirection int
)

const (
	Input  ParameterDirection = 1
	Output ParameterDirection = 2
	InOut  ParameterDirection = 3
)

type Out struct {
	Dest driver.Value
	Size int
	In   bool
}

type ParameterInfo struct {
	Name         string
	SchemaName   string
	DomainSchema string
	DomainName   string
	Direction    ParameterDirection
	IsNull       bool
	AllowNull    bool
	IsJSON       bool
	ColAlias     string
	IsXmlType    bool
	Precision    uint8
	Scale        uint8
	//MaxNoOfArrayElements int

	Value             driver.Value
	encoder           codec.OracleParameterEncoder
	iPrimValue        driver.Value
	oPrimValue        driver.Value
	getDataFromServer bool
	oaccollid         int
	cusType           *oraTypes.Object
	parent            *ParameterInfo
	Annotations       map[string]string
	codec.BasicParameter
}

// load get parameter information form network session
func (par *ParameterInfo) load(conn *Connection) error {
	session := conn.session
	par.getDataFromServer = true
	dataType, err := session.ReadByte()
	if err != nil {
		return err
	}
	par.DataType = uint16(dataType)
	par.Flag, err = session.ReadByte()
	if err != nil {
		return err
	}
	par.Precision, err = session.ReadByte()
	switch par.DataType {
	case oraTypes.NUMBER:
		fallthrough
	case oraTypes.TimestampDTY:
		fallthrough
	case oraTypes.TimestampTZDTY:
		fallthrough
	case oraTypes.IntervalDSDTY:
		fallthrough
	case oraTypes.TIMESTAMP:
		fallthrough
	case oraTypes.TIMESTAMPTZ:
		fallthrough
	case oraTypes.INTERVALDS:
		fallthrough
	case oraTypes.TimestampLTZDTY:
		fallthrough
	case oraTypes.TimestampLTZ:
		scale, err := session.ReadInt(2, true, true)
		if err != nil {
			return err
		}

		if scale == -127 {
			par.Precision = uint8(math.Ceil(float64(par.Precision) * 0.30103))
			par.Scale = 0xFF
		} else {
			par.Scale = uint8(scale)
		}
	default:
		par.Scale, err = session.ReadByte()
	}
	if par.DataType == oraTypes.NUMBER && par.Precision == 0 && (par.Scale == 0 || par.Scale == 0xFF) {
		par.Precision = 38
		par.Scale = 0xFF
	}

	par.MaxLen, err = session.ReadInt64(4, true, true)
	if err != nil {
		return err
	}
	switch par.DataType {
	case oraTypes.ROWID:
		par.MaxLen = 128
	case oraTypes.DATE:
		// a DATE column takes the buffer of a TIMESTAMP, 11 bytes
		par.MaxLen = oraTypes.MaxLenTimestamp
	case oraTypes.IBFLOAT:
		par.MaxLen = 4
	case oraTypes.IBDOUBLE:
		par.MaxLen = 8
	case oraTypes.TimestampTZDTY:
		par.MaxLen = oraTypes.MaxLenTimestampTZ
	case oraTypes.IntervalYMDTY:
		fallthrough
	case oraTypes.IntervalDSDTY:
		fallthrough
	case oraTypes.INTERVALYM:
		fallthrough
	case oraTypes.INTERVALDS:
		par.MaxLen = 11
	}
	par.ArraySize, err = session.ReadInt(4, true, true)
	if err != nil {
		return err
	}
	if session.TTCVersion >= 10 {
		par.ContFlag, err = session.ReadInt(8, true, true)
	} else {
		par.ContFlag, err = session.ReadInt(4, true, true)
	}
	if err != nil {
		return err
	}
	par.ToID, err = session.ReadDLC()
	par.Version, err = session.ReadInt(2, true, true)
	if err != nil {
		return err
	}
	par.CharsetID, err = session.ReadInt(2, true, true)
	if err != nil {
		return err
	}
	par.CharsetForm, err = session.ReadInt(1, false, false)
	if err != nil {
		return err
	}
	par.MaxCharLen, err = session.ReadInt64(4, true, true)
	if err != nil {
		return err
	}
	if session.TTCVersion >= 8 {
		par.oaccollid, err = session.ReadInt(4, true, true)
	}
	num1, err := session.ReadInt(1, false, false)
	if err != nil {
		return err
	}
	par.AllowNull = num1 > 0
	_, err = session.ReadByte() //  v7 length of name
	if err != nil {
		return err
	}
	bName, err := session.ReadDLC()
	if err != nil {
		return err
	}
	par.Name = session.StrConv.Decode(bName)
	bName, err = session.ReadDLC() // schema name
	if err != nil {
		return err
	}
	par.SchemaName = strings.ToUpper(session.StrConv.Decode(bName))
	bName, err = session.ReadDLC()
	if err != nil {
		return err
	}
	par.TypeName = strings.ToUpper(session.StrConv.Decode(bName))
	if par.DataType == oraTypes.XMLType && par.TypeName != "XMLTYPE" {
		for typName, cusTyp := range conn.cusTyp {
			if typName == par.TypeName {
				par.cusType = new(oraTypes.Object)
				*par.cusType = cusTyp
			}
		}
	}
	if par.TypeName == "XMLTYPE" {
		par.DataType = oraTypes.XMLType
		par.IsXmlType = true
	}
	if session.TTCVersion < 3 {
		return nil
	}
	_, err = session.ReadInt(2, true, true)
	if session.TTCVersion < 6 {
		return nil
	}
	var udsFlags int
	udsFlags, err = session.ReadInt(4, true, true)
	par.IsJSON = (udsFlags & 0x500) > 0
	if session.TTCVersion < 17 {
		return nil
	}
	bName, err = session.ReadDLC()
	if err != nil {
		return err
	}
	par.DomainSchema = strings.ToUpper(session.StrConv.Decode(bName))
	bName, err = session.ReadDLC()
	if err != nil {
		return err
	}
	par.DomainName = strings.ToUpper(session.StrConv.Decode(bName))
	if session.TTCVersion < 20 {
		return nil
	}
	numAnnotations, err := session.ReadInt(4, true, true)
	if err != nil {
		return err
	}
	if numAnnotations > 0 {
		par.Annotations = make(map[string]string)
		_, err = session.ReadByte()
		if err != nil {
			return err
		}
		numAnnotations, err = session.ReadInt(4, true, true)
		if err != nil {
			return err
		}
		_, err = session.ReadByte()
		if err != nil {
			return err
		}
		for i := 0; i < numAnnotations; i++ {
			bKey, bValue, _, err := session.ReadKeyVal()
			if err != nil {
				return err
			}
			key := session.StrConv.Decode(bKey)
			value := session.StrConv.Decode(bValue)
			par.Annotations[key] = value
		}
		_, err = session.ReadInt(4, true, true)
		if err != nil {
			return err
		}
	}
	if session.TTCVersion < 24 {
		return nil
	}
	if par.DataType == oraTypes.VECTOR {
		par.VectorDim, err = session.ReadInt(4, true, true)
		if err != nil {
			return err
		}
		par.VectorFormat, err = session.ReadByte()
		if err != nil {
			return err
		}
		par.VectorFlag, err = session.ReadByte()

		par.VectorType = oraTypes.VectorDense
		if par.VectorFlag&2 == 2 {
			par.VectorType = oraTypes.VectorSparse
		}
	} else {
		_, err = session.ReadInt(4, true, true)
		if err != nil {
			return err
		}
		_, err = session.ReadInt(1, true, true)
		if err != nil {
			return err
		}
		_, err = session.ReadInt(1, true, true)
		if err != nil {
			return err
		}
	}

	return nil
}

// write parameter information to network session
func (par *ParameterInfo) write(session *tns.Session) error {
	session.PutBytes(uint8(par.DataType), par.Flag, par.Precision, par.Scale)
	session.PutUint(par.MaxLen, 4, true, true)
	// MaxNoOfArrayElements should be 0 in case of XML type
	session.PutInt(par.ArraySize, 4, true, true)
	if session.TTCVersion >= 10 {
		session.PutInt(par.ContFlag, 8, true, true)
	} else {
		session.PutInt(par.ContFlag, 4, true, true)
	}
	if par.ToID == nil {
		session.PutBytes(0)
	} else {
		session.PutInt(len(par.ToID), 4, true, true)
		session.PutClr(par.ToID)
	}
	session.PutUint(par.Version, 2, true, true)
	session.PutUint(par.CharsetID, 2, true, true)
	session.PutBytes(uint8(par.CharsetForm))
	session.PutUint(par.MaxCharLen, 4, true, true)
	if session.TTCVersion >= 8 {
		session.PutInt(par.oaccollid, 4, true, true)
	}
	return nil
}

func (par *ParameterInfo) isLongType() bool {
	return par.DataType == oraTypes.LONG ||
		par.DataType == oraTypes.LongRaw ||
		par.DataType == oraTypes.LongVarChar ||
		par.DataType == oraTypes.LongVarRaw
}

func (par *ParameterInfo) isLobType() bool {
	return par.DataType == oraTypes.OCIBlobLocator ||
		par.DataType == oraTypes.OCIClobLocator ||
		par.DataType == oraTypes.OCIFileLocator ||
		par.DataType == oraTypes.VECTOR ||
		par.DataType == oraTypes.JSONType
}

func (par *ParameterInfo) decodePrimValue(conn *Connection, udt bool) error {
	session := conn.session
	var err error
	par.oPrimValue = nil
	par.BValue = nil
	var decoder codec.OracleParameterCoder
	if par.ArraySize > 0 {
		decoder = &codec.ArrayParameter{}
	} else if par.DataType == 0 {
		// untyped NULL column (e.g. SELECT NULL FROM dual): read the
		// null value from the stream without needing a registered coder
		par.BValue, err = session.ReadCLR()
		if err != nil {
			return err
		}
		par.oPrimValue = nil
		par.IsNull = true
		return nil
	} else {
		if par.DataType == oraTypes.XMLType {
			decoder, err = conn.ParameterCoder(par.TypeName)
		} else {
			decoder, err = conn.ParameterCoder(par.DataType)
		}
	}
	if err != nil {
		return err
	}
	decoder.SetParameterInfo(par.ParameterInfo())
	if par.isLobType() {
		streamer := &LobStream{conn: conn}
		decoder.SetLobStreamer(streamer)
	}
	err = decoder.Read(session)
	if err != nil {
		return err
	}
	par.oPrimValue, err = decoder.Decode(conn)
	return err

}

func (par *ParameterInfo) decodeParameterValue(connection *Connection) error {
	currentLobFetch := connection.connOption.Lob
	connection.connOption.Lob = config.STREAM
	defer func() {
		connection.connOption.Lob = currentLobFetch
	}()
	err := par.decodePrimValue(connection, false)
	if err != nil {
		return err
	}
	return nil
}

func (par *ParameterInfo) decodeColumnValue(connection *Connection, udt bool) error {
	if err := par.decodePrimValue(connection, udt); err != nil {
		return err
	}
	if udt {
		// the attribute of an object goes to a field of a struct, which the
		// copy of the object fills from the value of the decoder
		return nil
	}
	v, err := columnValue(par, par.oPrimValue)
	if err != nil {
		return err
	}
	par.oPrimValue = v
	return nil
}
