package integration

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"strings"
	"testing"

	"github.com/xo/oracle"
)

// TestBatchBlob inserts a batch of BLOBs with a NULL among them, as []byte
// and as oracle.Blob. It was bulkinsert_blob_test.go of go-ora.
func TestBatchBlob(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.Exec("DROP TABLE go_batch_blob PURGE")
	if err := execCmd(db, "CREATE TABLE go_batch_blob (id NUMBER, data BLOB)"); err != nil {
		t.Fatal(err)
	}
	defer execCmd(db, "DROP TABLE go_batch_blob PURGE")
	data := []byte("this is a test")
	if _, err := db.Exec("INSERT INTO go_batch_blob VALUES (:1, :2)", []int{1, 2, 3}, [][]byte{nil, data, data}); err != nil {
		t.Fatal(err)
	}
	blob, err := oracle.CreateBlob(context.Background(), db, data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO go_batch_blob VALUES (:1, :2)", []int{4, 5}, []*oracle.Blob{blob, blob}); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query("SELECT id, data FROM go_batch_blob ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id int
		var got []byte
		if err := rows.Scan(&id, &got); err != nil {
			t.Fatal(err)
		}
		want := data
		if id == 1 {
			want = nil
		}
		if !bytes.Equal(got, want) {
			t.Errorf("row %d: got %q, want %q", id, got, want)
		}
		n++
	}
	if err := rows.Err(); err != nil || n != 5 {
		t.Errorf("got %d rows, %v", n, err)
	}
}

// TestBatchBFile inserts a batch of BFILEs with a NULL among them. The
// server does not open a BFILE that it stores, so the file need not exist.
// It was bulkinsert_bfile_test.go of go-ora.
func TestBatchBFile(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.Exec("DROP TABLE go_batch_bfile PURGE")
	if err := execCmd(db, "CREATE TABLE go_batch_bfile (id NUMBER, data BFILE)"); err != nil {
		t.Fatal(err)
	}
	defer execCmd(db, "DROP TABLE go_batch_bfile PURGE")
	ctx := context.Background()
	file, err := oracle.CreateBFile(ctx, db, "GO_DIR", "file.txt")
	if err != nil {
		t.Fatal(err)
	}
	null, err := oracle.CreateBFile(ctx, db, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO go_batch_bfile VALUES (:1, :2)", []int{1, 2, 3}, []*oracle.BFile{file, null, file}); err != nil {
		t.Fatal(err)
	}
	var names string
	err = db.QueryRow(`SELECT LISTAGG(id || ':' || NVL2(data, 'file', 'null'), ',') WITHIN GROUP (ORDER BY id)
		FROM go_batch_bfile`).Scan(&names)
	if err != nil || names != "1:file,2:null,3:file" {
		t.Errorf("got %q, %v", names, err)
	}
}

// TestInOutClob passes an IN OUT CLOB of more than 32,767 characters to a
// procedure with three other OUT arguments. It was inout_lob_test.go of
// go-ora.
func TestInOutClob(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	err = execCmd(db, `CREATE OR REPLACE PROCEDURE go_inout_clob(
	p1 IN OUT CLOB, p2 OUT NUMBER, p3 OUT VARCHAR2, p4 OUT VARCHAR2) AS
BEGIN
	p1 := p1 || ' output string';
	p2 := 15;
	p3 := 'this is a test1';
	p4 := 'this is a test2';
END;`)
	if err != nil {
		t.Fatal(err)
	}
	defer execCmd(db, "DROP PROCEDURE go_inout_clob")
	in := strings.Repeat("a", 0x8010)
	clob, err := oracle.CreateClob(context.Background(), db, in, false)
	if err != nil {
		t.Fatal(err)
	}
	var (
		p2     int
		p3, p4 string
	)
	_, err = db.Exec("BEGIN go_inout_clob(:p1, :p2, :p3, :p4); END;",
		sql.Named("p1", oracle.Out{Dest: clob, In: true}),
		sql.Named("p2", oracle.Out{Dest: &p2}),
		sql.Named("p3", oracle.Out{Dest: &p3, Size: 100}),
		sql.Named("p4", oracle.Out{Dest: &p4, Size: 100}))
	if err != nil {
		t.Fatal(err)
	}
	got, err := clob.Value()
	if err != nil || got != in+" output string" {
		t.Errorf("p1: got %d characters, %v", len(got.(string)), err)
	}
	if p2 != 15 || p3 != "this is a test1" || p4 != "this is a test2" {
		t.Errorf("got %d, %q and %q", p2, p3, p4)
	}
	// a string IN OUT longer than 32,767 bytes goes as a CLOB too (D24)
	s := in
	_, err = db.Exec("BEGIN go_inout_clob(:p1, :p2, :p3, :p4); END;",
		sql.Named("p1", oracle.Out{Dest: &s, Size: 50000, In: true}),
		sql.Named("p2", oracle.Out{Dest: &p2}),
		sql.Named("p3", oracle.Out{Dest: &p3, Size: 100}),
		sql.Named("p4", oracle.Out{Dest: &p4, Size: 100}))
	if err != nil || s != in+" output string" {
		t.Errorf("a string: got %d characters, %v", len(s), err)
	}
}

// TestClobFunction reads a CLOB of 96,004 characters that a function
// returns, 100 times on one connection, into an oracle.Clob. A string OUT
// binds as a VARCHAR2, which holds 32,767 bytes in PL/SQL. It was
// issue_173_test.go of go-ora.
func TestClobFunction(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	err = execCmd(db, `CREATE OR REPLACE FUNCTION go_clob_text(p VARCHAR2) RETURN CLOB AS
	v CLOB;
BEGIN
	FOR i IN 1 .. 4000 LOOP
		v := v || 'tttttttttttttttttttttttt';
	END LOOP;
	RETURN v || '\n' || p;
END;`)
	if err != nil {
		t.Fatal(err)
	}
	defer execCmd(db, "DROP FUNCTION go_clob_text")
	want := strings.Repeat("tttttttttttttttttttttttt", 4000) + `\n` + "ss"
	for i := range 100 {
		var clob oracle.Clob
		_, err := db.Exec("BEGIN :1 := go_clob_text(:2); END;", oracle.Out{Dest: &clob}, "ss")
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		got, err := clob.Value()
		if err != nil || got != want {
			t.Fatalf("call %d: the values differ, %v", i, err)
		}
	}
}

// TestUpdateBlob inserts a BLOB as a []byte and updates it through a
// prepared statement with an oracle.Blob. It was issue_264_test.go of
// go-ora.
func TestUpdateBlob(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.Exec("DROP TABLE go_update_blob PURGE")
	if err := execCmd(db, "CREATE TABLE go_update_blob (id NUMBER(10) NOT NULL, data BLOB)"); err != nil {
		t.Fatal(err)
	}
	defer execCmd(db, "DROP TABLE go_update_blob PURGE")
	if _, err := db.Exec("INSERT INTO go_update_blob VALUES (:1, :2)", 1, []byte("123456789012345678901234567890")); err != nil {
		t.Fatal(err)
	}
	want := []byte("123456789012345678901234567890 123456789012345678901234567890")
	blob, err := oracle.CreateBlob(context.Background(), db, want)
	if err != nil {
		t.Fatal(err)
	}
	stmt, err := db.Prepare("UPDATE go_update_blob SET data = :1 WHERE id = 1")
	if err != nil {
		t.Fatal(err)
	}
	defer stmt.Close()
	if _, err := stmt.Exec(blob); err != nil {
		t.Fatal(err)
	}
	var got []byte
	if err := db.QueryRow("SELECT data FROM go_update_blob").Scan(&got); err != nil || !bytes.Equal(got, want) {
		t.Errorf("got %q, %v", got, err)
	}
}

// TestRefCursorClob reads a CLOB column through a REF CURSOR into an
// oracle.Clob, which reads through NewReader in either mode of lob_fetch,
// and a sql.Null[string]. It was issue_320_test.go of go-ora.
func TestRefCursorClob(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.Exec("DROP TABLE go_cursor_clob PURGE")
	err = execCmd(db, "CREATE TABLE go_cursor_clob (id VARCHAR2(100), response CLOB)",
		`CREATE OR REPLACE FUNCTION go_cursor_clob_f RETURN SYS_REFCURSOR AS
	c SYS_REFCURSOR;
BEGIN
	OPEN c FOR SELECT id, response, response FROM go_cursor_clob;
	RETURN c;
END;`,
		"INSERT INTO go_cursor_clob VALUES ('1', 'THIS IS A TEST')")
	if err != nil {
		t.Fatal(err)
	}
	defer execCmd(db, "DROP FUNCTION go_cursor_clob_f", "DROP TABLE go_cursor_clob PURGE")
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var cursor oracle.RefCursor
	if _, err := conn.ExecContext(ctx, "BEGIN :1 := go_cursor_clob_f(); END;", sql.Out{Dest: &cursor}); err != nil {
		t.Fatal(err)
	}
	rows, err := oracle.WrapRefCursor(ctx, conn, &cursor)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id sql.Null[string]
		var clob oracle.Clob
		var text sql.Null[string]
		if err := rows.Scan(&id, &clob, &text); err != nil {
			t.Fatal(err)
		}
		// with lob_fetch=stream an oracle.Clob holds the locator (D22)
		got, err := io.ReadAll(clob.NewReader(ctx))
		if err != nil || string(got) != "THIS IS A TEST" || text.V != "THIS IS A TEST" || id.V != "1" {
			t.Errorf("got %q, %v, %q and %q", got, err, text.V, id.V)
		}
		n++
	}
	if err := rows.Err(); err != nil || n != 1 {
		t.Errorf("got %d rows, %v", n, err)
	}
}
