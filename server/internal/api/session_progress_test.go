package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/Teamthy/i-confess/internal/sessions"
)

// queueItemID returns one queue item belonging to a session.
func (f *audioFixture) queueItemID(t *testing.T, sessionID string) string {
	t.Helper()
	var id string
	err := f.db.QueryRowContext(context.Background(),
		`SELECT id FROM session_items WHERE session_id = $1 ORDER BY position LIMIT 1`, sessionID).Scan(&id)
	if err != nil {
		t.Fatalf("no queue items for %s: %v", sessionID, err)
	}
	return id
}

// TestProgressCannotRewriteAnotherUsersQueue covers a gap the playback
// endpoints otherwise close.
//
// skipSessionItem deliberately verifies the submitted item belongs to the
// session being addressed. syncProgress did not: it passed queue_item_id
// straight to UpdateItemStatus, which is keyed on the item alone. So a user
// with any session of their own could address someone else's queue item through
// their own session's URL - an endpoint the victim never authorised, on the
// only path that writes item status during normal playback.
func TestProgressCannotRewriteAnotherUsersQueue(t *testing.T) {
	f := newAudioFixture(t)

	_, mine := f.createSession(t, f.freeTok, f.voiceStd, 60)
	_, theirs := f.createSession(t, f.premTok, f.voiceStd, 60)
	if mine.ID == "" || theirs.ID == "" {
		t.Fatalf("fixture did not build both sessions (%q, %q)", mine.ID, theirs.ID)
	}

	victim := f.queueItemID(t, theirs.ID)

	// Addressed through the attacker's own session, which they do own.
	code, _ := f.call(t, "POST", "/sessions/"+mine.ID+"/progress", f.freeTok, map[string]any{
		"queue_item_id": victim,
		"item_status":   "skipped",
		"position_ms":   1000,
	})
	if code == http.StatusOK {
		t.Errorf("progress accepted a queue item from another user's session; want a 4xx")
	}

	var status string
	if err := f.db.QueryRowContext(context.Background(),
		`SELECT status FROM session_items WHERE id = $1`, victim).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(sessions.ItemQueued) {
		t.Errorf("another user's queue item was rewritten to %q; it must stay %s",
			status, sessions.ItemQueued)
	}
}

// TestProgressStillRecordsYourOwnQueue is the other half: closing the hole must
// not break the legitimate push, which is how a client keeps its position
// across devices.
func TestProgressStillRecordsYourOwnQueue(t *testing.T) {
	f := newAudioFixture(t)

	_, mine := f.createSession(t, f.freeTok, f.voiceStd, 60)
	item := f.queueItemID(t, mine.ID)

	code, out := f.call(t, "POST", "/sessions/"+mine.ID+"/progress", f.freeTok, map[string]any{
		"queue_item_id": item,
		"item_status":   "completed",
		"position_ms":   4200,
	})
	if code != http.StatusOK {
		t.Fatalf("legitimate progress push: %d %v", code, out)
	}
	if applied, _ := out["applied"].(bool); !applied {
		t.Errorf("progress push was not applied: %v", out)
	}

	var status string
	if err := f.db.QueryRowContext(context.Background(),
		`SELECT status FROM session_items WHERE id = $1`, item).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(sessions.ItemCompleted) {
		t.Errorf("own item status = %q, want %s", status, sessions.ItemCompleted)
	}
}
