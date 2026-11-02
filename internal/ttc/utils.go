package ttc

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"strings"
	"syscall"
	"time"

	"github.com/xo/oracle/internal/types"

	"github.com/xo/oracle/internal/tns"
)

func generateTokenHeader(host, service string, port int) string {
	ret := fmt.Sprintf("date:%s\n(request-target):%s\nhost:%s:%d", time.Now().UTC().Format(time.RFC1123), service, host, port)
	ret = strings.ReplaceAll(ret, "UTC", "GMT")
	return ret
}

func signTokenHeader(data string, privateKey []byte) (signature string, err error) {
	block, _ := pem.Decode(privateKey)
	if block == nil || block.Type != "RSA PRIVATE KEY" && block.Type != "PRIVATE KEY" {
		err = errors.New("invalid private key PEM")
		return
	}
	var key any
	key, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			err = errors.New("failed to parse private key")
			return
		}
	}

	priv, ok := key.(*rsa.PrivateKey)
	if !ok {
		err = errors.New("not an RSA private key")
		return
	}

	hashed := sha256.Sum256([]byte(data))
	var sig []byte
	sig, err = rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, hashed[:])
	if err != nil {
		return
	}
	signature = base64.StdEncoding.EncodeToString(sig)
	return
}

func tSigned(input reflect.Type) bool {
	switch input.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	default:
		return false
	}
}

func tUnsigned(input reflect.Type) bool {
	switch input.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	default:
		return false
	}
}

func tInteger(input reflect.Type) bool {
	return tSigned(input) || tUnsigned(input)
}

func tFloat(input reflect.Type) bool {
	return input.Kind() == reflect.Float32 || input.Kind() == reflect.Float64
}

func processReset(err error, conn *Conn) error {
	if errors.Is(err, tns.ErrConnReset) {
		err = conn.read()
	}
	if err != nil && isBadConn(err) {
		conn.setBad()
	}
	return err
}

func isBadConn(err error) bool {
	var opError *net.OpError
	var oraError *tns.OracleError
	switch {
	case errors.Is(err, io.EOF):
		return true
	case errors.Is(err, syscall.EPIPE):
		return true
	case errors.As(err, &opError):
		if opError.Net == "tcp" && opError.Op == "write" {
			return true
		}
	case errors.As(err, &oraError):
		return oraError.Bad()
	}
	return false
}

// packageType splits name into a package and a type, in upper case, when it
// names a type of a package, as PKG.TYPE.
func packageType(name string) (pkg, typ string, ok bool) {
	if _, ok := rowType(name); ok {
		return "", "", false
	}
	pkg, typ, ok = strings.Cut(name, ".")
	return strings.ToUpper(pkg), strings.ToUpper(typ), ok
}

// rowType returns the table of name, in upper case, when name is the record
// of a row of a table, as TABLE%ROWTYPE.
func rowType(name string) (table string, ok bool) {
	table, ok = strings.CutSuffix(strings.ToUpper(name), "%ROWTYPE")
	return table, ok
}

// getTOID2 returns the type OID of a type, from ALL_TYPES, or of a type of a
// package, from ALL_PLSQL_TYPES (12.1).
func getTOID2(conn *sql.DB, owner, typeName string) ([]byte, error) {
	var toid []byte
	var err error
	if table, ok := rowType(typeName); ok {
		// the record of a row is in no view, and python-oracledb reads its
		// OID from the pickler
		_, err = conn.Exec(`DECLARE
	t_name VARCHAR2(400) := :1;
	t_oid RAW(16);
	t_version PLS_INTEGER;
	t_tds BLOB;
	t_instantiable VARCHAR2(3);
	t_super_owner VARCHAR2(128);
	t_super_name VARCHAR2(128);
	t_attrs SYS_REFCURSOR;
	t_subtypes SYS_REFCURSOR;
	t_result PLS_INTEGER;
BEGIN
	t_result := DBMS_PICKLER.GET_TYPE_SHAPE(t_name, t_oid, t_version, t_tds,
		t_instantiable, t_super_owner, t_super_name, t_attrs, t_subtypes);
	:2 := t_oid;
END;`, strings.ToUpper(owner)+"."+table+"%ROWTYPE", Out{Dest: &toid, Size: 16})
		if err == nil && len(toid) == 0 {
			err = sql.ErrNoRows
		}
	} else if pkg, typ, ok := packageType(typeName); ok {
		err = conn.QueryRow(`SELECT type_oid FROM ALL_PLSQL_TYPES WHERE owner = :1 AND package_name = :2 AND type_name = :3`,
			strings.ToUpper(owner), pkg, typ).Scan(&toid)
	} else {
		err = conn.QueryRow(`SELECT type_oid FROM ALL_TYPES WHERE UPPER(OWNER)=:1 AND UPPER(TYPE_NAME)=:2`,
			strings.ToUpper(owner), strings.ToUpper(typeName)).Scan(&toid)
	}
	if errors.Is(err, sql.ErrNoRows) {
		err = fmt.Errorf("type: %s is not present or wrong type name", typeName)
	}
	return toid, err
}

func parseInputField(structValue reflect.Value, name, _type string, fieldIndex int) (tempPar *ParameterInfo, err error) {
	tempPar = &ParameterInfo{
		Name:      name,
		Direction: Input,
	}
	fieldValue := structValue.Field(fieldIndex)
	for fieldValue.Type().Kind() == reflect.Pointer {
		if fieldValue.IsNil() {
			tempPar.Value = nil
			return
		}
		fieldValue = fieldValue.Elem()
	}
	if !fieldValue.IsValid() {
		tempPar.Value = nil
		return
	}
	if fieldValue.CanInterface() && fieldValue.Interface() == nil {
		tempPar.Value = nil
		return
	}
	typeErr := fmt.Errorf("error passing field %s as type %s", fieldValue.Type().String(), _type)
	switch _type {
	case "number":
		tempPar.Value = getString(fieldValue.Interface())
	case "varchar":
		tempPar.Value = getString(fieldValue.Interface())
	case "nvarchar":
		temp := &types.String{}
		temp.UseNCharset = true
		err = temp.SetValue(getString(fieldValue.Interface()))
		if err != nil {
			return nil, err
		}
		tempPar.Value = temp
	case "date":
		tempPar.Value, err = getDate(fieldValue.Interface())
		if err != nil {
			err = typeErr
			return
		}
	case "timestamp":
		var fieldVal = &types.Date{}
		var temp time.Time
		temp, err = getDate(fieldValue.Interface())
		if err != nil {
			err = typeErr
			return
		}
		fieldVal.SetDataType(types.TIMESTAMP)
		err = fieldVal.SetValue(temp)
		tempPar.Value = fieldVal
	case "timestamptz":
		var fieldVal = &types.Date{}
		var temp time.Time
		temp, err = getDate(fieldValue.Interface())
		if err != nil {
			err = typeErr
			return
		}
		fieldVal.SetDataType(types.TIMESTAMPTZ)
		err = fieldVal.SetValue(temp)
		tempPar.Value = fieldVal
	case "raw":
		tempPar.Value, err = getBytes(fieldValue.Interface())
		if err != nil {
			err = typeErr
			return
		}
	case "clob":
		fieldVal := getString(fieldValue.Interface())
		clob := &types.Clob{}
		if len(fieldVal) == 0 {
			err = clob.SetValue(nil)
		} else {
			err = clob.SetValue(fieldVal)
		}
		if err != nil {
			return nil, err
		}
		tempPar.Value = clob
	case "nclob":
		fieldVal := getString(fieldValue.Interface())
		clob := &types.Clob{}
		clob.UseNCharset = true
		if len(fieldVal) == 0 {
			err = clob.SetValue(nil)
		} else {
			err = clob.SetValue(fieldVal)
		}
		if err != nil {
			return nil, err
		}
		tempPar.Value = clob
	case "blob":
		var fieldVal []byte
		fieldVal, err = getBytes(fieldValue.Interface())
		if err != nil {
			err = typeErr
			return
		}
		blob := &types.Blob{}
		err = blob.SetValue(fieldVal)
		if err != nil {
			return nil, err
		}
		tempPar.Value = blob
	case "":
		tempPar.Value = fieldValue.Interface()
	default:
		err = typeErr
	}
	return
}
