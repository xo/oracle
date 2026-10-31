package oracle_test

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/xo/oracle"
)

// records is a slog.Handler that keeps each record it handles.
type records struct {
	mu   sync.Mutex
	recs []slog.Record
}

func (h *records) Enabled(context.Context, slog.Level) bool { return true }

func (h *records) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.recs = append(h.recs, r.Clone())
	return nil
}

func (h *records) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *records) WithGroup(string) slog.Handler      { return h }

// TestLogger checks that Config.Logger receives the packets of a
// connection, and only the length of each packet until the logon ends, since
// those carry the encrypted session key and password (D14).
func TestLogger(t *testing.T) {
	dsn := os.Getenv("ORACLE_DSN")
	if dsn == "" {
		t.Skip("ORACLE_DSN is empty")
	}
	cfg, err := oracle.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	h := new(records)
	cfg.Logger = slog.New(h)
	c, err := oracle.NewConnector(cfg)
	if err != nil {
		t.Fatal(err)
	}
	db := sql.OpenDB(c)
	t.Cleanup(func() { db.Close() })
	var n int
	if err := db.QueryRowContext(t.Context(), "SELECT 42 FROM dual").Scan(&n); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	connected, withheld, shown, query := false, 0, 0, false
	for _, r := range h.recs {
		var bytes string
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "bytes" {
				bytes = a.Value.Resolve().String()
			}
			if a.Key == "query" && strings.Contains(a.Value.String(), "SELECT 42") {
				query = true
			}
			return true
		})
		switch {
		case r.Message == "connected":
			connected = true
		case strings.HasSuffix(r.Message, " packet"):
			if bytes == "withheld" {
				if connected {
					t.Errorf("a packet after the logon is withheld")
				}
				withheld++
				continue
			}
			if !connected {
				t.Errorf("%s before the logon shows its bytes", r.Message)
			}
			shown++
		}
		if pass := cfg.Password; pass != "" && strings.Contains(bytes, pass) {
			t.Errorf("%s holds the password", r.Message)
		}
	}
	if !connected || withheld == 0 || shown == 0 || !query {
		t.Errorf("connected %t, %d packets withheld, %d shown, the query logged %t", connected, withheld, shown, query)
	}
}
