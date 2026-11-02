package integration

import (
	"database/sql"
	"fmt"
	"testing"
)

func TestMultiResulSets(t *testing.T) {
	var createTable = func(db *sql.DB) error {
		return execCmd(db, `create table TTB_573_1(
    ID number(10),
    NAME varchar2(100)
    )`, `create table TTB_573_2(
    ID number(10),
    CUST_ID number(10),
    NAME varchar2(100)
    )`)
	}

	var dropTable = func(db *sql.DB) error {
		return execCmd(db, "drop table TTB_573_1 purge", "drop table TTB_573_2 purge")
	}

	var insert = func(db *sql.DB) error {
		_, err := db.Exec("INSERT INTO TTB_573_1(ID, NAME) VALUES (:1, :2)", []int{1, 2}, []string{"Tom", "Julia"})
		if err != nil {
			return err
		}
		_, err = db.Exec("INSERT INTO TTB_573_2(ID, CUST_ID, NAME) VALUES(:1, :2, :3)",
			[]int{1000, 2000}, []int{1, 2}, []string{"BOOKS", "FURNITURE"})
		if err != nil {
			return err
		}
		return nil
	}
	var query = func(db *sql.DB) error {
		rows, err := db.Query(`declare
    cust_cur sys_refcursor;
    sales_cur sys_refcursor;
begin
    open cust_cur for SELECT ID, NAME FROM TTB_573_1;
    dbms_sql.return_result(cust_cur);

    open sales_cur for SELECT ID, CUST_ID, NAME FROM TTB_573_2;
    dbms_sql.return_result(sales_cur);
end;`)
		if err != nil {
			return nil
		}
		defer func() {
			err = rows.Close()
			if err != nil {
				fmt.Println("can't close rows: ", err)
			}
		}()
		type Customer struct {
			id   int
			name string
		}
		type Product struct {
			id     int
			custID int
			name   string
		}
		customer := Customer{}
		product := Product{}

		// get first result set
		fmt.Println("Customers: ")
		for rows.Next() {
			err = rows.Scan(&customer.id, &customer.name)
			if err != nil {
				return err
			}
			fmt.Println(customer)
		}
		fmt.Println("Products: ")
		if rows.NextResultSet() {
			for rows.Next() {
				err = rows.Scan(&product.id, &product.custID, &product.name)
				if err != nil {
					return err
				}
				fmt.Println(product)
			}
		}
		return nil
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
	err = createTable(db)
	if err != nil {
		t.Error(err)
		return
	}
	defer func() {
		err = dropTable(db)
		if err != nil {
			t.Error(err)
		}
	}()
	err = insert(db)
	if err != nil {
		t.Error(err)
		return
	}
	err = query(db)
	if err != nil {
		t.Error(err)
		return
	}
}

// TestCursorExpression reads a query with a CURSOR expression in each row,
// and checks that each nested cursor returns the value of its row
// (sijms/go-ora#468).
func TestCursorExpression(t *testing.T) {
	var createTable = func(db *sql.DB) error {
		return execCmd(db, `CREATE TABLE TTB_468(N NUMBER)`)
	}
	var dropTable = func(db *sql.DB) error {
		return execCmd(db, "DROP TABLE TTB_468 PURGE")
	}
	var insert = func(db *sql.DB) error {
		data := make([]int, 100)
		for index := range data {
			data[index] = index + 1
		}
		_, err := db.Exec("INSERT INTO TTB_468(N) VALUES (:1)", data)
		return err
	}
	var query = func(db *sql.DB) error {
		var (
			n1, n2 int32
			cursor sql.Rows
		)
		rows, err := db.Query("select n,cursor(select TTB_468.n from dual) from TTB_468 order by n")
		if err != nil {
			return err
		}
		defer func() {
			err = rows.Close()
			if err != nil {
				t.Error(err)
			}
		}()
		index := 0
		for rows.Next() {
			err = rows.Scan(&n1, &cursor)
			if err != nil {
				return err
			}
			for cursor.Next() {
				err = cursor.Scan(&n2)
				if err != nil {
					return err
				}
			}
			err = cursor.Close()
			if err != nil {
				return err
			}
			if n1 != n2 {
				return fmt.Errorf("n1: %d, n2: %d is not equal", n1, n2)
			}
			index++
		}
		t.Logf("%d rows scanned", index)
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
	err = query(db)
	if err != nil {
		t.Fatal(err)
	}
}
