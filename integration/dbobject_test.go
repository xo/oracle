package integration

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/xo/oracle"
)

type allSub struct {
	Number float64 `udt:"SUBNUMBERVALUE"`
	String string  `udt:"SUBSTRINGVALUE"`
}

type allTypes struct {
	Number       float64   `udt:"NUMBERVALUE"`
	String       string    `udt:"STRINGVALUE"`
	FixedChar    string    `udt:"FIXEDCHARVALUE"`
	NString      string    `udt:"NSTRINGVALUE"`
	NFixedChar   string    `udt:"NFIXEDCHARVALUE"`
	Raw          []byte    `udt:"RAWVALUE"`
	Int          int64     `udt:"INTVALUE"`
	SmallInt     int64     `udt:"SMALLINTVALUE"`
	Real         float64   `udt:"REALVALUE"`
	Double       float64   `udt:"DOUBLEPRECISIONVALUE"`
	Float        float64   `udt:"FLOATVALUE"`
	BinaryFloat  float32   `udt:"BINARYFLOATVALUE"`
	BinaryDouble float64   `udt:"BINARYDOUBLEVALUE"`
	Date         time.Time `udt:"DATEVALUE"`
	Timestamp    time.Time `udt:"TIMESTAMPVALUE"`
	TimestampLTZ time.Time `udt:"TIMESTAMPLTZVALUE"`
	Clob         string    `udt:"CLOBVALUE"`
	NClob        string    `udt:"NCLOBVALUE"`
	Blob         []byte    `udt:"BLOBVALUE"`
	Sub          allSub    `udt:"SUBOBJECTVALUE"`
	Subs         []allSub  `udt:"SUBOBJECTARRAY"`
}

// TestObjectAllTypes stores an object with an attribute of each data type,
// with a sub-object and a collection of them, in a table, and reads it back,
// and binds a NULL one. It holds the cases of
// tests/data_types/test_1900_dbobject.py of python-oracledb that test the
// protocol (1900, 1901, 1903, 1907), written again here. The type is
// udt_Object of python-oracledb, less its TIMESTAMP WITH TIME ZONE.
func TestObjectAllTypes(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	drop := func() {
		execCmd(db, "DROP TABLE go_all_table PURGE", "DROP TYPE go_all_types", "DROP TYPE go_all_subs", "DROP TYPE go_all_sub")
	}
	drop()
	err = execCmd(db, "CREATE TYPE go_all_sub AS OBJECT (SubNumberValue NUMBER, SubStringValue VARCHAR2(60))",
		"CREATE TYPE go_all_subs AS TABLE OF go_all_sub",
		`CREATE TYPE go_all_types AS OBJECT (
	NumberValue NUMBER,
	StringValue VARCHAR2(60),
	FixedCharValue CHAR(10),
	NStringValue NVARCHAR2(60),
	NFixedCharValue NCHAR(10),
	RawValue RAW(16),
	IntValue INTEGER,
	SmallIntValue SMALLINT,
	RealValue REAL,
	DoublePrecisionValue DOUBLE PRECISION,
	FloatValue FLOAT,
	BinaryFloatValue BINARY_FLOAT,
	BinaryDoubleValue BINARY_DOUBLE,
	DateValue DATE,
	TimestampValue TIMESTAMP,
	TimestampLTZValue TIMESTAMP WITH LOCAL TIME ZONE,
	CLOBValue CLOB,
	NCLOBValue NCLOB,
	BLOBValue BLOB,
	SubObjectValue go_all_sub,
	SubObjectArray go_all_subs)`,
		"CREATE TABLE go_all_table (id NUMBER, obj go_all_types) NESTED TABLE obj.SubObjectArray STORE AS go_all_table_subs")
	if err != nil {
		t.Fatal(err)
	}
	defer drop()
	if err := oracle.RegisterType(db, "go_all_sub", "go_all_subs", allSub{}); err != nil {
		t.Fatal(err)
	}
	if err := oracle.RegisterType(db, "go_all_types", "", allTypes{}); err != nil {
		t.Fatal(err)
	}
	in := allTypes{
		Number:       5,
		String:       "A string",
		FixedChar:    "Fixed str",
		NString:      "A NCHAR string 😀",
		NFixedChar:   "Fixed N",
		Raw:          []byte("Raw Value"),
		Int:          27,
		SmallInt:     13,
		Real:         184.875,
		Double:       1.375,
		Float:        23,
		BinaryFloat:  14.25,
		BinaryDouble: 29.1625,
		Date:         time.Date(2017, 5, 9, 0, 0, 0, 0, time.UTC),
		Timestamp:    time.Date(2017, 5, 9, 9, 41, 13, 0, time.UTC),
		TimestampLTZ: time.Date(1999, 11, 12, 23, 5, 2, 0, time.UTC),
		Clob:         "A short CLOB",
		NClob:        "A short NCLOB",
		Blob:         []byte("A short BLOB"),
		Sub:          allSub{23, "Substring value"},
		Subs:         []allSub{{1, "first"}, {2, "second"}},
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO go_all_table VALUES (1, :1)", in); err != nil {
		t.Fatal(err)
	}
	// a NULL object, 1900
	if _, err := db.ExecContext(ctx, "INSERT INTO go_all_table VALUES (2, :1)", (*allTypes)(nil)); err != nil {
		t.Fatal(err)
	}
	// an empty collection, 1903
	empty := in
	empty.Subs = []allSub{}
	if _, err := db.ExecContext(ctx, "INSERT INTO go_all_table VALUES (3, :1)", empty); err != nil {
		t.Fatal(err)
	}
	var out allTypes
	if err := db.QueryRowContext(ctx, "SELECT obj FROM go_all_table WHERE id = 1").Scan(&out); err != nil {
		t.Fatal(err)
	}
	want := in
	// CHAR and NCHAR come back padded to their length
	want.FixedChar, want.NFixedChar = "Fixed str ", "Fixed N   "
	got, wanted := fmt.Sprintf("%+v", out), fmt.Sprintf("%+v", want)
	if !out.TimestampLTZ.Equal(want.TimestampLTZ) {
		t.Errorf("the TIMESTAMP WITH LOCAL TIME ZONE: got %v, want %v", out.TimestampLTZ, want.TimestampLTZ)
	}
	out.TimestampLTZ, want.TimestampLTZ = time.Time{}, time.Time{}
	if got, wanted = fmt.Sprintf("%+v", out), fmt.Sprintf("%+v", want); got != wanted {
		t.Errorf("got  %s\nwant %s", got, wanted)
	}
	var isNull int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM go_all_table WHERE id = 2 AND obj IS NULL").Scan(&isNull); err != nil || isNull != 1 {
		t.Errorf("a NULL object: got %d, %v", isNull, err)
	}
	var three allTypes
	if err := db.QueryRowContext(ctx, "SELECT obj FROM go_all_table WHERE id = 3").Scan(&three); err != nil || three.Subs == nil || len(three.Subs) != 0 {
		t.Errorf("an empty collection: got %#v, %v", three.Subs, err)
	}
}

// TestNestedCollections binds and reads collections of collections and a
// collection of many entries. It holds the cases of
// tests/data_types/test_1900_dbobject.py of python-oracledb (1927, 1943 to
// 1947), written again here.
func TestNestedCollections(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	drop := func() {
		execCmd(db, "DROP TYPE go_nums_tabs", "DROP TYPE go_nums_varrs", "DROP TYPE go_nums_varr", "DROP TYPE go_nums_tab")
	}
	drop()
	err = execCmd(db, "CREATE TYPE go_nums_varr AS VARRAY(10) OF NUMBER",
		"CREATE TYPE go_nums_tab AS TABLE OF NUMBER",
		"CREATE TYPE go_nums_varrs AS TABLE OF go_nums_varr",
		"CREATE TYPE go_nums_tabs AS TABLE OF go_nums_tab")
	if err != nil {
		t.Fatal(err)
	}
	defer drop()
	for _, r := range [][2]string{{"number", "go_nums_varr"}, {"number", "go_nums_tab"}, {"go_nums_varr", "go_nums_varrs"}, {"go_nums_tab", "go_nums_tabs"}} {
		if err := oracle.RegisterType(db, r[0], r[1], nil); err != nil {
			t.Fatalf("%s: %v", r[1], err)
		}
	}
	// a VARRAY and a TABLE of numbers, 1943 and 1944
	for _, name := range []string{"go_nums_varr", "go_nums_tab"} {
		var out []float64
		_, err := db.ExecContext(ctx, "BEGIN :1 := :2; END;",
			oracle.Out{Dest: oracle.Object{Name: name, Value: &out}}, oracle.Object{Name: name, Value: []float64{1, 2.5, 3}})
		if err != nil || fmt.Sprint(out) != "[1 2.5 3]" {
			t.Errorf("%s: got %v, %v", name, out, err)
		}
	}
	// a TABLE of VARRAYs and a TABLE of TABLEs, 1945 to 1947
	for _, name := range []string{"go_nums_varrs", "go_nums_tabs"} {
		in := [][]float64{{1, 2}, {}, {3, 4, 5}}
		var out [][]float64
		_, err := db.ExecContext(ctx, "BEGIN :1 := :2; END;",
			oracle.Out{Dest: oracle.Object{Name: name, Value: &out}}, oracle.Object{Name: name, Value: in})
		if err != nil || fmt.Sprint(out) != fmt.Sprint(in) {
			t.Errorf("%s: got %v, %v", name, out, err)
		}
	}
	// thousands of entries, 1927
	many := make([]float64, 10000)
	for i := range many {
		many[i] = float64(i)
	}
	var out []float64
	var sum float64
	_, err = db.ExecContext(ctx, "DECLARE t go_nums_tab := :1; s NUMBER := 0; BEGIN FOR i IN 1..t.COUNT LOOP s := s + t(i); END LOOP; :2 := s; :3 := t; END;",
		oracle.Object{Name: "go_nums_tab", Value: many}, oracle.Out{Dest: &sum}, oracle.Out{Dest: oracle.Object{Name: "go_nums_tab", Value: &out}})
	if err != nil || sum != 49995000 || len(out) != len(many) || out[9999] != 9999 {
		t.Errorf("10,000 entries: got the sum %v and %d entries, %v", sum, len(out), err)
	}
}

type xmlDoc struct {
	ID  int            `udt:"ID"`
	XML sql.NullString `udt:"XMLVALUE"`
}

// TestObjectXMLType reads an object with an XMLType attribute, set and
// NULL. It holds the cases 1939 and 1949 of
// tests/data_types/test_1900_dbobject.py of python-oracledb, written again
// here.
func TestObjectXMLType(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.Exec("DROP TYPE go_xml_doc")
	if err := execCmd(db, "CREATE TYPE go_xml_doc AS OBJECT (id NUMBER, xmlvalue SYS.XMLTYPE)"); err != nil {
		t.Fatal(err)
	}
	defer execCmd(db, "DROP TYPE go_xml_doc")
	if err := oracle.RegisterType(db, "go_xml_doc", "", xmlDoc{}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		xml  string
		want string
	}{
		{"SYS.XMLTYPE('<a><b>value</b></a>')", "<a><b>value</b></a>"},
		{"NULL", ""},
	} {
		var out xmlDoc
		_, err := db.Exec("BEGIN :1 := go_xml_doc(1, "+test.xml+"); END;", oracle.Out{Dest: &out})
		got := strings.Join(strings.Fields(out.XML.String), "")
		if err != nil || got != test.want || out.XML.Valid != (test.want != "") {
			t.Errorf("%s: got %q, %v", test.xml, out.XML.String, err)
		}
	}
}
