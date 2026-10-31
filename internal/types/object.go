package types

import (
	"database/sql/driver"
)

type Object struct {
	Basic
	Name  string
	Owner string
	Value driver.Value
	//typ      reflect.Type
	//toid     []byte
	//fieldMap map[string]int
	//isArray  bool
	//attribs
	//fields OracleTyper
}
