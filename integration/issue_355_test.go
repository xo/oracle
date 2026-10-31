package integration

import (
	"database/sql"
	"testing"

	"github.com/xo/oracle"
)

func TestIssue355(t *testing.T) {
	var createPackage = func(db *sql.DB) error {
		return execCmd(db, `CREATE OR REPLACE PACKAGE GOORA_TEMP IS
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
END GOORA_TEMP;`,
			`CREATE OR REPLACE PACKAGE BODY GOORA_TEMP IS
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
END GOORA_TEMP;`)
	}
	var dropPackage = func(db *sql.DB) error {
		return execCmd(db, "DROP PACKAGE GOORA_TEMP")
	}
	var callString = func(db *sql.DB, input string) error {
		_, err := db.Exec("BEGIN GOORA_TEMP.TEST_PROC_STRING(:1); END;", input)
		return err
	}
	var callStringArray = func(db *sql.DB, input []string) error {
		var output sql.NullInt64
		_, err := db.Exec("BEGIN GOORA_TEMP.TEST_PROC_STRINGARRAY2(:1, :2); END;", input, oracle.Out{Dest: &output})
		return err
	}
	var callStringPointerArray = func(db *sql.DB, input []*string) error {
		_, err := db.Exec("BEGIN GOORA_TEMP.TEST_PROC_STRINGARRAY(:1); END;", input)
		return err
	}
	var callSQLNullStringArray = func(db *sql.DB, input []sql.NullString) error {
		_, err := db.Exec(`BEGIN GOORA_TEMP.TEST_PROC_STRINGARRAY(:1); END;`, input)
		return err
	}
	var callByteArray = func(db *sql.DB, input []byte) error {
		_, err := db.Exec(`BEGIN GOORA_TEMP.TEST_PROC_BYTEARRAY(:1); END;`, input)
		return err
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
	err = callString(db, "")
	if err != nil {
		t.Error(err)
		return
	}
	err = callString(db, "  ")
	if err != nil {
		t.Error(err)
		return
	}
	err = callStringArray(db, []string{})
	if err != nil {
		t.Error(err)
		return
	}
	err = callStringArray(db, []string{""})
	if err != nil {
		t.Error(err)
		return
	}
	err = callStringArray(db, []string{" "})
	if err != nil {
		t.Error(err)
		return
	}
	err = callStringArray(db, []string{"", ""})
	if err != nil {
		t.Error(err)
		return
	}
	err = callStringArray(db, []string{"", " "})
	if err != nil {
		t.Error(err)
		return
	}
	err = callStringArray(db, []string{" ", ""})
	if err != nil {
		t.Error(err)
		return
	}
	err = callStringPointerArray(db, []*string{})
	if err != nil {
		t.Error(err)
		return
	}
	s0 := ""
	s1 := " "
	err = callStringPointerArray(db, []*string{&s0})
	if err != nil {
		t.Error(err)
		return
	}
	err = callStringPointerArray(db, []*string{&s1})
	if err != nil {
		t.Error(err)
		return
	}
	err = callStringPointerArray(db, []*string{&s0, &s0})
	if err != nil {
		t.Error(err)
		return
	}
	err = callStringPointerArray(db, []*string{&s0, &s1})
	if err != nil {
		t.Error(err)
		return
	}
	err = callStringPointerArray(db, []*string{&s1, &s0})
	if err != nil {
		t.Error(err)
		return
	}
	err = callSQLNullStringArray(db, []sql.NullString{})
	if err != nil {
		t.Error(err)
		return
	}
	err = callSQLNullStringArray(db, []sql.NullString{{Valid: false}})
	if err != nil {
		t.Error(err)
		return
	}
	err = callSQLNullStringArray(db, []sql.NullString{{Valid: true}})
	if err != nil {
		t.Error(err)
		return
	}
	err = callSQLNullStringArray(db, []sql.NullString{{String: " ", Valid: true}})
	if err != nil {
		t.Error(err)
		return
	}
	err = callSQLNullStringArray(db, []sql.NullString{{String: "", Valid: true}, {String: "", Valid: true}})
	if err != nil {
		t.Error(err)
		return
	}
	err = callSQLNullStringArray(db, []sql.NullString{{String: "", Valid: true}, {String: " ", Valid: true}})
	if err != nil {
		t.Error(err)
		return
	}
	err = callSQLNullStringArray(db, []sql.NullString{{String: " ", Valid: true}, {String: "", Valid: true}})
	if err != nil {
		t.Error(err)
		return
	}
	err = callByteArray(db, []byte{})
	if err != nil {
		t.Error(err)
		return
	}
	err = callByteArray(db, []byte{0x0})
	if err != nil {
		t.Error(err)
		return
	}
}
