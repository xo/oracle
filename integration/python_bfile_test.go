package integration

import (
	"database/sql"
	"io"
	"testing"

	"github.com/xo/oracle"
)

// TestBFile reads files of a directory of the server through BFILEs. It
// holds 2100, 2103 and 2104 of tests/ext/test_ext_2100_bfile_type.py of
// python-oracledb, written again here, with files that UTL_FILE writes on
// the server. A BFILE of a row never opened, as go-ora looked for its
// locator where a row does not keep it, and Exists asked for an open file,
// which a file that does not exist cannot be. It skips when the server refuses the directory, as an
// autonomous database does.
func TestBFile(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := t.Context()
	if _, err := db.ExecContext(ctx, "CREATE OR REPLACE DIRECTORY GO_BFILE_DIR AS '/tmp'"); err != nil {
		t.Skipf("creating the directory: %v", err)
	}
	defer db.Exec("DROP DIRECTORY GO_BFILE_DIR")
	const contents = "Some arbitrary data for test 2100"
	if _, err := db.ExecContext(ctx, `DECLARE
	f UTL_FILE.FILE_TYPE;
BEGIN
	f := UTL_FILE.FOPEN('GO_BFILE_DIR', 'go_bfile.txt', 'wb');
	UTL_FILE.PUT_RAW(f, UTL_RAW.CAST_TO_RAW(:1));
	UTL_FILE.FCLOSE(f);
END;`, contents); err != nil {
		t.Skipf("writing the file: %v", err)
	}
	defer db.Exec("BEGIN UTL_FILE.FREMOVE('GO_BFILE_DIR', 'go_bfile.txt'); END;")
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// a BFILE of a row reads until its rows close (D22), so each case keeps
	// the rows open while it uses the file
	query := func(t *testing.T, name string) (*oracle.BFile, func()) {
		t.Helper()
		rows, err := conn.QueryContext(ctx, "SELECT BFILENAME(:1, :2) FROM dual", "GO_BFILE_DIR", name)
		if err != nil {
			t.Fatal(err)
		}
		var file oracle.BFile
		if !rows.Next() {
			t.Fatalf("no row: %v", rows.Err())
		}
		if err := rows.Scan(&file); err != nil {
			t.Fatal(err)
		}
		return &file, func() { rows.Close() }
	}

	t.Run("2100 a file that exists", func(t *testing.T) {
		file, done := query(t, "go_bfile.txt")
		defer done()
		if file.DirName() != "GO_BFILE_DIR" || file.FileName() != "go_bfile.txt" {
			t.Errorf("got the names %q and %q", file.DirName(), file.FileName())
		}
		if ok, err := file.Exists(); err != nil || !ok {
			t.Errorf("exists: got %t, %v", ok, err)
		}
		if err := file.Open(ctx); err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		b, err := io.ReadAll(file.NewReader(ctx))
		if err != nil || string(b) != contents {
			t.Errorf("got %q, %v", b, err)
		}
	})

	// CreateBFile wrote the names in AL16UTF16, and the server found no
	// file for the BFILE that it stored
	t.Run("2103 a bind of a BFILE", func(t *testing.T) {
		db.Exec("DROP TABLE go_bfiles PURGE")
		if err := execCmd(db, "CREATE TABLE go_bfiles (id NUMBER, f BFILE)"); err != nil {
			t.Fatal(err)
		}
		defer db.Exec("DROP TABLE go_bfiles PURGE")
		file, err := oracle.CreateBFile(ctx, db, "GO_BFILE_DIR", "go_bfile.txt")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.ExecContext(ctx, "INSERT INTO go_bfiles VALUES (1, :1)", file); err != nil {
			t.Fatal(err)
		}
		var n sql.Null[int64]
		if err := conn.QueryRowContext(ctx, "SELECT DBMS_LOB.GETLENGTH(f) FROM go_bfiles").Scan(&n); err != nil || n.V != int64(len(contents)) {
			t.Errorf("got the length %+v, %v", n, err)
		}
	})

	t.Run("2104 a missing file", func(t *testing.T) {
		file, done := query(t, "go_no_such_file.txt")
		defer done()
		if file.FileName() != "go_no_such_file.txt" {
			t.Errorf("got the name %q", file.FileName())
		}
		if ok, err := file.Exists(); err != nil || ok {
			t.Errorf("exists: got %t, %v", ok, err)
		}
		err := file.Open(ctx)
		if err == nil {
			_, err = io.ReadAll(file.NewReader(ctx))
			file.Close()
		}
		requireErr(t, "a read", err, "ORA-22288")
	})
}
