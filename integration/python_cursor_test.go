package integration

import (
	"database/sql"
	"testing"
)

// TestBindNames binds by name in statements whose text held a trap for the
// parser of go-ora: a colon in a q-string, the colon of a JSON constant, a
// name that is not ASCII and a quoted name. It follows
// tests/cursor/test_2400_sql_parser.py of python-oracledb, which only reads
// the names, and runs each statement on the server. The binds go in the
// reverse order of the text, so each one must find its place by its name.
func TestBindNames(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, test := range []struct {
		name    string
		release int
		sql     string
		args    []any
		want    string
	}{
		{"q-string", 0, "SELECT :a || q'[it's :x]' || :b FROM dual",
			[]any{sql.Named("b", "2"), sql.Named("a", "1")}, "1it's :x2"},
		{"national q-string", 0, "SELECT :a || nq'{:x}' || :b FROM dual",
			[]any{sql.Named("b", "2"), sql.Named("a", "1")}, "1:x2"},
		{"comments", 0, "SELECT /* :x ' */ :a -- :y \"\n || :b FROM dual",
			[]any{sql.Named("b", "2"), sql.Named("a", "1")}, "12"},
		{"not ASCII", 0, "SELECT :méil$ || :b FROM dual",
			[]any{sql.Named("b", "2"), sql.Named("méil$", "1")}, "12"},
		{"quoted", 0, `SELECT :"VaLue_2" || :b FROM dual`,
			[]any{sql.Named("b", "2"), sql.Named("VaLue_2", "1")}, "12"},
		{"a name twice", 0, "SELECT :a || :b || :a FROM dual",
			[]any{sql.Named("b", "2"), sql.Named("a", "1")}, "121"},
		{"JSON constant", 19, "SELECT JSON_SERIALIZE(JSON_OBJECT('k':dummy)) || :a || :b FROM dual",
			[]any{sql.Named("b", "2"), sql.Named("a", "1")}, `{"k":"X"}12`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.release != 0 {
				requireRelease(t, db, test.release)
			}
			var got string
			if err := db.QueryRowContext(t.Context(), test.sql, test.args...).Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

// TestCallBindNames runs CALL with an OUT, by position and by name, and with
// a name that appears twice. go-ora read CALL as a statement that is neither
// SQL nor PL/SQL, so a CALL with an OUT failed with ORA-03146 and the server
// then closed the connection with ORA-03138. It bound a name that appears
// twice once, which the server refused with ORA-01008. The server takes a
// bind for each place in CALL and sends the direction of each name once.
func TestCallBindNames(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := execCmd(db, `CREATE OR REPLACE PROCEDURE go_call_names (a NUMBER, b NUMBER, c OUT NUMBER) AS
BEGIN
	c := a + b;
END;`); err != nil {
		t.Fatal(err)
	}
	defer execCmd(db, "DROP PROCEDURE go_call_names")
	for _, test := range []struct {
		sql  string
		args []any
		want int
	}{
		{"CALL go_call_names(:1, :2, :3)", []any{2, 3}, 5},
		{"call go_call_names(:1, :1, :2)", []any{2, 2}, 4},
		{"CALL go_call_names(:x, :z, :y)", []any{sql.Named("x", 2), sql.Named("z", 4)}, 6},
		{"CALL go_call_names(:x, :x, :y)", []any{sql.Named("x", 3)}, 6},
	} {
		var n int
		out := sql.Out{Dest: &n}
		args := append(test.args, out)
		if len(test.args) > 0 {
			if _, ok := test.args[0].(sql.NamedArg); ok {
				args = append([]any{sql.Named("y", out)}, test.args...)
			}
		}
		if _, err := db.ExecContext(t.Context(), test.sql, args...); err != nil {
			t.Errorf("%s: %v", test.sql, err)
			continue
		}
		if n != test.want {
			t.Errorf("%s: got %d, want %d", test.sql, n, test.want)
		}
	}
}
