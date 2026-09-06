package api

import (
	"net/http"
	"testing"

	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

// TestSessionAPISurfaceIsComplete encodes the PHASE 17 requirement as an
// assertion: every operation the directive names must have a route.
//
// It reads the live route table rather than the source, so a route registered
// under a typo'd pattern does not count. The /v1/ twins are checked by
// TestRouteParityBetweenPrefixes.
func TestSessionAPISurfaceIsComplete(t *testing.T) {
	h := NewHandler(Config{JWTSecret: "test-secret-value", TokenTTL: "24h"}, dbtest.New(t))
	h.BuildEngine()
	h.Routes()

	type key struct{ method, path string }
	have := map[key]bool{}
	for _, r := range h.RouteTable() {
		have[key{r.Method, r.Path}] = true
	}

	required := []key{
		{"POST", "/sessions"},               // creation
		{"GET", "/sessions"},                // retrieval (history)
		{"GET", "/sessions/{id}"},           // retrieval (one)
		{"PATCH", "/sessions/{id}"},         // update
		{"DELETE", "/sessions/{id}"},        // delete
		{"POST", "/sessions/{id}/start"},    // start
		{"POST", "/sessions/{id}/pause"},    // pause
		{"POST", "/sessions/{id}/resume"},   // resume
		{"POST", "/sessions/{id}/complete"}, // complete
		{"POST", "/sessions/{id}/skip"},     // skip
		{"GET", "/sessions/{id}/queue"},     // queue
		{"POST", "/sessions/{id}/progress"}, // progress
	}
	for _, want := range required {
		if !have[want] {
			t.Errorf("%s %s is missing from the session API", want.method, want.path)
		}
	}
}

// TestDedicatedPlaybackEndpoints covers the start/pause/resume endpoints
// directly. Every existing test drove those transitions through PATCH
// /sessions/{id}, so the dedicated routes - the ones a player actually calls -
// were routed but never exercised.
func TestDedicatedPlaybackEndpoints(t *testing.T) {
	f := newAudioFixture(t)
	_, sess := f.createSession(t, f.freeTok, f.voiceStd, 180)
	base := "/sessions/" + sess.ID + "/"

	// start -> pause -> resume -> pause -> complete
	for _, step := range []struct {
		op   string
		want string
	}{
		{"start", "ACTIVE"},
		{"pause", "PAUSED"},
		{"resume", "ACTIVE"},
		{"pause", "PAUSED"},
		{"complete", "COMPLETED"},
	} {
		code, out := f.call(t, "POST", base+step.op, f.freeTok, nil)
		if code != http.StatusOK {
			t.Fatalf("%s = %d %v, want 200", step.op, code, out)
		}
		if got, _ := out["status"].(string); got != step.want && step.op != "complete" {
			t.Errorf("%s left the session %q, want %q", step.op, got, step.want)
		}
	}
}

// TestPlaybackEndpointRejectsIllegalTransitions checks the dedicated routes go
// through the state machine and not around it. COMPLETED is terminal, so nothing
// may follow it.
func TestPlaybackEndpointRejectsIllegalTransitions(t *testing.T) {
	f := newAudioFixture(t)
	_, sess := f.createSession(t, f.freeTok, f.voiceStd, 180)
	base := "/sessions/" + sess.ID + "/"

	if code, _ := f.call(t, "POST", base+"complete", f.freeTok, nil); code != http.StatusConflict {
		t.Fatalf("completing a session that never played = %d, want 409", code)
	}
	if code, _ := f.call(t, "POST", base+"start", f.freeTok, nil); code != http.StatusOK {
		t.Fatalf("start = %d, want 200", code)
	}
	if code, _ := f.call(t, "POST", base+"complete", f.freeTok, nil); code != http.StatusOK {
		t.Fatalf("complete from ACTIVE = %d, want 200", code)
	}
	if code, out := f.call(t, "POST", base+"resume", f.freeTok, nil); code != http.StatusConflict {
		t.Errorf("resuming a COMPLETED session = %d %v, want 409", code, out)
	}
}

// TestPlaybackEndpointsAreAuthorised checks ownership on the dedicated routes.
// The generic PATCH endpoint has its own check; these go through ownSession, and
// a missing check here would let any signed-in user drive anyone's player.
func TestPlaybackEndpointsAreAuthorised(t *testing.T) {
	f := newAudioFixture(t)
	_, theirs := f.createSession(t, f.premTok, f.voiceStd, 180)
	base := "/sessions/" + theirs.ID + "/"

	for _, op := range []string{"start", "pause", "resume", "complete", "skip"} {
		if code, _ := f.call(t, "POST", base+op, f.freeTok, nil); code != http.StatusForbidden {
			t.Errorf("%s on another user's session = %d, want 403", op, code)
		}
	}
	if code, _ := f.call(t, "GET", "/sessions/"+theirs.ID+"/queue", f.freeTok, nil); code != http.StatusForbidden {
		t.Errorf("queue on another user's session = %d, want 403", code)
	}
	if code, _ := f.getSession(t, f.freeTok, theirs.ID); code != http.StatusForbidden {
		t.Errorf("reading another user's session = %d, want 403", code)
	}
}

// TestHistoryPaginatesWithoutRepeats covers retrieval past the first page. The
// list used to be capped at a hardcoded 50 with no way past it, which for a
// daily listener is a few weeks and then silence.
func TestHistoryPaginatesWithoutRepeats(t *testing.T) {
	f := newAudioFixture(t)

	const total = 7
	created := map[string]bool{}
	for i := 0; i < total; i++ {
		_, s := f.createSession(t, f.freeTok, f.voiceStd, 60)
		if s.ID == "" {
			t.Fatalf("session %d was not created", i)
		}
		created[s.ID] = true
	}

	seen := map[string]int{}
	cursor := ""
	for page := 0; page < 10; page++ {
		path := "/sessions?limit=3"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		code, out := f.call(t, "GET", path, f.freeTok, nil)
		if code != http.StatusOK {
			t.Fatalf("page %d = %d", page, code)
		}
		list := listOf(t, out)
		if len(list) > 3 {
			t.Fatalf("page %d returned %d sessions for limit=3", page, len(list))
		}
		for _, s := range list {
			id, _ := s["id"].(string)
			seen[id]++
		}
		cursor, _ = out["next_cursor"].(string)
		if cursor == "" {
			break
		}
	}

	if len(seen) != total {
		t.Errorf("paged through %d distinct sessions, want %d", len(seen), total)
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("session %s appeared on %d pages; keyset paging must not repeat", id, n)
		}
	}
	for id := range created {
		if seen[id] == 0 {
			t.Errorf("session %s was never returned by any page", id)
		}
	}
}

func TestHistoryRejectsBadLimitAndCursor(t *testing.T) {
	f := newAudioFixture(t)
	for _, q := range []string{"limit=0", "limit=101", "limit=abc", "cursor=not-base64!!"} {
		if code, _ := f.call(t, "GET", "/sessions?"+q, f.freeTok, nil); code != http.StatusBadRequest {
			t.Errorf("GET /sessions?%s = %d, want 400", q, code)
		}
	}
	// A cursor that decodes but holds no position is still a bad cursor.
	if code, _ := f.call(t, "GET", "/sessions?cursor="+encodeSessionCursor("", ""), f.freeTok, nil); code != http.StatusBadRequest {
		t.Errorf("an empty-position cursor = %d, want 400", code)
	}
}

func TestHistoryIsEmptyForANewUser(t *testing.T) {
	f := newAudioFixture(t)
	code, out := f.call(t, "GET", "/sessions", f.freeTok, nil)
	if code != http.StatusOK {
		t.Fatalf("GET /sessions = %d", code)
	}
	if n := len(listOf(t, out)); n != 0 {
		t.Errorf("a new user has %d sessions, want 0", n)
	}
	if cur, _ := out["next_cursor"].(string); cur != "" {
		t.Errorf("an empty page offers a next cursor %q", cur)
	}
}

// TestHistoryIsScopedToTheCaller is the isolation check on the list itself.
func TestHistoryIsScopedToTheCaller(t *testing.T) {
	f := newAudioFixture(t)
	_, mine := f.createSession(t, f.freeTok, f.voiceStd, 60)
	_, theirs := f.createSession(t, f.premTok, f.voiceStd, 60)

	_, out := f.call(t, "GET", "/sessions", f.freeTok, nil)
	for _, s := range listOf(t, out) {
		id, _ := s["id"].(string)
		if id == theirs.ID {
			t.Error("another user's session appears in my history")
		}
	}
	found := false
	for _, s := range listOf(t, out) {
		if id, _ := s["id"].(string); id == mine.ID {
			found = true
		}
	}
	if !found {
		t.Error("my own session is missing from my history")
	}
}
