package types

import (
	"database/sql/driver"
	"encoding/json/v2"
	"fmt"
	"time"
)

// TimestampTZ is a time that binds as TIMESTAMP WITH TIME ZONE, where a
// time.Time binds as TIMESTAMP. A nullable one is sql.Null[TimestampTZ].
type TimestampTZ time.Time

// Value returns ts itself, which the driver binds as TIMESTAMP WITH TIME
// ZONE. It has a value receiver, so that sql.Null[TimestampTZ] reaches it.
func (ts TimestampTZ) Value() (driver.Value, error) {
	return ts, nil
}

// Scan reads a time.Time.
func (ts *TimestampTZ) Scan(value any) error {
	switch v := value.(type) {
	case TimestampTZ:
		*ts = v
	case time.Time:
		*ts = TimestampTZ(v)
	default:
		return fmt.Errorf("scanning %T into a TimestampTZ", value)
	}
	return nil
}

func (ts TimestampTZ) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Time(ts))
}

func (ts *TimestampTZ) UnmarshalJSON(data []byte) error {
	var t time.Time
	if err := json.Unmarshal(data, &t); err != nil {
		return err
	}
	*ts = TimestampTZ(t)
	return nil
}
