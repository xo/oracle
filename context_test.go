package oracle_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/xo/oracle"
)

// slowQuery runs for minutes on every release, and needs no grant: it counts
// the rows of a cross join of a catalog view with itself.
const slowQuery = "SELECT COUNT(*) FROM all_objects a, all_objects b"

// openLive opens the database that ORACLE_DSN names, with one connection, so
// that a test can see whether a break left that connection usable. It skips
// the test when ORACLE_DSN is empty.
func openLive(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("ORACLE_DSN")
	if dsn == "" {
		t.Skip("ORACLE_DSN is empty")
	}
	cfg, err := oracle.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	c, err := oracle.NewConnector(cfg)
	if err != nil {
		t.Fatal(err)
	}
	db := sql.OpenDB(c)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	return db
}

// usable fails the test unless db answers a query.
func usable(t *testing.T, db *sql.DB) {
	t.Helper()
	var n int
	if err := db.QueryRowContext(t.Context(), "SELECT 1 FROM dual").Scan(&n); err != nil || n != 1 {
		t.Fatalf("the connection is not usable after the break: %v", err)
	}
}

func TestContextDeadlineBreaksQuery(t *testing.T) {
	db := openLive(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	start := time.Now()
	var n int64
	err := db.QueryRowContext(ctx, slowQuery).Scan(&n)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want context.DeadlineExceeded", err)
	}
	if d := time.Since(start); d > 15*time.Second {
		t.Errorf("the break took %v", d)
	}
	usable(t, db)
}

func TestContextDeadlineBreaksExec(t *testing.T) {
	db := openLive(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, err := db.ExecContext(ctx, "DECLARE n NUMBER; BEGIN SELECT COUNT(*) INTO n FROM all_objects a, all_objects b; END;")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want context.DeadlineExceeded", err)
	}
	usable(t, db)
}

func TestContextCancelledBeforeTheCall(t *testing.T) {
	db := openLive(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := db.ExecContext(ctx, "SELECT 1 FROM dual"); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	usable(t, db)
}

// TestCancelledTransactionDoesNotCommit is sijms/go-ora#767. When the context
// of a transaction ends, database/sql rolls it back, and the rollback must not
// be cancelled by the context that ended, or a commit after it commits.
func TestCancelledTransactionDoesNotCommit(t *testing.T) {
	db := openLive(t)
	const table = "xo_oracle_tx_cancel"
	ctx := t.Context()
	if _, err := db.ExecContext(ctx, "BEGIN EXECUTE IMMEDIATE 'DROP TABLE "+table+" PURGE'; EXCEPTION WHEN OTHERS THEN NULL; END;"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TABLE "+table+" (id NUMBER)"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.WithoutCancel(ctx), "DROP TABLE "+table+" PURGE")
	})
	txCtx, cancel := context.WithCancel(ctx)
	tx, err := db.BeginTx(txCtx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(txCtx, "INSERT INTO "+table+" (id) VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	cancel()
	// database/sql rolls the transaction back when txCtx ends
	time.Sleep(100 * time.Millisecond)
	if err := tx.Commit(); !errors.Is(err, sql.ErrTxDone) {
		t.Errorf("committing after the cancel: got %v, want sql.ErrTxDone", err)
	}
	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("the cancelled transaction committed %d rows", n)
	}
}

func TestBeginTxIsolation(t *testing.T) {
	db := openLive(t)
	ctx := t.Context()
	for _, opts := range []*sql.TxOptions{
		{Isolation: sql.LevelSerializable},
		{Isolation: sql.LevelReadCommitted},
		{ReadOnly: true},
	} {
		tx, err := db.BeginTx(ctx, opts)
		if err != nil {
			t.Errorf("BeginTx(%+v): %v", opts, err)
			continue
		}
		var n int
		if err := tx.QueryRowContext(ctx, "SELECT 1 FROM dual").Scan(&n); err != nil {
			t.Errorf("querying in %+v: %v", opts, err)
		}
		if err := tx.Rollback(); err != nil {
			t.Errorf("rolling back %+v: %v", opts, err)
		}
	}
	for _, opts := range []*sql.TxOptions{
		{Isolation: sql.LevelRepeatableRead},
		{Isolation: sql.LevelSerializable, ReadOnly: true},
	} {
		if _, err := db.BeginTx(ctx, opts); !errors.Is(err, oracle.ErrNotSupported) {
			t.Errorf("BeginTx(%+v): got %v, want ErrNotSupported", opts, err)
		}
	}
	usable(t, db)
}
