package integration

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/xo/oracle"
)

type longStringData struct {
	DateCreated time.Time `udt:"DATE_CREATED"`
	Qty         int       `udt:"QTY"`
	FirstName   string    `udt:"FIRST_NAME"`
}

// TestObjectLongString writes and reads an object whose VARCHAR2 attribute
// holds more than 251 characters, which an image of an object writes with a
// length of more than one byte. It was udt_string_test.go of go-ora, for
// sijms/go-ora#578.
func TestObjectLongString(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	drop := func() {
		execCmd(db, "DROP PROCEDURE go_obj_load", "DROP PROCEDURE go_obj_save", "DROP TYPE go_obj_data", "DROP TABLE go_obj_table PURGE")
	}
	drop()
	err = execCmd(db, "CREATE TABLE go_obj_table (first_name VARCHAR2(4000), date_created DATE, qty NUMBER)",
		"CREATE TYPE go_obj_data AS OBJECT (date_created DATE, qty NUMBER, first_name VARCHAR2(4000))",
		`CREATE PROCEDURE go_obj_save(p IN go_obj_data) AS
BEGIN
	INSERT INTO go_obj_table VALUES (p.first_name, p.date_created, LENGTH(p.first_name));
END;`,
		`CREATE PROCEDURE go_obj_load(p OUT go_obj_data, q NUMBER) AS
BEGIN
	p := go_obj_data(NULL, NULL, NULL);
	SELECT first_name, date_created, qty INTO p.first_name, p.date_created, p.qty
	FROM go_obj_table WHERE qty = q;
END;`)
	if err != nil {
		t.Fatal(err)
	}
	defer drop()
	if err := oracle.RegisterType(db, "go_obj_data", "", longStringData{}); err != nil {
		t.Fatal(err)
	}
	date := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, n := range []int{241, 261, 4000} {
		in := longStringData{date, n, strings.Repeat("a", n)}
		if _, err := db.Exec("BEGIN go_obj_save(:1); END;", in); err != nil {
			t.Fatalf("%d characters: %v", n, err)
		}
		var out longStringData
		if _, err := db.Exec("BEGIN go_obj_load(:1, :2); END;", oracle.Out{Dest: &out}, n); err != nil {
			t.Fatalf("%d characters: %v", n, err)
		}
		if out != in {
			t.Errorf("%d characters: got %d characters, qty %d and %v", n, len(out.FirstName), out.Qty, out.DateCreated)
		}
	}
}

type customer struct {
	ID        int                   `udt:"ID"`
	Name      string                `udt:"NAME"`
	Visits    []sql.Null[time.Time] `udt:"VISITS"`
	Prices    []float64             `udt:"PRICES"`
	BirthDate time.Time             `udt:"BIRTH_DATE"`
}

// TestObjectCollectionAttributes passes an object whose attributes are
// collections of DATE and NUMBER to PL/SQL and back. It was the
// nestedRegularArray case of regular_type_array_test.go of go-ora.
func TestObjectCollectionAttributes(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	drop := func() {
		execCmd(db, "DROP TYPE go_customer", "DROP TYPE go_cust_dates", "DROP TYPE go_cust_prices")
	}
	drop()
	err = execCmd(db, "CREATE TYPE go_cust_dates AS TABLE OF DATE",
		"CREATE TYPE go_cust_prices AS TABLE OF NUMBER(10, 2)",
		`CREATE TYPE go_customer AS OBJECT (
	id NUMBER(10),
	name VARCHAR2(100),
	visits go_cust_dates,
	prices go_cust_prices,
	birth_date DATE)`)
	if err != nil {
		t.Fatal(err)
	}
	defer drop()
	for _, r := range [][2]string{{"date", "go_cust_dates"}, {"number", "go_cust_prices"}} {
		if err := oracle.RegisterType(db, r[0], r[1], nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := oracle.RegisterType(db, "go_customer", "", customer{}); err != nil {
		t.Fatal(err)
	}
	date := time.Date(2024, 11, 11, 11, 11, 11, 0, time.UTC)
	in := customer{ID: 10, Name: "Name1", BirthDate: date}
	var out customer
	_, err = db.Exec(`DECLARE c go_customer := :1;
BEGIN
	c.visits := go_cust_dates();
	c.prices := go_cust_prices();
	FOR x IN 1..5 LOOP
		c.visits.EXTEND;
		c.prices.EXTEND;
		IF x = 3 THEN
			c.visits(x) := NULL;
		ELSE
			c.visits(x) := :2 + x;
		END IF;
		c.prices(x) := x + 0.5;
	END LOOP;
	:3 := c;
END;`, in, date, oracle.Out{Dest: &out})
	if err != nil {
		t.Fatal(err)
	}
	if out.ID != 10 || out.Name != "Name1" || !out.BirthDate.Equal(date) {
		t.Errorf("got %+v", out)
	}
	if len(out.Visits) != 5 || len(out.Prices) != 5 {
		t.Fatalf("got %d visits and %d prices", len(out.Visits), len(out.Prices))
	}
	for x := range 5 {
		want := sql.Null[time.Time]{V: date.AddDate(0, 0, x+1), Valid: x != 2}
		if !want.Valid {
			want.V = time.Time{}
		}
		if out.Visits[x] != want || out.Prices[x] != float64(x+1)+0.5 {
			t.Errorf("element %d: got %v and %v", x, out.Visits[x], out.Prices[x])
		}
	}
}

type lobDoc struct {
	ID   int         `udt:"ID"`
	Body oracle.Clob `udt:"BODY"`
	Data oracle.Blob `udt:"DATA"`
}

type lobDocText struct {
	ID   int    `udt:"ID"`
	Body string `udt:"BODY"`
	Data []byte `udt:"DATA"`
}

type lobDocBox struct {
	Name string     `udt:"NAME"`
	Doc  lobDocText `udt:"DOC"`
}

// TestObjectLobAttributes reads and binds objects whose attributes are a
// CLOB and a BLOB, alone, in a collection, in a row and inside another
// object. A read panicked, because an attribute had no LOB streamer, and a
// bind failed with ORA-03101, because an attribute was written with a
// length of 4 bytes before its locator. The decode runs while the reply is
// read, when no LOB can load, so the fields of such an object are filled
// after the reply.
func TestObjectLobAttributes(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	drop := func() {
		execCmd(db, "DROP TABLE go_lob_doc_table PURGE", "DROP TYPE go_lob_box", "DROP TYPE go_lob_docs", "DROP TYPE go_lob_doc")
	}
	drop()
	err = execCmd(db, "CREATE TYPE go_lob_doc AS OBJECT (id NUMBER, body CLOB, data BLOB)",
		"CREATE TYPE go_lob_docs AS TABLE OF go_lob_doc",
		"CREATE TYPE go_lob_box AS OBJECT (name VARCHAR2(20), doc go_lob_doc)",
		"CREATE TABLE go_lob_doc_table (id NUMBER, doc go_lob_doc)")
	if err != nil {
		t.Fatal(err)
	}
	defer drop()
	long := strings.Repeat("x", 5000)
	// into oracle.Clob and oracle.Blob fields
	if err := oracle.RegisterType(db, "go_lob_doc", "go_lob_docs", lobDoc{}); err != nil {
		t.Fatal(err)
	}
	var out lobDoc
	_, err = db.Exec("BEGIN :1 := go_lob_doc(1, RPAD('x', 5000, 'x'), UTL_RAW.CAST_TO_RAW('abc')); END;", oracle.Out{Dest: &out})
	if err != nil {
		t.Fatal(err)
	}
	if body, err := out.Body.Value(); err != nil || body != long {
		t.Errorf("an oracle.Clob: got %v, %v", body, err)
	}
	if data, err := out.Data.Value(); err != nil || string(data.([]byte)) != "abc" {
		t.Errorf("an oracle.Blob: got %v, %v", data, err)
	}
	var outs []lobDoc
	_, err = db.Exec("BEGIN :1 := go_lob_docs(go_lob_doc(1, 'short', NULL), go_lob_doc(2, NULL, NULL)); END;", oracle.Out{Dest: &outs})
	if err != nil || len(outs) != 2 {
		t.Fatalf("a collection: got %d, %v", len(outs), err)
	}
	if body, err := outs[0].Body.Value(); err != nil || body != "short" {
		t.Errorf("a collection: got %v, %v", body, err)
	}
	if body, err := outs[1].Body.Value(); err != nil || body != nil {
		t.Errorf("a NULL CLOB: got %v, %v", body, err)
	}
	// into string and []byte fields, on a database of its own registry
	db2, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	if err := oracle.RegisterType(db2, "go_lob_doc", "", lobDocText{}); err != nil {
		t.Fatal(err)
	}
	if err := oracle.RegisterType(db2, "go_lob_box", "", lobDocBox{}); err != nil {
		t.Fatal(err)
	}
	var text lobDocText
	_, err = db2.Exec("BEGIN :1 := go_lob_doc(1, RPAD('x', 5000, 'x'), UTL_RAW.CAST_TO_RAW('abc')); END;", oracle.Out{Dest: &text})
	if err != nil || text.Body != long || string(text.Data) != "abc" {
		t.Errorf("a string and a []byte: got %d characters and %q, %v", len(text.Body), text.Data, err)
	}
	var box lobDocBox
	_, err = db2.Exec("BEGIN :1 := go_lob_box('box', go_lob_doc(2, 'inner', NULL)); END;", oracle.Out{Dest: &box})
	if err != nil || box.Name != "box" || box.Doc.Body != "inner" || box.Doc.Data != nil {
		t.Errorf("an object in an object: got %+v, %v", box, err)
	}
	// a bind, of text and of LOBs, which the server refused
	var s string
	_, err = db2.Exec("DECLARE d go_lob_doc := :1; BEGIN :2 := DBMS_LOB.GETLENGTH(d.body) || ':' || DBMS_LOB.GETLENGTH(d.data); END;",
		lobDocText{3, long, []byte{1, 2}}, oracle.Out{Dest: &s, Size: 100})
	if err != nil || s != "5000:2" {
		t.Errorf("a bind of text: got %q, %v", s, err)
	}
	if _, err := db2.Exec("INSERT INTO go_lob_doc_table VALUES (1, :1)", lobDocText{4, long, []byte{5}}); err != nil {
		t.Fatal(err)
	}
	// a row
	var row lobDocText
	if err := db2.QueryRow("SELECT doc FROM go_lob_doc_table WHERE id = 1").Scan(&row); err != nil || row.Body != long || string(row.Data) != "\x05" {
		t.Errorf("a row: got %d characters and %v, %v", len(row.Body), row.Data, err)
	}
	clob, err := oracle.CreateClob(context.Background(), db, "from a locator", false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("DECLARE d go_lob_doc := :1; BEGIN :2 := d.body || ':' || NVL(TO_CHAR(DBMS_LOB.GETLENGTH(d.data)), 'null'); END;",
		lobDoc{ID: 5, Body: *clob}, oracle.Out{Dest: &s, Size: 100})
	if err != nil || s != "from a locator:null" {
		t.Errorf("a bind of an oracle.Clob: got %q, %v", s, err)
	}
}
