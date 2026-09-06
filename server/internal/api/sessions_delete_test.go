package api

import (
	"context"
	"net/http"
	"testing"
)

// TestOwnerCanDeleteASession covers the operation the directive lists and the
// API did not have. Before this there was no way for a listener to remove a
// session from their history.
func TestOwnerCanDeleteASession(t *testing.T) {
	f := newAudioFixture(t)
	_, sess := f.createSession(t, f.freeTok, f.voiceStd, 120)

	if code, _ := f.call(t, "DELETE", "/sessions/"+sess.ID, f.freeTok, nil); code != http.StatusNoContent {
		t.Fatalf("DELETE = %d, want 204", code)
	}
	if code, _ := f.getSession(t, f.freeTok, sess.ID); code != http.StatusNotFound {
		t.Errorf("GET after delete = %d, want 404", code)
	}

	// Gone from history too, not just from the direct read.
	code, out := f.call(t, "GET", "/sessions", f.freeTok, nil)
	if code != http.StatusOK {
		t.Fatalf("list = %d", code)
	}
	for _, s := range listOf(t, out) {
		if id, _ := s["id"].(string); id == sess.ID {
			t.Error("a deleted session is still listed in history")
		}
	}
}

// TestDeleteIsSoft verifies the row survives. It is the record of what the
// listener heard, and streaks and completion metrics are derived from it, so a
// deletion they can perform must not silently edit those numbers.
func TestDeleteIsSoft(t *testing.T) {
	f := newAudioFixture(t)
	ctx := context.Background()
	_, sess := f.createSession(t, f.freeTok, f.voiceStd, 120)

	if code, _ := f.call(t, "DELETE", "/sessions/"+sess.ID, f.freeTok, nil); code != http.StatusNoContent {
		t.Fatalf("DELETE = %d, want 204", code)
	}

	var deletedAt string
	err := f.db.QueryRowContext(ctx,
		`SELECT COALESCE(deleted_at,'') FROM sessions WHERE id = $1`, sess.ID).Scan(&deletedAt)
	if err != nil {
		t.Fatalf("the session row is gone; deletion must be soft: %v", err)
	}
	if deletedAt == "" {
		t.Error("deleted_at is empty on a deleted session")
	}

	// The queue snapshot goes with it, which is what makes the history entry
	// reconstructable if a listener ever asks for it back.
	var items int
	if err := f.db.QueryRowContext(ctx,
		`SELECT count(*) FROM session_items WHERE session_id = $1`, sess.ID).Scan(&items); err != nil {
		t.Fatal(err)
	}
	if items == 0 {
		t.Error("deleting a session destroyed its queue")
	}
}

// TestDeleteCancelsALiveSession is the state-machine half. Removing a session
// the player believes is playing, without cancelling it, leaves a client holding
// a live session and nothing that can tell it otherwise.
func TestDeleteCancelsALiveSession(t *testing.T) {
	f := newAudioFixture(t)
	_, sess := f.createSession(t, f.freeTok, f.voiceStd, 120)

	if code, _ := f.call(t, "POST", "/sessions/"+sess.ID+"/start", f.freeTok, nil); code != http.StatusOK {
		t.Fatalf("start = %d, want 200", code)
	}
	if code, _ := f.call(t, "DELETE", "/sessions/"+sess.ID, f.freeTok, nil); code != http.StatusNoContent {
		t.Fatalf("DELETE on an active session = %d, want 204", code)
	}

	var status string
	if err := f.db.QueryRowContext(context.Background(),
		`SELECT status FROM sessions WHERE id = $1`, sess.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "CANCELLED" {
		t.Errorf("an active session was deleted with status %q, want CANCELLED", status)
	}
}

// TestDeletedSessionCannotBeResumed checks the deletion actually takes the
// session out of service. A row that is merely hidden from the list but still
// playable is not deleted in any sense a listener means.
func TestDeletedSessionCannotBeResumed(t *testing.T) {
	f := newAudioFixture(t)
	_, sess := f.createSession(t, f.freeTok, f.voiceStd, 120)
	if code, _ := f.call(t, "POST", "/sessions/"+sess.ID+"/start", f.freeTok, nil); code != http.StatusOK {
		t.Fatalf("start = %d", code)
	}
	if code, _ := f.call(t, "DELETE", "/sessions/"+sess.ID, f.freeTok, nil); code != http.StatusNoContent {
		t.Fatalf("DELETE = %d", code)
	}

	for _, op := range []string{"start", "pause", "resume", "complete", "skip"} {
		if code, _ := f.call(t, "POST", "/sessions/"+sess.ID+"/"+op, f.freeTok, nil); code != http.StatusNotFound {
			t.Errorf("%s on a deleted session = %d, want 404", op, code)
		}
	}
	if code, _ := f.call(t, "GET", "/sessions/"+sess.ID+"/queue", f.freeTok, nil); code != http.StatusNotFound {
		t.Errorf("queue on a deleted session = %d, want 404", code)
	}
}

// TestCannotDeleteAnotherUsersSession is the authorisation check. Ownership is
// verified before anything is written, so a guessed id cannot remove someone
// else's history.
func TestCannotDeleteAnotherUsersSession(t *testing.T) {
	f := newAudioFixture(t)
	_, theirs := f.createSession(t, f.premTok, f.voiceStd, 120)

	code, _ := f.call(t, "DELETE", "/sessions/"+theirs.ID, f.freeTok, nil)
	if code != http.StatusForbidden {
		t.Fatalf("DELETE of another user's session = %d, want 403", code)
	}
	if code, _ := f.getSession(t, f.premTok, theirs.ID); code != http.StatusOK {
		t.Errorf("the victim's session is no longer readable: %d", code)
	}
}

// TestDeleteIsIdempotent covers the double-tap. A second DELETE must not report
// success for a deletion that did not happen.
func TestDeleteIsIdempotent(t *testing.T) {
	f := newAudioFixture(t)
	_, sess := f.createSession(t, f.freeTok, f.voiceStd, 120)

	if code, _ := f.call(t, "DELETE", "/sessions/"+sess.ID, f.freeTok, nil); code != http.StatusNoContent {
		t.Fatalf("first DELETE = %d, want 204", code)
	}
	if code, _ := f.call(t, "DELETE", "/sessions/"+sess.ID, f.freeTok, nil); code != http.StatusNotFound {
		t.Errorf("second DELETE = %d, want 404", code)
	}
}

func TestDeleteUnknownSessionIsNotFound(t *testing.T) {
	f := newAudioFixture(t)
	if code, _ := f.call(t, "DELETE", "/sessions/does-not-exist", f.freeTok, nil); code != http.StatusNotFound {
		t.Errorf("DELETE of an unknown id = %d, want 404", code)
	}
}

func TestDeleteRequiresAuthentication(t *testing.T) {
	f := newAudioFixture(t)
	_, sess := f.createSession(t, f.freeTok, f.voiceStd, 120)
	if code, _ := f.call(t, "DELETE", "/sessions/"+sess.ID, "", nil); code != http.StatusUnauthorized {
		t.Errorf("unauthenticated DELETE = %d, want 401", code)
	}
}

// listOf pulls the sessions array out of the history envelope.
func listOf(t *testing.T, out map[string]any) []map[string]any {
	t.Helper()
	raw, ok := out["sessions"].([]any)
	if !ok {
		t.Fatalf("history response has no sessions array: %v", out)
	}
	list := make([]map[string]any, 0, len(raw))
	for _, v := range raw {
		if m, ok := v.(map[string]any); ok {
			list = append(list, m)
		}
	}
	return list
}
