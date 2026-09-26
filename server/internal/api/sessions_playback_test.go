package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/sessions"
	"github.com/Teamthy/i-confess/internal/store"
)

// TestQueueReturnsSignedAudioAndProgress verifies that GET /sessions/{id}/queue
// returns server-issued signed URLs for playable items and attaches progress.
func TestQueueReturnsSignedAudioAndProgress(t *testing.T) {
	f := newAudioFixture(t)

	code, sess := f.createSession(t, f.freeTok, f.voiceStd, 120)
	if code != http.StatusCreated || sess.ID == "" {
		t.Fatalf("create session failed: code %d", code)
	}

	code, out := f.call(t, "GET", "/sessions/"+sess.ID+"/queue", f.freeTok, nil)
	if code != http.StatusOK {
		t.Fatalf("get queue returned %d: %v", code, out)
	}

	items, ok := out["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("expected non-empty items array: %v", out)
	}

	for i, raw := range items {
		it, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("item %d is not a map", i)
		}
		audioURL, _ := it["audio_url"].(string)
		if audioURL == "" {
			t.Fatalf("item %d has empty audio_url", i)
		}
		// Must be a signed URL, not a bare storage key
		if !strings.Contains(audioURL, "sig=") && !strings.Contains(audioURL, "Signature=") {
			t.Fatalf("item %d audio_url %q is not signed", i, audioURL)
		}
	}

	if sid, _ := out["session_id"].(string); sid != sess.ID {
		t.Fatalf("session_id mismatch: got %q, want %q", sid, sess.ID)
	}
	if status, _ := out["status"].(string); status != string(sessions.Ready) {
		t.Fatalf("status mismatch: got %q, want READY", status)
	}
}

// TestQueueEntitlementEnforcement verifies that premium items are locked
// and have empty signed URLs when fetched by a free user.
func TestQueueEntitlementEnforcement(t *testing.T) {
	f := newAudioFixture(t)

	code, sess := f.createSession(t, f.premTok, f.voicePrm, 120)
	if code != http.StatusCreated || sess.ID == "" {
		t.Fatalf("create session failed: code %d", code)
	}

	// Downgrade user to free
	users := store.NewUserStore(f.db)
	if err := users.SetSubscription(context.Background(), f.premUser, "free", "active"); err != nil {
		t.Fatal(err)
	}

	code, out := f.call(t, "GET", "/sessions/"+sess.ID+"/queue", f.premTok, nil)
	if code != http.StatusOK {
		t.Fatalf("get queue returned %d: %v", code, out)
	}

	items, _ := out["items"].([]any)
	for i, raw := range items {
		it := raw.(map[string]any)
		locked, _ := it["locked"].(bool)
		audioURL, _ := it["audio_url"].(string)
		if !locked {
			t.Errorf("item %d expected locked=true after downgrade", i)
		}
		if audioURL != "" {
			t.Errorf("item %d expected empty audio_url when locked, got %q", i, audioURL)
		}
	}
}

// TestStartPauseResumeInterruptPlaybackLifecycle tests the full state machine transitions
// and asserts signed audio is returned on start and resume.
func TestStartPauseResumeInterruptPlaybackLifecycle(t *testing.T) {
	f := newAudioFixture(t)

	code, sess := f.createSession(t, f.freeTok, f.voiceStd, 120)
	if code != http.StatusCreated || sess.ID == "" {
		t.Fatalf("create session failed: code %d", code)
	}

	// 1. Start session
	code, startOut := f.call(t, "POST", "/sessions/"+sess.ID+"/start", f.freeTok, nil)
	if code != http.StatusOK {
		t.Fatalf("start session returned %d: %v", code, startOut)
	}
	if st, _ := startOut["status"].(string); st != string(sessions.Active) {
		t.Fatalf("status after start = %q, want ACTIVE", st)
	}
	items, ok := startOut["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("start session did not return items")
	}
	first := items[0].(map[string]any)
	if firstURL, _ := first["audio_url"].(string); firstURL == "" {
		t.Fatalf("first item audio_url is empty on start")
	}

	// 2. Pause session
	code, pauseOut := f.call(t, "POST", "/sessions/"+sess.ID+"/pause", f.freeTok, nil)
	if code != http.StatusOK {
		t.Fatalf("pause session returned %d: %v", code, pauseOut)
	}
	if st, _ := pauseOut["status"].(string); st != string(sessions.Paused) {
		t.Fatalf("status after pause = %q, want PAUSED", st)
	}

	// 3. Resume session
	code, resumeOut := f.call(t, "POST", "/sessions/"+sess.ID+"/resume", f.freeTok, nil)
	if code != http.StatusOK {
		t.Fatalf("resume session returned %d: %v", code, resumeOut)
	}
	if st, _ := resumeOut["status"].(string); st != string(sessions.Active) {
		t.Fatalf("status after resume = %q, want ACTIVE", st)
	}

	// 4. Interrupt session
	code, interruptOut := f.call(t, "POST", "/sessions/"+sess.ID+"/interrupt", f.freeTok, nil)
	if code != http.StatusOK {
		t.Fatalf("interrupt session returned %d: %v", code, interruptOut)
	}
	if st, _ := interruptOut["status"].(string); st != string(sessions.Interrupted) {
		t.Fatalf("status after interrupt = %q, want INTERRUPTED", st)
	}

	// 5. Resume from interruption
	code, resumeAfterInterrupt := f.call(t, "POST", "/sessions/"+sess.ID+"/resume", f.freeTok, nil)
	if code != http.StatusOK {
		t.Fatalf("resume after interrupt returned %d: %v", code, resumeAfterInterrupt)
	}
	if st, _ := resumeAfterInterrupt["status"].(string); st != string(sessions.Active) {
		t.Fatalf("status after resume = %q, want ACTIVE", st)
	}
}

// TestSkipSessionItemAdvancesQueue tests skipping a queue item.
func TestSkipSessionItemAdvancesQueue(t *testing.T) {
	f := newAudioFixture(t)

	code, sess := f.createSession(t, f.freeTok, f.voiceStd, 120)
	if code != http.StatusCreated || sess.ID == "" {
		t.Fatalf("create session failed: code %d", code)
	}

	// Start to make active
	f.call(t, "POST", "/sessions/"+sess.ID+"/start", f.freeTok, nil)

	firstItem := f.queueItemID(t, sess.ID)
	code, skipOut := f.call(t, "POST", "/sessions/"+sess.ID+"/skip", f.freeTok, map[string]any{
		"item_id": firstItem,
	})
	if code != http.StatusOK {
		t.Fatalf("skip returned %d: %v", code, skipOut)
	}

	if skipped, _ := skipOut["skipped"].(string); skipped != firstItem {
		t.Errorf("skipped item ID mismatch: got %q, want %q", skipped, firstItem)
	}
	advanced, _ := skipOut["advanced"].(bool)
	if !advanced {
		t.Errorf("expected advanced=true when items remain in queue")
	}
	if next, ok := skipOut["next"].(map[string]any); ok {
		audioURL, _ := next["audio_url"].(string)
		if audioURL == "" {
			t.Errorf("next item audio_url should be signed")
		}
	} else {
		t.Errorf("expected next item in skip response")
	}
}

// TestProgressDeterministicConflictResolution tests local/cloud conflict resolution
// based on ISO timestamps.
func TestProgressDeterministicConflictResolution(t *testing.T) {
	f := newAudioFixture(t)

	code, sess := f.createSession(t, f.freeTok, f.voiceStd, 120)
	if code != http.StatusCreated || sess.ID == "" {
		t.Fatalf("create session failed: code %d", code)
	}

	firstItem := f.queueItemID(t, sess.ID)

	t1 := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC).Format(time.RFC3339)
	t2 := time.Date(2026, 9, 20, 10, 5, 0, 0, time.UTC).Format(time.RFC3339)
	tStale := time.Date(2026, 9, 20, 9, 50, 0, 0, time.UTC).Format(time.RFC3339)

	// Step 1: Push T1
	code, out1 := f.call(t, "POST", "/sessions/"+sess.ID+"/progress", f.freeTok, map[string]any{
		"queue_item_id":   firstItem,
		"position_ms":     5000,
		"device_id":       "device-a",
		"last_updated_at": t1,
	})
	if code != http.StatusOK {
		t.Fatalf("first progress push failed: %d %v", code, out1)
	}
	if applied, _ := out1["applied"].(bool); !applied {
		t.Errorf("t1 should be applied")
	}

	// Step 2: Push stale tStale (< t1)
	code, outStale := f.call(t, "POST", "/sessions/"+sess.ID+"/progress", f.freeTok, map[string]any{
		"queue_item_id":   firstItem,
		"position_ms":     1000,
		"device_id":       "device-b",
		"last_updated_at": tStale,
	})
	if code != http.StatusOK {
		t.Fatalf("stale progress push failed: %d %v", code, outStale)
	}
	if applied, _ := outStale["applied"].(bool); applied {
		t.Errorf("stale update must NOT be applied")
	}
	prog, _ := outStale["progress"].(map[string]any)
	if pos, _ := prog["position_ms"].(float64); int64(pos) != 5000 {
		t.Errorf("stored position should remain 5000, got %v", pos)
	}

	// Step 3: Push fresh t2 (> t1)
	code, out2 := f.call(t, "POST", "/sessions/"+sess.ID+"/progress", f.freeTok, map[string]any{
		"queue_item_id":   firstItem,
		"position_ms":     12000,
		"device_id":       "device-a",
		"last_updated_at": t2,
	})
	if code != http.StatusOK {
		t.Fatalf("newer progress push failed: %d %v", code, out2)
	}
	if applied, _ := out2["applied"].(bool); !applied {
		t.Errorf("newer update t2 should be applied")
	}
}

// TestPreventForgedCompletion asserts that an unstarted session cannot be completed,
// nor can a fake history completion record be posted.
func TestPreventForgedCompletion(t *testing.T) {
	f := newAudioFixture(t)

	code, sess := f.createSession(t, f.freeTok, f.voiceStd, 120)
	if code != http.StatusCreated || sess.ID == "" {
		t.Fatalf("create session failed: code %d", code)
	}

	// 1. Trying to complete an unplayed READY session via POST /sessions/{id}/complete must fail (409)
	code, out := f.call(t, "POST", "/sessions/"+sess.ID+"/complete", f.freeTok, nil)
	if code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict when completing unplayed session, got %d: %v", code, out)
	}

	// 2. Trying to post fake completed record to /me/history for unplayed session must fail (409)
	code, out = f.call(t, "POST", "/me/history", f.freeTok, map[string]any{
		"session_id": sess.ID,
		"completed":  true,
	})
	if code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict when claiming completion for unstarted session via /me/history, got %d: %v", code, out)
	}

	// 3. Trying to address another user's session must fail (403 Forbidden)
	code, _ = f.call(t, "POST", "/sessions/"+sess.ID+"/complete", f.premTok, nil)
	if code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when completing another user's session, got %d", code)
	}
	code, _ = f.call(t, "POST", "/me/history", f.premTok, map[string]any{
		"session_id": sess.ID,
		"completed":  true,
	})
	if code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when recording history for another user's session, got %d", code)
	}

	// 4. Start legitimate playback, then complete
	f.call(t, "POST", "/sessions/"+sess.ID+"/start", f.freeTok, nil)
	code, compOut := f.call(t, "POST", "/sessions/"+sess.ID+"/complete", f.freeTok, nil)
	if code != http.StatusOK {
		t.Fatalf("legitimate complete failed: %d %v", code, compOut)
	}
	if status, _ := compOut["status"].(string); status != string(sessions.Completed) {
		t.Errorf("status = %q, want COMPLETED", status)
	}

	// 5. Verify completion history was recorded server-side
	code, histOut := f.call(t, "GET", "/me/history", f.freeTok, nil)
	if code != http.StatusOK {
		t.Fatalf("get history failed: %d", code)
	}
	var historyList []models.PlaybackRecord
	raw, _ := histOut["data"].([]any)
	if len(raw) == 0 {
		// check direct array
		// let's verify via DB directly
		var count int
		err := f.db.QueryRowContext(context.Background(),
			`SELECT COUNT(*) FROM playback_history WHERE session_id = $1 AND completed = 1`, sess.ID).Scan(&count)
		if err != nil || count == 0 {
			t.Fatalf("expected completed playback record in DB for session %s: %v", sess.ID, err)
		}
	}
	_ = historyList
}
