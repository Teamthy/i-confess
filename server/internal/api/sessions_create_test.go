package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// createSessionRaw posts an arbitrary session-creation body and returns the
// status code plus the decoded session. Used to exercise request validation,
// where the point is what the server refuses.
func (f *audioFixture) createSessionRaw(t *testing.T, token string, body map[string]any) (int, sessionResp) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/sessions", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	var out sessionResp
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestCreateSessionAcceptsDurationPresets(t *testing.T) {
	f := newAudioFixture(t)

	// The 30-minute preset exceeds the free plan's 15-minute ceiling, so this
	// needs a premium listener.
	code, sess := f.createSessionRaw(t, f.premTok, map[string]any{
		"category_ids": []string{f.catID}, "duration_preset": "30m",
	})
	if code != http.StatusCreated {
		t.Fatalf("create with preset = %d, want 201", code)
	}
	if sess.TargetDuration != 1800 {
		t.Errorf("target_duration = %d, want 1800 for the 30-minute preset", sess.TargetDuration)
	}

	// Clients send several spellings; all must resolve to the same preset.
	for _, spelling := range []string{"30", "30min", "30_min", "30 minutes"} {
		code, sess := f.createSessionRaw(t, f.premTok, map[string]any{
			"category_ids": []string{f.catID}, "duration_preset": spelling,
		})
		if code != http.StatusCreated || sess.TargetDuration != 1800 {
			t.Errorf("preset %q: code=%d target=%d, want 201/1800", spelling, code, sess.TargetDuration)
		}
	}

	// A free listener asking for the same preset is refused on plan grounds,
	// not given a silently shortened session. Length is a plan capability and
	// the server is the only authority on it.
	if code, _ := f.createSessionRaw(t, f.freeTok, map[string]any{
		"category_ids": []string{f.catID}, "duration_preset": "30m",
	}); code != http.StatusPaymentRequired {
		t.Errorf("free plan with 30m preset = %d, want 402", code)
	}
	// 15 minutes is exactly the free ceiling and must still be allowed.
	if code, _ := f.createSessionRaw(t, f.freeTok, map[string]any{
		"category_ids": []string{f.catID}, "duration_preset": "15m",
	}); code != http.StatusCreated {
		t.Errorf("free plan with 15m preset = %d, want 201", code)
	}
}

func TestCreateSessionRejectsBadPresets(t *testing.T) {
	f := newAudioFixture(t)

	// Off the ladder.
	if code, _ := f.createSessionRaw(t, f.freeTok, map[string]any{
		"category_ids": []string{f.catID}, "duration_preset": "7m",
	}); code != http.StatusBadRequest {
		t.Errorf("unknown preset = %d, want 400", code)
	}
	// "custom" is a real preset but carries no length of its own.
	if code, _ := f.createSessionRaw(t, f.freeTok, map[string]any{
		"category_ids": []string{f.catID}, "duration_preset": "custom",
	}); code != http.StatusBadRequest {
		t.Errorf("custom preset without duration_seconds = %d, want 400", code)
	}
	// An explicit length still wins, and custom with a length is fine.
	if code, _ := f.createSessionRaw(t, f.freeTok, map[string]any{
		"category_ids": []string{f.catID}, "duration_preset": "custom", "duration_seconds": 120,
	}); code != http.StatusCreated {
		t.Errorf("custom preset with duration_seconds = %d, want 201", code)
	}
}

func TestCreateSessionRejectsInvalidStrategy(t *testing.T) {
	f := newAudioFixture(t)
	for _, bad := range []string{"FAST", "balanced-ish", "1", "NULL"} {
		if code, _ := f.createSessionRaw(t, f.freeTok, map[string]any{
			"category_ids": []string{f.catID}, "duration_seconds": 120, "strategy": bad,
		}); code != http.StatusBadRequest {
			t.Errorf("strategy %q = %d, want 400", bad, code)
		}
	}
}

// The fixture's only variant is 60s, so a 100s request cannot be met exactly:
// the deepest under-fill is one item and the only overshoot is two. UNDER and
// OVER must therefore disagree, which proves the strategy reached the engine
// rather than being accepted and ignored.
func TestCreateSessionStrategyReachesTheEngine(t *testing.T) {
	f := newAudioFixture(t)

	code, under := f.createSessionRaw(t, f.freeTok, map[string]any{
		"category_ids": []string{f.catID}, "duration_seconds": 100, "strategy": "UNDER",
	})
	if code != http.StatusCreated {
		t.Fatalf("UNDER = %d, want 201", code)
	}
	if under.ActualDuration != 60 {
		t.Errorf("UNDER actual = %d, want 60", under.ActualDuration)
	}
	if under.Strategy != "UNDER" {
		t.Errorf("strategy echoed as %q, want UNDER", under.Strategy)
	}

	code, over := f.createSessionRaw(t, f.freeTok, map[string]any{
		"category_ids": []string{f.catID}, "duration_seconds": 100, "strategy": "OVER",
	})
	if code != http.StatusCreated {
		t.Fatalf("OVER = %d, want 201", code)
	}
	if over.ActualDuration != 120 {
		t.Errorf("OVER actual = %d, want 120", over.ActualDuration)
	}
	if under.ActualDuration == over.ActualDuration {
		t.Error("UNDER and OVER produced the same session; the strategy is being ignored")
	}
	// Both must agree on what was asked for.
	if under.TargetDuration != 100 || over.TargetDuration != 100 {
		t.Errorf("target = %d/%d, want 100 for both", under.TargetDuration, over.TargetDuration)
	}
}

func TestCreateSessionDefaultsStrategyToBalanced(t *testing.T) {
	f := newAudioFixture(t)
	code, sess := f.createSessionRaw(t, f.freeTok, map[string]any{
		"category_ids": []string{f.catID}, "duration_seconds": 120,
	})
	if code != http.StatusCreated {
		t.Fatalf("create = %d, want 201", code)
	}
	if sess.Strategy != "BALANCED" {
		t.Errorf("strategy = %q, want BALANCED", sess.Strategy)
	}
}

// The persisted session must carry the strategy and both durations, otherwise
// reloading a session loses the information the player needs to label progress.
func TestCreatedSessionPersistsTargetAndStrategy(t *testing.T) {
	f := newAudioFixture(t)
	code, created := f.createSessionRaw(t, f.freeTok, map[string]any{
		"category_ids": []string{f.catID}, "duration_seconds": 100, "strategy": "UNDER",
	})
	if code != http.StatusCreated {
		t.Fatalf("create = %d", code)
	}
	code, reloaded := f.getSession(t, f.freeTok, created.ID)
	if code != http.StatusOK {
		t.Fatalf("get = %d", code)
	}
	if reloaded.Strategy != "UNDER" {
		t.Errorf("reloaded strategy = %q, want UNDER", reloaded.Strategy)
	}
	if reloaded.TargetDuration != 100 || reloaded.ActualDuration != 60 {
		t.Errorf("reloaded durations = %d/%d, want 100/60", reloaded.TargetDuration, reloaded.ActualDuration)
	}
}
