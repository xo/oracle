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

// TestErrVerbose checks that an error of the server writes itself as SQL*Plus
// does, with the line of the statement, a * under the position of the error,
// and the error stack of PL/SQL, through the interface that usql asks for.
func TestErrVerbose(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, test := range []struct {
		query string
		// head is the line, the * and the number of the line
		head string
		// codes are the codes that start the lines of the message, whose
		// text changes with the release
		codes []string
	}{
		{
			"select * from nosuch",
			"select * from nosuch\n              *\nERROR at line 1:",
			[]string{"ORA-00942"},
		},
		{
			"select 'éé', 1\nfrom dual where nosuch = 1",
			"from dual where nosuch = 1\n                *\nERROR at line 2:",
			[]string{"ORA-00904"},
		},
		{
			"begin\n  raise_application_error(-20001, 'boom');\nend;",
			"begin\n*\nERROR at line 1:",
			[]string{"ORA-20001", "ORA-06512"},
		},
		{
			"begin\n  nosuch;\nend;",
			"  nosuch;\n  *\nERROR at line 2:",
			[]string{"ORA-06550", "PLS-00201", "ORA-06550", "PL/SQL"},
		},
	} {
		_, err := db.ExecContext(t.Context(), test.query)
		var v verboseError
		if !errors.As(err, &v) {
			t.Errorf("%q: got %v (%T), want an error with ErrVerbose", test.query, err, err)
			continue
		}
		got := v.ErrVerbose(test.query)
		head, msg, _ := strings.Cut(got, ":\n")
		var codes []string
		for line := range strings.Lines(msg) {
			// 23ai and later add a line of help to each error
			if !strings.HasPrefix(line, "Help: ") {
				code, _, _ := strings.Cut(line, ":")
				codes = append(codes, code)
			}
		}
		if head+":" != test.head || !slices.Equal(codes, test.codes) {
			t.Errorf("%q: got:\n%s\nwant:\n%s\nand the codes %v", test.query, got, test.head, test.codes)
		}
	}
}
