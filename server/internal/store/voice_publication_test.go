package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/voiceengine"
	"github.com/Teamthy/i-confess/internal/voicegov"
	"github.com/lib/pq"
)

// An unlocked MVCC read could see APPROVED through an uncommitted withdrawal.
// Prove publication actually waits on the writer, rather than relying on how
// fast two goroutines happen to run. No clock sleeps or external I/O in a tx.
func TestRenderHandoffSerializesWithGrantWriter(t *testing.T) {
	s, id := newVoiceFixture(t)
	approve(t, s, id, allCaps())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	g, err := s.Grant(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	use := voicegov.RenderUse{VoiceID: id, GrantVersion: g.Version, Request: voicegov.Request{Action: voicegov.ActionStream, Purpose: voicegov.PurposeReflection}}
	writer, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback()
	if _, err := writer.ExecContext(ctx, `UPDATE voice_rights_grants SET status = 'SUSPENDED', grant_version = grant_version + 1 WHERE voice_id = ?`, id); err != nil {
		t.Fatal(err)
	}
	// The lock timeout belongs to this transaction only; a plain SELECT of
	// the previously committed grant would succeed instead of timing out.
	err = s.withRenderGrant(ctx, use, func(tx *db.Tx) error {
		if _, err := tx.ExecContext(ctx, `SET LOCAL lock_timeout = '200ms'`); err != nil {
			return err
		}
		return appendAudit(ctx, tx, RightsAuditEntry{VoiceID: id, Actor: "fixture", Action: "VOICE_STREAMED", Decision: "allowed"})
	})
	var pgErr *pq.Error
	if !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
		t.Fatalf("handoff did not wait for the grant writer: %v", err)
	}
	if err := writer.Commit(); err != nil {
		t.Fatal(err)
	}
	err = s.RecordStreamStart(ctx, use, RightsAuditEntry{Actor: "fixture"})
	var denied *voiceengine.ErrRightsDenied
	if !errors.As(err, &denied) || denied.Decision.Reason != voicegov.ReasonSuspended {
		t.Fatalf("handoff ignored the winning writer: %v", err)
	}
	entries, err := s.RightsAudit(ctx, id, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Action == "VOICE_STREAMED" {
			t.Fatal("a rolled-back handoff left an allowed audit")
		}
	}
}
