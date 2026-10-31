package integration

import (
	"bytes"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/xo/oracle"
)

// collectionTypes are collections of each basic type, which
// TestBasicCollections registers, and the name of the element type of each.
var collectionTypes = []struct{ name, elem, ddl string }{
	{"go_coll_text", "varchar2", "TABLE OF VARCHAR2(500)"},
	{"go_coll_ntext", "nvarchar2", "TABLE OF NVARCHAR2(500)"},
	{"go_coll_num", "number", "TABLE OF NUMBER(10, 2)"},
	{"go_coll_date", "date", "TABLE OF DATE"},
	{"go_coll_raw", "raw", "TABLE OF RAW(500)"},
	{"go_coll_blob", "blob", "TABLE OF BLOB"},
	{"go_coll_clob", "clob", "TABLE OF CLOB"},
	{"go_coll_varray", "varchar2", "VARRAY(10) OF VARCHAR2(20)"},
}

// createCollections creates the types of collectionTypes, and registers
// each without a Go type, so that a bind names it with oracle.Object.
func createCollections(t *testing.T, db *sql.DB) {
	t.Helper()
	dropCollections(db)
	for _, c := range collectionTypes {
		if err := execCmd(db, "CREATE TYPE "+c.name+" AS "+c.ddl); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { dropCollections(db) })
	for _, c := range collectionTypes {
		if err := oracle.RegisterType(db, c.elem, c.name, nil); err != nil {
			t.Fatal(err)
		}
	}
}

func dropCollections(db *sql.DB) {
	for _, c := range collectionTypes {
		db.Exec("DROP TYPE " + c.name)
	}
}

// TestBasicCollections passes a collection of each basic type to PL/SQL and
// back through oracle.Object, with a NULL in every fifth element. It was
// regular_type_array_test.go and varray_test.go of go-ora.
func TestBasicCollections(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	createCollections(t, db)
	const n = 10
	date := time.Date(2024, 11, 11, 11, 11, 11, 0, time.UTC)
	var (
		texts, ntexts, clobs []sql.Null[string]
		nums                 []sql.Null[float64]
		dates                []sql.Null[time.Time]
		raws, blobs          [][]byte
	)
	for x := range n {
		valid := x%5 != 0
		texts = append(texts, sql.Null[string]{V: "text_", Valid: valid})
		ntexts = append(ntexts, sql.Null[string]{V: "안녕하세요_😀_", Valid: valid})
		clobs = append(clobs, sql.Null[string]{V: strings.Repeat("c", 5000), Valid: valid})
		nums = append(nums, sql.Null[float64]{V: float64(x) + 0.5, Valid: valid})
		dates = append(dates, sql.Null[time.Time]{V: date, Valid: valid})
		var b []byte
		if valid {
			b = []byte{byte(x), 0, 0xFF}
		}
		raws = append(raws, b)
		blobs = append(blobs, bytes.Repeat(b, 1000))
	}
	var (
		outTexts, outNTexts, outClobs []sql.Null[string]
		outNums                       []sql.Null[float64]
		outDates                      []sql.Null[time.Time]
		outRaws, outBlobs             [][]byte
	)
	_, err = db.Exec(`DECLARE
	p1 go_coll_text := :1;
	p2 go_coll_ntext := :2;
	p3 go_coll_num := :3;
	p4 go_coll_date := :4;
	p5 go_coll_raw := :5;
	p6 go_coll_blob := :6;
	p7 go_coll_clob := :7;
BEGIN
	FOR x IN 1..p1.COUNT LOOP
		IF p1(x) IS NOT NULL THEN
			p1(x) := p1(x) || x;
			p2(x) := p2(x) || x;
			p3(x) := p3(x) + 10;
			p4(x) := ADD_MONTHS(p4(x), x);
			p7(x) := p7(x) || x;
		END IF;
	END LOOP;
	:8 := p1;
	:9 := p2;
	:10 := p3;
	:11 := p4;
	:12 := p5;
	:13 := p6;
	:14 := p7;
END;`,
		oracle.Object{Name: "go_coll_text", Value: texts},
		oracle.Object{Name: "go_coll_ntext", Value: ntexts},
		oracle.Object{Name: "go_coll_num", Value: nums},
		oracle.Object{Name: "go_coll_date", Value: dates},
		oracle.Object{Name: "go_coll_raw", Value: raws},
		oracle.Object{Name: "go_coll_blob", Value: blobs},
		oracle.Object{Name: "go_coll_clob", Value: clobs},
		oracle.Out{Dest: oracle.Object{Name: "go_coll_text", Value: &outTexts}},
		oracle.Out{Dest: oracle.Object{Name: "go_coll_ntext", Value: &outNTexts}},
		oracle.Out{Dest: oracle.Object{Name: "go_coll_num", Value: &outNums}},
		oracle.Out{Dest: oracle.Object{Name: "go_coll_date", Value: &outDates}},
		oracle.Out{Dest: oracle.Object{Name: "go_coll_raw", Value: &outRaws}},
		oracle.Out{Dest: oracle.Object{Name: "go_coll_blob", Value: &outBlobs}},
		oracle.Out{Dest: oracle.Object{Name: "go_coll_clob", Value: &outClobs}},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range []int{len(outTexts), len(outNTexts), len(outNums), len(outDates), len(outRaws), len(outBlobs), len(outClobs)} {
		if l != n {
			t.Fatalf("got a collection of %d elements, want %d", l, n)
		}
	}
	for x := range n {
		if x%5 == 0 {
			if outTexts[x].Valid || outNTexts[x].Valid || outNums[x].Valid || outDates[x].Valid ||
				outRaws[x] != nil || len(outBlobs[x]) != 0 || outClobs[x].Valid {
				t.Errorf("element %d: want NULL in each collection", x)
			}
			continue
		}
		suffix := fmt.Sprint(x + 1)
		check := func(name string, got, want any) {
			t.Helper()
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Errorf("%s element %d: got %v, want %v", name, x, got, want)
			}
		}
		check("text", outTexts[x].V, texts[x].V+suffix)
		check("ntext", outNTexts[x].V, ntexts[x].V+suffix)
		check("num", outNums[x].V, nums[x].V+10)
		check("date", outDates[x].V, date.AddDate(0, x+1, 0))
		check("raw", outRaws[x], raws[x])
		check("blob", len(outBlobs[x]), len(blobs[x]))
		check("clob", outClobs[x].V, clobs[x].V+suffix)
		if !bytes.Equal(outBlobs[x], blobs[x]) {
			t.Errorf("blob element %d differs", x)
		}
	}
	// a VARRAY
	var varray []string
	_, err = db.Exec(`DECLARE a go_coll_varray := go_coll_varray();
BEGIN
	FOR x IN 1..10 LOOP
		a.EXTEND;
		a(x) := 'string_' || x;
	END LOOP;
	:1 := a;
END;`, oracle.Out{Dest: oracle.Object{Name: "go_coll_varray", Value: &varray}})
	if err != nil {
		t.Fatal(err)
	}
	if len(varray) != 10 || varray[9] != "string_10" {
		t.Errorf("a VARRAY: got %v", varray)
	}
}

type collDay time.Time

type collBytes []byte

// TestNamedElementCollections binds collections whose element is a named
// type of time.Time and of []byte, which RegisterType maps to the
// collection, with no oracle.Object.
func TestNamedElementCollections(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.Exec("DROP TYPE go_coll_days")
	db.Exec("DROP TYPE go_coll_bytes")
	if err := execCmd(db, "CREATE TYPE go_coll_days AS TABLE OF DATE", "CREATE TYPE go_coll_bytes AS TABLE OF RAW(100)"); err != nil {
		t.Fatal(err)
	}
	defer execCmd(db, "DROP TYPE go_coll_days", "DROP TYPE go_coll_bytes")
	if err := oracle.RegisterType(db, "date", "go_coll_days", collDay{}); err != nil {
		t.Fatal(err)
	}
	if err := oracle.RegisterType(db, "raw", "go_coll_bytes", collBytes(nil)); err != nil {
		t.Fatal(err)
	}
	days := []collDay{collDay(time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)), collDay(time.Date(2021, 3, 4, 0, 0, 0, 0, time.UTC))}
	var outDays []collDay
	var outBytes []collBytes
	_, err = db.Exec(`DECLARE d go_coll_days := :1; b go_coll_bytes := :2;
BEGIN
	FOR x IN 1..d.COUNT LOOP
		d(x) := d(x) + 1;
	END LOOP;
	:3 := d;
	:4 := b;
END;`, days, []collBytes{{1, 2}, {3}},
		oracle.Out{Dest: &outDays}, oracle.Out{Dest: &outBytes})
	if err != nil {
		t.Fatal(err)
	}
	if len(outDays) != 2 || !time.Time(outDays[1]).Equal(time.Date(2021, 3, 5, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("dates: got %v", outDays)
	}
	if len(outBytes) != 2 || !bytes.Equal(outBytes[0], []byte{1, 2}) || !bytes.Equal(outBytes[1], []byte{3}) {
		t.Errorf("bytes: got %v", outBytes)
	}
}
