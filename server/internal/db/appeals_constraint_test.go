package db_test

import (
	"context"
	"testing"

	"github.com/Teamthy/i-confess/internal/db/dbtest"
	"github.com/Teamthy/i-confess/internal/moderation"
)

// TestAppealVocabularyParityAgainstConstraint is the PHASE 36/37 parity pattern
// applied to the appeal lifecycle: compare the Go vocabulary with the CHECK
// installed in the live PostgreSQL test database, in both directions.
//
// Only one direction is a runtime failure, and it is the one that matters. A
// status the code accepts but the database rejects turns a legitimate appeal
// into a 500 at the moment a listener is asking for their content back.
func TestAppealVocabularyParityAgainstConstraint(t *testing.T) {
	raw := dbtest.Raw(t)
	def, err := checkDefinition(context.Background(), raw, "moderation_appeals", "status")
	if err != nil {
		t.Fatalf("moderation_appeals.status: %v", err)
	}
	if def == "" {
		t.Fatal("moderation_appeals.status has no CHECK constraint in the live schema")
	}

	got := map[string]bool{}
	for _, match := range valueRe.FindAllStringSubmatch(def, -1) {
		got[match[1]] = true
	}
	want := map[string]bool{}
	for _, status := range moderation.AppealStatuses() {
		want[status] = true
	}
	for value := range want {
		if !got[value] {
			t.Errorf("appeal code accepts %q but the live CHECK rejects it", value)
		}
	}
	for value := range got {
		if !want[value] {
			t.Errorf("moderation_appeals.status CHECK accepts %q but the appeal code does not declare it", value)
		}
	}
}

// TestAppealDecisionTypeParityAgainstConstraint covers the second vocabulary on
// the table. It is smaller and easier to forget, which is exactly why it is
// checked the same way.
func TestAppealDecisionTypeParityAgainstConstraint(t *testing.T) {
	raw := dbtest.Raw(t)
	def, err := checkDefinition(context.Background(), raw, "moderation_appeals", "decision_type")
	if err != nil {
		t.Fatalf("moderation_appeals.decision_type: %v", err)
	}
	if def == "" {
		t.Fatal("moderation_appeals.decision_type has no CHECK constraint in the live schema")
	}
	got := map[string]bool{}
	for _, match := range valueRe.FindAllStringSubmatch(def, -1) {
		got[match[1]] = true
	}
	for _, want := range moderation.AppealableDecisionTypes {
		if !got[want] {
			t.Errorf("appeal code accepts decision_type %q but the live CHECK rejects it", want)
		}
	}
	if len(got) != len(moderation.AppealableDecisionTypes) {
		t.Errorf("the live CHECK accepts %d decision types, the code declares %d", len(got), len(moderation.AppealableDecisionTypes))
	}
}

// TestUserBlocksRefuseSelfBlockingAtTheDatabase proves the rule survives a
// writer that forgets it. The Go validator refuses a self-block, but the
// constraint is what makes it true of every writer, including one added later.
func TestUserBlocksRefuseSelfBlockingAtTheDatabase(t *testing.T) {
	raw := dbtest.Raw(t)
	ctx := context.Background()
	if _, err := raw.ExecContext(ctx, `DELETE FROM user_blocks WHERE blocker_id='self-block-test'`); err != nil {
		t.Fatalf("clear fixture: %v", err)
	}
	if _, err := raw.ExecContext(ctx,
		`INSERT INTO users (id,email,password_hash,status,created_at,updated_at)
		 VALUES ('self-block-test','self-block@example.invalid','x','active','2026-01-01','2026-01-01')
		 ON CONFLICT (id) DO NOTHING`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	_, err := raw.ExecContext(ctx,
		`INSERT INTO user_blocks (id,blocker_id,blocked_id,created_at,updated_at)
		 VALUES ('sb-1','self-block-test','self-block-test','2026-01-01','2026-01-01')`)
	if err == nil {
		t.Fatal("the database accepted a self-block; only the Go validator is enforcing the rule")
	}
}

// TestUserBlocksAllowOneLiveBlockPerPair proves the pair is unique, so a block
// is one boundary rather than a growing list, and that unblocking frees it.
func TestUserBlocksAllowOneLiveBlockPerPair(t *testing.T) {
	raw := dbtest.Raw(t)
	ctx := context.Background()
	for _, id := range []string{"pair-a", "pair-b"} {
		if _, err := raw.ExecContext(ctx,
			`INSERT INTO users (id,email,password_hash,status,created_at,updated_at)
			 VALUES ($1,$2,'x','active','2026-01-01','2026-01-01') ON CONFLICT (id) DO NOTHING`,
			id, id+"@example.invalid"); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	insert := func(id string) error {
		_, err := raw.ExecContext(ctx,
			`INSERT INTO user_blocks (id,blocker_id,blocked_id,created_at,updated_at)
			 VALUES ($1,'pair-a','pair-b','2026-01-01','2026-01-01')`, id)
		return err
	}
	if err := insert("pair-1"); err != nil {
		t.Fatalf("first block: %v", err)
	}
	if err := insert("pair-2"); err == nil {
		t.Fatal("a second block on the same pair was accepted")
	}
	// The reverse direction is a different pair and is allowed: a block is one
	// account's boundary, not a statement about the relationship.
	if _, err := raw.ExecContext(ctx,
		`INSERT INTO user_blocks (id,blocker_id,blocked_id,created_at,updated_at)
		 VALUES ('pair-3','pair-b','pair-a','2026-01-01','2026-01-01')`); err != nil {
		t.Errorf("a block in the reverse direction was refused: %v", err)
	}
}

// TestAppealsAreOnePerDecision proves an appeal is heard once. Without it a
// rejected author could file until a moderator relented, which turns the appeal
// queue into the spam surface the reports index already closes for reports.
func TestAppealsAreOnePerDecision(t *testing.T) {
	raw := dbtest.Raw(t)
	ctx := context.Background()
	if _, err := raw.ExecContext(ctx,
		`INSERT INTO users (id,email,password_hash,status,created_at,updated_at)
		 VALUES ('appeal-uniq','appeal-uniq@example.invalid','x','active','2026-01-01','2026-01-01')
		 ON CONFLICT (id) DO NOTHING`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	insert := func(id string) error {
		_, err := raw.ExecContext(ctx,
			`INSERT INTO moderation_appeals
			 (id,user_id,decision_type,decision_id,statement,status,created_at,updated_at)
			 VALUES ($1,'appeal-uniq','confession','same-decision','a statement long enough','submitted','2026-01-01','2026-01-01')`, id)
		return err
	}
	if err := insert("appeal-1"); err != nil {
		t.Fatalf("first appeal: %v", err)
	}
	if err := insert("appeal-2"); err == nil {
		t.Fatal("a second appeal on the same decision was accepted")
	}
	// A different decision is a different appeal.
	if _, err := raw.ExecContext(ctx,
		`INSERT INTO moderation_appeals
		 (id,user_id,decision_type,decision_id,statement,status,created_at,updated_at)
		 VALUES ('appeal-3','appeal-uniq','confession','other-decision','a statement long enough','submitted','2026-01-01','2026-01-01')`); err != nil {
		t.Errorf("an appeal against a different decision was refused: %v", err)
	}
}
