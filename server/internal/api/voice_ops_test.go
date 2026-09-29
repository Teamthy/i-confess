package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// roleToken registers a user, gives them an admin role and returns a fresh
// token that carries it.
func (h *voicePlatformHarness) roleToken(t *testing.T, email, role string) string {
	t.Helper()
	_, id := h.registerWithID(t, email)
	if err := h.h.users.SetAdminRole(context.Background(), id, role); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"email": email, "password": "test-passphrase-2026"})
	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, httptest.NewRequest("POST", "/auth/login", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s: %d %s", email, rec.Code, rec.Body.String())
	}
	var out struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out.Token
}

func TestAudioSessionIsRightsCheckedPerSectionAndPrivate(t *testing.T) {
	h := newVoicePlatformHarness(t)
	id := h.onboard(t)

	// The grant has no can_use_in_bible_audio, so a scripture section is
	// refused even though reflections are allowed.
	h.must(t, 403, "POST", "/v1/voice-sessions", map[string]any{"voiceId": id, "purpose": "reflection",
		"sections": []map[string]any{{"type": "SCRIPTURE", "text": "The Lord is my shepherd."}}}, h.freeTok)

	out := h.must(t, 202, "POST", "/v1/voice-sessions", map[string]any{"voiceId": id, "purpose": "reflection", "title": "Evening",
		"sections": []map[string]any{
			{"type": "INTRO", "text": "Welcome. Let us be still."},
			{"type": "PAUSE", "pauseMs": 3000},
			{"type": "REFLECTION", "text": "Grace meets us where we are."},
			{"type": "CONFESSION", "text": "I confess my impatience today.", "style": "reflection"},
		}}, h.freeTok)
	sess := out["session"].(map[string]any)
	sid := sess["id"].(string)
	if sess["status"] != "PROCESSING" || sess["total"].(float64) != 3 {
		t.Fatalf("new session: %v", sess)
	}
	h.drain(t)
	got := h.must(t, 200, "GET", "/v1/voice-sessions/"+sid, nil, h.freeTok)["session"].(map[string]any)
	if got["status"] != "READY" {
		t.Fatalf("session not ready: %v", got)
	}
	items := got["items"].([]any)
	if items[1].(map[string]any)["type"] != "PAUSE" || items[1].(map[string]any)["audioUrl"] != nil {
		t.Fatalf("pause item: %v", items[1])
	}
	for _, i := range []int{0, 2, 3} {
		if u, _ := items[i].(map[string]any)["audioUrl"].(string); u == "" {
			t.Fatalf("item %d has no signed URL: %v", i, items[i])
		}
	}
	// Private: another user cannot see it, and cannot tell it exists.
	other := h.register(t, "someone@test.com")
	h.must(t, 404, "GET", "/v1/voice-sessions/"+sid, nil, other)

	// Revocation withholds already-generated audio (post-termination policy
	// defaults to unpublish), on sessions and on single jobs alike.
	genID := items[0].(map[string]any)["generationId"].(string)
	h.must(t, 200, "POST", "/v1/admin/voices/"+id+"/revoke", map[string]any{"reason": "test"}, h.admin)
	got = h.must(t, 200, "GET", "/v1/voice-sessions/"+sid, nil, h.freeTok)["session"].(map[string]any)
	if got["status"] != "UNAVAILABLE" {
		t.Fatalf("session after revoke: %v", got["status"])
	}
	for _, it := range got["items"].([]any) {
		if it.(map[string]any)["audioUrl"] != nil {
			t.Fatalf("signed URL served after revocation: %v", it)
		}
	}
	job := h.must(t, 200, "GET", "/v1/audio/jobs/"+genID, nil, h.freeTok)
	if job["audioUrl"] != nil || job["available"] != false {
		t.Fatalf("job after revoke must be unavailable without URL: %v", job)
	}
}

func TestBatchGenerationQueuesLowPriorityAndRecordsRefusals(t *testing.T) {
	h := newVoicePlatformHarness(t)
	id := h.onboard(t)
	out := h.must(t, 202, "POST", "/v1/admin/voices/"+id+"/batches", map[string]any{
		"title": "Psalms 1-3", "purpose": "reflection", "style": "reflection",
		"items": []map[string]any{
			{"label": "Psalm 1", "text": "Blessed is the one who walks not in the counsel of the wicked."},
			{"label": "Psalm 2", "text": ""},
			{"label": "Psalm 3", "text": "But you, O Lord, are a shield about me."},
		}}, h.admin)
	b := out["batch"].(map[string]any)
	if b["total"].(float64) != 3 || b["refused"].(float64) != 1 {
		t.Fatalf("batch counts: %v", b)
	}
	items := b["items"].([]any)
	gen := items[0].(map[string]any)["generationId"].(string)
	job := h.must(t, 200, "GET", "/v1/audio/jobs/"+gen, nil, h.admin)
	if q := job["generation"].(map[string]any)["queue"]; q != "tts.low" {
		t.Fatalf("batch renders must use the low-priority queue, got %v", q)
	}
	h.drain(t)
	got := h.must(t, 200, "GET", "/v1/admin/batches/"+b["id"].(string), nil, h.admin)["batch"].(map[string]any)
	prog := got["progress"].(map[string]any)
	if prog["COMPLETED"].(float64) != 2 || prog["REFUSED"].(float64) != 1 {
		t.Fatalf("progress: %v", prog)
	}
	// A voice without generation rights refuses the whole batch up front.
	v := h.must(t, 201, "POST", "/v1/admin/minister-voices", map[string]any{"name": "Pending", "rightsHolder": "X"}, h.admin)
	pending := v["voice"].(map[string]any)["id"].(string)
	h.must(t, 403, "POST", "/v1/admin/voices/"+pending+"/batches", map[string]any{"title": "x", "purpose": "reflection",
		"items": []map[string]any{{"text": "Peace."}}}, h.admin)
}

func TestBlindEvaluationGatesOnRealRatings(t *testing.T) {
	h := newVoicePlatformHarness(t)
	id := h.onboard(t)
	m := h.must(t, 201, "POST", "/v1/admin/voices/"+id+"/models", map[string]any{
		"engine": "cosyvoice", "engineVersion": "3", "modelVersion": "zs-2", "mode": "zero_shot", "licenseReviewed": true}, h.admin)
	modelID := m["model"].(map[string]any)["id"].(string)

	bt := h.must(t, 201, "POST", "/v1/admin/voices/"+id+"/blind-tests", map[string]any{"title": "zs-2 vs real",
		"clips": []map[string]any{{"source": "REAL", "audioKey": "voice-private/eval/real.wav"},
			{"source": modelID, "audioKey": "voice-private/eval/zs2.wav"}}}, h.admin)["test"].(map[string]any)
	testID := bt["id"].(string)
	for _, c := range bt["clips"].([]any) {
		if _, leaked := c.(map[string]any)["source"]; leaked {
			t.Fatalf("clip source visible while the test is open: %v", c)
		}
	}
	h.must(t, 409, "GET", "/v1/admin/blind-tests/"+testID+"/results", nil, h.admin)

	metrics := map[string]float64{}
	for _, d := range []string{"speaker_similarity", "naturalness", "pronunciation", "accent_preservation", "prosody",
		"emotional_consistency", "style_consistency", "long_form_stability", "intelligibility", "text_accuracy", "artifact_score"} {
		metrics[d] = 0.85
	}
	rate := func(tok string) {
		var rs []map[string]any
		for _, c := range bt["clips"].([]any) {
			rs = append(rs, map[string]any{"clipId": c.(map[string]any)["id"], "dimension": "naturalness", "value": 4})
		}
		h.must(t, 200, "POST", "/v1/admin/blind-tests/"+testID+"/ratings", map[string]any{"ratings": rs}, tok)
	}
	rate(h.roleToken(t, "e1@test.com", "audio_producer"))
	rate(h.roleToken(t, "e2@test.com", "auditor"))
	h.must(t, 200, "POST", "/v1/admin/blind-tests/"+testID+"/close", nil, h.admin)

	// Two evaluators is below the default minimum of three.
	ev := h.must(t, 200, "POST", "/v1/admin/voices/"+id+"/evaluate", map[string]any{"modelId": modelID, "goldenSet": "g1",
		"metrics": metrics, "humanApproved": true, "blindTestId": testID}, h.admin)
	if ev["gate"].(map[string]any)["verdict"] == "PASS" || ev["blindEvaluation"].(map[string]any)["completed"] != false {
		t.Fatalf("insufficient blind test must not pass the gate: %v", ev)
	}
	t.Setenv("VOICE_BLIND_MIN_EVALUATORS", "2")
	ev = h.must(t, 200, "POST", "/v1/admin/voices/"+id+"/evaluate", map[string]any{"modelId": modelID, "goldenSet": "g1",
		"metrics": metrics, "humanApproved": true, "blindTestId": testID}, h.admin)
	if ev["gate"].(map[string]any)["verdict"] != "PASS" {
		t.Fatalf("closed blind test with enough evaluators should pass: %v", ev)
	}
	res := h.must(t, 200, "GET", "/v1/admin/blind-tests/"+testID+"/results", nil, h.admin)
	if _, ok := res["bySource"].(map[string]any)[modelID]; !ok {
		t.Fatalf("results must be unblinded per source: %v", res)
	}
	// Ratings after close are refused.
	clip := bt["clips"].([]any)[0].(map[string]any)["id"]
	h.must(t, 409, "POST", "/v1/admin/blind-tests/"+testID+"/ratings", map[string]any{"ratings": []map[string]any{
		{"clipId": clip, "dimension": "naturalness", "value": 5}}}, h.admin)
}

func TestVoiceRolesAreEnforced(t *testing.T) {
	h := newVoicePlatformHarness(t)
	id := h.onboard(t)
	auditor := h.roleToken(t, "aud@test.com", "auditor")
	ml := h.roleToken(t, "ml@test.com", "ml_engineer")

	h.must(t, 200, "GET", "/v1/admin/voices/"+id+"/audit", nil, auditor)
	h.must(t, 200, "GET", "/v1/admin/voices/"+id+"/models", nil, auditor)
	h.must(t, 200, "GET", "/v1/admin/voice-metrics", nil, auditor)
	h.must(t, 403, "PUT", "/v1/admin/voices/"+id+"/grant", map[string]any{"rightsHolder": "x", "attestation": true}, auditor)
	h.must(t, 403, "POST", "/v1/admin/voices/"+id+"/training-runs", map[string]any{}, auditor)

	h.must(t, 200, "GET", "/v1/admin/voices/"+id+"/datasets", nil, ml)
	h.must(t, 403, "POST", "/v1/admin/voices/"+id+"/approve", nil, ml)
	h.must(t, 403, "GET", "/v1/admin/voices/"+id+"/audit", nil, ml)

	rec := h.do(t, "GET", "/v1/admin/voice-metrics?format=prometheus", nil, auditor)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "icf_voice_cache_hits_total") {
		t.Fatalf("prometheus metrics: %d %s", rec.Code, rec.Body.String())
	}
}

func TestRightsSweepExpiresGrantsAndPurgesUnderDeletePolicy(t *testing.T) {
	h := newVoicePlatformHarness(t)
	id := h.onboard(t)
	out := h.must(t, 202, "POST", "/v1/voices/generate", map[string]any{"voiceId": id, "text": "Grace and peace.",
		"style": "reflection", "purpose": "reflection"}, h.admin)
	genID := out["generation"].(map[string]any)["generationId"].(string)
	h.drain(t)
	if job := h.must(t, 200, "GET", "/v1/audio/jobs/"+genID, nil, h.admin); job["audioUrl"] == nil {
		t.Fatalf("expected a URL before expiry: %v", job)
	}

	past := time.Now().UTC().Add(-time.Hour).Format("2006-01-02T15:04:05.000000Z")
	if _, err := h.db.ExecContext(context.Background(), `UPDATE voice_rights_grants SET expires_at = ?, post_termination = 'delete' WHERE voice_id = ?`, past, id); err != nil {
		t.Fatal(err)
	}
	expired, purged, err := h.h.SweepVoiceRights(context.Background())
	if err != nil || expired != 1 || purged != 1 {
		t.Fatalf("sweep: expired=%d purged=%d err=%v", expired, purged, err)
	}
	g := h.must(t, 200, "GET", "/v1/admin/voices/"+id+"/grant", nil, h.admin)
	if st := g["grant"].(map[string]any)["status"]; st != "EXPIRED" {
		t.Fatalf("grant status after sweep: %v", st)
	}
	// The purged row is soft-deleted: gone for everyone.
	h.must(t, 404, "GET", "/v1/audio/jobs/"+genID, nil, h.admin)
	// A second sweep is a no-op.
	if e, p, _ := h.h.SweepVoiceRights(context.Background()); e != 0 || p != 0 {
		t.Fatalf("second sweep not idempotent: %d %d", e, p)
	}
}

func TestRendersGetSignedDeliveryVariantsWithheldAfterRevocation(t *testing.T) {
	h, _ := newIntakeHarness(t)
	id := h.onboard(t)
	out := h.must(t, 202, "POST", "/v1/voices/generate", map[string]any{"voiceId": id, "text": "Grace and peace.",
		"style": "reflection", "purpose": "reflection"}, h.admin)
	genID := out["generation"].(map[string]any)["generationId"].(string)
	h.drain(t)
	job := h.must(t, 200, "GET", "/v1/audio/jobs/"+genID, nil, h.admin)
	vs, ok := job["variants"].(map[string]any)
	if !ok || len(vs) != 3 {
		t.Fatalf("expected aac/opus/mp3 variants, got %v", job["variants"])
	}
	for _, f := range []string{"aac", "opus", "mp3"} {
		v := vs[f].(map[string]any)
		if v["url"] == "" || v["contentType"] != "audio/"+f {
			t.Fatalf("variant %s: %v", f, v)
		}
	}
	h.must(t, 200, "POST", "/v1/admin/voices/"+id+"/revoke", map[string]any{"reason": "licence withdrawn"}, h.admin)
	job = h.must(t, 200, "GET", "/v1/audio/jobs/"+genID, nil, h.admin)
	if job["variants"] != nil || job["audioUrl"] != nil || job["available"] != false {
		t.Fatalf("revoked render still delivers: %v", job)
	}
}
