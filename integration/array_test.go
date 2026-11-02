package integration

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/xo/oracle"
)

func TestArray(t *testing.T) {
	var expectedTime = time.Now()
	var insert = func(db *sql.DB) error {
		sqlText := `INSERT INTO TTB_MAIN(ID, NAME, VAL, LDATE, DATA) VALUES(:ID, :NAME, :VAL, :LDATE, :DATA)`
		length := 10
		type TempStruct struct {
			Id   int             `db:"ID"`
			Name sql.NullString  `db:"NAME"`
			Val  sql.NullFloat64 `db:"VAL"`
			Date sql.NullTime    `db:"LDATE"`
			Data []byte          `db:"DATA"`
		}
		args := make([]TempStruct, length)
		for x := range length {

			args[x] = TempStruct{
				Id:   x + 1,
				Name: sql.NullString{String: strings.Repeat("*", 20), Valid: true},
				Val:  sql.NullFloat64{Float64: float64(length) / float64(x+1), Valid: true},
				Date: sql.NullTime{Time: expectedTime, Valid: true},
				Data: bytes.Repeat([]byte{55}, 20),
			}
			if x == 2 {
				args[x].Name.Valid = false
				args[x].Val.Valid = false
				args[x].Date.Valid = false
			}
		}
		_, err := db.Exec(sqlText, args)
		return err
	}

	var createPackage = func(db *sql.DB) error {
		sqlText := `create or replace package GOORA_ARRAY_PKG as
		type t_visit_id is table of TTB_MAIN.id%type index by binary_integer;
	   type t_visit_name is table of TTB_MAIN.name%type index by binary_integer;
		type t_visit_val is table of TTB_MAIN.val%type index by binary_integer;
	   type t_visit_date is table of TTB_MAIN.ldate%type index by binary_integer;
	
		procedure test_get1(p_visit_id t_visit_id, l_cursor out SYS_REFCURSOR);
	   procedure test_get2(p_visit_id t_visit_id, p_visit_name out t_visit_name,
	       p_visit_val out t_visit_val, p_visit_date out t_visit_date);
		procedure test_empty_array(p_visit_id t_visit_id, p_visit_name out t_visit_name,
	       p_visit_val out t_visit_val, p_visit_date out t_visit_date);
	end GOORA_ARRAY_PKG;
	`
		err := execCmd(db, sqlText)
		if err != nil {
			return err
		}
		sqlText = `create or replace PACKAGE BODY GOORA_ARRAY_PKG as
		procedure test_get1(p_visit_id t_visit_id, l_cursor out SYS_REFCURSOR) as
			temp t_visit_id := p_visit_id;
		begin
			OPEN l_cursor for select id, name, val, ldate from TTB_MAIN
			    where id in (select column_value from table(temp));
		end test_get1;
	
	   procedure test_get2(p_visit_id t_visit_id, p_visit_name out t_visit_name,
	       p_visit_val out t_visit_val, p_visit_date out t_visit_date) as
	       temp t_visit_id := p_visit_id;
	       cursor tempCur is select id, name, val, ldate from TTB_MAIN
	           where id in (select column_value from table(temp));
	       tempRow tempCur%rowtype;
	       idx number := 1;
	   begin
	       for tempRow in tempCur loop
	           p_visit_name(idx) := tempRow.name;
	           p_visit_val(idx) := tempRow.val;
	           p_visit_date(idx) := tempRow.ldate;
	           idx := idx + 1;
	       end loop;
	   end test_get2;
	
		procedure test_empty_array(p_visit_id t_visit_id, p_visit_name out t_visit_name,
			p_visit_val out t_visit_val, p_visit_date out t_visit_date) as
		BEGIN
			NULL;
		END test_empty_array;
	end GOORA_ARRAY_PKG;
	`
		return execCmd(db, sqlText)
	}

	var dropPackage = func(db *sql.DB) error {
		return execCmd(db, "drop package GOORA_ARRAY_PKG")
	}

	var query1 = func(db *sql.DB) error {
		var cursor oracle.RefCursor
		// sql code take input array of integer and return a cursor that can be queried for result
		_, err := db.Exec(`BEGIN GOORA_ARRAY_PKG.TEST_GET1(:1, :2); END;`, []int64{1, 3, 5}, sql.Out{Dest: &cursor})
		if err != nil {
			return err
		}
		rows, err := oracle.WrapRefCursor(context.Background(), db, &cursor)
		if err != nil {
			return err
		}
		defer func() {
			_ = rows.Close()
		}()
		var (
			id   int64
			name sql.NullString
			val  sql.NullFloat64
			date sql.NullTime
		)
		for rows.Next() {
			err = rows.Scan(&id, &name, &val, &date)
			if err != nil {
				return err
			}
			if id == 1 {
				if name.String != strings.Repeat("*", 20) {
					return fmt.Errorf("expected name to be %s, got %s at id: %d", strings.Repeat("*", 20), name.String, id)
				}
				if val.Float64 != 10.0 {
					return fmt.Errorf("expected val to be %f, got %f at id: %d", 10.0, val.Float64, id)
				}
				if !isEqualTime(date.Time, expectedTime, false) {
					return fmt.Errorf("expected date to be %v, and got %v at id: %d", expectedTime, date.Time, id)
				}
			}
			if id == 3 {
				if name.Valid {
					return fmt.Errorf("expected name to be null, got %s at id: %d", name.String, id)
				}
				if val.Valid {
					return fmt.Errorf("expected val to be null, got %f at id: %d", val.Float64, id)
				}
				if date.Valid {
					return fmt.Errorf("expected date to be null, and got %v at id: %d", date.Time, id)
				}
			}
			if id == 5 {
				if name.String != strings.Repeat("*", 20) {
					return fmt.Errorf("expected name to be %s, got %s at id: %d", strings.Repeat("*", 20), name.String, id)
				}
				if val.Float64 != 2.0 {
					return fmt.Errorf("expected val to be %f, got %f at id: %d", 2.0, val.Float64, id)
				}
				if !isEqualTime(date.Time, expectedTime, false) {
					return fmt.Errorf("expected date to be %v, and got %v at id: %d", expectedTime, date.Time, id)
				}
			}

			t.Log("ID: ", id, "\tName: ", name.String, "\tVal: ", val.Float64, "\tDate: ", date.Time)
		}
		return rows.Err()
	}

	var query2 = func(db *sql.DB) error {
		var (
			nameArray []sql.NullString
			valArray  []sql.NullFloat64
			dateArray []sql.NullTime
		)
		// note size here is important and equal to max number of items that array can accommodate
		_, err := db.Exec(`BEGIN GOORA_ARRAY_PKG.TEST_GET2(:1, :2, :3, :4); END;`,
			[]int{1, 3, 4, 5, 7, 8}, oracle.Out{Dest: &nameArray, Size: 10},
			oracle.Out{Dest: &valArray, Size: 10},
			oracle.Out{Dest: &dateArray, Size: 10})
		if err != nil {
			return err
		}
		expectedString := sql.NullString{String: strings.Repeat("*", 20), Valid: true}

		if nameArray[0] != expectedString {
			return fmt.Errorf("expected name %s, got %s at position 0", expectedString.String, nameArray[0].String)
		}
		if nameArray[1].Valid {
			return fmt.Errorf("expected name to be null at position 1, got %s", nameArray[1].String)
		}
		if nameArray[2] != expectedString {
			return fmt.Errorf("expected name %s, got %s at position 2", expectedString.String, nameArray[2].String)
		}
		if nameArray[3] != expectedString {
			return fmt.Errorf("expected name %s, got %s at position 3", expectedString.String, nameArray[3].String)
		}
		if nameArray[4] != expectedString {
			return fmt.Errorf("expected name %s, got %s at position 4", expectedString.String, nameArray[4].String)
		}
		if nameArray[5] != expectedString {
			return fmt.Errorf("expected name %s, got %s at position 5", expectedString.String, nameArray[5].String)
		}

		if valArray[0].Float64 != 10.0 {
			return fmt.Errorf("expected val %v and got %v at position: 0", 10.0, valArray[0].Float64)
		}
		if valArray[1].Valid {
			return fmt.Errorf("expected null val at position 1 got: %v", valArray[1].Valid)
		}
		if valArray[2].Float64 != 2.5 {
			return fmt.Errorf("expected val %v and got %v at position: 2", 2.5, valArray[2].Float64)
		}
		if valArray[3].Float64 != 2.0 {
			return fmt.Errorf("expected val %v and got %v at position: 3", 2.0, valArray[3].Float64)
		}
		if valArray[4].Float64 != 1.43 {
			return fmt.Errorf("expected val %v and got %v at position: 4", 1.43, valArray[4].Float64)
		}
		if valArray[5].Float64 != 1.25 {
			return fmt.Errorf("expected val %v and got %v at position: 5", 1.25, valArray[5].Float64)
		}

		if !isEqualTime(dateArray[0].Time, expectedTime, false) {
			return fmt.Errorf("expected date %v, got %v at position: 0", expectedTime, dateArray[0].Time)
		}
		if dateArray[1].Valid {
			return fmt.Errorf("expected null date at position 1 got: %v", dateArray[1].Valid)
		}
		if !isEqualTime(dateArray[2].Time, expectedTime, false) {
			return fmt.Errorf("expected date: %v, got %v at position: 2", expectedTime, dateArray[2].Time)
		}
		if !isEqualTime(dateArray[3].Time, expectedTime, false) {
			return fmt.Errorf("expected date: %v, got %v at position: 3", expectedTime, dateArray[3].Time)
		}
		if !isEqualTime(dateArray[4].Time, expectedTime, false) {
			return fmt.Errorf("expected date: %v, got %v at position: 4", expectedTime, dateArray[4].Time)
		}
		if !isEqualTime(dateArray[5].Time, expectedTime, false) {
			return fmt.Errorf("expected date: %v, got %v at position: 5", expectedTime, dateArray[5].Time)
		}
		return nil
	}

	var query3 = func(db *sql.DB) error {
		var (
			nameArray []sql.NullString
			valArray  []sql.NullFloat64
			dateArray []sql.NullTime
		)
		// note size here is important and equal to max number of items that array can accommodate
		_, err := db.Exec(`BEGIN GOORA_ARRAY_PKG.test_empty_array(:1, :2, :3, :4); END;`,
			[]int{1, 3, 5}, oracle.Out{Dest: &nameArray, Size: 10},
			oracle.Out{Dest: &valArray, Size: 10},
			oracle.Out{Dest: &dateArray, Size: 10})
		if err != nil {
			return err
		}
		if len(nameArray) != 0 {
			return fmt.Errorf("expected empty array for name value and got: %d array size", len(nameArray))
		}
		if len(valArray) != 0 {
			return fmt.Errorf("expected empty array for numeric value and got: %d array size", len(valArray))
		}
		if len(dateArray) != 0 {
			return fmt.Errorf("expected empty array for date value and got: %d array size", len(dateArray))
		}
		return nil
	}

	db, err := getDB(t)
	if err != nil {
		t.Error(err)
		return
	}
	// TABLE() over an index-by table needs 12c
	requireRelease(t, db, 12)
	defer func() {
		err = db.Close()
		if err != nil {
			t.Error(err)
		}
	}()
	err = createMainTable(db)
	if err != nil {
		t.Error(err)
		return
	}
	defer func() {
		err = dropMainTable(db)
		if err != nil {
			t.Error(err)
		}
	}()

	err = createPackage(db)
	if err != nil {
		t.Error(err)
		return
	}
	defer func() {
		err = dropPackage(db)
		if err != nil {
			t.Error(err)
		}
	}()
	err = insert(db)
	if err != nil {
		t.Error(err)
		return
	}
	err = query1(db)
	if err != nil {
		t.Error(err)
		return
	}
	err = query2(db)
	if err != nil {
		t.Error(err)
		return
	}
	err = query3(db)
	if err != nil {
		t.Error(err)
		return
	}
}

// TestBlankStringArrays calls procedures with empty and blank strings: as a
// string, as PL/SQL arrays of string, *string and sql.NullString, and as an
// empty and a one byte RAW (sijms/go-ora#355).
func TestBlankStringArrays(t *testing.T) {
	var createPackage = func(db *sql.DB) error {
		return execCmd(db, `CREATE OR REPLACE PACKAGE GOORA_355_PKG IS
	TYPE VARCHAR2TABLE_T IS TABLE OF VARCHAR2(32767) INDEX BY BINARY_INTEGER;
	PROCEDURE TEST_PROC_STRING(
		STRING_IN IN VARCHAR2
	);
	PROCEDURE TEST_PROC_STRINGARRAY(
		STRINGARRAY_IN IN VARCHAR2TABLE_T
	);
    PROCEDURE TEST_PROC_STRINGARRAY2(
        STRINGARRAY_IN IN VARCHAR2TABLE_T, P_OUTPUT OUT number 
    );
	PROCEDURE TEST_PROC_BYTEARRAY(
		BYTEARRAY_IN IN RAW
	);
END GOORA_355_PKG;`,
			`CREATE OR REPLACE PACKAGE BODY GOORA_355_PKG IS
	PROCEDURE TEST_PROC_STRING(
		STRING_IN IN VARCHAR2
	) IS
	BEGIN
		NULL;
	END;
	PROCEDURE TEST_PROC_STRINGARRAY(
		STRINGARRAY_IN IN VARCHAR2TABLE_T
	) IS
	BEGIN
		NULL;
	END;
    PROCEDURE TEST_PROC_STRINGARRAY2(
		STRINGARRAY_IN IN VARCHAR2TABLE_T,  P_OUTPUT OUT number
	) IS
	BEGIN
		P_OUTPUT := STRINGARRAY_IN.COUNT;
		--FOR X IN 1..STRINGARRAY_IN.COUNT LOOP
		--	P_OUTPUT := nvl(P_OUTPUT, '') || nvl(STRINGARRAY_IN(X), '');
		--END LOOP;
	END;
	PROCEDURE TEST_PROC_BYTEARRAY(
		BYTEARRAY_IN IN RAW
	) IS
	BEGIN
		NULL;
	END;
END GOORA_355_PKG;`)
	}
	var dropPackage = func(db *sql.DB) error {
		return execCmd(db, "DROP PACKAGE GOORA_355_PKG")
	}
	var callString = func(db *sql.DB, input string) error {
		_, err := db.Exec("BEGIN GOORA_355_PKG.TEST_PROC_STRING(:1); END;", input)
		return err
	}
	var callStringArray = func(db *sql.DB, input []string) error {
		var output sql.NullInt64
		_, err := db.Exec("BEGIN GOORA_355_PKG.TEST_PROC_STRINGARRAY2(:1, :2); END;", input, oracle.Out{Dest: &output})
		return err
	}
	var callStringPointerArray = func(db *sql.DB, input []*string) error {
		_, err := db.Exec("BEGIN GOORA_355_PKG.TEST_PROC_STRINGARRAY(:1); END;", input)
		return err
	}
	var callSQLNullStringArray = func(db *sql.DB, input []sql.NullString) error {
		_, err := db.Exec(`BEGIN GOORA_355_PKG.TEST_PROC_STRINGARRAY(:1); END;`, input)
		return err
	}
	var callByteArray = func(db *sql.DB, input []byte) error {
		_, err := db.Exec(`BEGIN GOORA_355_PKG.TEST_PROC_BYTEARRAY(:1); END;`, input)
		return err
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
	err = createPackage(db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err = dropPackage(db)
		if err != nil {
			t.Error(err)
		}
	}()
	err = callString(db, "")
	if err != nil {
		t.Fatal(err)
	}
	err = callString(db, "  ")
	if err != nil {
		t.Fatal(err)
	}
	err = callStringArray(db, []string{})
	if err != nil {
		t.Fatal(err)
	}
	err = callStringArray(db, []string{""})
	if err != nil {
		t.Fatal(err)
	}
	err = callStringArray(db, []string{" "})
	if err != nil {
		t.Fatal(err)
	}
	err = callStringArray(db, []string{"", ""})
	if err != nil {
		t.Fatal(err)
	}
	err = callStringArray(db, []string{"", " "})
	if err != nil {
		t.Fatal(err)
	}
	err = callStringArray(db, []string{" ", ""})
	if err != nil {
		t.Fatal(err)
	}
	err = callStringPointerArray(db, []*string{})
	if err != nil {
		t.Fatal(err)
	}
	s0 := ""
	s1 := " "
	err = callStringPointerArray(db, []*string{&s0})
	if err != nil {
		t.Fatal(err)
	}
	err = callStringPointerArray(db, []*string{&s1})
	if err != nil {
		t.Fatal(err)
	}
	err = callStringPointerArray(db, []*string{&s0, &s0})
	if err != nil {
		t.Fatal(err)
	}
	err = callStringPointerArray(db, []*string{&s0, &s1})
	if err != nil {
		t.Fatal(err)
	}
	err = callStringPointerArray(db, []*string{&s1, &s0})
	if err != nil {
		t.Fatal(err)
	}
	err = callSQLNullStringArray(db, []sql.NullString{})
	if err != nil {
		t.Fatal(err)
	}
	err = callSQLNullStringArray(db, []sql.NullString{{Valid: false}})
	if err != nil {
		t.Fatal(err)
	}
	err = callSQLNullStringArray(db, []sql.NullString{{Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	err = callSQLNullStringArray(db, []sql.NullString{{String: " ", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	err = callSQLNullStringArray(db, []sql.NullString{{String: "", Valid: true}, {String: "", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	err = callSQLNullStringArray(db, []sql.NullString{{String: "", Valid: true}, {String: " ", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	err = callSQLNullStringArray(db, []sql.NullString{{String: " ", Valid: true}, {String: "", Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	err = callByteArray(db, []byte{})
	if err != nil {
		t.Fatal(err)
	}
	err = callByteArray(db, []byte{0x0})
	if err != nil {
		t.Fatal(err)
	}
}
