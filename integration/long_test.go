package integration

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
)

// TestLongBatch inserts a batch of LONG values of 20,480 characters, with
// NULL in every second row, and reads them back with a query and with the OUT
// binds of a PL/SQL block (sijms/go-ora#205).
func TestLongBatch(t *testing.T) {
	var createTable = func(db *sql.DB) error {
		return execCmd(db, `CREATE TABLE TTB_205(
	ID NUMBER(10) NOT NULL,
		BIG_TEXT LONG,
		PRIMARY KEY (ID)
	) NOCOMPRESS`)
	}
	var dropTable = func(db *sql.DB) error {
		return execCmd(db, `DROP TABLE TTB_205 PURGE`)
	}
	var insert = func(db *sql.DB) error {
		type ttb205 struct {
			Id   int            `db:"ID"`
			Name sql.NullString `db:"NAME"`
		}
		data := make([]ttb205, 100)
		for index := range data {
			data[index].Id = index + 1
			if (index+1)%2 == 0 {
				data[index].Name.Valid = false
			} else {
				data[index].Name.String = strings.Repeat("a", 0x5000)
				data[index].Name.Valid = true
			}
		}
		_, err := db.Exec("INSERT ALL INTO TTB_205(BIG_TEXT, ID) VALUES(:NAME, :ID) SELECT * FROM DUAL", data)
		if err != nil {
			return err
		}
		return nil
	}
	var sqlQuery = func(db *sql.DB) error {
		rows, err := db.Query("SELECT BIG_TEXT, ID FROM TTB_205 WHERE ID < 3")
		if err != nil {
			return err
		}
		defer func() {
			err = rows.Close()
			if err != nil {
				t.Error(err)
			}
		}()
		var id int
		var name sql.NullString
		for rows.Next() {
			err = rows.Scan(&name, &id)
			if err != nil {
				return err
			}
			if id == 1 {
				expected := strings.Repeat("a", 0x5000)
				if name.String != expected {
					return errors.New("long data is not correct")
				}
			} else {
				if name.Valid {
					return errors.New("long should be null")
				}
			}
		}
		return rows.Err()
	}
	var outputQuery = func(db *sql.DB, id int) error {
		temp := struct {
			ID   int            `db:"ID,,,output"`
			Name sql.NullString `db:"NAME,,100000,output"`
		}{}
		_, err := db.Exec("BEGIN SELECT ID, BIG_TEXT INTO :ID, :NAME FROM TTB_205 WHERE ID = :IID; END;", &temp,
			sql.Named("IID", id))
		if err != nil {
			return err
		}
		if temp.ID%2 == 0 {
			if temp.Name.Valid {
				return errors.New("long should be null")
			}
		} else {
			expected := strings.Repeat("a", 0x5000)
			if temp.Name.String != expected {
				return errors.New("long data is not correct")
			}
		}
		return nil
	}
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err = db.Close()
		if err != nil {
			t.Error(err)
		}
	}()
	err = createTable(db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err = dropTable(db)
		if err != nil {
			t.Error(err)
		}
	}()
	err = insert(db)
	if err != nil {
		t.Fatal(err)
	}
	err = sqlQuery(db)
	if err != nil {
		t.Fatal(err)
	}
	err = outputQuery(db, 1)
	if err != nil {
		t.Fatal(err)
	}
	err = outputQuery(db, 2)
	if err != nil {
		t.Fatal(err)
	}
}

// TestLongIndexExpression reads the LONG column_expression of a function
// based index (sijms/go-ora#543). The issue read an index of the APEX schema.
// This test makes its own index.
func TestLongIndexExpression(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	_, _ = db.ExecContext(ctx, "DROP TABLE TTB_543 PURGE")
	if _, err := db.ExecContext(ctx, "CREATE TABLE TTB_543 (session_id NUMBER, security_group_id NUMBER)"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DROP TABLE TTB_543 PURGE") })
	if _, err := db.ExecContext(ctx, `CREATE UNIQUE INDEX TTB_543_UK ON TTB_543 (CASE WHEN session_id IS NULL THEN security_group_id END)`); err != nil {
		t.Fatal(err)
	}
	var result string
	err = db.QueryRowContext(ctx, "SELECT column_expression FROM user_ind_expressions WHERE index_name = 'TTB_543_UK' AND column_position = 1").Scan(&result)
	if err != nil {
		t.Fatal(err)
	}
	const expected = `CASE  WHEN "SESSION_ID" IS NULL THEN "SECURITY_GROUP_ID" END `
	if result != expected {
		t.Errorf("expected: %q and got: %q", expected, result)
	}
}
