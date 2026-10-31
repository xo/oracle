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
	"regexp"
	"strings"
	"syscall"
	"time"

	oraTypes "github.com/xo/oracle/internal/types"

	"github.com/xo/oracle/internal/tns"
)

var ()

func refineSqlText(text string) string {
	index := 0
	length := len(text)
	inSingleQuote := false
	inDoubleQuote := false
	skip := false
	lineComment := false
	textBuffer := make([]byte, 0, len(text))
	for ; index < length; index++ {
		ch := text[index]
		switch ch {
		case '\\':
			// bypass next character
			continue
		case '/':
			if !inDoubleQuote && !inSingleQuote {
				if index+1 < length && text[index+1] == '*' {
					index += 1
					skip = true
				}
			}
		case '*':
			if !inDoubleQuote && !inSingleQuote {
				if index+1 < length && text[index+1] == '/' {
					index += 1
					skip = false
				}
			}
		case '\'':
			if !skip && !lineComment && !inDoubleQuote {
				inSingleQuote = !inSingleQuote
			}
		case '"':
			if !skip && !lineComment && !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
		case '-':
			if !skip {
				if index+1 < length && text[index+1] == '-' {
					index += 1
					lineComment = true
				}
			}
		case '\n':
			if lineComment {
				lineComment = false
			} else {
				textBuffer = append(textBuffer, ch) // oheurtel : keep the line feed character
			}
		default:
			if skip || lineComment || inSingleQuote || inDoubleQuote {
				continue
			}
			textBuffer = append(textBuffer, text[index])
		}
	}
	return strings.TrimSpace(string(textBuffer))
}

var parameterNameRegexp = regexp.MustCompile(`:(\w+)`)

func parseQueryParametersNames(text string) (names []string, err error) {
	refinedSql := refineSqlText(text)

	names = make([]string, 0, 10)
	matches := parameterNameRegexp.FindAllStringSubmatch(refinedSql, -1)
	for _, match := range matches {
		if len(match) > 1 {
			names = append(names, match[1])
		}
	}
	return names, nil
}

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

func processReset(err error, conn *Connection) error {
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

func getTOID2(conn *sql.DB, owner, typeName string) ([]byte, error) {
	var toid []byte
	err := conn.QueryRow(`SELECT type_oid FROM ALL_TYPES WHERE UPPER(OWNER)=:1 AND UPPER(TYPE_NAME)=:2`,
		strings.ToUpper(owner), strings.ToUpper(typeName)).Scan(&toid)
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
		temp := &oraTypes.String{}
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
		var fieldVal = &oraTypes.Date{}
		var temp time.Time
		temp, err = getDate(fieldValue.Interface())
		if err != nil {
			err = typeErr
			return
		}
		fieldVal.SetDataType(oraTypes.TIMESTAMP)
		err = fieldVal.SetValue(temp)
		tempPar.Value = fieldVal
	case "timestamptz":
		var fieldVal = &oraTypes.Date{}
		var temp time.Time
		temp, err = getDate(fieldValue.Interface())
		if err != nil {
			err = typeErr
			return
		}
		fieldVal.SetDataType(oraTypes.TIMESTAMPTZ)
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
		clob := &oraTypes.Clob{}
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
		clob := &oraTypes.Clob{}
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
		blob := &oraTypes.Blob{}
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
