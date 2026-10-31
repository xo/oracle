package integration

import (
	"bytes"
	"testing"

	"github.com/xo/oracle"
)

func TestSessionPars(t *testing.T) {
	db, err := getDB(t,
		oracle.WithSessionParam("cursor_sharing", "force"),
		oracle.WithSessionParam("nls_language", "arabic"))
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
	_, err = db.Exec("INSERT INTO TEST_S(ID, name) VALUES(1, 's')")
	if err != nil {
		expected := []byte{79, 82, 65, 45, 48, 48, 57, 52, 50, 58, 32, 216, 167, 217,
			132, 216, 172, 216, 175, 217, 136, 217, 132, 32, 216, 163, 217, 136, 32,
			216, 167, 217, 132, 216, 172, 216, 175, 217, 136, 217, 132, 32, 216, 167,
			217, 132, 216, 167, 216, 185, 216, 170, 216, 168, 216, 167, 216, 177, 217,
			138, 32, 216, 186, 217, 138, 216, 177, 32, 217, 133, 217, 136, 216, 172, 217,
			136, 216, 175}
		got := []byte(err.Error())
		if bytes.Equal(got, expected) {
			t.Errorf("expected: %v and got: %v", string(expected), err.Error())
		}
		return
	}
}
