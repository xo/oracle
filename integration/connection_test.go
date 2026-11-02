package integration

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

// TestConnQueryAfterPing pings the server and then runs a query twice on one
// connection, two seconds apart (sijms/go-ora#213). When the ping fails, the
// test logs the error and ends.
func TestConnQueryAfterPing(t *testing.T) {
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

	ctx1 := t.Context()

	ctx2, cancel := context.WithTimeout(ctx1, 3*time.Second)
	if err = db.PingContext(ctx2); err != nil {
		t.Log(err)
		cancel()
		return
	}
	cancel()
	t.Log("Successfully connected.")

	conn, err := db.Conn(ctx1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err = conn.Close()
		if err != nil {
			t.Error(err)
		}
	}()

	val := 0

	r := conn.QueryRowContext(ctx1, "select 1 from dual where 1 = 1")
	err = r.Scan(&val)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("1 done")
	time.Sleep(2 * time.Second)

	r = conn.QueryRowContext(ctx1, "select 1 from dual where 1 = 1")
	err = r.Scan(&val)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("2 done")
}

// TestBatchAfterTimeout ends a PL/SQL sleep with a context timeout, and then
// inserts a batch of 100,000 rows and queries them with a timeout on the same
// pool (sijms/go-ora#274).
func TestBatchAfterTimeout(t *testing.T) {
	var createTable = func(db *sql.DB) error {
		return execCmd(db, `CREATE TABLE TTB_274(
    ID NUMBER(10) NOT NULL,
    NAME VARCHAR(200),
    VAL NUMBER(10, 2),
    LDATE DATE,
    PRIMARY KEY (ID)
    )`)
	}
	var dropTable = func(db *sql.DB) error {
		return execCmd(db, "DROP TABLE TTB_274 PURGE")
	}
	var dbLock = func(db *sql.DB) error {
		execCtx, execCancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer execCancel()
		_, err := db.ExecContext(execCtx, "begin DBMS_LOCK.sleep(5); end;")
		return err
	}
	var insert = func(db *sql.DB, rowNum int) error {
		type TTB274 struct {
			ID   int       `db:"ID"`
			Name string    `db:"NAME"`
			Val  float64   `db:"VAL"`
			Date time.Time `db:"LDATE"`
		}
		interval := 1.1
		data := make([]TTB274, rowNum)
		for index := range data {
			data[index].ID = index + 1
			data[index].Name = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
			data[index].Val = float64(index) + interval
			data[index].Date = time.Now()
		}
		_, err := db.Exec("INSERT INTO TTB_274(ID, NAME, VAL, LDATE) VALUES(:ID, :NAME, :VAL, :LDATE)", data)
		if err != nil {
			return err
		}
		return nil
	}
	var query = func(db *sql.DB, rowNum int) error {
		execCtx, execCancel := context.WithTimeout(t.Context(), 4*time.Second)
		defer execCancel()
		rows, err := db.QueryContext(execCtx, `SELECT ID, NAME, VAL, LDATE FROM TTB_274 WHERE ID < :1 ORDER BY ID`, rowNum)
		if err != nil {
			return err
		}
		defer func() {
			err = rows.Close()
			if err != nil {
				t.Error(err)
			}
		}()
		var id int
		var name string
		var val float64
		var date time.Time
		for rows.Next() {
			err = rows.Scan(&id, &name, &val, &date)
			if err != nil {
				return err
			}
		}
		return rows.Err()
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
	err = dbLock(db)
	if err != nil {
		t.Log(err)
	}
	err = insert(db, 100000)
	if err != nil {
		t.Fatal(err)
	}
	err = query(db, 10000)
	if err != nil {
		t.Fatal(err)
	}
}

// TestFastLoginReconnect pings three times with no idle connection in the
// pool, so that each ping opens a new connection (sijms/go-ora#732). Fast
// login replays a cookie on each connection after the first. In go-ora the
// second ping failed with io.EOF, because a 23c server refused the cookie.
func TestFastLoginReconnect(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)

	ctx := t.Context()
	for i := 1; i <= 3; i++ {
		err = db.PingContext(ctx)
		if err != nil {
			t.Fatalf("ping #%d: %v", i, err)
		}
	}
}
