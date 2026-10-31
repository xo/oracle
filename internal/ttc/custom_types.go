package ttc

import (
	"database/sql/driver"
)

type (
	//PLBool   bool
	//NVarChar string
	batch struct {
		array driver.Value
	}
)

func NewBatch(array driver.Value) driver.Value {
	return &batch{array: array}
}
