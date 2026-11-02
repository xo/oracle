package integration

import (
	"strings"
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
		// the message is in Arabic, the language of the session, and 26ai
		// names the table in it. The check was the wrong way round, and
		// passed while Error added a line with the position
		// 11g XE has the messages of English alone
		want := "ORA-00942: الجدول"
		var release int
		if db.QueryRow(`SELECT TO_NUMBER(REGEXP_SUBSTR(version, '^[0-9]+'))
			FROM product_component_version WHERE product LIKE 'Oracle%' AND ROWNUM = 1`).Scan(&release); release < 12 {
			want = "ORA-00942: table or view does not exist"
		}
		got := err.Error()
		if !strings.HasPrefix(got, want) || strings.Contains(got, "\n") {
			t.Errorf("got %q, want %q on one line", got, want)
		}
		return
	}
}
