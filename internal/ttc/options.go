package ttc

import (
	"context"
	"database/sql/driver"
	"fmt"
	"slices"
	"time"

	"github.com/xo/oracle/internal/config"
	"github.com/xo/oracle/internal/tns"
)

// Options are the options of one statement (D109). The DSN gives the first
// value of each, then the context through WithOptions, then the arguments
// of the statement, and a later one wins. This copies the machinery of
// dbimp, which the module cannot import.
type Options struct {
	// Timeout ends the call of a statement after it, through the break of
	// the context.
	Timeout time.Duration
	// Prefetch is the number of rows that one fetch asks for, and LobFetch
	// says how a query returns a LOB, as the keys of the DSN do.
	Prefetch int
	LobFetch config.LobFetch
	// Readonly, Params and Database are the options of every dbimp driver
	// that Oracle has no setting for one statement for, and each fails.
	Readonly bool
	Params   map[string]any
	Database string
	// BatchErrors runs a batch of DML past the rows that fail, and returns
	// a *BatchError with the error of each, and RowCounts receives the count
	// of each row of a batch (D34).
	BatchErrors bool
	RowCounts   *[]int64
	// Invalid is the error of an option whose value the DSN would refuse,
	// which fails the statement before it is sent.
	Invalid error
}

// Option sets one option of a statement.
type Option func(*Options)

// optionsKey is the key of the options in a context.
type optionsKey struct{}

// WithOptions returns a context that carries opts, after the options that
// ctx carries.
func WithOptions(ctx context.Context, opts ...Option) context.Context {
	prev, _ := ctx.Value(optionsKey{}).([]Option)
	return context.WithValue(ctx, optionsKey{}, slices.Concat(prev, opts))
}

// IsOption reports whether v is an Option, which CheckNamedValue keeps so
// that database/sql hands it to the statement.
func IsOption(v any) bool {
	_, ok := v.(Option)
	return ok
}

// resolve returns the options of a statement of conn, with the options of
// ctx and of args applied, and args without the options, numbered again
// from 1. It fails for an option that Oracle cannot honor for one
// statement, so that a caller never believes that it holds.
func resolve(ctx context.Context, conn *Connection, args []driver.NamedValue) (Options, []driver.NamedValue, error) {
	opts := Options{
		Prefetch: conn.connOption.PrefetchRows,
		LobFetch: conn.lobFetch,
	}
	fromCtx, _ := ctx.Value(optionsKey{}).([]Option)
	for _, opt := range fromCtx {
		opt(&opts)
	}
	var rest []driver.NamedValue
	for _, a := range args {
		if opt, ok := a.Value.(Option); ok {
			opt(&opts)
			continue
		}
		a.Ordinal = len(rest) + 1
		rest = append(rest, a)
	}
	switch {
	case opts.Invalid != nil:
		return opts, nil, opts.Invalid
	case opts.Timeout < 0:
		return opts, nil, fmt.Errorf("applying the option WithTimeout: %v: %w", opts.Timeout, tns.ErrInvalidValue)
	case opts.Prefetch < 1:
		return opts, nil, fmt.Errorf("applying the option WithPrefetch: %d: %w", opts.Prefetch, tns.ErrInvalidValue)
	case opts.Readonly:
		return opts, nil, unsupported("WithReadonly")
	case len(opts.Params) > 0:
		return opts, nil, unsupported("WithParameter")
	case opts.Database != "":
		return opts, nil, unsupported("WithDatabase")
	}
	return opts, rest, nil
}

// unsupported returns the error of an option that Oracle cannot honor for
// one statement.
func unsupported(option string) error {
	return fmt.Errorf("applying the option %s: Oracle has no such setting for one statement: %w", option, tns.ErrNotSupported)
}

// apply sets the options on stmt, and returns the context of its call.
func (stmt *Stmt) apply(ctx context.Context, opts Options) (context.Context, context.CancelFunc) {
	stmt.batchErrors, stmt.rowCounts = opts.BatchErrors, opts.RowCounts
	stmt.prefetch = opts.Prefetch
	stmt.lob = opts.LobFetch
	stmt.connection.connOption.Lob = stmt.lob
	if opts.Timeout > 0 {
		return context.WithTimeout(ctx, opts.Timeout)
	}
	return ctx, func() {}
}
