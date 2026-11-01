package ttc

import (
	"database/sql"
	"testing"

	"github.com/xo/oracle/internal/types"
)

// TestNVarCharCoder checks that an NVarChar and a NULL one bind in the form of
// the national character set, 2, a string in the form of the database, 1,
// and that database/sql leaves a NULL NVarChar to the driver (D31).
func TestNVarCharCoder(t *testing.T) {
	for _, test := range []struct {
		v    any
		form int
	}{
		{"", 1},
		{sql.Null[string]{}, 1},
		{types.NVarChar(""), 2},
		{sql.Null[types.NVarChar]{}, 2},
	} {
		coder := builtinCoder(test.v)
		if coder == nil {
			t.Errorf("%T: no coder", test.v)
			continue
		}
		coder.Init()
		if form := coder.ParameterInfo().CharsetForm; form != test.form {
			t.Errorf("%T: the form %d, want %d", test.v, form, test.form)
		}
	}
	if !ownValue(sql.Null[types.NVarChar]{}) {
		t.Error("database/sql converts a sql.Null[NVarChar] to a string")
	}
}
