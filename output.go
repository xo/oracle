package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Execer runs a statement. *sql.Conn and *sql.Tx are each one, and each
// holds one session, which DBMS_OUTPUT belongs to. A *sql.DB is one too, but
// each call can take another session from its pool, so it reads no output.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// EnableOutput enables DBMS_OUTPUT in the session of e, with a buffer of
// size bytes, or with no limit when size is 0.
func EnableOutput(ctx context.Context, e Execer, size int) error {
	var limit any
	if size > 0 {
		limit = size
	}
	if _, err := e.ExecContext(ctx, "BEGIN DBMS_OUTPUT.ENABLE(:1); END;", limit); err != nil {
		return fmt.Errorf("enabling DBMS_OUTPUT: %w", err)
	}
	return nil
}

// DisableOutput disables DBMS_OUTPUT in the session of e, and drops the
// lines that it holds.
func DisableOutput(ctx context.Context, e Execer) error {
	if _, err := e.ExecContext(ctx, "BEGIN DBMS_OUTPUT.DISABLE; END;"); err != nil {
		return fmt.Errorf("disabling DBMS_OUTPUT: %w", err)
	}
	return nil
}

// readOutput reads every line of DBMS_OUTPUT into a CLOB, a line feed after
// each, in one round trip.
const readOutput = `DECLARE
	l VARCHAR2(32767);
	s INTEGER;
	c CLOB;
BEGIN
	DBMS_LOB.CREATETEMPORARY(c, TRUE);
	LOOP
		DBMS_OUTPUT.GET_LINE(l, s);
		EXIT WHEN s = 1;
		DBMS_LOB.WRITEAPPEND(c, NVL(LENGTH(l), 0) + 1, l || CHR(10));
	END LOOP;
	:1 := c;
END;`

// ReadOutput reads and removes the lines that DBMS_OUTPUT holds in the
// session of e, which EnableOutput enabled. go-ora read them through
// dbms.NewOutput.
func ReadOutput(ctx context.Context, e Execer) ([]string, error) {
	var clob Clob
	if _, err := e.ExecContext(ctx, readOutput, sql.Out{Dest: &clob}); err != nil {
		return nil, fmt.Errorf("reading DBMS_OUTPUT: %w", err)
	}
	v, err := clob.Value()
	if err != nil {
		return nil, fmt.Errorf("reading DBMS_OUTPUT: %w", err)
	}
	text, _ := v.(string)
	if text == "" {
		return nil, nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n"), nil
}
