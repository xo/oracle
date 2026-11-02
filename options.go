package oracle

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/xo/oracle/internal/config"
	"github.com/xo/oracle/internal/ttc"
)

// Option sets one option of one statement (D109). An option comes from the
// DSN, then from the context through WithOptions, then from an argument of
// the statement, and a later one wins:
//
//	rows, err := db.QueryContext(ctx, query, oracle.WithPrefetch(1000))
type Option = ttc.Option

// WithOptions returns a context that carries opts, after the options that
// ctx carries. Each statement that runs with the context applies them.
func WithOptions(ctx context.Context, opts ...Option) context.Context {
	return ttc.WithOptions(ctx, opts...)
}

// WithTimeout ends the call of a statement after d, through the break of
// Oracle Net, which leaves the connection usable. Zero asks for nothing. It
// bounds the call that runs the statement, and not the fetches of its rows
// after it.
func WithTimeout(d time.Duration) Option {
	return func(o *ttc.Options) { o.Timeout = d }
}

// WithPrefetch sets the number of rows that one fetch asks for, as the key
// prefetch does.
func WithPrefetch(rows int) Option {
	return func(o *ttc.Options) { o.Prefetch = rows }
}

// WithBatchErrors runs a batch of DML past the rows that fail. The batch
// returns a *BatchError, which holds the index and the error of each row
// that failed, and the other rows run. A statement that is not DML fails
// with ErrNotSupported (D34).
func WithBatchErrors() Option {
	return func(o *ttc.Options) { o.BatchErrors = true }
}

// WithRowCounts fills counts with the count of the rows that each row of a
// batch of DML changed, or with the count of the one row of a statement
// that is not a batch. A statement that is not DML fails with
// ErrNotSupported (D34).
func WithRowCounts(counts *[]int64) Option {
	return func(o *ttc.Options) { o.RowCounts = counts }
}

// WithLobFetch sets how a query returns a LOB, as the key lob_fetch does.
func WithLobFetch(f LobFetch) Option {
	return func(o *ttc.Options) {
		switch f {
		case LobFetchInline:
			o.LobFetch = config.INLINE
		case LobFetchStream:
			o.LobFetch = config.STREAM
		default:
			o.Invalid = fmt.Errorf("applying the option WithLobFetch: %q: %w", f, ErrInvalidValue)
		}
	}
}

// WithReadonly asks that the server refuse a write. Oracle has it for a
// transaction, through sql.TxOptions, and not for one statement, so true
// fails the statement with ErrNotSupported.
func WithReadonly(readonly bool) Option {
	return func(o *ttc.Options) { o.Readonly = readonly }
}

// WithParameter sets a setting of the server by its name, for a setting
// that the driver has no option for. Oracle has none for one statement, so
// it fails the statement with ErrNotSupported. ALTER SESSION, or
// WithSessionParam on the connector, sets a setting of the session.
func WithParameter(name string, value any) Option {
	return func(o *ttc.Options) {
		o.Params = maps.Clone(o.Params)
		if o.Params == nil {
			o.Params = map[string]any{}
		}
		o.Params[name] = value
	}
}

// WithDatabase names the database of one statement. A connection of Oracle
// reaches one database, so a name fails the statement with
// ErrNotSupported.
func WithDatabase(name string) Option {
	return func(o *ttc.Options) { o.Database = name }
}
