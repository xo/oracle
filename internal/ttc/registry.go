package ttc

import (
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/xo/oracle/internal/codec"
	"github.com/xo/oracle/internal/config"
	"github.com/xo/oracle/internal/types"
)

// Registry holds the type coders and the registered types of one database.
// Each Connector of the root package has its own, so that two databases share
// none of it (D9).
type Registry struct {
	dataCollected   bool
	oracleTypeCoder map[uint16]codec.OracleParameterCoder
	goTypeCoder     map[reflect.Type]codec.OracleParameterCoder
	nameTypeCoder   map[string]codec.OracleParameterCoder
	cusTyp          map[string]types.Object
	cookies         cookieJar
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

// NewRegistry returns a Registry with the coders of every built in type.
func NewRegistry() *Registry {
	drv := &Registry{
		goTypeCoder:     make(map[reflect.Type]codec.OracleParameterCoder),
		oracleTypeCoder: make(map[uint16]codec.OracleParameterCoder),
		nameTypeCoder:   make(map[string]codec.OracleParameterCoder),
		//typeDecoder:  make(map[uint16]type_coder.OracleTypeDecoder),
		cusTyp: map[string]types.Object{},
	}
	drv.init()
	return drv
}

func (driver *Registry) init() {
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

	driver.oracleTypeCoder[types.DATE] = &codec.DateParameter{DataType: types.DATE, MaxLen: types.MaxLenTimestamp}
	driver.oracleTypeCoder[types.TIMESTAMP] = &codec.DateParameter{DataType: types.TIMESTAMP, MaxLen: types.MaxLenTimestamp}
	driver.oracleTypeCoder[types.TIMESTAMPTZ] = &codec.DateParameter{DataType: types.TIMESTAMPTZ, MaxLen: types.MaxLenTimestampTZ}
	driver.oracleTypeCoder[types.TimestampDTY] = &codec.DateParameter{DataType: types.TimestampDTY, MaxLen: types.MaxLenTimestampTZ}
	driver.oracleTypeCoder[types.TimestampTZDTY] = &codec.DateParameter{DataType: types.TimestampTZDTY, MaxLen: types.MaxLenTimestampTZ}
	driver.oracleTypeCoder[types.TimestampLTZ] = &codec.DateParameter{DataType: types.TimestampLTZ, MaxLen: types.MaxLenTimestampTZ}
	driver.oracleTypeCoder[types.TimestampLTZDTY] = &codec.DateParameter{DataType: types.TimestampTZDTY, MaxLen: types.MaxLenTimestampTZ}

	driver.oracleTypeCoder[types.IntervalYMDTY] = &codec.IntervalParameter{DataType: types.IntervalYMDTY, MaxLen: types.MaxLenIntervalYM}
	driver.oracleTypeCoder[types.IntervalDSDTY] = &codec.IntervalParameter{DataType: types.IntervalDSDTY, MaxLen: types.MaxLenIntervalDS}

	driver.oracleTypeCoder[types.VECTOR] = &codec.VectorParameter{}
	driver.oracleTypeCoder[types.JSONType] = &codec.JSONParameter{}
	driver.oracleTypeCoder[types.OCIBlobLocator] = &codec.BlobParameter{}
	driver.oracleTypeCoder[types.OCIClobLocator] = &codec.ClobParameter{}
	driver.oracleTypeCoder[types.OCIFileLocator] = &codec.BFileParameter{}

	driver.oracleTypeCoder[types.ROWID] = &codec.RowIDParameter{}
	driver.oracleTypeCoder[types.UROWID] = &codec.RowIDParameter{}

	driver.oracleTypeCoder[types.BOOLEAN] = &codec.BoolParameter{}

	driver.oracleTypeCoder[types.REFCURSOR] = &codec.CursorParameter{}

	// name type coder
	driver.nameTypeCoder["NUMBER"] = &codec.NumberParameter{DataType: types.NUMBER, MaxLen: types.MaxLenNumber}
	driver.nameTypeCoder["IBFLOAT"] = &codec.NumberParameter{DataType: types.IBFLOAT, MaxLen: types.MaxLenBFloat}
	driver.nameTypeCoder["IBDOUBLE"] = &codec.NumberParameter{DataType: types.IBDOUBLE, MaxLen: types.MaxLenBDouble}
	driver.nameTypeCoder["VARCHAR2"] = &codec.StringParameter{}
	driver.nameTypeCoder["NVARCHAR2"] = &codec.StringParameter{CharsetForm: 2}
	driver.nameTypeCoder["TIMESTAMP"] = &codec.DateParameter{DataType: types.TIMESTAMP, MaxLen: types.MaxLenTimestampTZ}
	driver.nameTypeCoder["DATE"] = &codec.DateParameter{DataType: types.DATE, MaxLen: types.MaxLenTimestamp}
	driver.nameTypeCoder["TIMESTAMP WITH LOCAL TIME ZONE"] = &codec.DateParameter{DataType: types.TimestampLTZDTY, MaxLen: types.MaxLenTimestampTZ}
	driver.nameTypeCoder["RAW"] = &codec.RawParameter{}
	driver.nameTypeCoder["BLOB"] = &codec.BlobParameter{}
	driver.nameTypeCoder["CLOB"] = &codec.ClobParameter{}
	tempClob := &codec.ClobParameter{}
	tempClob.CharsetForm = 2
	driver.nameTypeCoder["NCLOB"] = tempClob
	driver.nameTypeCoder["XMLTYPE"] = &xmlParameter{}

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

// SessionParam is a parameter that a connector sets on each session with
// ALTER SESSION.
type SessionParam struct {
	Key, Value string
}

// InitFromConn records the user of the first connection, which RegisterType
// takes as the owner of a type, and sets params on conn.
func (driver *Registry) InitFromConn(conn *Connection, params []SessionParam) error {
	driver.mu.Lock()
	if !driver.dataCollected {
		driver.UserId = conn.connOption.UserID
		driver.dataCollected = true
	}
	driver.mu.Unlock()
	for _, p := range params {
		if _, err := conn.Exec(fmt.Sprintf("alter session set %s=%s", p.Key, p.Value)); err != nil {
			return fmt.Errorf("setting the session parameter %s: %w", p.Key, err)
		}
	}
	return nil
}

// RegisterType registers typeName, and arrayTypeName when it is not empty,
// for the type of typeObj, owned by the user of the first connection. It
// reads the attributes of the type through db.
func (drv *Registry) RegisterType(db *sql.DB, typeName, arrayTypeName string, typeObj any) error {
	// ping first to avoid error when calling register type after open connection
	err := db.Ping()
	if err != nil {
		return err
	}
	return drv.RegisterTypeWithOwner(db, drv.UserId, typeName, arrayTypeName, typeObj)
}

// RegisterTypeWithOwner is RegisterType for a type that owner owns.
func (drv *Registry) RegisterTypeWithOwner(db *sql.DB, owner, typeName, arrayTypeName string, typeObj any) error {
	if len(owner) == 0 {
		return errors.New("owner can't be empty")
	}
	typeName = strings.TrimSpace(typeName)
	arrayTypeName = strings.TrimSpace(arrayTypeName)
	var err error

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
		err = objCoder.loadObjectTypeInfo(db, drv, owner, typeName)
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
		drv.mu.Unlock()
	}
	return nil
}

// NewConnection returns a connection for config, not open yet, with the
// coders and the types of drv.
func (drv *Registry) NewConnection(config *config.ConnectionConfig) (*Connection, error) {
	conn, err := NewConnection(config)
	if err != nil {
		return nil, err
	}
	conn.buildParameterCoderMap(drv)
	conn.cusTyp = drv.cusTyp
	conn.cookies = &drv.cookies
	return conn, nil
}
