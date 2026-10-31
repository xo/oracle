package integration

import (
	"testing"
)

// TestColumnType checks the name that ColumnTypeDatabaseTypeName gives each
// type, which is the name of the data dictionary. go-ora gave none.
func TestColumnType(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	columns := []struct{ def, want string }{
		{"CHAR(32)", "CHAR"},
		{"NCHAR(32)", "NCHAR"},
		{"VARCHAR2(32)", "VARCHAR2"},
		{"NVARCHAR2(32)", "NVARCHAR2"},
		{"NUMBER", "NUMBER"},
		{"NUMBER(5, 2)", "NUMBER"},
		{"FLOAT", "FLOAT"},
		{"BINARY_FLOAT", "BINARY_FLOAT"},
		{"BINARY_DOUBLE", "BINARY_DOUBLE"},
		{"DATE", "DATE"},
		{"TIMESTAMP", "TIMESTAMP"},
		{"TIMESTAMP WITH TIME ZONE", "TIMESTAMP WITH TIME ZONE"},
		{"TIMESTAMP WITH LOCAL TIME ZONE", "TIMESTAMP WITH LOCAL TIME ZONE"},
		{"INTERVAL YEAR TO MONTH", "INTERVAL YEAR TO MONTH"},
		{"INTERVAL DAY TO SECOND", "INTERVAL DAY TO SECOND"},
		{"RAW(16)", "RAW"},
		{"CLOB", "CLOB"},
		{"NCLOB", "NCLOB"},
		{"BLOB", "BLOB"},
	}
	def := ""
	for i, c := range columns {
		if i > 0 {
			def += ", "
		}
		def += "c" + string(rune('a'+i)) + " " + c.def
	}
	_, _ = db.ExecContext(ctx, "DROP TABLE TTB_COLUMN_TYPE PURGE")
	if _, err := db.ExecContext(ctx, "CREATE TABLE TTB_COLUMN_TYPE ("+def+")"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DROP TABLE TTB_COLUMN_TYPE PURGE") })
	rows, err := db.QueryContext(ctx, "SELECT * FROM TTB_COLUMN_TYPE")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	types, err := rows.ColumnTypes()
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range columns {
		if got := types[i].DatabaseTypeName(); got != c.want {
			t.Errorf("%s: got %q, want %q", c.def, got, c.want)
		}
	}
	rows.Close()
	rows, err = db.QueryContext(ctx, "SELECT ROWID FROM dual")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	types, err = rows.ColumnTypes()
	if err != nil {
		t.Fatal(err)
	}
	if got := types[0].DatabaseTypeName(); got != "ROWID" && got != "UROWID" {
		t.Errorf("ROWID: got %q", got)
	}
}
