package integration

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

// TestTime binds a time.Time with a fraction of a second into each date type,
// which a TIMESTAMP(9) holds whole. go-ora bound it as a DATE and dropped the
// fraction.
func TestTime(t *testing.T) {
	var createTable = func(db *sql.DB) error {
		return execCmd(db, `
CREATE TABLE TTB_TIME(
    ID NUMBER,
    DATE1 DATE,
    DATE2 TIMESTAMP(9),
    DATE3 TIMESTAMP(9) WITH TIME ZONE,
    DATE4 TIMESTAMP(9) WITH LOCAL TIME ZONE
)`)
	}

	var dropTable = func(db *sql.DB) error { return execCmd(db, `DROP TABLE TTB_TIME PURGE`) }
	var date = time.Now()
	loc, _ := time.LoadLocation("Asia/Shanghai")
	var insert = func(db *sql.DB) error {
		_, err := db.Exec("INSERT INTO TTB_TIME(ID, DATE1, DATE2, DATE3, DATE4) VALUES(:1, :2, :3, :4, :5)",
			1, date, date, date.In(loc), date)
		return err
	}
	var query = func(db *sql.DB) error {
		var (
			id                         int
			date1, date2, date3, date4 time.Time
		)
		err := db.QueryRow("SELECT ID, DATE1, DATE2, DATE3, DATE4 FROM TTB_TIME").Scan(&id, &date1, &date2, &date3, &date4)
		if err != nil {
			return err
		}
		if !isEqualTime(date1, date, false) {
			return fmt.Errorf("date value expected %v and got %v", date, date1)
		}
		if !isEqualTime(date2, date, true) {
			return fmt.Errorf("timestamp value expected %v and got %v", date, date2)
		}
		if !isEqualTime(date3, date.In(loc), true) {
			return fmt.Errorf("timestamp with time zone value expected %v and got %v", date.In(loc), date3)
		}
		if !isEqualTime(date4.In(time.Local), date, true) {
			return fmt.Errorf("timestamp with local time zone expected %v and got %v", date, date4)
		}
		return nil
	}
	db, err := getDB(t)
	if err != nil {
		t.Error(err)
		return
	}
	defer func() {
		err = db.Close()
		if err != nil {
			t.Error(err)
		}
	}()

	err = createTable(db)
	if err != nil {
		t.Error(err)
		return
	}
	defer func() {
		err = dropTable(db)
		if err != nil {
			t.Error(err)
		}
	}()

	err = insert(db)
	if err != nil {
		t.Error(err)
		return
	}
	err = query(db)
	if err != nil {
		t.Error(err)
		return
	}
}

// TestTimestampIntoAny scans a TIMESTAMP column with a default value into an
// any, and logs the value as JSON (sijms/go-ora#287).
func TestTimestampIntoAny(t *testing.T) {
	var createTable = func(db *sql.DB) error {
		return execCmd(db, `CREATE TABLE TTB_287(
	TS TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	}
	var dropTable = func(db *sql.DB) error {
		return execCmd(db, "DROP TABLE TTB_287 PURGE")
	}
	var insert = func(db *sql.DB) error {
		_, err := db.Exec("INSERT INTO TTB_287 VALUES(DEFAULT)")
		return err
	}
	var query = func(db *sql.DB) error {
		rows, err := db.Query("SELECT * FROM TTB_287")
		if err != nil {
			return err
		}
		defer func() {
			err = rows.Close()
			if err != nil {
				t.Error(err)
			}
		}()
		cols, err := rows.Columns()
		if err != nil {
			return err
		}
		values := make([]any, len(cols))
		valuesWrapped := make([]any, 0, len(cols))
		for i := range cols {
			valuesWrapped = append(valuesWrapped, &values[i])
		}
		_ = rows.Next()
		err = rows.Scan(valuesWrapped...)
		if err != nil {
			return err
		}
		jObj := map[string]any{}
		for i, v := range values {
			jObj[cols[i]] = v
		}
		b, _ := json.Marshal(jObj["TS"])
		t.Log(string(b))
		return nil
	}
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err = db.Close()
		if err != nil {
			t.Error(err)
		}
	}()
	err = createTable(db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err = dropTable(db)
		if err != nil {
			t.Error(err)
		}
	}()
	err = insert(db)
	if err != nil {
		t.Fatal(err)
	}
	err = query(db)
	if err != nil {
		t.Fatal(err)
	}
}

// TestCurrentTimes reads CURRENT_DATE, CURRENT_TIMESTAMP, SYSDATE and
// SYSTIMESTAMP as text and as time.Time, and logs them (sijms/go-ora#422).
func TestCurrentTimes(t *testing.T) {
	type TimeDebug struct {
		CurrentDate          string    `db:"CURRENT_DATE"`
		CurrentTimestamp     string    `db:"CURRENT_TIMESTAMP"`
		Sysdate              string    `db:"SYSDATE"`
		Systimestamp         string    `db:"SYSTIMESTAMP"`
		TimeCurrentDate      time.Time `db:"T_CURRENT_DATE"`
		TimeCurrentTimestamp time.Time `db:"T_CURRENT_TIMESTAMP"`
		TimeSysdate          time.Time `db:"T_SYSDATE"`
		TimeSystimestamp     time.Time `db:"T_SYSTIMESTAMP"`
	}
	var dumpJSON = func(d any) {
		b, _ := json.MarshalIndent(d, "", "  ")
		t.Logf("%s\n", string(b))
	}

	var dumpTime = func(r TimeDebug) {
		t.Logf("%16s: %s\n", "Sysdate", r.TimeSysdate.Format(time.UnixDate))
		t.Logf("%16s: %s\n", "Systimestamp", r.TimeSystimestamp.Format(time.UnixDate))
		t.Logf("%16s: %s\n", "Real Time", time.Now().Format(time.UnixDate))
	}

	var query = func(db *sql.DB) error {
		sqlText := `
        SELECT
            to_char(CURRENT_DATE, 'YYYY-MM-DD HH24:MI:SS') "CURRENT_DATE",
            to_char(CURRENT_TIMESTAMP, 'YYYY-MM-DD HH24:MI:SS TZR') "CURRENT_TIMESTAMP",
            to_char(SYSDATE, 'YYYY-MM-DD HH24:MI:SS') "SYSDATE",
            to_char(SYSTIMESTAMP, 'YYYY-MM-DD HH24:MI:SS TZR') "SYSTIMESTAMP",
            CURRENT_DATE as T_CURRENT_DATE,
            CURRENT_TIMESTAMP as T_CURRENT_TIMESTAMP,
            SYSDATE as T_SYSDATE,
            SYSTIMESTAMP as T_SYSTIMESTAMP
         FROM DUAL`
		var r = TimeDebug{}
		err := queryStruct(db.QueryRow(sqlText), &r)
		if err != nil {
			return err
		}
		dumpJSON(r)
		dumpTime(r)
		return nil
	}
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err = db.Close()
		if err != nil {
			t.Error(err)
		}
	}()
	err = query(db)
	if err != nil {
		t.Fatal(err)
	}
}

// queryStruct scans row into the fields of the struct that s points to, in
// the order of the fields.
func queryStruct(row *sql.Row, s any) error {
	sValue := reflect.ValueOf(s)
	if sValue.Kind() != reflect.Pointer {
		return errors.New("you should pass pointer")
	}
	if sValue.IsNil() {
		return errors.New("you should pass non nil value")
	}
	if sValue.Elem().Kind() != reflect.Struct {
		return errors.New("only accept pointer to struct")
	}
	sValue = sValue.Elem()
	fieldCount := sValue.NumField()
	scanValue := make([]any, fieldCount)
	for x := range fieldCount {
		fieldValue := sValue.Field(x)
		scanValue[x] = fieldValue.Addr().Interface()
	}
	return row.Scan(scanValue...)
}
