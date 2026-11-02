package integration

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"strconv"
	"testing"
	"time"
)

// TestOutputStruct selects a row into the OUT fields of a struct in a PL/SQL
// block, and checks the NUMBER(10,2) value (sijms/go-ora#430).
func TestOutputStruct(t *testing.T) {
	var insert = func(db *sql.DB) error {
		type TTBData struct {
			Id   int64     `db:"ID"`
			Name string    `db:"NAME"`
			Val  float64   `db:"VAL"`
			Date time.Time `db:"LDATE"`
		}
		data := make([]TTBData, 100)
		for x := range data {
			data[x].Id = int64(1 + x)
			data[x].Name = "test_" + strconv.Itoa(x)
			data[x].Val = 100.23 + 1
			data[x].Date = time.Now()
		}
		_, err := db.Exec("INSERT INTO TTB_MAIN (ID, NAME, VAL, LDATE) VALUES(:ID, :NAME, :VAL, :LDATE)", data)
		if err != nil {
			return err
		}
		return nil
	}
	var query = func(db *sql.DB) error {
		result := struct {
			Id   int64     `db:"ID,number,,output"`
			Name string    `db:"NAME,,200,output"`
			Val  float64   `db:"VAL,number,,output"`
			Date time.Time `db:"LDATE,,,output"`
		}{}
		_, err := db.Exec(`BEGIN
SELECT ID, NAME, VAL, LDATE INTO :ID, :NAME, :VAL, :LDATE FROM TTB_MAIN WHERE ID = 1;
END;`, &result)
		if err != nil {
			return err
		}
		if result.Val != 101.23 {
			return fmt.Errorf("expected: %f and got: %f", 100.23, result.Val)
		}
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
	err = createMainTable(db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err = dropMainTable(db)
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

// TestNestedPointerBind binds a pointer to a pointer to a string
// (sijms/go-ora#485).
func TestNestedPointerBind(t *testing.T) {
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
	s := "ra"
	ps := &s
	rows, err := db.Query("SELECT 1 FROM DUAL WHERE 'A' = :1", &ps)
	if err != nil {
		t.Fatal(err)
	}
	err = rows.Close()
	if err != nil {
		t.Fatal(err)
	}
}

// ValuerExample holds a time in seconds of Unix time. Its pointer is a
// driver.Valuer that returns the time as a time.Time.
type ValuerExample struct {
	TheDate int64
}

func (ve *ValuerExample) Value() (driver.Value, error) {
	return time.Unix(ve.TheDate, 0), nil
}

// TestValuerBind inserts into a DATE column a value whose pointer type is a
// driver.Valuer that returns a time.Time (sijms/go-ora#662).
func TestValuerBind(t *testing.T) {
	ve := &ValuerExample{
		TheDate: 1744732127,
	}
	var create = func(db *sql.DB) error {
		return execCmd(db, `CREATE TABLE TTB_662(
    THEDATE          date not null
	)`)
	}
	var drop = func(db *sql.DB) error {
		return execCmd(db,
			"DROP TABLE TTB_662",
		)
	}

	var insert = func(db *sql.DB) error {
		_, err := db.Exec("INSERT INTO TTB_662(THEDATE) VALUES(:1)", ve)
		if err != nil {
			return err
		}
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
	err = create(db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err = drop(db)
		if err != nil {
			t.Error(err)
		}
	}()
	err = insert(db)
	if err != nil {
		t.Fatal(err)
	}
}
