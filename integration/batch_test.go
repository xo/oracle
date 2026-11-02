package integration

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cockroachdb/apd/v3"

	"github.com/xo/oracle"
)

// TestBatches runs DML in batches, an Exec whose arguments are slices. It
// holds the cases of tests/cursor/test_1200_executemany.py of python-oracledb
// that a batch of DML covers, written again here. Each names the number of
// its test there. A batch of PL/SQL, which python-oracledb runs, binds its
// slices as PL/SQL arrays in this driver.
func TestBatches(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	db.Exec("DROP TABLE go_batch PURGE")
	if err := execCmd(db, "CREATE TABLE go_batch (id NUMBER(9) PRIMARY KEY, s VARCHAR2(100), n NUMBER)"); err != nil {
		t.Fatal(err)
	}
	defer execCmd(db, "DROP TABLE go_batch PURGE")
	reset := func(t *testing.T) {
		t.Helper()
		if _, err := db.ExecContext(ctx, "TRUNCATE TABLE go_batch"); err != nil {
			t.Fatal(err)
		}
	}
	// rows reads the table as "id:s:n" for each row, in the order of id
	rows := func(t *testing.T) []string {
		t.Helper()
		r, err := db.QueryContext(ctx, "SELECT id, s, TO_CHAR(n) FROM go_batch ORDER BY id")
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		var out []string
		for r.Next() {
			var id int
			var s, n sql.Null[string]
			if err := r.Scan(&id, &s, &n); err != nil {
				t.Fatal(err)
			}
			out = append(out, fmt.Sprintf("%d:%s:%s", id, s.V, n.V))
		}
		if err := r.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}
	ids := func(from, to int) []int {
		var v []int
		for i := from; i < to; i++ {
			v = append(v, i)
		}
		return v
	}
	t.Run("1200 a batch of 250", func(t *testing.T) {
		reset(t)
		if _, err := db.ExecContext(ctx, "INSERT INTO go_batch (id) VALUES (:1)", ids(0, 250)); err != nil {
			t.Fatal(err)
		}
		if got := len(rows(t)); got != 250 {
			t.Errorf("got %d rows", got)
		}
	})
	t.Run("1203 two batches of one statement", func(t *testing.T) {
		reset(t)
		stmt, err := db.PrepareContext(ctx, "INSERT INTO go_batch (id) VALUES (:1)")
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Close()
		for _, batch := range [][]int{ids(0, 50), ids(50, 235)} {
			if _, err := stmt.ExecContext(ctx, batch); err != nil {
				t.Fatal(err)
			}
		}
		if got := len(rows(t)); got != 235 {
			t.Errorf("got %d rows", got)
		}
	})
	t.Run("1205 NULLs, then a string, in the next batch", func(t *testing.T) {
		reset(t)
		stmt, err := db.PrepareContext(ctx, "INSERT INTO go_batch (id, s) VALUES (:1, :2)")
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Close()
		if _, err := stmt.ExecContext(ctx, []int{1, 2}, []sql.Null[string]{{}, {}}); err != nil {
			t.Fatal(err)
		}
		if _, err := stmt.ExecContext(ctx, []int{3, 4}, []sql.Null[string]{{}, {V: "Testing", Valid: true}}); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(rows(t), " "); got != "1:: 2:: 3:: 4:Testing:" {
			t.Errorf("got %s", got)
		}
	})
	t.Run("1206 numbers of several types", func(t *testing.T) {
		reset(t)
		big, _, _ := apd.NewFromString("36893488147419103232")
		dec, _, _ := apd.NewFromString("24.5")
		if _, err := db.ExecContext(ctx, "INSERT INTO go_batch (id, n) VALUES (:1, :2)", []int{1, 2, 3, 4, 5}, []any{5, 7.0, 6.5, big, dec}); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(rows(t), " "); got != "1::5 2::7 3::6.5 4::36893488147419103232 5::24.5" {
			t.Errorf("got %s", got)
		}
	})
	t.Run("a NULL last row", func(t *testing.T) {
		// the batch writes every row with the coder of its last value
		// that is not NULL
		reset(t)
		if _, err := db.ExecContext(ctx, "INSERT INTO go_batch (id, s, n) VALUES (:1, :2, :3)", []int{1, 2}, []any{"a", nil}, []any{6.5, nil}); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(rows(t), " "); got != "1:a:6.5 2::" {
			t.Errorf("got %s", got)
		}
	})
	t.Run("1207 strings that grow", func(t *testing.T) {
		reset(t)
		s := []string{"First", "Second", "Third", "Fourth", "Fifth", "Sixth", "Seventh and the longest one"}
		if _, err := db.ExecContext(ctx, "INSERT INTO go_batch (id, s) VALUES (:1, :2)", ids(1, 8), s); err != nil {
			t.Fatal(err)
		}
		got := rows(t)
		if len(got) != 7 || got[6] != "7:Seventh and the longest one:" {
			t.Errorf("got %v", got)
		}
	})
	t.Run("1208 a key error in a batch", func(t *testing.T) {
		reset(t)
		_, err := db.ExecContext(ctx, "INSERT INTO go_batch (id) VALUES (:1)", []int{1, 2, 3, 2, 5})
		if err == nil || !strings.Contains(err.Error(), "ORA-00001") {
			t.Fatalf("got %v, want ORA-00001", err)
		}
		if got := len(rows(t)); got != 3 {
			t.Errorf("got %d rows, want the 3 before the error", got)
		}
	})
	t.Run("1221 a NULL first, then a number", func(t *testing.T) {
		reset(t)
		if _, err := db.ExecContext(ctx, "INSERT INTO go_batch (id, n) VALUES (:1, :2)", []int{1, 2}, []sql.Null[int64]{{}, {V: 25, Valid: true}}); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(rows(t), " "); got != "1:: 2::25" {
			t.Errorf("got %s", got)
		}
	})
	t.Run("1232 batches whose first holds only NULLs", func(t *testing.T) {
		reset(t)
		stmt, err := db.PrepareContext(ctx, "INSERT INTO go_batch (id, n) VALUES (:1, :2)")
		if err != nil {
			t.Fatal(err)
		}
		defer stmt.Close()
		var want []string
		for pos := 1; pos <= 20; pos += 4 {
			var id []int
			var n []sql.Null[float64]
			for i := pos; i < pos+4; i++ {
				id = append(id, i)
				if i <= 10 {
					n = append(n, sql.Null[float64]{})
					want = append(want, fmt.Sprintf("%d::", i))
				} else {
					n = append(n, sql.Null[float64]{V: float64(i) * 0.25, Valid: true})
					want = append(want, fmt.Sprintf("%d::%g", i, float64(i)*0.25))
				}
			}
			if _, err := stmt.ExecContext(ctx, id, n); err != nil {
				t.Fatalf("the batch from %d: %v", pos, err)
			}
		}
		if got := strings.Join(rows(t), " "); got != strings.Join(want, " ") {
			t.Errorf("got  %s\nwant %s", got, strings.Join(want, " "))
		}
	})
	t.Run("a batch of 100,000", func(t *testing.T) {
		reset(t)
		s := make([]string, 100_000)
		for i := range s {
			s[i] = fmt.Sprint("row ", i)
		}
		if _, err := db.ExecContext(ctx, "INSERT INTO go_batch (id, s) VALUES (:1, :2)", ids(0, len(s)), s); err != nil {
			t.Fatal(err)
		}
		var n int
		var last string
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*), MAX(s) KEEP (DENSE_RANK LAST ORDER BY id) FROM go_batch").Scan(&n, &last); err != nil || n != len(s) || last != "row 99999" {
			t.Errorf("got %d rows and %q, %v", n, last, err)
		}
	})
}

// TestJSONBatch runs a batch of JSON values that shrink, grow and end in a
// NULL. go-ora wrote every row with the locator of the last, which holds its
// length, so the server refused a longer row, and a NULL last row hung the
// connection.
func TestJSONBatch(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	requireRelease(t, db, 21)
	lobTable(t, db, "go_json_batch", "id NUMBER, j JSON")
	short, err := oracle.NewJSON(`{"a":1}`)
	if err != nil {
		t.Fatal(err)
	}
	long, err := oracle.NewJSON(`{"a":"a value much longer than the short one"}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO go_json_batch VALUES (:1, :2)", []int{1, 2, 3, 4}, []any{long, short, long, nil}); err != nil {
		t.Fatal(err)
	}
	r, err := db.Query("SELECT NVL(JSON_SERIALIZE(j), 'NULL') FROM go_json_batch ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var got []string
	for r.Next() {
		var s string
		if err := r.Scan(&s); err != nil {
			t.Fatal(err)
		}
		got = append(got, s)
	}
	want := []string{`{"a":"a value much longer than the short one"}`, `{"a":1}`, `{"a":"a value much longer than the short one"}`, "NULL"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

// TestMergeBatch runs a MERGE in a batch of 500 rows, with NULL in some
// columns of every tenth row. Its USING clause selects a bind from dual, and
// each bind name appears more than once (sijms/go-ora#343).
func TestMergeBatch(t *testing.T) {
	var createTable = func(db *sql.DB) error {
		return execCmd(db, `CREATE TABLE TTB_343 
   (	"ID" NUMBER(20,0) NOT NULL ENABLE, 
		"TM" VARCHAR2(30), 
		"SN" VARCHAR2(25) NOT NULL ENABLE, 
		"CUS" VARCHAR2(20) NOT NULL ENABLE, 
		"AID" VARCHAR2(20), 
		"TR" VARCHAR2(8) NOT NULL ENABLE, 
		"PID" VARCHAR2(20), 
		"CODE" VARCHAR2(20) NOT NULL ENABLE, 
		"TTNO" VARCHAR2(20), 
		"UPDATETIME" TIMESTAMP (6) DEFAULT systimestamp, 
		 PRIMARY KEY ("ID")
	)`)
	}
	var dropTable = func(db *sql.DB) error {
		return execCmd(db, "DROP TABLE TTB_343 PURGE")
	}
	var merge = func(db *sql.DB) error {
		sqlText := `MERGE INTO TTB_343 t1 USING(select :ID ID from dual) tmp ON (tmp.ID=t1.ID) 
    WHEN MATCHED THEN UPDATE SET TM=:TM,SN=:SN,CUS=:CUS, AID=:AID,TR=:TR,PID=:PID,CODE=:CODE,TTNO=:TTNO,UPDATETIME=:UPDATETIME WHERE t1.ID=:ID AND t1.UPDATETIME<=:UPDATETIME 
    WHEN NOT MATCHED THEN INSERT (ID,TM,SN,CUS,AID,TR,PID,CODE,TTNO,UPDATETIME) VALUES (:ID,:TM,:SN,:CUS,:AID,:TR,:PID,:CODE,:TTNO,:UPDATETIME)`
		length := 500
		type TestShort struct {
			Id         int            `db:"ID"`
			Tm         sql.NullString `db:"TM"`
			Sn         string         `db:"SN"`
			Cus        string         `db:"CUS"`
			Aid        sql.NullString `db:"AID"`
			Tr         string         `db:"TR"`
			Pid        string         `db:"PID"`
			Code       string         `db:"CODE"`
			Ttno       sql.NullString `db:"TTNO"`
			UpdateTime time.Time      `db:"UPDATETIME,timestamp"`
		}
		data := make([]TestShort, length)
		for index := range data {
			data[index].Id = index + 1
			if index > 0 && index%10 == 0 {
				data[index].Tm = sql.NullString{Valid: false}
				data[index].Aid = sql.NullString{Valid: false}
				data[index].Ttno = sql.NullString{Valid: false}
			} else {
				data[index].Tm = sql.NullString{String: "tm text", Valid: true}
				data[index].Aid = sql.NullString{String: "aid text", Valid: true}
				data[index].Ttno = sql.NullString{String: "ttno text", Valid: true}
			}
			data[index].Sn = "sn text"
			data[index].Cus = "cus text"
			data[index].Tr = "tr text"
			data[index].Pid = "pid text"
			data[index].Code = "code text"
			data[index].UpdateTime = time.Now()
		}
		_, err := db.Exec(sqlText, data)
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
	err = merge(db)
	if err != nil {
		t.Fatal(err)
	}
}

// TestBatchLargeID inserts a batch of 100 rows with IDs from 1,000,000,000,
// and reads the first ID back (sijms/go-ora#429).
func TestBatchLargeID(t *testing.T) {
	type TTBData struct {
		Id   int64     `db:"ID"`
		Name string    `db:"NAME"`
		Val  float64   `db:"VAL"`
		Date time.Time `db:"LDATE"`
	}
	var insert = func(db *sql.DB) error {
		data := make([]TTBData, 100)
		for x := range data {
			data[x].Id = int64(1000000000 + x)
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
		var id int
		err := db.QueryRow("SELECT ID FROM TTB_MAIN WHERE NAME = :1", "test_0").Scan(&id)
		if err != nil {
			return err
		}
		if id != 1000000000 {
			return fmt.Errorf("expected: %d and got: %d", 1000000000, id)
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

// TestBatchPreparedSizes runs one prepared INSERT with batches of 10, 10, 5,
// 10 and 10 rows, and checks the count of rows that each inserts
// (sijms/go-ora#466).
func TestBatchPreparedSizes(t *testing.T) {
	var createTable = func(db *sql.DB) error {
		return execCmd(db, `CREATE TABLE TTB_466(ID varchar2(100))`)
	}
	var dropTable = func(db *sql.DB) error {
		return execCmd(db, `DROP TABLE TTB_466 purge`)
	}
	var generateRows = func(startIndex, count int) []string {
		rows := make([]string, count)
		for i := range count {
			rows[i] = fmt.Sprintf("ID%02d", i+startIndex)
		}
		return rows
	}
	var insert = func(stmt *sql.Stmt, rows []string) error {
		result, err := stmt.Exec(rows)
		if err != nil {
			return err
		}
		inserted, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if int(inserted) != len(rows) {
			return fmt.Errorf("expected to insert: %d but the actual insert: %d", len(rows), inserted)
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
	stmt, err := db.Prepare("INSERT INTO TTB_466 (ID) VALUES (:1)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err = stmt.Close()
		if err != nil {
			t.Error(err)
		}
	}()
	err = insert(stmt, generateRows(1, 10))
	if err != nil {
		t.Fatal(err)
	}
	err = insert(stmt, generateRows(11, 10))
	if err != nil {
		t.Fatal(err)
	}
	err = insert(stmt, generateRows(21, 5))
	if err != nil {
		t.Fatal(err)
	}
	err = insert(stmt, generateRows(26, 10))
	if err != nil {
		t.Fatal(err)
	}
	err = insert(stmt, generateRows(36, 10))
	if err != nil {
		t.Fatal(err)
	}
}
