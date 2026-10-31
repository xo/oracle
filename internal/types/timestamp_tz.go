package types

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// TimeStampTZ is a time that binds as TIMESTAMP WITH TIME ZONE, where a
// time.Time binds as TIMESTAMP. A nullable one is sql.Null[TimeStampTZ].
type TimeStampTZ time.Time

// Value returns ts itself, which the driver binds as TIMESTAMP WITH TIME
// ZONE. It has a value receiver, so that sql.Null[TimeStampTZ] reaches it.
func (ts TimeStampTZ) Value() (driver.Value, error) {
	return ts, nil
}

// Scan reads a time.Time.
func (ts *TimeStampTZ) Scan(value any) error {
	switch v := value.(type) {
	case TimeStampTZ:
		*ts = v
	case time.Time:
		*ts = TimeStampTZ(v)
	default:
		return fmt.Errorf("scanning %T into a TimeStampTZ", value)
	}
	return nil
}

func (ts TimeStampTZ) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Time(ts))
}

func (ts *TimeStampTZ) UnmarshalJSON(data []byte) error {
	var t time.Time
	if err := json.Unmarshal(data, &t); err != nil {
		return err
	}
	*ts = TimeStampTZ(t)
	return nil
}
