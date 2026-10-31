package integration

import (
	"testing"
)

// TestIssue543 reads the LONG column_expression of a function based index.
// The issue read an index of the APEX schema, which this test makes itself.
func TestIssue543(t *testing.T) {
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
