package integration

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/xo/oracle"
)

func TestClientCharset(t *testing.T) {
	var createPackage = func(db *sql.DB) error {
		return execCmd(db,
			// create package
			`CREATE OR REPLACE PACKAGE GOORA_CHARSET_PKG IS
	TYPE VARCHAR2TABLE_T IS TABLE OF VARCHAR2(32767) INDEX BY BINARY_INTEGER;
	PROCEDURE TEST_PROC(
		STRING_ARRAY_IN IN VARCHAR2TABLE_T,
		STRING_OUT OUT VARCHAR2
	);
END GOORA_CHARSET_PKG;`,

			// create package body
			`CREATE OR REPLACE PACKAGE BODY GOORA_CHARSET_PKG IS
	PROCEDURE TEST_PROC(
		STRING_ARRAY_IN IN VARCHAR2TABLE_T,
		STRING_OUT OUT VARCHAR2
	) IS
	BEGIN
		FOR i IN STRING_ARRAY_IN.FIRST..STRING_ARRAY_IN.LAST LOOP
			STRING_OUT := STRING_OUT || STRING_ARRAY_IN(i);
		END LOOP;
	END;
END GOORA_CHARSET_PKG;`,
		)
	}

	var dropPackage = func(db *sql.DB) error {
		return execCmd(db, "DROP PACKAGE GOORA_CHARSET_PKG")
	}

	var callProc = func(db *sql.DB, stringsIn []string) error {
		var stringOut string
		_, err := db.Exec(`BEGIN GOORA_CHARSET_PKG.TEST_PROC(:1, :2); END;`, stringsIn,
			oracle.Out{Dest: &stringOut, Size: 256})
		if err != nil {
			return err
		}
		expected := strings.Join(stringsIn, "")
		if stringOut != expected {
			return fmt.Errorf("Expected %s and got %s", expected, stringOut)
		}
		return nil
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
	err = callProc(db, []string{"a", "b", "c"})
	if err != nil {
		t.Error(err)
		return
	}
	err = callProc(db, []string{"a", "b", "中国人中国人中国人"})
	if err != nil {
		t.Error(err)
		return
	}

}

// TestKoreanLiteral reads a string literal that holds Korean characters and
// checks it (sijms/go-ora#486).
func TestKoreanLiteral(t *testing.T) {
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
	var value string
	err = db.QueryRow("SELECT 'AAA안녕하세요' FROM dual").Scan(&value)
	if err != nil {
		t.Error(err)
	}
	if value != "AAA안녕하세요" {
		t.Errorf("expected %s and got %s", "AAA안녕하세요", value)
	}
}
