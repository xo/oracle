package oracle_test

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/xo/oracle"
	"github.com/xo/oracle/internal/tnstest"
)

// The exchanges under testdata/exchanges hold the packets of each scenario
// below, recorded against each release that dbrun starts. TestExchanges plays
// them back with no server. To record them again, start a release with dbrun
// and run:
//
//	ORACLE_DSN=$(dbrun dsn --json oracle-26ai | jq -r '.[0].url') \
//	ORACLE_RELEASE=oracle-26ai go test -run TestRecordExchanges -record .
//
// A change that keeps the packets and changes a result, such as the Go type of
// a column, rewrites the results with -update:
//
//	go test -run TestExchanges -update .
//
// See D15 and docs/DESIGN.md, under Testing.
var (
	record = flag.Bool("record", false, "record the exchanges against ORACLE_DSN into testdata/exchanges/$ORACLE_RELEASE")
	update = flag.Bool("update", false, "rewrite the results of each exchange from its replay, and keep its packets")
)

// exchangeHost and exchangePort are the address that every recorded
// connection names. The connect packet holds it, so a fixed one keeps the
// packets the same on every machine.
const (
	exchangeHost = "oracle.test"
	exchangePort = 1521
)

// exchangeClient is the client identity that every recorded connection
// sends, in place of the program, the host and the user of the machine.
var exchangeClient = oracle.Client{Program: "oracle.test", Host: "oracle.test", User: "oracle"}

// scenario runs statements on db and returns one line for each fact that a
// test compares.
type scenario func(ctx context.Context, db *sql.DB) ([]string, error)

var scenarios = map[string]scenario{
	"ping":  scenarioPing,
	"query": scenarioQuery,
	"error": scenarioError,
	"dml":   scenarioDML,
	"clob":  scenarioClob,
}

func scenarioPing(ctx context.Context, db *sql.DB) ([]string, error) {
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	return []string{"ping"}, nil
}

func scenarioQuery(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT
	1 AS n,
	'abc' AS s,
	1.5 AS f,
	CAST(NULL AS VARCHAR2(10)) AS nul,
	DATE '2024-01-02' AS d,
	TIMESTAMP '2024-01-02 03:04:05.123456' AS ts,
	HEXTORAW('DEADBEEF') AS r
FROM dual`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLines(rows)
}

func scenarioError(ctx context.Context, db *sql.DB) ([]string, error) {
	_, err := db.ExecContext(ctx, "SELECT * FROM xo_oracle_no_such_table")
	if err == nil {
		return nil, errors.New("selecting from a table that does not exist: no error")
	}
	return []string{"error: " + err.Error()}, nil
}

func scenarioDML(ctx context.Context, db *sql.DB) ([]string, error) {
	const table = "xo_oracle_exchange"
	stmts := []string{
		"BEGIN EXECUTE IMMEDIATE 'DROP TABLE " + table + " PURGE'; EXCEPTION WHEN OTHERS THEN NULL; END;",
		"CREATE TABLE " + table + " (id NUMBER(10) PRIMARY KEY, name VARCHAR2(50))",
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return nil, fmt.Errorf("running %q: %w", stmt, err)
		}
	}
	var lines []string
	for i, name := range []string{"one", "two", "three"} {
		res, err := db.ExecContext(ctx, "INSERT INTO "+table+" (id, name) VALUES (:1, :2)", i+1, name)
		if err != nil {
			return nil, fmt.Errorf("inserting %s: %w", name, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return nil, err
		}
		lines = append(lines, fmt.Sprintf("insert %s: %d", name, n))
	}
	res, err := db.ExecContext(ctx, "UPDATE "+table+" SET name = UPPER(name) WHERE id > :1", 1)
	if err != nil {
		return nil, fmt.Errorf("updating: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	lines = append(lines, fmt.Sprintf("update: %d", n))

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO "+table+" (id, name) VALUES (:1, :2)", 4, "four"); err != nil {
		_ = tx.Rollback()
		return nil, fmt.Errorf("inserting in a transaction: %w", err)
	}
	if err := tx.Rollback(); err != nil {
		return nil, fmt.Errorf("rolling back: %w", err)
	}

	rows, err := db.QueryContext(ctx, "SELECT id, name FROM "+table+" ORDER BY id")
	if err != nil {
		return nil, err
	}
	got, err := scanLines(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	lines = append(lines, got...)
	if _, err := db.ExecContext(ctx, "DROP TABLE "+table+" PURGE"); err != nil {
		return nil, fmt.Errorf("dropping: %w", err)
	}
	return lines, nil
}

func scenarioClob(ctx context.Context, db *sql.DB) ([]string, error) {
	var s string
	err := db.QueryRowContext(ctx, "SELECT TO_CLOB(RPAD('x', 4000, 'x')) || TO_CLOB(RPAD('y', 4000, 'y')) FROM dual").Scan(&s)
	if err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf("clob: %d %s %s", len(s), s[:3], s[len(s)-3:])}, nil
}

// scanLines returns one line for each row, with the Go type and the value of
// each column.
func scanLines(rows *sql.Rows) ([]string, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var lines []string
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		parts := make([]string, len(cols))
		for i, v := range vals {
			if t, ok := v.(time.Time); ok {
				v = t.Format(time.RFC3339Nano)
			}
			parts[i] = fmt.Sprintf("%s=%T(%v)", strings.ToLower(cols[i]), vals[i], v)
		}
		lines = append(lines, "row: "+strings.Join(parts, " "))
	}
	return lines, rows.Err()
}

// openExchange opens a database whose connections dial through dialer, at
// the fixed address and with the fixed client identity of a recording.
func openExchange(t *testing.T, dsn string, dialer oracle.Dialer) *sql.DB {
	t.Helper()
	cfg, err := oracle.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parsing the dsn: %v", err)
	}
	cfg.Host, cfg.Port = exchangeHost, exchangePort
	cfg.Client = exchangeClient
	cfg.Dialer = dialer
	conn, err := oracle.NewConnector(cfg)
	if err != nil {
		t.Fatal(err)
	}
	db := sql.OpenDB(conn)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return db
}

func TestRecordExchanges(t *testing.T) {
	if !*record {
		t.Skip("run with -record to record the exchanges")
	}
	dsn, release := os.Getenv("ORACLE_DSN"), os.Getenv("ORACLE_RELEASE")
	if dsn == "" || release == "" {
		t.Fatal("-record needs ORACLE_DSN and ORACLE_RELEASE")
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parsing ORACLE_DSN: %v", err)
	}
	dir := filepath.Join("testdata", "exchanges", release)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range slices.Sorted(maps.Keys(scenarios)) {
		t.Run(name, func(t *testing.T) {
			oracle.ResetCookies()
			rec := &tnstest.Recorder{Addr: u.Host}
			db := openExchange(t, dsn, rec)
			lines, err := scenarios[name](t.Context(), db)
			if err != nil {
				t.Fatalf("running the scenario: %v", err)
			}
			if err := db.Close(); err != nil {
				t.Fatalf("closing: %v", err)
			}
			exs, err := rec.Exchanges()
			if err != nil {
				t.Fatal(err)
			}
			if len(exs) != 1 {
				t.Fatalf("the scenario dialed %d connections, and an exchange holds one", len(exs))
			}
			ex := exs[0]
			// the password is the fixed test password of dbrun, and a replay
			// needs it to read the session key of the server
			pass, _ := u.User.Password()
			ex.Notes = append(ex.Notes,
				"release: "+release,
				"scenario: "+name,
				"user: "+u.User.Username(),
				"password: "+pass,
				"service: "+strings.TrimPrefix(u.Path, "/"),
			)
			for _, l := range lines {
				ex.Notes = append(ex.Notes, "result: "+strconv.Quote(l))
			}
			if err := ex.WriteFile(filepath.Join(dir, name+".tns")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestExchanges(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "exchanges", "*", "*.tns"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Skip("no recorded exchange")
	}
	for _, file := range files {
		release, name := filepath.Base(filepath.Dir(file)), strings.TrimSuffix(filepath.Base(file), ".tns")
		t.Run(release+"/"+name, func(t *testing.T) {
			ex, err := tnstest.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			run, ok := scenarios[ex.Note("scenario")]
			if !ok {
				t.Fatalf("no scenario %q", ex.Note("scenario"))
			}
			dsn := (&url.URL{
				Scheme: "oracle",
				User:   url.UserPassword(ex.Note("user"), ex.Note("password")),
				Host:   exchangeHost,
				Path:   "/" + ex.Note("service"),
			}).String()
			oracle.ResetCookies()
			player := tnstest.NewPlayer(ex)
			db := openExchange(t, dsn, player)
			got, err := run(t.Context(), db)
			if err != nil {
				t.Fatalf("running the scenario: %v", err)
			}
			if err := db.Close(); err != nil {
				t.Fatalf("closing: %v", err)
			}
			if err := player.Wait(); err != nil {
				t.Fatalf("playing the exchange: %v", err)
			}
			var want []string
			for _, v := range ex.NoteValues("result") {
				l, err := strconv.Unquote(v)
				if err != nil {
					t.Fatalf("reading the result %s: %v", v, err)
				}
				want = append(want, l)
			}
			if *update && !slices.Equal(got, want) {
				notes := slices.DeleteFunc(slices.Clone(ex.Notes), func(n string) bool {
					return strings.HasPrefix(n, "result:")
				})
				for _, l := range got {
					notes = append(notes, "result: "+strconv.Quote(l))
				}
				ex.Notes = notes
				if err := ex.WriteFile(file); err != nil {
					t.Fatal(err)
				}
				return
			}
			if !slices.Equal(got, want) {
				t.Errorf("results differ\n got: %q\nwant: %q", got, want)
			}
		})
	}
}
