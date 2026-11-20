package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/storage"
	"github.com/Teamthy/i-confess/internal/store"
	"github.com/Teamthy/i-confess/internal/voicegov"
)

// Observe issuance, not just whether a URL happened to appear in the JSON.
type signCounter struct {
	storage.ObjectStorage
	issued int
}

func (s *signCounter) GenerateSignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	s.issued++
	return s.ObjectStorage.GenerateSignedURL(ctx, key, ttl)
}

func privacyScript(id, text string) map[string]any {
	return map[string]any{"voiceId": id, "purpose": "reflection", "text": text}
}
func privacySession(id, text string) map[string]any {
	return map[string]any{"voiceId": id, "title": "Evening reflection", "sections": []map[string]string{{"type": "REFLECTION", "text": text}}}
}
func generationID(out map[string]any) string {
	return out["generation"].(map[string]any)["generationId"].(string)
}
func sectionGeneration(s map[string]any) string {
	return s["items"].([]any)[0].(map[string]any)["generationId"].(string)
}

func TestPrivateVoicePlaybackAndCancellationAreOwnerOnly(t *testing.T) {
	h, _ := newIntakeHarness(t)
	h.h.SetLimiter(allowAllRateLimiter{}) // exercise several real account roles from one test IP
	h.router = h.h.Routes()               // auth middleware captures its limiter at registration
	voiceID := h.onboard(t)
	s := &signCounter{ObjectStorage: h.store}
	h.h.SetSigner(s)
	sess := h.must(t, 202, "POST", "/v1/voice-sessions", privacySession(voiceID, "Let us be still."), h.freeTok)["session"].(map[string]any)
	genID, sid := sectionGeneration(sess), sess["id"].(string)
	if err := h.queue.ProcessNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	pending, _ := h.submitRender(t, voiceID)
	sessionPath, jobPath := "/voice-sessions/"+sid, "/audio/jobs/"+genID
	for _, pfx := range []string{"", "/v1"} {
		for _, rt := range []struct{ method, path string }{{"GET", jobPath}, {"GET", sessionPath}, {"POST", "/audio/jobs/" + pending + "/cancel"}} {
			h.must(t, 401, rt.method, pfx+rt.path, nil, "")
		}
	}
	listener := h.register(t, "listener@test.com")
	actors := map[string]string{"listener": listener, "super_admin": h.admin}
	for _, role := range []string{"voice_manager", "audio_producer", "content_admin", "auditor", "ml_engineer"} {
		actors[role] = h.roleToken(t, role+"@test.com", role)
	}
	for role, tok := range actors {
		t.Run(role, func(t *testing.T) {
			for _, prefix := range []string{"", "/v1"} {
				before := s.issued
				for _, route := range []struct{ method, path, absent string }{
					{"GET", jobPath, "/audio/jobs/absent"}, {"GET", sessionPath, "/voice-sessions/absent"}, {"POST", "/audio/jobs/" + pending + "/cancel", "/audio/jobs/absent/cancel"},
				} {
					w := h.do(t, route.method, prefix+route.path, nil, tok)
					missing := h.do(t, route.method, prefix+route.absent, nil, tok)
					if w.Code != http.StatusNotFound || missing.Code != w.Code || w.Body.String() != missing.Body.String() {
						t.Fatalf("%s %s: got %d; want the same 404 as an absent record", route.method, route.path, w.Code)
					}
				}
				if s.issued != before {
					t.Fatal("signed audio for a non-owner")
				}
			}
		})
	}
	if g := h.generation(t, pending); g.Status != "QUEUED" {
		t.Fatalf("non-owner changed job to %s", g.Status)
	}
	h.must(t, 200, "POST", "/audio/jobs/"+pending+"/cancel", nil, h.freeTok)
	out := h.must(t, 200, "GET", jobPath, nil, h.freeTok)
	if out["audioUrl"] == nil || len(out["variants"].(map[string]any)) != 3 {
		t.Fatal("owner lost the master or encodings")
	}
	w := h.do(t, "GET", sessionPath, nil, h.freeTok)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"audioUrl"`) || w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("owner playlist missing audio or sensitive response is cacheable")
	}
}

func TestStaffPersonalRendersAreNotCatalogSubmissions(t *testing.T) {
	h := newVoicePlatformHarness(t)
	ctx := context.Background()
	id := h.onboard(t)
	const text = "Peace to this house."
	req := privacyScript(id, text)
	// A user-facing surface is personal even if the account also runs the studio.
	a := generationID(h.must(t, 202, "POST", "/v1/voices/generate", req, h.admin))
	b := generationID(h.must(t, 202, "POST", "/v1/voices/generate", req, h.freeTok))
	if ga, gb := h.generation(t, a), h.generation(t, b); ga.Visibility != "private" || ga.OwnerUserID == "" || gb.OwnerUserID == "" || ga.OwnerUserID == gb.OwnerUserID || a == b {
		t.Fatal("staff and listener submissions are not distinct private jobs")
	}
	sess := h.must(t, 202, "POST", "/voice-sessions", privacySession(id, text), h.admin)["session"].(map[string]any)
	if sectionGeneration(sess) != a {
		t.Fatal("private staff session did not reuse its owner's render")
	}
	h.drain(t)
	if got := generationID(h.must(t, 200, "POST", "/voices/generate", req, h.admin)); got != a {
		t.Fatal("owner cache hit lost")
	}
	h.must(t, 404, "GET", "/audio/jobs/"+a, nil, h.freeTok)
	// The staff preview has the personal safety floor too, independent of
	// whether the current grant permits user-authored speech.
	for _, route := range []string{"/voices/generate", "/voices/stream"} {
		h.must(t, 422, "POST", route, privacyScript(id, "Write to listener@example.com."), h.admin)
	}
	h.must(t, 422, "POST", "/voice-sessions", privacySession(id, "Write to listener@example.com."), h.admin)
	// With user text removed, neither membership of the editorial team nor a
	// previous cache hit is a licence for typed-in words on a personal route.
	grant, err := h.h.vplat.Grant(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	grant.Capabilities[voicegov.CanUseUserSubmittedText] = false
	if _, err := h.h.vplat.UpdateGrantTerms(ctx, id, store.GrantTerms{Capabilities: grant.Capabilities}, "legal", ""); err != nil {
		t.Fatal(err)
	}
	h.must(t, 403, "POST", "/voices/generate", req, h.admin)
	h.must(t, 403, "POST", "/voices/stream", req, h.admin)
	h.must(t, 403, "POST", "/voice-sessions", privacySession(id, text), h.admin)
	// The explicitly editorial route still works without a user-text grant;
	// it must never turn either personal result into a catalog hit.
	catalog := h.batchRender(t, id, "reflection", "reflection", text)
	if g := h.generation(t, catalog); g.Visibility != "catalog" || g.OwnerUserID != "" || catalog == a || catalog == b {
		t.Fatal("catalog job reused private input")
	}
	h.must(t, 200, "GET", "/audio/jobs/"+catalog, nil, h.freeTok)
	h.must(t, 404, "POST", "/audio/jobs/"+catalog+"/cancel", nil, h.freeTok)
	h.must(t, 200, "POST", "/audio/jobs/"+catalog+"/cancel", nil, h.admin)
}

func TestPrivateVoiceSessionChecksEachSectionOwner(t *testing.T) {
	h := newVoicePlatformHarness(t)
	ctx := context.Background()
	id := h.onboard(t)
	genID, _ := h.submitRender(t, id)
	if err := h.queue.ProcessNext(ctx); err != nil {
		t.Fatal(err)
	}
	tok, uid := h.registerWithID(t, "second@test.com")
	// Historic or inconsistent links are not proof of access to their target.
	for _, tc := range []struct{ name, sessionOwner, voice, token string }{
		{"other owner", uid, id, tok}, {"other voice", h.generation(t, genID).OwnerUserID, h.voiceStd, h.freeTok},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other := &store.AudioSession{VoiceID: tc.voice, OwnerUserID: tc.sessionOwner, Visibility: "private", Purpose: "reflection", Items: []store.SessionItem{{SectionType: "REFLECTION", GenerationID: genID}}}
			if err := h.h.vplat.CreateAudioSession(ctx, other); err != nil {
				t.Fatal(err)
			}
			s := &signCounter{ObjectStorage: h.store}
			h.h.SetSigner(s)
			sess := h.must(t, 200, "GET", "/voice-sessions/"+other.ID, nil, tc.token)["session"].(map[string]any)
			item := sess["items"].([]any)[0].(map[string]any)
			if s.issued != 0 || sess["status"] != "UNAVAILABLE" || item["status"] != "UNAVAILABLE" || item["generationId"] != nil || item["durationMs"] != nil || item["audioUrl"] != nil {
				t.Fatalf("foreign section exposed: status=%v signed=%d", item["status"], s.issued)
			}
		})
	}
}

func (h *voicePlatformHarness) batchRender(t *testing.T, voiceID, purpose, style, text string) string {
	t.Helper()
	batch := h.must(t, 202, "POST", "/admin/voices/"+voiceID+"/batches", map[string]any{"title": "Reviewed readings", "purpose": purpose, "style": style, "items": []map[string]string{{"text": text}}}, h.admin)["batch"].(map[string]any)
	return batch["items"].([]any)[0].(map[string]any)["generationId"].(string)
}

func TestPrivateVoiceCacheConflictCannotDiscloseOrResetForeignJob(t *testing.T) {
	h := newVoicePlatformHarness(t)
	ctx := context.Background()
	id := h.onboard(t)
	const text = "A quiet moment today."
	req := privacyScript(id, text)
	genID := generationID(h.must(t, 202, "POST", "/voices/generate", req, h.freeTok))
	h.drain(t)
	original := h.generation(t, genID)
	s := &signCounter{ObjectStorage: h.store}
	h.h.SetSigner(s)
	// A bad historical row under the planned hash must neither be returned
	// as a completed hit nor adopted on resubmission of a failed render.
	for _, state := range []string{"COMPLETED", "FAILED", "CANCELLED"} {
		if _, err := h.db.ExecContext(ctx, `UPDATE voice_generations SET owner_user_id=?, status=? WHERE id=?`, h.premUser, state, genID); err != nil {
			t.Fatal(err)
		}
		w := h.do(t, "POST", "/voices/generate", req, h.freeTok)
		if w.Code != 503 || s.issued != 0 || strings.Contains(w.Body.String(), genID) {
			t.Fatalf("foreign cache conflict: status=%d issued=%d", w.Code, s.issued)
		}
		if g := h.generation(t, genID); g.Status != state || g.JobID != original.JobID || g.OwnerUserID != h.premUser {
			t.Fatal("cache conflict changed foreign row")
		}
	}
}

func TestPrivateVoiceLegacyCatalogLabelIsNotEditorialProvenance(t *testing.T) {
	h := newVoicePlatformHarness(t)
	ctx := context.Background()
	id := h.onboard(t)
	genID, _ := h.submitRender(t, id)
	h.drain(t)
	if _, err := h.db.ExecContext(ctx, `UPDATE voice_generations SET visibility='catalog',owner_user_id=NULL WHERE id=?`, genID); err != nil {
		t.Fatal(err)
	}
	s := &signCounter{ObjectStorage: h.store}
	h.h.SetSigner(s)
	h.must(t, 404, "GET", "/audio/jobs/"+genID, nil, h.freeTok)
	h.must(t, 404, "GET", "/audio/jobs/"+genID, nil, h.admin)
	h.must(t, 404, "POST", "/audio/jobs/"+genID+"/cancel", nil, h.admin)
	if s.issued != 0 {
		t.Fatal("signed an ambiguous legacy row")
	}
}

func TestPrivateVoiceConfessionNeverBecomesEditorialCacheHit(t *testing.T) {
	h := newVoicePlatformHarness(t)
	id := h.onboard(t)
	const text = "Today I will be patient."
	req := map[string]any{"voiceId": id, "purpose": "confession", "style": "reflection", "text": text}
	personal := generationID(h.must(t, 202, "POST", "/voices/generate", req, h.admin))
	editorial := h.batchRender(t, id, "confession", "reflection", text)
	if editorial == personal || h.generation(t, editorial).Visibility != "private" {
		t.Fatal("batch reused or shared a personal confession")
	}
	h.must(t, 404, "GET", "/audio/jobs/"+editorial, nil, h.freeTok)
	producer := h.roleToken(t, "reviewer@test.com", "audio_producer")
	list := h.must(t, 200, "GET", "/admin/voices/"+id+"/batches", nil, producer)["batches"].([]any)
	b := list[0].(map[string]any)
	item := h.must(t, 200, "GET", "/admin/batches/"+b["id"].(string), nil, producer)["batch"].(map[string]any)["items"].([]any)[0].(map[string]any)
	if b["progress"].(map[string]any)["UNAVAILABLE"] != float64(1) || item["generationId"] != nil || item["status"] != "UNAVAILABLE" {
		t.Fatal("batch exposed another account's private job")
	}

}

func TestGovernedRenderKeysRequirePublishedCatalogEvidenceForReview(t *testing.T) {
	h, _ := newIntakeHarness(t)
	ctx := context.Background()
	id := h.onboard(t)
	pid, _ := h.submitRender(t, id)
	cid := h.batchRender(t, id, "reflection", "reflection", "A reading for the catalog.")
	h.drain(t)
	private, catalog := h.generation(t, pid), h.generation(t, cid)
	s := &signCounter{ObjectStorage: h.store}
	h.h.SetSigner(s)
	create := func(key, source string) map[string]any {
		return map[string]any{"title": "Editorial review", "clips": []map[string]string{{"source": catalog.ModelID, "audioKey": catalog.StorageKey}, {"source": source, "audioKey": key}}}
	}
	for _, key := range []string{private.StorageKey, private.Variants["aac"].Key, "audio/voices/unknown.wav"} {
		h.must(t, 400, "POST", "/admin/voices/"+id+"/blind-tests", create(key, catalog.ModelID), h.admin)
	}
	h.must(t, 400, "POST", "/admin/voices/"+id+"/blind-tests", create(catalog.StorageKey, "REAL"), h.admin)
	h.must(t, 400, "POST", "/admin/voices/"+id+"/references", map[string]any{"style": "reflection", "audioKey": private.StorageKey, "transcript": "Fixture.", "rightsOk": true}, h.admin)
	h.must(t, 400, "POST", "/admin/audio", map[string]any{"voice_id": h.voiceStd, "confession_id": h.confessionID(t), "duration_seconds": 3, "url": private.StorageKey}, h.admin)
	if s.issued != 0 {
		t.Fatal("issued review audio for an ineligible clip")
	}
	opus := catalog.Variants["opus"].Key
	bt := h.must(t, 201, "POST", "/admin/voices/"+id+"/blind-tests", create(opus, catalog.ModelID), h.admin)["test"].(map[string]any)
	if s.issued != 2 {
		t.Fatal("valid editorial master and encoding should both play")
	}
	testID := bt["id"].(string)
	// Saving a key under earlier rules doesn't authorize today's reviewer.
	if _, err := h.db.ExecContext(ctx, `UPDATE voice_blind_clips SET audio_key=? WHERE test_id=? AND audio_key=?`, private.StorageKey, testID, catalog.StorageKey); err != nil {
		t.Fatal(err)
	}
	w := h.do(t, "GET", "/admin/blind-tests/"+testID, nil, h.admin)
	if w.Code != 200 || s.issued != 3 || !strings.Contains(w.Body.String(), opus) || strings.Contains(w.Body.String(), private.ContentHash) {
		t.Fatal("blind playlist did not isolate the ineligible clip")
	}
	// A corrupt variant pointer does not grant playback of a different master’s sibling.
	catalog.Variants["mp3"] = private.Variants["mp3"]
	variants, err := json.Marshal(catalog.Variants)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.ExecContext(ctx, `UPDATE voice_generations SET variants=? WHERE id=?`, string(variants), cid); err != nil {
		t.Fatal(err)
	}
	out := h.must(t, 200, "GET", "/audio/jobs/"+cid, nil, h.freeTok)
	vs := out["variants"].(map[string]any)
	if len(vs) != 2 || vs["mp3"] != nil || out["audioUrl"] == nil || s.issued != 6 {
		t.Fatal("catalog job signed an encoding from another render")
	}
	h.must(t, 200, "POST", "/admin/voices/"+id+"/revoke", map[string]string{"reason": "fixture withdrawal"}, h.admin)
	w = h.do(t, "GET", "/admin/blind-tests/"+testID, nil, h.admin)
	if w.Code != 200 || strings.Contains(w.Body.String(), `"audioUrl"`) || s.issued != 6 {
		t.Fatal("blind test reissued audio after withdrawal")
	}
}
