package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/backoff"
	"github.com/Teamthy/i-confess/internal/jobs"
	"github.com/Teamthy/i-confess/internal/storage"
	"github.com/Teamthy/i-confess/internal/store"
	"github.com/Teamthy/i-confess/internal/voicegov"
)

// The bytes have really reached the fixture's private object store before
// this hook runs. No sleep or guessed worker latency is needed.
type handoffStorage struct {
	storage.ObjectStorage
	keys     []string
	afterPut func(string)
}

func (s *handoffStorage) Upload(ctx context.Context, key string, data []byte, meta map[string]string) error {
	if err := s.ObjectStorage.Upload(ctx, key, data, meta); err != nil {
		return err
	}
	s.keys = append(s.keys, key)
	if s.afterPut != nil {
		s.afterPut(key)
	}
	return nil
}

func retainForHandoff(t *testing.T, h *voicePlatformHarness, id string) store.GrantTerms {
	t.Helper()
	ctx := context.Background()
	grant, err := h.h.vplat.Grant(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	terms := store.GrantTerms{Capabilities: grant.Capabilities, PostTermination: voicegov.AssetRetain}
	if _, err := h.h.vplat.UpdateGrantTerms(ctx, id, terms, "fixture", ""); err != nil {
		t.Fatal(err)
	}
	return terms
}

func TestVoiceGrantChangeDuringRenderCannotPublish(t *testing.T) {
	for _, at := range []string{"during inference", "during upload", "expiry without sweep", "terms edited while approved", "missing permission row"} {
		t.Run(at, func(t *testing.T) {
			h := newVoicePlatformHarness(t)
			ctx := context.Background()
			id := h.onboard(t, "can_retain_after_termination")
			// Retaining an existing master is not a licence to finish a new one.
			terms := retainForHandoff(t, h, id)
			genID, _ := h.submitRender(t, id)
			s := &handoffStorage{ObjectStorage: h.store}
			h.h.SetSigner(s)
			change := func() {
				var err error
				switch at {
				case "expiry without sweep": // same version and status; exercise the wall clock check itself
					_, err = h.db.ExecContext(ctx, `UPDATE voice_rights_grants SET expires_at=? WHERE voice_id=?`, time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano), id)
				case "missing permission row": // a version alone is not proof of permission
					_, err = h.db.ExecContext(ctx, `DELETE FROM voice_usage_permissions WHERE voice_id=? AND capability=?`, id, voicegov.CanUseInReflections)
				case "terms edited while approved":
					terms.Capabilities[voicegov.CanUseUserSubmittedText] = false
					_, err = h.h.vplat.UpdateGrantTerms(ctx, id, terms, "fixture", "")
				default:
					err = h.h.vplat.TransitionGrant(ctx, id, voicegov.StatusRevoked, "fixture", "withdrawn", "")
				}
				if err != nil {
					t.Errorf("change rights: %v", err)
				}
			}
			wantUploads := 0
			if at == "during upload" {
				wantUploads = 1
				s.afterPut = func(string) { change() }
			} else {
				h.worker.beforeAudio = change
			}
			err := h.queue.ProcessNext(ctx)
			g := h.generation(t, genID)
			if !jobs.IsPermanent(err) || g.ErrorClass != "rights" || g.Status != "FAILED" || g.StorageKey != "" || len(s.keys) != wantUploads {
				t.Fatalf("stale grant: err=%v status=%s class=%s uploads=%d", err, g.Status, g.ErrorClass, len(s.keys))
			}
			for _, key := range s.keys {
				if exists, err := s.Exists(ctx, key); err != nil || exists {
					t.Fatalf("discarded private upload remains: exists=%v err=%v", exists, err)
				}
			}
			if out := h.must(t, 200, "GET", "/v1/audio/jobs/"+genID, nil, h.freeTok); out["audioUrl"] != nil || out["variants"] != nil {
				t.Fatal("signed a withdrawn render")
			}
			audit, err := h.h.vplat.RightsAudit(ctx, id, 50)
			if err != nil {
				t.Fatal(err)
			}
			wantReason := voicegov.ReasonRevoked
			switch at {
			case "expiry without sweep":
				wantReason = voicegov.ReasonExpired
			case "terms edited while approved", "missing permission row":
				wantReason = voicegov.ReasonMissingCapability
			}
			var denied bool
			for _, entry := range audit {
				if entry.GenerationID != genID {
					continue
				}
				if strings.HasPrefix(entry.Action, "VOICE_GENERATED") {
					t.Fatal("allowed audit for withdrawn render")
				}
				if entry.Action == "GENERATION_REFUSED_RIGHTS" && entry.Decision == "denied" && entry.Reason == string(wantReason) {
					denied = true
				}
			}
			if !denied {
				t.Fatal("no rights refusal recorded for stale render")
			}
		})
	}
}

func TestVoicePreviewRechecksGrantBeforeFirstBytes(t *testing.T) {
	h := newVoicePlatformHarness(t)
	id := h.onboard(t)
	ctx := context.Background()
	grant, err := h.h.vplat.Grant(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	// Discovery succeeded and the rendering replica already sent the right
	// identity. Permission to stream is then removed before the first audio.
	h.ws.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/synthesize/stream" {
			h.worker.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("X-Engine", "cosyvoice")
		w.Header().Set("X-Engine-Version", testVoiceRevision)
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		grant.Capabilities[voicegov.CanStream] = false
		if _, err := h.h.vplat.UpdateGrantTerms(ctx, id, store.GrantTerms{Capabilities: grant.Capabilities}, "fixture", ""); err != nil {
			t.Errorf("withdraw stream permission: %v", err)
		}
		_, _ = w.Write([]byte("RIFF-not-for-delivery"))
	})
	w := h.do(t, "POST", "/v1/voices/stream", map[string]any{"voiceId": id, "purpose": "reflection", "text": "Grace and peace."}, h.freeTok)
	if w.Code != 403 || !strings.Contains(w.Body.String(), `"voice_unavailable"`) || strings.Contains(w.Body.String(), "RIFF") || w.Header().Get("X-Synthetic") != "" {
		t.Fatalf("preview started on withdrawn rights: HTTP %d, type %s", w.Code, w.Header().Get("Content-Type"))
	}
	audit, err := h.h.vplat.RightsAudit(ctx, id, 50)
	if err != nil {
		t.Fatal(err)
	}
	denied := false
	for _, entry := range audit {
		if entry.Action == "GENERATION_REFUSED_RIGHTS" && entry.Reason == string(voicegov.ReasonMissingCapability) && entry.Decision == "denied" {
			denied = true
		}
		if entry.Action == "VOICE_STREAMED" {
			t.Fatal("preview audited as streamed")
		}
	}
	if !denied {
		t.Fatal("preview refusal not audited")
	}
}

func TestVoiceGrantWithdrawalDuringVariantUploadKeepsOnlyRetainedMaster(t *testing.T) {
	for _, ext := range []string{".m4a", ".mp3"} {
		t.Run(ext, func(t *testing.T) {
			h, _ := newIntakeHarness(t)
			ctx := context.Background()
			id := h.onboard(t, "can_retain_after_termination")
			retainForHandoff(t, h, id)
			s := &handoffStorage{ObjectStorage: h.store}
			s.afterPut = func(key string) {
				if strings.HasSuffix(key, ext) {
					if err := h.h.vplat.TransitionGrant(ctx, id, voicegov.StatusRevoked, "fixture", "withdrawn", ""); err != nil {
						t.Fatal(err)
					}
				}
			}
			h.h.SetSigner(s)
			genID, _ := h.submitRender(t, id)
			if err := h.queue.ProcessNext(ctx); err != nil {
				t.Fatal(err)
			}
			g := h.generation(t, genID)
			if len(s.keys) < 2 || !strings.HasSuffix(s.keys[len(s.keys)-1], ext) || g.Status != "COMPLETED" || g.StorageKey != s.keys[0] || len(g.Variants) != 0 {
				t.Fatalf("unexpected master/variants: %s, %d variants, puts %v", g.Status, len(g.Variants), s.keys)
			}
			for _, key := range s.keys {
				exists, err := s.Exists(ctx, key)
				if err != nil || exists != (key == g.StorageKey) {
					t.Fatalf("wrong object retained: %s exists=%t err=%v", key, exists, err)
				}
			}
			if out := h.must(t, 200, "GET", "/v1/audio/jobs/"+genID, nil, h.freeTok); out["audioUrl"] == nil || out["variants"] != nil {
				t.Fatal("retention should deliver exactly the previously committed master")
			}
		})
	}
}

func TestVoiceDuplicateDeliveryCleansOnlyItsOwnUpload(t *testing.T) {
	h := newVoicePlatformHarness(t)
	ctx := context.Background()
	id, _ := h.submitRender(t, h.onboard(t))
	s := &handoffStorage{ObjectStorage: h.store}
	h.h.SetSigner(s)
	// A second physical delivery of even the SAME job ID wins while the first
	// put is returning. The loser must not remove the winner's object.
	s.afterPut = func(string) {
		s.afterPut = nil
		job := h.queue.List()[0]
		if err := h.h.runVoiceGeneration(ctx, &job); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.queue.ProcessNext(ctx); err != nil {
		t.Fatal(err)
	}
	g := h.generation(t, id)
	if len(s.keys) != 2 || s.keys[0] == s.keys[1] || g.StorageKey != s.keys[1] || g.Status != "COMPLETED" {
		t.Fatalf("lost winning render: %s puts=%v", g.Status, s.keys)
	}
	for i, key := range s.keys {
		if exists, err := s.Exists(ctx, key); err != nil || exists != (i == 1) {
			t.Fatalf("after duplicate: key %d exists=%t err=%v", i, exists, err)
		}
	}
}

func TestVoicePublicationAuditFailureRetriesWithoutPublishing(t *testing.T) {
	h := newVoicePlatformHarness(t)
	h.queue.WithPolicy(backoff.Policy{})
	ctx := context.Background()
	id := h.onboard(t)
	genID, _ := h.submitRender(t, id)
	s := &handoffStorage{ObjectStorage: h.store}
	h.h.SetSigner(s)
	if _, err := h.db.ExecContext(ctx, `ALTER TABLE voice_rights_audit_logs ADD CONSTRAINT reject_publish_audit CHECK (action <> 'VOICE_GENERATED')`); err != nil {
		t.Fatal(err)
	}
	err := h.queue.ProcessNext(ctx)
	g := h.generation(t, genID)
	if err == nil || jobs.IsPermanent(err) || g.Status != "QUEUED" || g.StorageKey != "" || len(s.keys) != 1 {
		t.Fatalf("publish despite audit failure: err=%v status=%s uploads=%d", err, g.Status, len(s.keys))
	}
	if exists, err := s.Exists(ctx, s.keys[0]); err != nil || exists {
		t.Fatalf("failed commit left private upload: %t %v", exists, err)
	}
	if _, err := h.db.ExecContext(ctx, `ALTER TABLE voice_rights_audit_logs DROP CONSTRAINT reject_publish_audit`); err != nil {
		t.Fatal(err)
	}
	// A fresh claim authorizes its own inference under the current terms,
	// not the version from the original submission or the discarded upload.
	grant, err := h.h.vplat.Grant(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	version, err := h.h.vplat.UpdateGrantTerms(ctx, id, store.GrantTerms{Capabilities: grant.Capabilities, Notes: "reviewed"}, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.queue.ProcessNext(ctx); err != nil {
		t.Fatal(err)
	}
	if g := h.generation(t, genID); g.Status != "COMPLETED" || g.GrantVersion != version || len(s.keys) != 2 || g.StorageKey != s.keys[1] {
		t.Fatal("repaired audit did not permit a fresh publish")
	}
}

func TestVoiceWithdrawalBeforeFallbackDoesNotForwardText(t *testing.T) {
	h := newVoicePlatformHarness(t)
	ctx := context.Background()
	id := h.onboard(t)
	_, fallback := h.registerTestFallback(t, id)
	h.worker.busyThrough.Store(1)
	h.ws.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/synthesize" {
			if err := h.h.vplat.TransitionGrant(ctx, id, voicegov.StatusSuspended, "fixture", "review", ""); err != nil {
				t.Errorf("suspend: %v", err)
			}
		}
		h.worker.ServeHTTP(w, r)
	})
	genID, _ := h.submitRender(t, id)
	err := h.queue.ProcessNext(ctx)
	if g := h.generation(t, genID); !jobs.IsPermanent(err) || g.ErrorClass != "rights" || g.StorageKey != "" || fallback.calls.Load() != 0 {
		t.Fatalf("fallback after suspension: err=%v status=%s fallback calls=%d", err, g.Status, fallback.calls.Load())
	}
}

func TestVoiceEmptyPreviewDoesNotStartPlayback(t *testing.T) {
	h := newVoicePlatformHarness(t)
	id := h.onboard(t)
	h.ws.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/synthesize/stream" {
			h.worker.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("X-Engine", "cosyvoice")
		w.Header().Set("X-Engine-Version", testVoiceRevision)
		w.WriteHeader(200)
	})
	w := h.do(t, "POST", "/v1/voices/stream", map[string]any{"voiceId": id, "purpose": "reflection", "text": "Grace and peace."}, h.freeTok)
	if w.Code != 503 || !strings.Contains(w.Body.String(), `"voice_temporarily_unavailable"`) || w.Header().Get("X-Synthetic") != "" {
		t.Fatalf("empty stream: HTTP %d", w.Code)
	}
	entries, err := h.h.vplat.RightsAudit(context.Background(), id, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Action == "VOICE_STREAMED" {
			t.Fatal("empty worker body audited as playback")
		}
	}
}
