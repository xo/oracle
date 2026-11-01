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

// TestLobSizes binds CLOBs, NCLOBs and BLOBs that grow by 25,000 characters
// to 250,000, as values and as LOBs that CreateClob and CreateBlob make,
// reads them back as values and in pieces, and binds three long LOBs with
// numbers between them. It holds the cases of
// tests/data_types/test_1500_lobs.py of python-oracledb that bind and fetch
// LOBs (1502, 1503, 1506, 1507, 1514, 1516) and that order the binds (1530
// to 1532), written again here.
func TestLobSizes(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	for _, kind := range []string{"CLOB", "NCLOB", "BLOB"} {
		t.Run(kind, func(t *testing.T) {
			table := "go_lob_" + strings.ToLower(kind)
			db.Exec("DROP TABLE " + table + " PURGE")
			err := execCmd(db, "CREATE TABLE "+table+" (id NUMBER, lob "+kind+", n1 NUMBER, extra1 "+kind+", n2 NUMBER, extra2 "+kind+")")
			if err != nil {
				t.Fatal(err)
			}
			defer execCmd(db, "DROP TABLE "+table+" PURGE")
			value := func(s string) any {
				if kind == "BLOB" {
					return []byte(s)
				}
				return s
			}
			lob := func(s string) any {
				var v any
				var err error
				switch kind {
				case "CLOB":
					v, err = oracle.CreateClob(ctx, db, s, false)
				case "NCLOB":
					v, err = oracle.CreateClob(ctx, db, s, true)
				default:
					v, err = oracle.CreateBlob(ctx, db, []byte(s))
				}
				if err != nil {
					t.Fatal(err)
				}
				return v
			}
			var want []string
			long := ""
			for i := 1; i <= 10; i++ {
				long += strings.Repeat(string(rune('A'+i-1)), 25000)
				want = append(want, long)
				var arg any
				// odd rows as values, even rows as LOBs
				if i%2 == 1 {
					arg = value(long)
				} else {
					arg = lob(long)
				}
				if _, err := db.ExecContext(ctx, "INSERT INTO "+table+" (id, lob) VALUES (:1, :2)", i, arg); err != nil {
					t.Fatalf("%d characters: %v", len(long), err)
				}
			}
			rows, err := db.QueryContext(ctx, "SELECT id, lob, lob FROM "+table+" ORDER BY id")
			if err != nil {
				t.Fatal(err)
			}
			n := 0
			for rows.Next() {
				var id int
				var got []byte
				var pieces any
				if kind == "BLOB" {
					pieces = new(oracle.Blob)
				} else {
					pieces = new(oracle.Clob)
				}
				if err := rows.Scan(&id, &got, pieces); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, []byte(want[id-1])) {
					t.Errorf("row %d: got %d bytes, want %d", id, len(got), len(want[id-1]))
				}
				var r io.Reader
				switch p := pieces.(type) {
				case *oracle.Clob:
					r = p.NewReaderSize(ctx, 7000)
				case *oracle.Blob:
					r = p.NewReaderSize(ctx, 7000)
				}
				if b, err := io.ReadAll(r); err != nil || !bytes.Equal(b, []byte(want[id-1])) {
					t.Errorf("row %d in pieces: got %d bytes, %v", id, len(b), err)
				}
				n++
			}
			if err := rows.Err(); err != nil || n != 10 {
				t.Fatalf("got %d rows, %v", n, err)
			}
			rows.Close()
			// three long LOBs with numbers between them, 1530 to 1532
			main, extra1, extra2 := strings.Repeat("A", 32768), strings.Repeat("B", 65536), strings.Repeat("C", 131072)
			if _, err := db.ExecContext(ctx, "DELETE FROM "+table); err != nil {
				t.Fatal(err)
			}
			_, err = db.ExecContext(ctx, "INSERT INTO "+table+" VALUES (:1, :2, :3, :4, :5, :6)",
				1, value(main), 8, value(extra1), 15, value(extra2))
			if err != nil {
				t.Fatal(err)
			}
			var id, n1, n2 int
			var lob0, lob1, lob2 sql.Null[string]
			if err := db.QueryRowContext(ctx, "SELECT * FROM "+table).Scan(&id, &lob0, &n1, &lob1, &n2, &lob2); err != nil {
				t.Fatal(err)
			}
			if id != 1 || n1 != 8 || n2 != 15 || lob0.V != main || lob1.V != extra1 || lob2.V != extra2 {
				t.Errorf("got %d, %d, %d and LOBs of %d, %d and %d", id, n1, n2, len(lob0.V), len(lob1.V), len(lob2.V))
			}
		})
	}
}
