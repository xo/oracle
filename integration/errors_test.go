package integration

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// verboseError is the interface that usql declares for \errverbose and finds
// with errors.As, since it cannot name the error types of this driver.
type verboseError interface {
	error
	ErrVerbose(query string) string
}

// TestErrVerbose checks that an error of the server writes itself as psql's
// \errverbose writes an error of PostgreSQL, with ERROR and the first error,
// the line of the statement with a ^ under the position of the error, and
// the rest of the stack in DETAIL, HINT and CONTEXT, through the interface
// that usql asks for.
func TestErrVerbose(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, test := range []struct {
		query string
		// head is the ERROR line up to its code, and the LINE and the ^
		head string
		// fields are the name and the code of each other line, whose text
		// changes with the release. A line of help of 23ai and later is
		// left out
		fields []string
	}{
		{
			"select * from nosuch",
			"ERROR:  ORA-00942\nLINE 1: select * from nosuch\n                      ^",
			nil,
		},
		{
			"select 'éé', 1\nfrom dual where nosuch = 1",
			"ERROR:  ORA-00904\nLINE 2: from dual where nosuch = 1\n                        ^",
			nil,
		},
		{
			"begin\n  raise_application_error(-20001, 'boom');\nend;",
			"ERROR:  ORA-20001",
			[]string{"CONTEXT:  ORA-06512"},
		},
		{
			"begin\n  nosuch;\nend;",
			"ERROR:  ORA-06550\nLINE 2:   nosuch;\n          ^",
			[]string{"DETAIL:  PLS-00201", "DETAIL:  ORA-06550", "DETAIL:  PL/SQL"},
		},
	} {
		_, err := db.ExecContext(t.Context(), test.query)
		var v verboseError
		if !errors.As(err, &v) {
			t.Errorf("%q: got %v (%T), want an error with ErrVerbose", test.query, err, err)
			continue
		}
		got := v.ErrVerbose(test.query)
		var head, fields []string
		for i, line := range strings.Split(got, "\n") {
			name, value, _ := strings.Cut(line, ":  ")
			code, _, _ := strings.Cut(value, ":")
			switch {
			case i == 0:
				head = append(head, name+":  "+code)
			case strings.HasPrefix(line, "LINE ") || strings.TrimSpace(line) == "^":
				head = append(head, line)
			case name == "HINT":
			default:
				fields = append(fields, name+":  "+code)
			}
		}
		if strings.Join(head, "\n") != test.head || !slices.Equal(fields, test.fields) {
			t.Errorf("%q: got:\n%s\nwant:\n%s\nand the fields %v", test.query, got, test.head, test.fields)
		}
	}
}
