package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// These tests drive PATCH /sessions/{id} through the real router. Before the
// session state machine existed, that endpoint validated only that the
// requested status was one of four known strings — so a client could move a
// freshly created session straight to "completed" without playing a second of
// audio. Session completion is a primary product metric; a metric the client
// can write is not a metric.
//
// The unit tests in internal/sessions prove the transition table is sound. What
// is proven here is that the HTTP layer actually consults it.

func (f *audioFixture) patchStatus(t *testing.T, token, sessionID, status string) (int, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"status": status})
	req := httptest.NewRequest("PATCH", "/sessions/"+sessionID, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	var out struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Status == "" {
		out.Status = out.Error
	}
	return rec.Code, out.Status
}

func (f *audioFixture) getSession(t *testing.T, token, sessionID string) (int, sessionResp) {
	t.Helper()
	req := httptest.NewRequest("GET", "/sessions/"+sessionID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	var out sessionResp
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

// TestSessionCompletionCannotBeForged is the headline: a session that never
// played audio cannot be marked complete, no matter what the client sends.
func TestSessionCompletionCannotBeForged(t *testing.T) {
	f := newAudioFixture(t)

	code, sess := f.createSession(t, f.freeTok, f.voiceStd, 120)
	if code != http.StatusCreated {
		t.Fatalf("create session: %d", code)
	}

	// A freshly built session is READY, not some ad-hoc initial string.
	code, got := f.getSession(t, f.freeTok, sess.ID)
	if code != http.StatusOK {
		t.Fatalf("get session: %d", code)
	}
	if got.Status != "READY" {
		t.Fatalf("new session status = %q, want READY", got.Status)
	}

	// The attack: declare completion directly.
	code, msg := f.patchStatus(t, f.freeTok, sess.ID, "COMPLETED")
	if code != http.StatusConflict {
		t.Fatalf("PATCH COMPLETED on an unplayed session = %d, want 409", code)
	}
	if msg == "" {
		t.Error("409 returned with no explanation for the client")
	}

	// Legacy spelling must be refused too, or it is a bypass.
	if code, _ := f.patchStatus(t, f.freeTok, sess.ID, "completed"); code != http.StatusConflict {
		t.Errorf("PATCH legacy 'completed' on an unplayed session = %d, want 409", code)
	}

	// Nothing was persisted by the refused attempts.
	if _, got := f.getSession(t, f.freeTok, sess.ID); got.Status != "READY" {
		t.Errorf("status after refused completion = %q, want READY (refused writes must not persist)", got.Status)
	}

	// The legitimate path still works.
	if code, s := f.patchStatus(t, f.freeTok, sess.ID, "STARTING"); code != http.StatusOK {
		t.Fatalf("PATCH STARTING = %d (%s), want 200", code, s)
	}
	if code, s := f.patchStatus(t, f.freeTok, sess.ID, "ACTIVE"); code != http.StatusOK {
		t.Fatalf("PATCH ACTIVE = %d (%s), want 200", code, s)
	}
	code, s := f.patchStatus(t, f.freeTok, sess.ID, "COMPLETED")
	if code != http.StatusOK {
		t.Fatalf("PATCH COMPLETED after playback = %d (%s), want 200", code, s)
	}
	if s != "COMPLETED" {
		t.Errorf("response status = %q, want canonical COMPLETED", s)
	}
}

// TestSessionStatusIsNormalisedOnTheWayIn proves legacy wire values still work
// for older clients, and that the server converges them onto the canonical
// vocabulary rather than storing two dialects.
func TestSessionStatusIsNormalisedOnTheWayIn(t *testing.T) {
	f := newAudioFixture(t)
	_, sess := f.createSession(t, f.freeTok, f.voiceStd, 120)

	code, s := f.patchStatus(t, f.freeTok, sess.ID, "playing")
	if code != http.StatusOK {
		t.Fatalf("PATCH legacy 'playing' = %d (%s), want 200", code, s)
	}
	if s != "ACTIVE" {
		t.Errorf("legacy 'playing' returned %q, want canonical ACTIVE", s)
	}
	if _, got := f.getSession(t, f.freeTok, sess.ID); got.Status != "ACTIVE" {
		t.Errorf("persisted status = %q, want ACTIVE", got.Status)
	}

	if code, _ := f.patchStatus(t, f.freeTok, sess.ID, "abandoned"); code != http.StatusOK {
		t.Errorf("PATCH legacy 'abandoned' = %d, want 200", code)
	}
	if _, got := f.getSession(t, f.freeTok, sess.ID); got.Status != "CANCELLED" {
		t.Errorf("legacy 'abandoned' persisted as %q, want CANCELLED", got.Status)
	}
}

func TestSessionStatusRejectsUnknownValues(t *testing.T) {
	f := newAudioFixture(t)
	_, sess := f.createSession(t, f.freeTok, f.voiceStd, 120)

	for _, bad := range []string{"", "finished", "COMPLETED!", "ready ready", "null"} {
		if code, _ := f.patchStatus(t, f.freeTok, sess.ID, bad); code != http.StatusBadRequest {
			t.Errorf("PATCH %q = %d, want 400", bad, code)
		}
	}
	// A rejected value must leave the session untouched.
	if _, got := f.getSession(t, f.freeTok, sess.ID); got.Status != "READY" {
		t.Errorf("status after rejected values = %q, want READY", got.Status)
	}
}

// TestSessionTerminalStatesAreFinal checks that a completed session cannot be
// reopened, which would let a client replay or retract a completion.
func TestSessionTerminalStatesAreFinal(t *testing.T) {
	f := newAudioFixture(t)
	_, sess := f.createSession(t, f.freeTok, f.voiceStd, 120)

	for _, s := range []string{"STARTING", "ACTIVE", "COMPLETED"} {
		if code, msg := f.patchStatus(t, f.freeTok, sess.ID, s); code != http.StatusOK {
			t.Fatalf("PATCH %s = %d (%s), want 200", s, code, msg)
		}
	}
	for _, back := range []string{"ACTIVE", "READY", "PAUSED", "SCHEDULED"} {
		if code, _ := f.patchStatus(t, f.freeTok, sess.ID, back); code != http.StatusConflict {
			t.Errorf("PATCH %s on a completed session = %d, want 409", back, code)
		}
	}
	// Replaying the completion is an idempotent no-op, not a conflict: a
	// retried request from a flaky mobile client must not error.
	if code, _ := f.patchStatus(t, f.freeTok, sess.ID, "COMPLETED"); code != http.StatusOK {
		t.Errorf("replaying COMPLETED = %d, want 200 (idempotent)", code)
	}
}

// TestSessionStartedAtSurvivesPauseResume guards listening-time accounting.
// Resuming must not move started_at, or every pause/resume would silently
// shorten the recorded session.
func TestSessionStartedAtSurvivesPauseResume(t *testing.T) {
	f := newAudioFixture(t)
	_, sess := f.createSession(t, f.freeTok, f.voiceStd, 120)

	if code, msg := f.patchStatus(t, f.freeTok, sess.ID, "ACTIVE"); code != http.StatusOK {
		t.Fatalf("PATCH ACTIVE = %d (%s)", code, msg)
	}
	_, first := f.getSession(t, f.freeTok, sess.ID)
	if first.StartedAt == "" {
		t.Fatal("started_at not stamped on ACTIVE")
	}

	for _, s := range []string{"PAUSED", "ACTIVE", "PAUSED", "ACTIVE"} {
		if code, msg := f.patchStatus(t, f.freeTok, sess.ID, s); code != http.StatusOK {
			t.Fatalf("PATCH %s = %d (%s)", s, code, msg)
		}
	}
	_, after := f.getSession(t, f.freeTok, sess.ID)
	if after.StartedAt != first.StartedAt {
		t.Errorf("started_at moved across pause/resume: %q -> %q", first.StartedAt, after.StartedAt)
	}
}

// TestSessionStatusOwnership confirms the rewrite kept the existing
// authorisation check: a session belongs to its creator.
func TestSessionStatusOwnership(t *testing.T) {
	f := newAudioFixture(t)
	_, sess := f.createSession(t, f.freeTok, f.voiceStd, 120)

	other := f.register(t, "intruder@test.com")
	if code, _ := f.patchStatus(t, other, sess.ID, "ACTIVE"); code != http.StatusForbidden {
		t.Errorf("PATCH by another user = %d, want 403", code)
	}
	if code, _ := f.patchStatus(t, "not-a-token", sess.ID, "ACTIVE"); code != http.StatusUnauthorized {
		t.Errorf("PATCH unauthenticated = %d, want 401", code)
	}
	if code, _ := f.patchStatus(t, f.freeTok, "no-such-session", "ACTIVE"); code != http.StatusNotFound {
		t.Errorf("PATCH unknown session = %d, want 404", code)
	}
}
