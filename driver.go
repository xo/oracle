package oracle

import (
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/xo/oracle/internal/codec"
	"github.com/xo/oracle/internal/codec/oson"
	"github.com/xo/oracle/types"
)

// registry holds the type coders and the registered types of one database.
// Each Connector has its own, so that two databases share none of it (D9).
type registry struct {
	dataCollected   bool
	oracleTypeCoder map[uint16]codec.OracleParameterCoder
	goTypeCoder     map[reflect.Type]codec.OracleParameterCoder
	nameTypeCoder   map[string]codec.OracleParameterCoder
	jsonEncoder     map[reflect.Type]oson.FieldEncoder
	cusTyp          map[string]types.Object
	mu              sync.Mutex
	UserId          string
	// Server    string
	// Port      int
	// Instance  string
	// Service   string
	// DBName    string

	// SessionId int
	// SerialNum int
}

func newRegistry() *registry {
	drv := &registry{
		goTypeCoder:     make(map[reflect.Type]codec.OracleParameterCoder),
		oracleTypeCoder: make(map[uint16]codec.OracleParameterCoder),
		nameTypeCoder:   make(map[string]codec.OracleParameterCoder),
		//typeDecoder:  make(map[uint16]type_coder.OracleTypeDecoder),
		cusTyp: map[string]types.Object{},
	}
	drv.init()
	return drv
}

func init() {
	sql.Register("oracle", Driver{})
}

func (driver *registry) init() {
	// add basic type decoders here
	driver.oracleTypeCoder[types.NCHAR] = &codec.StringParameter{}
	driver.oracleTypeCoder[types.CHAR] = &codec.StringParameter{}
	driver.oracleTypeCoder[types.LONG] = &codec.StringParameter{}
	driver.oracleTypeCoder[types.LongVarChar] = &codec.StringParameter{}

	driver.oracleTypeCoder[types.RAW] = &codec.RawParameter{}
	driver.oracleTypeCoder[types.LongRaw] = &codec.RawParameter{}

	driver.oracleTypeCoder[types.NUMBER] = &codec.NumberParameter{DataType: types.NUMBER, MaxLen: types.MaxLenNumber}
	driver.oracleTypeCoder[types.IBFLOAT] = &codec.NumberParameter{DataType: types.IBFLOAT, MaxLen: types.MaxLenBFloat}
	driver.oracleTypeCoder[types.IBDOUBLE] = &codec.NumberParameter{DataType: types.IBDOUBLE, MaxLen: types.MaxLenBDouble}

	driver.oracleTypeCoder[types.DATE] = &codec.DateParameter{DataType: types.DATE, MaxLen: types.MaxLenTimeStamp}
	driver.oracleTypeCoder[types.TIMESTAMP] = &codec.DateParameter{DataType: types.TIMESTAMP, MaxLen: types.MaxLenTimeStamp}
	driver.oracleTypeCoder[types.TIMESTAMPTZ] = &codec.DateParameter{DataType: types.TIMESTAMPTZ, MaxLen: types.MaxLenTimeStampTZ}
	driver.oracleTypeCoder[types.TimeStampDTY] = &codec.DateParameter{DataType: types.TimeStampDTY, MaxLen: types.MaxLenTimeStampTZ}
	driver.oracleTypeCoder[types.TimeStampTZ_DTY] = &codec.DateParameter{DataType: types.TimeStampTZ_DTY, MaxLen: types.MaxLenTimeStampTZ}
	driver.oracleTypeCoder[types.TimeStampLTZ] = &codec.DateParameter{DataType: types.TimeStampLTZ, MaxLen: types.MaxLenTimeStampTZ}
	driver.oracleTypeCoder[types.TimeStampLTZ_DTY] = &codec.DateParameter{DataType: types.TimeStampTZ_DTY, MaxLen: types.MaxLenTimeStampTZ}

	driver.oracleTypeCoder[types.INTERVALYM_DTY] = &codec.IntervalParameter{DataType: types.INTERVALYM_DTY, MaxLen: types.MaxLenIntervalYM}
	driver.oracleTypeCoder[types.INTERVALDS_DTY] = &codec.IntervalParameter{DataType: types.INTERVALDS_DTY, MaxLen: types.MaxLenIntervalDS}

	driver.oracleTypeCoder[types.VECTOR] = &codec.VectorParameter{}
	driver.oracleTypeCoder[types.JSON] = &codec.JsonParameter{}
	driver.oracleTypeCoder[types.OCIBlobLocator] = &codec.BlobParameter{}
	driver.oracleTypeCoder[types.OCIClobLocator] = &codec.ClobParameter{}
	driver.oracleTypeCoder[types.OCIFileLocator] = &codec.BFileParameter{}

	driver.oracleTypeCoder[types.ROWID] = &codec.RowIDParameter{}
	driver.oracleTypeCoder[types.UROWID] = &codec.RowIDParameter{}

	driver.oracleTypeCoder[types.BOOLEAN] = &codec.BoolParameter{}

	driver.oracleTypeCoder[types.REFCURSOR] = &codec.CursorParameter{}

	driver.goTypeCoder[types.TyString] = &codec.StringParameter{}
	driver.goTypeCoder[types.TyNullString] = &codec.StringParameter{}
	driver.goTypeCoder[types.TyVarchar] = &codec.StringParameter{}

	driver.goTypeCoder[types.TyBytes] = &codec.RawParameter{}
	driver.goTypeCoder[types.TyRaw] = &codec.RawParameter{}

	driver.goTypeCoder[types.TyInt] = &codec.NumberParameter{}
	driver.goTypeCoder[types.TyInt8] = &codec.NumberParameter{}
	driver.goTypeCoder[types.TyInt16] = &codec.NumberParameter{}
	driver.goTypeCoder[types.TyInt32] = &codec.NumberParameter{}
	driver.goTypeCoder[types.TyInt64] = &codec.NumberParameter{}
	driver.goTypeCoder[types.TyUint] = &codec.NumberParameter{}
	driver.goTypeCoder[types.TyUint8] = &codec.NumberParameter{}
	driver.goTypeCoder[types.TyUint16] = &codec.NumberParameter{}
	driver.goTypeCoder[types.TyUint32] = &codec.NumberParameter{}
	driver.goTypeCoder[types.TyUint64] = &codec.NumberParameter{}
	driver.goTypeCoder[types.TyFloat32] = &codec.NumberParameter{}
	driver.goTypeCoder[types.TyFloat64] = &codec.NumberParameter{}
	driver.goTypeCoder[types.TyNullByte] = &codec.NumberParameter{}
	driver.goTypeCoder[types.TyNullInt16] = &codec.NumberParameter{}
	driver.goTypeCoder[types.TyNullInt32] = &codec.NumberParameter{}
	driver.goTypeCoder[types.TyNullInt64] = &codec.NumberParameter{}
	driver.goTypeCoder[types.TyNullFloat64] = &codec.NumberParameter{}
	driver.goTypeCoder[types.TyNumber] = &codec.NumberParameter{}

	driver.goTypeCoder[types.TyBoolean] = &codec.BoolParameter{}

	driver.goTypeCoder[types.TyTime] = &codec.DateParameter{}
	driver.goTypeCoder[types.TyNullTime] = &codec.DateParameter{}
	driver.goTypeCoder[types.TyDate] = &codec.DateParameter{}

	driver.goTypeCoder[types.TyInterval] = &codec.IntervalParameter{}

	driver.goTypeCoder[types.TyVector] = &codec.VectorParameter{}
	driver.goTypeCoder[types.TyJson] = &codec.JsonParameter{}

	driver.goTypeCoder[types.TyClob] = &codec.ClobParameter{}
	driver.goTypeCoder[types.TyBlob] = &codec.BlobParameter{}
	driver.goTypeCoder[types.TyBFile] = &codec.BFileParameter{}

	driver.goTypeCoder[reflect.TypeFor[RefCursor]()] = &codec.CursorParameter{}

	// name type coder
	driver.nameTypeCoder["NUMBER"] = &codec.NumberParameter{DataType: types.NUMBER, MaxLen: types.MaxLenNumber}
	driver.nameTypeCoder["IBFLOAT"] = &codec.NumberParameter{DataType: types.IBFLOAT, MaxLen: types.MaxLenBFloat}
	driver.nameTypeCoder["IBDOUBLE"] = &codec.NumberParameter{DataType: types.IBDOUBLE, MaxLen: types.MaxLenBDouble}
	driver.nameTypeCoder["VARCHAR2"] = &codec.StringParameter{}
	driver.nameTypeCoder["NVARCHAR2"] = &codec.StringParameter{CharsetForm: 2}
	driver.nameTypeCoder["TIMESTAMP"] = &codec.DateParameter{DataType: types.TIMESTAMP, MaxLen: types.MaxLenTimeStampTZ}
	driver.nameTypeCoder["DATE"] = &codec.DateParameter{DataType: types.DATE, MaxLen: types.MaxLenTimeStamp}
	driver.nameTypeCoder["TIMESTAMP WITH LOCAL TIME ZONE"] = &codec.DateParameter{DataType: types.TimeStampLTZ_DTY, MaxLen: types.MaxLenTimeStampTZ}
	driver.nameTypeCoder["RAW"] = &codec.RawParameter{}
	driver.nameTypeCoder["BLOB"] = &codec.BlobParameter{}
	driver.nameTypeCoder["CLOB"] = &codec.ClobParameter{}
	tempClob := &codec.ClobParameter{}
	tempClob.CharsetForm = 2
	driver.nameTypeCoder["NCLOB"] = tempClob

	// initialize all
	for _, coder := range driver.goTypeCoder {
		coder.Init()
	}
	for _, coder := range driver.oracleTypeCoder {
		coder.Init()
	}
	for _, coder := range driver.nameTypeCoder {
		coder.Init()
	}
}

// initFromConn records the user of the first connection, which RegisterType
// takes as the owner of a type, and sets the session parameters of the
// connector on conn.
func (driver *registry) initFromConn(conn *Connection, params []sessionParam) error {
	driver.mu.Lock()
	if !driver.dataCollected {
		driver.UserId = conn.connOption.UserID
		driver.dataCollected = true
	}
	driver.mu.Unlock()
	for _, p := range params {
		if _, err := conn.Exec(fmt.Sprintf("alter session set %s=%s", p.key, p.value)); err != nil {
			return fmt.Errorf("setting the session parameter %s: %w", p.key, err)
		}
	}
	return nil
}

//func RegisterJsonFieldEncoder(db GetDriverInterface, key reflect.Type, encoder oson.FieldEncoder) error {
//	if drv, ok := db.Driver().(*OracleDriver); ok {
//		drv.mu.Lock()
//		drv.jsonEncoder[key] = encoder
//		drv.mu.Unlock()
//		return nil
//	}
//	return errors.New("the driver used is not a go-ora driver type")
//}

//	func RegisterJsonFieldDecoder(db GetDriverInterface, opCode int, decoder oson.FieldDecoder) error {
//		if drv, ok := db.Driver().(*OracleDriver); ok {
//			drv.mu.Lock()
//			drv.jsonDecoder[opCode] = decoder
//			drv.mu.Unlock()
//			return nil
//		}
//		return errors.New("the driver used is not a go-ora driver type")
//	}
//func RegisterParameterEncoder(db *sql.DB, _type reflect.Type, encoder parameter_coder.OracleParameterEncoder) error {
//	if drv, ok := db.Driver().(*OracleDriver); ok {
//		drv.mu.Lock()
//		drv.goTypeCoder[_type] = encoder
//		drv.mu.Unlock()
//	}
//	return errors.New("the driver used is not a go-ora driver type")
//}
//func RegisterParameterDecoder(db *sql.DB, typeId uint16, decoder parameter_coder.OracleParameterDecoder) error {
//	if drv, ok := db.Driver().(*OracleDriver); ok {
//		drv.mu.Lock()
//		drv.oracleTypeCoder[typeId] = decoder
//		drv.mu.Unlock()
//		return nil
//	}
//	return errors.New("the driver used is not a go-ora driver type")
//}

// func RegisterRegularTypeArray(conn *sql.DB, regularTypeName, arrayTypeName string, itemMaxSize int) error {
// 	err := conn.Ping()
// 	if err != nil {
// 		return err
// 	}
//
// 	if drv, ok := conn.Driver().(*OracleDriver); ok {
// 		return RegisterRegularTypeArrayWithOwner(conn, drv.UserId, regularTypeName, arrayTypeName, itemMaxSize)
// 	}
// 	return errors.New("the driver used is not a go-ora driver type")
// }
// func RegisterRegularTypeArrayWithOwner(conn *sql.DB, owner, regularTypeName, arrayTypeName string, itemMaxSize int) error {
// 	drv := conn.Driver().(*OracleDriver)
// 	regularTypeName = strings.TrimSpace(regularTypeName)
// 	arrayTypeName = strings.TrimSpace(arrayTypeName)
// 	if len(regularTypeName) == 0 {
// 		return errors.New("typeName shouldn't be empty")
// 	}
// 	if len(arrayTypeName) == 0 {
// 		return errors.New("array type name shouldn't be empty")
// 	}
// 	cust := customType{
// 		owner:         owner,
// 		name:          regularTypeName,
// 		arrayTypeName: arrayTypeName,
// 		isArray:       true,
// 	}
// 	var err error
// 	cust.arrayTOID, err = getTOID2(conn, owner, arrayTypeName)
// 	if err != nil {
// 		return err
// 	}
// 	param := ParameterInfo{Direction: Input, Flag: 3, TypeName: regularTypeName, MaxLen: 1, MaxCharLen: 1}
// 	switch strings.ToUpper(regularTypeName) {
// 	case "NUMBER":
// 		param.DataType = NUMBER
// 	case "VARCHAR2":
// 		param.DataType = NCHAR
// 		param.CharsetForm = 1
// 		param.ContFlag = 16
// 		param.CharsetID = drv.sStrConv.GetLangID()
// 	case "NVARCHAR2":
// 		param.DataType = NCHAR
// 		param.CharsetForm = 2
// 		param.ContFlag = 16
// 		param.CharsetID = drv.nStrConv.GetLangID()
// 	case "TIMESTAMP":
// 		param.DataType = TimeStampDTY
// 	case "DATE":
// 		param.DataType = DATE
// 	case "TIMESTAMP WITH LOCAL TIME ZONE":
// 		param.DataType = TimeStampLTZ_DTY
// 	// case "TIMESTAMP WITH TIME ZONE":
// 	// 	param.DataType = TimeStampTZ_DTY
// 	// 	param.MaxLen = converters.MAX_LEN_TIMESTAMP
// 	case "RAW":
// 		param.DataType = RAW
// 	case "BLOB":
// 		param.DataType = OCIBlobLocator
// 	case "CLOB":
// 		param.DataType = OCIClobLocator
// 		param.CharsetForm = 1
// 		param.ContFlag = 16
// 		param.CharsetID = drv.sStrConv.GetLangID()
// 	case "NCLOB":
// 		param.DataType = OCIClobLocator
// 		param.CharsetForm = 2
// 		param.ContFlag = 16
// 		param.CharsetID = drv.nStrConv.GetLangID()
// 	default:
// 		return fmt.Errorf("unsupported regular type: %s", regularTypeName)
// 	}
// 	cust.attribs = append(cust.attribs, param)
// 	drv.mu.Lock()
// 	defer drv.mu.Unlock()
// 	drv.cusTyp[strings.ToUpper(arrayTypeName)] = cust
// 	return nil
// }

func RegisterType(db *sql.DB, typeName, arrayTypeName string, typeObj any) error {
	// ping first to avoid error when calling register type after open connection
	err := db.Ping()
	if err != nil {
		return err
	}

	drv, err := registryOf(db)
	if err != nil {
		return err
	}
	return RegisterTypeWithOwner(db, drv.UserId, typeName, arrayTypeName, typeObj)
}

func RegisterTypeWithOwner(db *sql.DB, owner, typeName, arrayTypeName string, typeObj any) error {
	if len(owner) == 0 {
		return errors.New("owner can't be empty")
	}
	typeName = strings.TrimSpace(typeName)
	arrayTypeName = strings.TrimSpace(arrayTypeName)
	drv, err := registryOf(db)
	if err != nil {
		return err
	}

	//if typeObj == nil {
	//	return errors.New("type object cannot be nil")
	//}
	typ := getType(typeObj)
	if typeObj != nil {
		switch typ.Kind() {
		case reflect.Pointer:
			return errors.New("unsupported type object: Ptr")
		case reflect.Array:
			return errors.New("unsupported type object: Array")
		case reflect.Chan:
			return errors.New("unsupported type object: Chan")
		case reflect.Map:
			return errors.New("unsupported type object: Map")
		case reflect.Slice:
			return errors.New("unsupported type object: Slice")
		}
	}

	if len(typeName) == 0 {
		return errors.New("typeName shouldn't be empty")
	}
	// process object first
	arrayParam, ok := drv.nameTypeCoder[strings.ToUpper(typeName)]
	if !ok {
		// we should add this type as new type
		objCoder := &ObjectParameter{
			typ: typ,
		}
		objCoder.Init()
		objCoder.TypeName = typeName
		err = objCoder.loadObjectTypeInfo(db, owner, typeName)
		if err != nil {
			return err
		}
		drv.mu.Lock()
		// map name to coder
		drv.nameTypeCoder[strings.ToUpper(typeName)] = objCoder
		// map type to coder
		drv.goTypeCoder[typ] = objCoder
		drv.mu.Unlock()
		arrayParam = objCoder.Copy()
	}

	if len(arrayTypeName) > 0 {
		arrayCoder := &ObjectParameter{
			isArray: true,
		}
		arrayCoder.Init()
		if typeObj != nil {
			arrayCoder.typ = reflect.SliceOf(typ)
		}
		arrayCoder.TypeName = arrayTypeName
		arrayCoder.attribs = make(map[string]codec.OracleParameterCoder)
		arrayParam.SetAsArrayPar()
		arrayCoder.attribs[""] = arrayParam
		arrayCoder.ToID, err = getTOID2(db, owner, arrayTypeName)
		if err != nil {
			return err
		}
		drv.mu.Lock()
		drv.nameTypeCoder[strings.ToUpper(arrayTypeName)] = arrayCoder
		// map type to coder
		if typeObj != nil {
			drv.goTypeCoder[arrayCoder.typ] = arrayCoder
		}
		//drv.goTypeCoder[reflect.SliceOf(typ)] = arrayParam
		drv.mu.Unlock()
	}
	return nil
	//arrayParam := ParameterInfo{
	//	Direction: Input,
	//	BasicParameter: parameter_coder.BasicParameter{
	//		Flag:       3,
	//		MaxLen:     1,
	//		MaxCharLen: 1,
	//	},
	//	TypeName: typeName,
	//}
	//switch strings.ToUpper(typeName) {
	//case "NUMBER":
	//	arrayParam.DataType = types.NUMBER
	//case "VARCHAR2":
	//	arrayParam.DataType = types.NCHAR
	//	arrayParam.CharsetForm = 1
	//	arrayParam.ContFlag = 16
	//	arrayParam.CharsetID = drv.sStrConv.GetLangID()
	//case "NVARCHAR2":
	//	arrayParam.DataType = types.NCHAR
	//	arrayParam.CharsetForm = 2
	//	arrayParam.ContFlag = 16
	//	arrayParam.CharsetID = drv.nStrConv.GetLangID()
	//case "TIMESTAMP":
	//	arrayParam.DataType = types.TimeStampDTY
	//case "DATE":
	//	arrayParam.DataType = types.DATE
	//case "TIMESTAMP WITH LOCAL TIME ZONE":
	//	arrayParam.DataType = types.TimeStampLTZ_DTY
	//// case "TIMESTAMP WITH TIME ZONE":
	////	arrayParam.DataType = TimeStampTZ_DTY
	////	arrayParam.MaxLen = converters.MAX_LEN_TIMESTAMP
	//case "RAW":
	//	arrayParam.DataType = types.RAW
	//case "BLOB":
	//	arrayParam.DataType = types.OCIBlobLocator
	//case "CLOB":
	//	arrayParam.DataType = types.OCIClobLocator
	//	arrayParam.CharsetForm = 1
	//	arrayParam.ContFlag = 16
	//	arrayParam.CharsetID = drv.sStrConv.GetLangID()
	//case "NCLOB":
	//	arrayParam.DataType = types.OCIClobLocator
	//	arrayParam.CharsetForm = 2
	//	arrayParam.ContFlag = 16
	//	arrayParam.CharsetID = drv.nStrConv.GetLangID()
	//default:
	//	if cust.typ == nil {
	//		return errors.New("type object cannot be nil")
	//	}
	//	if typ.Kind() != reflect.Struct {
	//		return errors.New("type object should be of structure type")
	//	}
	//	cust.activeFields = map[string]int{}
	//	cust.toid, err = getTOID2(db, owner, typeName)
	//	if err != nil {
	//		return err
	//	}
	//	sqlText := `SELECT ATTR_NAME, ATTR_TYPE_NAME, LENGTH, ATTR_NO
	//				FROM ALL_TYPE_ATTRS
	//				WHERE UPPER(OWNER)=:1 AND UPPER(TYPE_NAME)=:2
	//				ORDER BY ATTR_NO`
	//	rows, err := db.Query(sqlText, strings.ToUpper(owner), strings.ToUpper(typeName))
	//	if err != nil {
	//		return err
	//	}
	//	var (
	//		attName     sql.NullString
	//		attOrder    int64
	//		attTypeName sql.NullString
	//		length      sql.NullInt64
	//	)
	//	for rows.Next() {
	//		err = rows.Scan(&attName, &attTypeName, &length, &attOrder)
	//		if err != nil {
	//			return err
	//		}
	//		param := ParameterInfo{Direction: Input, BasicParameter: parameter_coder.BasicParameter{Flag: 3}}
	//		param.Name = attName.String
	//		param.TypeName = attTypeName.String
	//		switch strings.ToUpper(attTypeName.String) {
	//		case "NUMBER":
	//			param.DataType = types.NUMBER
	//			param.MaxLen = int64(converters.MAX_LEN_NUMBER)
	//		case "VARCHAR2":
	//			param.DataType = types.NCHAR
	//			param.CharsetForm = 1
	//			param.ContFlag = 16
	//			param.MaxCharLen = length.Int64
	//			param.CharsetID = drv.sStrConv.GetLangID()
	//			param.MaxLen = length.Int64 * int64(converters.MaxBytePerChar(param.CharsetID))
	//		case "NVARCHAR2":
	//			param.DataType = types.NCHAR
	//			param.CharsetForm = 2
	//			param.ContFlag = 16
	//			param.MaxCharLen = length.Int64
	//			param.CharsetID = drv.nStrConv.GetLangID()
	//			param.MaxLen = length.Int64 * int64(converters.MaxBytePerChar(param.CharsetID))
	//		case "TIMESTAMP":
	//			param.DataType = types.TimeStampDTY
	//			param.MaxLen = int64(converters.MAX_LEN_DATE)
	//		case "DATE":
	//			param.DataType = types.DATE
	//			param.MaxLen = int64(converters.MAX_LEN_DATE)
	//		case "TIMESTAMP WITH LOCAL TIME ZONE":
	//			param.DataType = types.TimeStampLTZ_DTY
	//			param.MaxLen = int64(converters.MAX_LEN_DATE)
	//		case "TIMESTAMP WITH TIME ZONE":
	//			param.DataType = types.TimeStampTZ_DTY
	//			param.MaxLen = int64(converters.MAX_LEN_TIMESTAMP)
	//		case "RAW":
	//			param.DataType = types.RAW
	//			param.MaxLen = length.Int64
	//		case "BLOB":
	//			param.DataType = types.OCIBlobLocator
	//			param.MaxLen = length.Int64
	//		case "CLOB":
	//			param.DataType = types.OCIClobLocator
	//			param.CharsetForm = 1
	//			param.ContFlag = 16
	//			param.CharsetID = drv.sStrConv.GetLangID()
	//			param.MaxCharLen = length.Int64
	//			param.MaxLen = length.Int64 * int64(converters.MaxBytePerChar(param.CharsetID))
	//		case "NCLOB":
	//			param.DataType = types.OCIClobLocator
	//			param.CharsetForm = 2
	//			param.ContFlag = 16
	//			param.CharsetID = drv.nStrConv.GetLangID()
	//			param.MaxCharLen = length.Int64
	//			param.MaxLen = length.Int64 * int64(converters.MaxBytePerChar(param.CharsetID))
	//		default:
	//			found := false
	//			for name, value := range drv.cusTyp {
	//				if strings.EqualFold(name, attTypeName.String) {
	//					found = true
	//					param.DataType = types.XMLType
	//					param.cusType = new(Object)
	//					*param.cusType = value
	//					param.ToID = value.toid
	//					break
	//				}
	//			}
	//			if !found {
	//				return fmt.Errorf("unsupported attribute type: %s", attTypeName.String)
	//			}
	//		}
	//		cust.attribs = append(cust.attribs, param)
	//	}
	//	if len(cust.attribs) == 0 {
	//		return fmt.Errorf("unknown or empty type: %s", typeName)
	//	}
	//	arrayParam.DataType = types.XMLType
	//	arrayParam.cusType = new(Object)
	//	*arrayParam.cusType = cust
	//
	//	cust.loadActiveFields()
	//	drv.mu.Lock()
	//	drv.cusTyp[strings.ToUpper(typeName)] = cust
	//	drv.mu.Unlock()
	//}
	//if len(arrayTypeName) > 0 {
	//	var err error
	//	arrayCust.toid, err = getTOID2(db, owner, arrayTypeName)
	//	if err != nil {
	//		return err
	//	}
	//	arrayCust.attribs = append(arrayCust.attribs, arrayParam)
	//	drv.mu.Lock()
	//	drv.cusTyp[strings.ToUpper(arrayTypeName)] = arrayCust
	//	drv.mu.Unlock()
	//}
	//return nil
}

// registryOf returns the registry of the connector of db.
func registryOf(db *sql.DB) (*registry, error) {
	d, ok := db.Driver().(Driver)
	if !ok || d.c == nil {
		return nil, fmt.Errorf("finding the registry: the database does not use this driver: %w", ErrNotSupported)
	}
	return d.c.reg, nil
}
