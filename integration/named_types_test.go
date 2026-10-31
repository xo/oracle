package integration

import (
	"testing"

	"github.com/xo/oracle"
)

type namedStatus string

type namedLevel int

type namedItem struct {
	Name   namedStatus `udt:"NAME"`
	Amount namedLevel  `udt:"AMOUNT"`
}

// TestNamedTypesInObjects binds an object whose fields have named types of
// a basic kind, and a collection of such a type. database/sql converts a
// named value of a statement, but not one inside an object or a collection,
// and the driver failed with "cannot set value of type namedStatus into
// string".
func TestNamedTypesInObjects(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	execCmd(db, "DROP TYPE go_named_items", "DROP TYPE go_named_item", "DROP TYPE go_named_list")
	err = execCmd(db,
		"CREATE TYPE go_named_item AS OBJECT (name VARCHAR2(20), amount NUMBER(10))",
		"CREATE TYPE go_named_items AS TABLE OF go_named_item",
		"CREATE TYPE go_named_list AS TABLE OF VARCHAR2(20)")
	if err != nil {
		t.Fatal(err)
	}
	defer execCmd(db, "DROP TYPE go_named_items", "DROP TYPE go_named_item", "DROP TYPE go_named_list")
	if err := oracle.RegisterType(db, "go_named_item", "go_named_items", namedItem{}); err != nil {
		t.Fatal(err)
	}
	if err := oracle.RegisterType(db, "varchar2", "go_named_list", namedStatus("")); err != nil {
		t.Fatal(err)
	}
	var s string
	_, err = db.Exec("DECLARE x go_named_item := :1; BEGIN :2 := x.name || ':' || x.amount; END;",
		namedItem{"open", 7}, oracle.Out{Dest: &s, Size: 100})
	if err != nil || s != "open:7" {
		t.Errorf("an object: got %q, %v", s, err)
	}
	err = db.QueryRow("SELECT LISTAGG(name || ':' || amount, ',') WITHIN GROUP (ORDER BY amount) FROM TABLE(:1)",
		[]namedItem{{"a", 1}, {"b", 2}}).Scan(&s)
	if err != nil || s != "a:1,b:2" {
		t.Errorf("a collection of objects: got %q, %v", s, err)
	}
	err = db.QueryRow("SELECT LISTAGG(column_value, ',') WITHIN GROUP (ORDER BY column_value) FROM TABLE(:1)",
		[]namedStatus{"x", "y", "z"}).Scan(&s)
	if err != nil || s != "x,y,z" {
		t.Errorf("a collection of a named string: got %q, %v", s, err)
	}
	var out []namedStatus
	_, err = db.Exec("DECLARE a go_named_list := go_named_list('n1', 'n2'); BEGIN :1 := a; END;", oracle.Out{Dest: &out})
	if err != nil || len(out) != 2 || out[0] != "n1" || out[1] != "n2" {
		t.Errorf("a collection out: got %v, %v", out, err)
	}
}
