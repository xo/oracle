package oracle_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/xo/oracle"
)

// These tests hold the faults of sijms/go-ora that this driver fixed, each
// named for its issue, against a live server.

// TestUntypedNullRow is sijms/go-ora#776: a query whose only columns are
// NULL returned no row and no error.
func TestUntypedNullRow(t *testing.T) {
	db := openLive(t)
	tests := []struct {
		query string
		want  [][]any
	}{
		{"SELECT NULL a FROM dual", [][]any{{nil}}},
		{"SELECT '' a FROM dual", [][]any{{nil}}},
		{"SELECT CASE WHEN 1=0 THEN 0 END a FROM dual", [][]any{{nil}}},
		{"SELECT CASE WHEN 1=0 THEN 0 END a, NULL b FROM dual CONNECT BY LEVEL <= 3", [][]any{{nil, nil}, {nil, nil}, {nil, nil}}},
		{"SELECT 'x' b, NULL a FROM dual", [][]any{{"x", nil}}},
		{"SELECT NULL a FROM dual WHERE 1=0", nil},
	}
	for _, test := range tests {
		rows, err := db.QueryContext(t.Context(), test.query)
		if err != nil {
			t.Errorf("%s: %v", test.query, err)
			continue
		}
		cols, _ := rows.Columns()
		var got [][]any
		for rows.Next() {
			row := make([]any, len(cols))
			dest := make([]any, len(row))
			for i := range row {
				dest[i] = &row[i]
			}
			if err := rows.Scan(dest...); err != nil {
				t.Errorf("%s: %v", test.query, err)
			}
			got = append(got, row)
		}
		if err := rows.Err(); err != nil {
			t.Errorf("%s: %v", test.query, err)
		}
		_ = rows.Close()
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("%s: got %v, want %v", test.query, got, test.want)
		}
	}
}

// TestReturningStringWithNoSize is sijms/go-ora#703: RETURNING into a string
// through sql.Out with no size broke the connection, and later cut the value.
func TestReturningStringWithNoSize(t *testing.T) {
	db := openLive(t)
	ctx := t.Context()
	const table = "xo_oracle_returning"
	if _, err := db.ExecContext(ctx, "BEGIN EXECUTE IMMEDIATE 'DROP TABLE "+table+" PURGE'; EXCEPTION WHEN OTHERS THEN NULL; END;"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE "+table+" (id NUMBER(10), name VARCHAR2(50))"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DROP TABLE " + table + " PURGE") })
	var id int64
	var name string
	_, err := db.ExecContext(ctx, "INSERT INTO "+table+" (id, name) VALUES (:1, :2) RETURNING id, name INTO :3, :4",
		7, "TestTest", sql.Out{Dest: &id}, sql.Out{Dest: &name})
	if err != nil {
		t.Fatal(err)
	}
	if id != 7 || name != "TestTest" {
		t.Errorf("got %d and %q, want 7 and TestTest", id, name)
	}
}

type (
	namedString string
	namedInt    int64
)

// TestNamedTypes is sijms/go-ora#699: a named type such as type ID string
// gave unsupported go type.
func TestNamedTypes(t *testing.T) {
	db := openLive(t)
	ctx := t.Context()
	var s string
	if err := db.QueryRowContext(ctx, "SELECT :1 FROM dual", namedString("abc")).Scan(&s); err != nil || s != "abc" {
		t.Errorf("named string: got %q, %v", s, err)
	}
	var n int64
	if err := db.QueryRowContext(ctx, "SELECT :1 FROM dual", namedInt(42)).Scan(&n); err != nil || n != 42 {
		t.Errorf("named int: got %d, %v", n, err)
	}
	var d time.Duration
	if err := db.QueryRowContext(ctx, "SELECT :1 FROM dual", 90*time.Second).Scan(&d); err != nil || d != 90*time.Second {
		t.Errorf("time.Duration, which the driver binds as an interval: got %v, %v", d, err)
	}
}

// TestUnopenedRefCursor is sijms/go-ora#671: a REF CURSOR that the procedure
// never opened broke the connection. It is an error, and the connection
// stays usable.
func TestUnopenedRefCursor(t *testing.T) {
	db := openLive(t)
	ctx := t.Context()
	if _, err := db.ExecContext(ctx, `CREATE OR REPLACE PROCEDURE xo_oracle_cursor(c OUT SYS_REFCURSOR) AS BEGIN NULL; END;`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DROP PROCEDURE xo_oracle_cursor") })
	var cur oracle.RefCursor
	_, err := db.ExecContext(ctx, "BEGIN xo_oracle_cursor(:1); END;", sql.Out{Dest: &cur})
	if err == nil {
		t.Error("an unopened cursor gives no error")
	}
	usable(t, db)
}

// TestBatchLengths is sijms/go-ora#695: an empty array in a batch panicked,
// and arrays of different lengths were cut to the shortest.
func TestBatchLengths(t *testing.T) {
	db := openLive(t)
	ctx := t.Context()
	const table = "xo_oracle_batch"
	if _, err := db.ExecContext(ctx, "BEGIN EXECUTE IMMEDIATE 'DROP TABLE "+table+" PURGE'; EXCEPTION WHEN OTHERS THEN NULL; END;"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE "+table+" (id NUMBER(10), name VARCHAR2(50))"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DROP TABLE " + table + " PURGE") })
	insert := "INSERT INTO " + table + " (id, name) VALUES (:1, :2)"
	if _, err := db.ExecContext(ctx, insert, []int64{}, []string{}); err == nil {
		t.Error("an empty batch gives no error")
	}
	if _, err := db.ExecContext(ctx, insert, []int64{1, 2, 3}, []string{"a", "b"}); err == nil {
		t.Error("arrays of 3 and 2 rows give no error")
	}
	if _, err := db.ExecContext(ctx, insert, []int64{1, 2}, []string{"a", "b", "c"}); err == nil {
		t.Error("arrays of 2 and 3 rows give no error")
	}
	res, err := db.ExecContext(ctx, insert, []int64{1, 2}, []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := res.RowsAffected(); err != nil || n != 2 {
		t.Errorf("a batch of 2 rows affects %d, %v", n, err)
	}
	usable(t, db)
}

// TestBatchError is sijms/go-ora#698: a batch whose row broke a key waited
// until the network timed out on 11g, where it returns ORA-00001.
func TestBatchError(t *testing.T) {
	db := openLive(t)
	const table = "xo_oracle_batch_error"
	if _, err := db.ExecContext(t.Context(), "BEGIN EXECUTE IMMEDIATE 'DROP TABLE "+table+" PURGE'; EXCEPTION WHEN OTHERS THEN NULL; END;"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), "CREATE TABLE "+table+" (id NUMBER(10) PRIMARY KEY, name VARCHAR2(20), val NUMBER(10))"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DROP TABLE " + table + " PURGE") })
	merge := `MERGE INTO ` + table + ` t
		USING (SELECT :id id, :name name, :val val FROM dual) s
		ON (s.id = t.id AND s.name = t.name)
		WHEN MATCHED THEN UPDATE SET t.val = s.val
		WHEN NOT MATCHED THEN INSERT (id, name, val) VALUES (s.id, s.name, s.val)`
	tests := []struct {
		name  string
		query string
		args  []any
	}{
		{"insert", "INSERT INTO " + table + " (id, name, val) VALUES (:1, :2, :3)", []any{[]int64{1, 1}, []string{"a", "b"}, []int64{1, 2}}},
		{"merge", merge, []any{sql.Named("id", []int64{2, 2}), sql.Named("name", []string{"a", "b"}), sql.Named("val", []int64{1, 2})}},
	}
	for _, test := range tests {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		_, err := db.ExecContext(ctx, test.query, test.args...)
		cancel()
		var serr *oracle.ServerError
		if !errors.As(err, &serr) || serr.ErrCode != 1 {
			t.Errorf("%s: got %v, want ORA-00001", test.name, err)
		}
	}
	usable(t, db)
}

// TestClobOutsideBMP is sijms/go-ora#769: on 19c a CLOB of over about 8K
// UTF-16 units with characters outside the BMP read back with U+FFFD in
// place of some of them.
func TestClobOutsideBMP(t *testing.T) {
	db := openLive(t)
	ctx := t.Context()
	const table = "xo_oracle_clob_bmp"
	if _, err := db.ExecContext(ctx, "BEGIN EXECUTE IMMEDIATE 'DROP TABLE "+table+" PURGE'; EXCEPTION WHEN OTHERS THEN NULL; END;"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE "+table+" (v CLOB)"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DROP TABLE " + table + " PURGE") })
	// the server builds the value, so that only the read is tested
	_, err := db.ExecContext(ctx, `DECLARE c CLOB; BEGIN
		DBMS_LOB.CREATETEMPORARY(c, TRUE);
		FOR i IN 1..10000 LOOP DBMS_LOB.WRITEAPPEND(c, 3, UNISTR('\D55C\D83D\DE00')); END LOOP;
		DBMS_LOB.WRITEAPPEND(c, 1, UNISTR('\B05D'));
		INSERT INTO `+table+` VALUES (c); END;`)
	if err != nil {
		t.Fatal(err)
	}
	var got string
	if err := db.QueryRowContext(ctx, "SELECT v FROM "+table).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if want := strings.Repeat("한😀", 10000) + "끝"; got != want {
		t.Errorf("got %d bytes with %d U+FFFD, want %d bytes", len(got), strings.Count(got, "�"), len(want))
	}
}

// TestXMLType is sijms/go-ora#771: an XMLType came back NULL on Autonomous
// Database, whose flag word is 0x8014, and one of about 254 bytes or more
// hung. go-ora read no XMLType at all in v3.
func TestXMLType(t *testing.T) {
	db := openLive(t)
	ctx := t.Context()
	createTable(t, db, "xo_oracle_xml", "id NUMBER, x XMLTYPE")
	tests := []struct {
		insert string
		want   string
	}{
		{"XMLTYPE('<a>1</a>')", "<a>1</a>"},
		{"XMLTYPE('<a>' || RPAD('x', 3000, 'x') || '</a>')", "<a>" + strings.Repeat("x", 3000) + "</a>"},
		{"XMLTYPE(TO_CLOB('<a>') || RPAD(TO_CLOB('y'), 32000, 'y') || TO_CLOB('</a>'))", "<a>" + strings.Repeat("y", 32000) + "</a>"},
		{"NULL", ""},
	}
	for i, test := range tests {
		if _, err := db.ExecContext(ctx, "INSERT INTO xo_oracle_xml VALUES (:1, "+test.insert+")", i); err != nil {
			t.Fatalf("%s: %v", test.insert, err)
		}
	}
	for i, test := range tests {
		qctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		var got sql.NullString
		err := db.QueryRowContext(qctx, "SELECT x FROM xo_oracle_xml WHERE id = :1", i).Scan(&got)
		cancel()
		if err != nil {
			t.Errorf("%s: %v", test.insert, err)
			continue
		}
		// the server can indent or end the text with a newline
		if g := strings.TrimSpace(got.String); g != test.want {
			t.Errorf("%s: got %d bytes %.40q, want %d bytes", test.insert, len(g), g, len(test.want))
		}
	}
	usable(t, db)
}

// TestClobCharset is sijms/go-ora#733: text outside ASCII written through
// the LOB API read back garbled. Here it reads back whole, into a CLOB
// column and as a CLOB bound into a VARCHAR2.
func TestClobCharset(t *testing.T) {
	db := openLive(t)
	ctx := t.Context()
	createTable(t, db, "xo_oracle_clob_charset", "id NUMBER, c CLOB, v VARCHAR2(4000 CHAR)")
	long := strings.Repeat("سلام دنیا ", 300)
	short := strings.Repeat("سلام ", 100)
	c1, err := oracle.CreateClob(ctx, db, long, false)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := oracle.CreateClob(ctx, db, short, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO xo_oracle_clob_charset VALUES (1, :1, :2)", c1, c2); err != nil {
		t.Fatal(err)
	}
	var c, v string
	if err := db.QueryRowContext(ctx, "SELECT c, v FROM xo_oracle_clob_charset").Scan(&c, &v); err != nil {
		t.Fatal(err)
	}
	if c != long || v != short {
		t.Errorf("got %d and %d bytes, want %d and %d", len(c), len(v), len(long), len(short))
	}
}

// TestSkipNLS is sijms/go-ora#611: a logon ran a query of the NLS settings
// that nothing reads, where the logon itself did not send them, as 11g does
// not. skip_nls leaves it out.
func TestSkipNLS(t *testing.T) {
	dsn := os.Getenv("ORACLE_DSN")
	if dsn == "" {
		t.Skip("ORACLE_DSN is empty")
	}
	for _, skip := range []bool{false, true} {
		cfg, err := oracle.ParseDSN(dsn)
		if err != nil {
			t.Fatal(err)
		}
		cfg.SkipNLS = skip
		h := new(records)
		cfg.Logger = slog.New(h)
		c, err := oracle.NewConnector(cfg)
		if err != nil {
			t.Fatal(err)
		}
		db := sql.OpenDB(c)
		var n int
		if err := db.QueryRowContext(t.Context(), "SELECT 1 FROM dual").Scan(&n); err != nil {
			t.Fatal(err)
		}
		db.Close()
		ran := false
		h.mu.Lock()
		for _, r := range h.recs {
			r.Attrs(func(a slog.Attr) bool {
				if a.Key == "query" && strings.Contains(a.Value.String(), "nls_session_parameters") {
					ran = true
				}
				return true
			})
		}
		h.mu.Unlock()
		if skip && ran {
			t.Error("skip_nls=true: the NLS query ran")
		}
	}
}

// TestTNSAlias connects through an alias of tnsnames.ora, which
// sijms/go-ora#616 asks for and D20 decided.
func TestTNSAlias(t *testing.T) {
	dsn := os.Getenv("ORACLE_DSN")
	if dsn == "" {
		t.Skip("ORACLE_DSN is empty")
	}
	cfg, err := oracle.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host == "" || cfg.Service == "" {
		t.Skip("ORACLE_DSN names no host and service")
	}
	protocol := "TCP"
	if cfg.SSLMode != "" && cfg.SSLMode != oracle.SSLDisable {
		protocol = "TCPS"
	}
	port := cfg.Port
	if port == 0 {
		port = oracle.DefaultPort
	}
	dir := t.TempDir()
	ora := fmt.Sprintf("# written by TestTNSAlias\nXO_TEST =\n  (DESCRIPTION =\n    (ADDRESS = (PROTOCOL = %s)(HOST = %s)(PORT = %d))\n    (CONNECT_DATA = (SERVICE_NAME = %s))\n  )\n", protocol, cfg.Host, port, cfg.Service)
	if err := os.WriteFile(filepath.Join(dir, "tnsnames.ora"), []byte(ora), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg.Host, cfg.Port, cfg.Service = "", 0, ""
	cfg.Alias, cfg.TNSAdmin = "xo_test", dir
	c, err := oracle.NewConnector(cfg)
	if err != nil {
		t.Fatal(err)
	}
	db := sql.OpenDB(c)
	defer db.Close()
	var n int
	if err := db.QueryRowContext(t.Context(), "SELECT 1 FROM dual").Scan(&n); err != nil || n != 1 {
		t.Errorf("through the alias: got %d, %v", n, err)
	}
}

type xoNumbers struct {
	ID   int64     `udt:"ID"`
	Vals []float64 `udt:"VALS"`
}

// TestObjectCollection is sijms/go-ora#721: a VARRAY in an object went as
// its body alone, which the server read as a length that it never stopped
// allocating for, a NULL object read two bytes too many, and a NULL object
// in a batch was stored as the object of the row before. A VARRAY of any
// size, a NULL one, an empty one, and a NULL object each go and come back
// whole, one by one and in a batch.
func TestObjectCollection(t *testing.T) {
	db := openLive(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	drop := []string{"DROP TABLE xo_oracle_objects PURGE", "DROP TYPE xo_oracle_object FORCE", "DROP TYPE xo_oracle_numbers FORCE"}
	for _, s := range drop {
		_, _ = db.ExecContext(ctx, s)
	}
	for _, s := range []string{
		"CREATE TYPE xo_oracle_numbers AS VARRAY(2000) OF NUMBER",
		"CREATE TYPE xo_oracle_object AS OBJECT (id NUMBER, vals xo_oracle_numbers)",
		"CREATE TABLE xo_oracle_objects (k NUMBER, o xo_oracle_object) TABLESPACE USERS",
	} {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	t.Cleanup(func() {
		for _, s := range drop {
			_, _ = db.Exec(s)
		}
	})
	if err := oracle.RegisterType(db, "NUMBER", "XO_ORACLE_NUMBERS", nil); err != nil {
		t.Fatal(err)
	}
	if err := oracle.RegisterType(db, "XO_ORACLE_OBJECT", "", xoNumbers{}); err != nil {
		t.Fatal(err)
	}
	big := make([]float64, 300)
	for i := range big {
		big[i] = float64(i) + 0.5
	}
	want := []*xoNumbers{
		{ID: 1, Vals: []float64{1, 2, 3}},
		{ID: 2, Vals: nil},
		nil,
		{ID: 4, Vals: []float64{}},
		{ID: 5, Vals: big},
	}
	for k, o := range want {
		if _, err := db.ExecContext(ctx, "INSERT INTO xo_oracle_objects VALUES (:1, :2)", k, o); err != nil {
			t.Fatalf("row %d: %v", k, err)
		}
	}
	// the same rows again, as one batch, at k + 10
	keys := make([]int64, len(want))
	for k := range want {
		keys[k] = int64(k + 10)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO xo_oracle_objects VALUES (:1, :2)", keys, want); err != nil {
		t.Fatalf("the batch: %v", err)
	}
	rows, err := db.QueryContext(ctx, "SELECT k, o FROM xo_oracle_objects ORDER BY k")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var k int
		var got any
		if err := rows.Scan(&k, &got); err != nil {
			t.Fatal(err)
		}
		n++
		k %= 10
		if want[k] == nil {
			if got != nil {
				t.Errorf("row %d: got %v, want NULL", k, got)
			}
			continue
		}
		if g, ok := got.(xoNumbers); !ok || g.ID != want[k].ID || !reflect.DeepEqual(g.Vals, want[k].Vals) {
			t.Errorf("row %d: got %#v, want %#v", k, got, *want[k])
		}
	}
	if err := rows.Err(); err != nil || n != 2*len(want) {
		t.Errorf("read %d rows, %v", n, err)
	}
}

// resetListener accepts each connection and resets it, as a gateway that
// accepts the dial to a port that nothing listens on does. It returns its
// port.
func resetListener(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			if tc, ok := c.(*net.TCPConn); ok {
				_ = tc.SetLinger(0)
			}
			_ = c.Close()
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}

// TestFailover is sijms/go-ora#619 and sijms/go-ora#707: a server whose dial
// succeeds and whose connection resets gave an error, where the next server
// was not tried. The next server takes the connection, named with server
// and in a descriptor of two addresses.
func TestFailover(t *testing.T) {
	dsn := os.Getenv("ORACLE_DSN")
	if dsn == "" {
		t.Skip("ORACLE_DSN is empty")
	}
	live, err := oracle.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if live.Host == "" || live.Service == "" || (live.SSLMode != "" && live.SSLMode != oracle.SSLDisable) {
		t.Skip("ORACLE_DSN names no host and service over plain TCP")
	}
	port := live.Port
	if port == 0 {
		port = oracle.DefaultPort
	}
	dead := resetListener(t)
	withServer := live
	withServer.Host, withServer.Port = "127.0.0.1", dead
	withServer.Servers = []string{net.JoinHostPort(live.Host, strconv.Itoa(port))}
	withDescriptor := live
	withDescriptor.Host, withDescriptor.Port, withDescriptor.Service = "", 0, ""
	withDescriptor.Descriptor = fmt.Sprintf("(DESCRIPTION=(FAILOVER=ON)(ADDRESS_LIST=(ADDRESS=(PROTOCOL=TCP)(HOST=127.0.0.1)(PORT=%d))(ADDRESS=(PROTOCOL=TCP)(HOST=%s)(PORT=%d)))(CONNECT_DATA=(SERVICE_NAME=%s)))", dead, live.Host, port, live.Service)
	for name, cfg := range map[string]oracle.Config{"server": withServer, "descriptor": withDescriptor} {
		c, err := oracle.NewConnector(cfg)
		if err != nil {
			t.Fatal(err)
		}
		db := sql.OpenDB(c)
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		var n int
		err = db.QueryRowContext(ctx, "SELECT 1 FROM dual").Scan(&n)
		cancel()
		db.Close()
		if err != nil || n != 1 {
			t.Errorf("%s: got %d, %v", name, n, err)
		}
	}
}
