package deletion

import (
	"context"
	"strings"
	"testing"
)

func TestPolicySchemaCheckReportsMissingTableBeforeErasure(t *testing.T) {
	conn := newDB(t)
	tx, err := conn.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(context.Background(), `ALTER TABLE user_blocks RENAME TO user_blocks_hidden`); err != nil {
		t.Fatalf("simulate missing policy table: %v", err)
	}
	if err := checkPolicySchema(context.Background(), tx); err == nil || !strings.Contains(err.Error(), `policy table "user_blocks"`) {
		t.Fatalf("schema check error = %v, want missing user_blocks table", err)
	}
}

func TestEraseFailsBeforeAnyPolicyWhenSchemaIsIncomplete(t *testing.T) {
	conn := newDB(t)
	seedUser(t, conn, "schema-user", "schema-user@example.com")
	if _, err := conn.Exec(`ALTER TABLE user_blocks RENAME TO user_blocks_hidden`); err != nil {
		t.Fatalf("simulate missing policy table: %v", err)
	}

	_, err := newService(t, conn).Erase(context.Background(), "schema-user")
	if err == nil || !strings.Contains(err.Error(), `policy table "user_blocks"`) {
		t.Fatalf("Erase error = %v, want a preflight error naming user_blocks", err)
	}
	if got := count(t, conn, "refresh_tokens", "user_id = ?", "schema-user"); got != 1 {
		t.Fatalf("preflight failure applied earlier policies: refresh_tokens = %d, want 1", got)
	}
	var email string
	if err := conn.QueryRow(`SELECT email FROM users WHERE id = ?`, "schema-user").Scan(&email); err != nil {
		t.Fatal(err)
	}
	if email != "schema-user@example.com" {
		t.Fatalf("preflight failure tombstoned the user: email=%q", email)
	}
}

func TestPolicySchemaCheckReportsMissingColumnBeforeErasure(t *testing.T) {
	conn := newDB(t)
	tx, err := conn.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(context.Background(), `ALTER TABLE community_posts RENAME COLUMN author_id TO author_id_hidden`); err != nil {
		t.Fatalf("simulate missing policy column: %v", err)
	}
	if err := checkPolicySchema(context.Background(), tx); err == nil || !strings.Contains(err.Error(), `policy column "community_posts"."author_id"`) {
		t.Fatalf("schema check error = %v, want missing community_posts.author_id column", err)
	}
}

func TestPolicyExecutionStopsAtTheFailureThatAbortsPostgresTransaction(t *testing.T) {
	conn := newDB(t)
	tx, err := conn.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	report := &ErasureReport{PerTable: map[string]int{}}
	policies := []TablePolicy{
		{Table: "user_blocks", Action: Erase, Column: "missing_column"},
		{Table: "favorites", Action: Erase},
	}
	err = applyPolicies(context.Background(), tx, policies, "no-such-user", report)
	if err == nil || !strings.Contains(err.Error(), "apply policy for user_blocks") {
		t.Fatalf("policy error = %v, want the original user_blocks failure", err)
	}
	if strings.Contains(err.Error(), "favorites") {
		t.Fatalf("failure was misattributed to a later policy: %v", err)
	}
}
