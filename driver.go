package oracle

import (
	"database/sql"
	"fmt"

	"github.com/xo/oracle/internal/ttc"
)

func init() {
	sql.Register("oracle", Driver{})
}

// RegisterType registers the user defined type typeName, and its collection
// arrayTypeName when that is not empty, for the Go type of typeObj, with the
// database of db alone. The owner of the type is the user of the database.
// It reads the attributes of the type through db, so the type must exist.
func RegisterType(db *sql.DB, typeName, arrayTypeName string, typeObj any) error {
	reg, err := registryOf(db)
	if err != nil {
		return err
	}
	return reg.RegisterType(db, typeName, arrayTypeName, typeObj)
}

// RegisterTypeWithOwner is RegisterType for a type that owner owns.
func RegisterTypeWithOwner(db *sql.DB, owner, typeName, arrayTypeName string, typeObj any) error {
	reg, err := registryOf(db)
	if err != nil {
		return err
	}
	return reg.RegisterTypeWithOwner(db, owner, typeName, arrayTypeName, typeObj)
}

// registryOf returns the registry of the connector of db.
func registryOf(db *sql.DB) (*ttc.Registry, error) {
	d, ok := db.Driver().(Driver)
	if !ok || d.c == nil {
		return nil, fmt.Errorf("finding the registry: the database does not use this driver: %w", ErrNotSupported)
	}
	return d.c.reg, nil
}
