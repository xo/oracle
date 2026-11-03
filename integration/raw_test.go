package integration

import (
	"bytes"
	"database/sql"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/xo/oracle"
)

func TestRaw(t *testing.T) {
	var insert = func(db *sql.DB) error {
		sqlText := `INSERT INTO TTB_MAIN(ID, NAME, VAL, LDATE, DATA) VALUES(:ID, :NAME, :VAL, :LDATE, :DATA)`
		length := 500
		type TempStruct struct {
			Id   int       `db:"ID"`
			Name string    `db:"NAME"`
			Val  float32   `db:"VAL"`
			Date time.Time `db:"LDATE"`
			Data []byte    `db:"DATA"`
		}
		args := make([]TempStruct, length)
		for x := range length {
			args[x] = TempStruct{
				Id:   x + 1,
				Name: strings.Repeat("*", 20),
				Val:  float32(length) / float32(x+1),
				Date: time.Now(),
				Data: bytes.Repeat([]byte{55}, 20),
			}
			if x > 0 && x%10 == 0 {
				args[x].Data = []byte{}
			}
		}
		_, err := db.Exec(sqlText, args)
		return err
	}
	db, err := getDB(t)
	if err != nil {
		t.Error(err)
		return
	}
	defer func() {
		err = db.Close()
		if err != nil {
			t.Error(err)
		}
	}()
	err = createMainTable(db)
	if err != nil {
		t.Error(err)
		return
	}
	defer func() {
		err = dropMainTable(db)
		if err != nil {
			t.Error(err)
		}
	}()
	err = insert(db)
	if err != nil {
		t.Error(err)
		return
	}
	var got []byte
	err = db.QueryRow(`SELECT DATA FROM TTB_MAIN WHERE ID = 10`).Scan(&got)
	if err != nil {
		t.Error(err)
		return
	}
	expected := bytes.Repeat([]byte{55}, 20)
	if bytes.Compare(got, expected) != 0 {
		t.Errorf("expected %#v and got %#v", expected, got)
	}
}

// TestUUIDs binds a uuid.UUID as a RAW(16), which is how Oracle stores a
// UUID, and reads one back (D37). The driver took a uuid.UUID, a [16]byte,
// for a PL/SQL array, and the server refused it with ORA-01484. A pointer, a
// sql.Null[uuid.UUID] and a batch of them bind too, and an OUT fills a
// uuid.UUID. A RAW(16) scans into *any as []byte, as any RAW does.
func TestUUIDs(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := t.Context()
	db.Exec("DROP TABLE go_uuids PURGE")
	if err := execCmd(db, "CREATE TABLE go_uuids (n NUMBER, id RAW(16))"); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP TABLE go_uuids PURGE")
	ids := []uuid.UUID{uuid.NewV7(), uuid.NewV4(), uuid.NewV4(), uuid.NewV4(), uuid.NewV4()}
	insert := "INSERT INTO go_uuids VALUES (:1, :2)"
	for i, v := range []any{ids[0], &ids[1], sql.Null[uuid.UUID]{V: ids[2], Valid: true}} {
		if _, err := db.ExecContext(ctx, insert, i, v); err != nil {
			t.Fatalf("binding %T: %v", v, err)
		}
	}
	if _, err := db.ExecContext(ctx, insert, []int{3, 4}, ids[3:5]); err != nil {
		t.Fatalf("a batch: %v", err)
	}
	var nilID *uuid.UUID
	for i, v := range []any{sql.Null[uuid.UUID]{}, nilID} {
		if _, err := db.ExecContext(ctx, insert, 5+i, v); err != nil {
			t.Fatalf("binding a NULL %T: %v", v, err)
		}
	}
	rows, err := db.QueryContext(ctx, "SELECT n, id, id, id FROM go_uuids ORDER BY n")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var i int
		var id sql.Null[uuid.UUID]
		var b []byte
		var a any
		if err := rows.Scan(&i, &id, &b, &a); err != nil {
			t.Fatal(err)
		}
		if i < len(ids) {
			if !id.Valid || id.V != ids[i] || !bytes.Equal(b, ids[i][:]) {
				t.Errorf("row %d: got %v and %x, want %v", i, id, b, ids[i])
			}
			if _, ok := a.([]byte); !ok {
				t.Errorf("row %d: got %T into *any, want []byte", i, a)
			}
		} else if id.Valid || b != nil || a != nil {
			t.Errorf("row %d: got %v, %x and %v, want NULL", i, id, b, a)
		}
		n++
	}
	if err := rows.Err(); err != nil || n != 7 {
		t.Fatalf("got %d rows, %v", n, err)
	}
	var found int
	if err := db.QueryRowContext(ctx, "SELECT n FROM go_uuids WHERE id = :1", ids[3]).Scan(&found); err != nil || found != 3 {
		t.Errorf("a lookup: got %d, %v", found, err)
	}
	var out uuid.UUID
	var outNull sql.Null[uuid.UUID]
	if _, err := db.ExecContext(ctx, "BEGIN :1 := SYS_GUID(); :2 := SYS_GUID(); END;",
		oracle.Out{Dest: &out}, oracle.Out{Dest: &outNull}); err != nil {
		t.Fatal(err)
	}
	if out == (uuid.UUID{}) || !outNull.Valid || outNull.V == (uuid.UUID{}) || out == outNull.V {
		t.Errorf("the OUT values: got %v and %v", out, outNull)
	}
}
