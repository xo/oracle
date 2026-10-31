package integration

import (
	"database/sql"
	"github.com/xo/oracle"
	"testing"
)

func TestClientCharsetWithStringArray(t *testing.T) {
	var createPackage = func(db *sql.DB) error {
		return execCmd(db,
			`CREATE OR REPLACE PACKAGE GOORA_TEMP_PKG IS
	TYPE VARCHAR2TABLE_T IS TABLE OF VARCHAR2(32767) INDEX BY BINARY_INTEGER;
	PROCEDURE TEST_PROC(
		STRING_ARRAY_IN IN VARCHAR2TABLE_T,
		STRING_OUT OUT VARCHAR2
	);
END GOORA_TEMP_PKG;`,

			`CREATE OR REPLACE PACKAGE BODY GOORA_TEMP_PKG IS
	PROCEDURE TEST_PROC(
		STRING_ARRAY_IN IN VARCHAR2TABLE_T,
		STRING_OUT OUT VARCHAR2
	) IS
	BEGIN
		FOR i IN STRING_ARRAY_IN.FIRST..STRING_ARRAY_IN.LAST LOOP
			STRING_OUT := STRING_OUT || STRING_ARRAY_IN(i);
		END LOOP;
	END;
END GOORA_TEMP_PKG;`,
		)
	}

	var dropPackage = func(db *sql.DB) error {
		return execCmd(db, "DROP PACKAGE GOORA_TEMP_PKG")
	}
	var stringOut string
	var callProc = func(db *sql.DB, stringIn []string) (string, error) {
		_, err := db.Exec("BEGIN GOORA_TEMP_PKG.TEST_PROC(:1, :2); END;", stringIn,
			oracle.Out{Dest: &stringOut, Size: 256})
		if err != nil {
			return "", err
		}
		return stringOut, nil
	}

	urlOptions["charset"] = "UTF8"
	defer func() {
		delete(urlOptions, "charset")
	}()
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
	var got string
	got, err = callProc(db, []string{"a", "b", "c"})
	if err != nil {
		t.Error(err)
		return
	}
	if got != "abc" {
		t.Errorf("expected: %s and got: %s", "abc", got)
	}
	got, err = callProc(db, []string{"a", "b", "中国人中国人中国人"})
	if err != nil {
		t.Error(err)
		return
	}
	expected := "ab中国人中国人中国人"
	if got != expected {
		t.Errorf("expected: %s and got: %s", expected, got)
	}
}
