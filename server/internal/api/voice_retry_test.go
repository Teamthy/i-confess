package api

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/backoff"
	"github.com/Teamthy/i-confess/internal/jobs"
	"github.com/Teamthy/i-confess/internal/storage"
	"github.com/Teamthy/i-confess/internal/store"
)

func (h *voicePlatformHarness) submitRender(t *testing.T, voiceID string) (genID, jobID string) {
	t.Helper()
	req := map[string]any{"voiceId": voiceID, "text": "Grace and peace.", "purpose": "reflection"}
	g := h.must(t, 202, "POST", "/v1/voices/generate", req, h.freeTok)["generation"].(map[string]any)
	return g["generationId"].(string), g["jobId"].(string)
}

func (h *voicePlatformHarness) generation(t *testing.T, genID string) *store.Generation {
	t.Helper()
	g, err := h.h.vplat.GenerationByID(context.Background(), genID)
	if err != nil || g == nil {
		t.Fatalf("load generation: %v", err)
	}
	return g
}

// Drive the same PostgreSQL queue and worker loop used by the server. A queued
// retry is not success: the second claim must actually produce the asset.
func TestVoiceRetryAfterBusyWorker(t *testing.T) {
	h := newVoicePlatformHarness(t)
	id := h.onboard(t)
	ctx := context.Background()
	var now atomic.Int64
	now.Store(time.Now().Unix())
	q := store.NewJobQueue(h.db).WithClock(func() time.Time { return time.Unix(now.Load(), 0) }).WithPolicy(backoff.New(time.Minute, time.Hour))
	h.h.SetQueue(q)
	h.h.RegisterVoiceJobs()
	h.worker.busyThrough.Store(1)
	genID, jobID := h.submitRender(t, id)
	w := jobs.NewWorker(q, 1)
	w.PollInterval = 5 * time.Millisecond
	w.Start(ctx)
	t.Cleanup(w.Stop)
	wait := func(status string, attempts int) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); ; {
			j, err := q.ByID(ctx, jobID)
			if err != nil {
				t.Fatal(err)
			}
			if j.Status == status && j.Attempts == attempts {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("job = %s (%d attempts), want %s (%d): %s", j.Status, j.Attempts, status, attempts, j.LastError)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	wait(jobs.StatusQueued, 1)
	// Inspect state during backoff, and re-submit as a polling client might.
	out := h.must(t, 200, "GET", "/v1/audio/jobs/"+genID, nil, h.freeTok)
	if st := out["generation"].(map[string]any)["status"]; st != "QUEUED" || out["audioUrl"] != nil {
		t.Fatalf("after busy response: status %v, audioUrl %v", st, out["audioUrl"])
	}
	if g, j := h.submitRender(t, id); g != genID || j != jobID {
		t.Fatal("resubmitting during backoff created another job")
	}
	now.Add(60)
	wait(jobs.StatusCompleted, 2)
	out = h.must(t, 200, "GET", "/v1/audio/jobs/"+genID, nil, h.freeTok)
	if st := out["generation"].(map[string]any)["status"]; st != "COMPLETED" || out["audioUrl"] == nil || h.worker.calls.Load() != 2 {
		t.Fatalf("queue completed but render = %v, worker calls = %d, audioUrl present = %v", st, h.worker.calls.Load(), out["audioUrl"] != nil)
	}
}

func TestVoiceRetryExhaustion(t *testing.T) {
	h := newVoicePlatformHarness(t)
	h.queue.WithPolicy(backoff.Policy{}) // immediately due, actual claim still increments Attempts
	h.worker.busyThrough.Store(4)
	genID, _ := h.submitRender(t, h.onboard(t))
	for attempt := 1; attempt <= 4; attempt++ {
		if err := h.queue.ProcessNext(context.Background()); err == nil || jobs.IsPermanent(err) {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		g, j := h.generation(t, genID), h.queue.List()[0]
		if j.Attempts != attempt || h.worker.calls.Load() != int32(attempt) || g.StorageKey != "" {
			t.Fatalf("attempt %d: claims %d, provider calls %d, key %q", attempt, j.Attempts, h.worker.calls.Load(), g.StorageKey)
		}
		if attempt < 4 {
			if g.Status != "QUEUED" || j.Status != jobs.StatusQueued {
				t.Fatalf("after attempt %d: generation=%s job=%s", attempt, g.Status, j.Status)
			}
		} else if g.Status != "FAILED" || g.ErrorClass != "transient" || j.Status != jobs.StatusDeadLetter || !strings.Contains(j.DeadLetterReason, "exhausted") {
			t.Fatalf("after exhaustion: generation=%s (%s) job=%s (%s)", g.Status, g.ErrorClass, j.Status, j.DeadLetterReason)
		} else if !strings.Contains(j.LastError, "synthesis worker busy") || strings.Contains(g.ErrorMessage, "synthesis worker busy") {
			t.Fatal("raw inference diagnostics belong on the queue job, not the listener-facing generation")
		}
	}
}

func TestVoiceRetryRechecksRevokedGrant(t *testing.T) {
	h := newVoicePlatformHarness(t)
	h.queue.WithPolicy(backoff.Policy{})
	h.worker.busyThrough.Store(1)
	id := h.onboard(t)
	genID, _ := h.submitRender(t, id)
	if err := h.queue.ProcessNext(context.Background()); err == nil {
		t.Fatal("expected busy worker")
	}
	h.must(t, 200, "POST", "/v1/admin/voices/"+id+"/revoke", map[string]any{"reason": "licence terminated"}, h.admin)
	if err := h.queue.ProcessNext(context.Background()); !jobs.IsPermanent(err) {
		t.Fatalf("retry after revocation: %v", err)
	}
	g := h.generation(t, genID)
	if g.Status != "FAILED" || g.ErrorClass != "rights" || h.worker.calls.Load() != 1 || h.queue.List()[0].Status != jobs.StatusDeadLetter {
		t.Fatalf("retry: generation=%s (%s), calls=%d", g.Status, g.ErrorClass, h.worker.calls.Load())
	}
}

func TestVoiceCancelledRetryCannotTakeOverResubmission(t *testing.T) {
	h := newVoicePlatformHarness(t)
	h.queue.WithPolicy(backoff.Policy{})
	h.worker.busyThrough.Store(1)
	id := h.onboard(t)
	genID, oldJob := h.submitRender(t, id)
	if err := h.queue.ProcessNext(context.Background()); err == nil {
		t.Fatal("expected busy worker")
	}
	h.must(t, 200, "POST", "/v1/audio/jobs/"+genID+"/cancel", nil, h.freeTok)
	// The same hash reuses the generation row, NOT the job. Deliver the old
	// retry first; it must neither call the worker nor change the new row.
	g, newJob := h.submitRender(t, id)
	if g != genID || oldJob == newJob {
		t.Fatal("expected a fresh job for the same generation")
	}
	if err := h.queue.ProcessNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if h.worker.calls.Load() != 1 || h.generation(t, genID).Status != "QUEUED" {
		t.Fatal("stale delivery took over resubmitted generation")
	}
	if err := h.queue.ProcessNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if h.worker.calls.Load() != 2 || h.generation(t, genID).Status != "COMPLETED" {
		t.Fatal("new job did not render")
	}
}

func TestVoiceBusyStreamIsUnavailableNotQueued(t *testing.T) {
	h := newVoicePlatformHarness(t)
	h.worker.busyThrough.Store(1)
	out := h.must(t, 503, "POST", "/v1/voices/stream", map[string]any{"voiceId": h.onboard(t), "text": "Grace and peace.", "purpose": "reflection"}, h.freeTok)
	if out["code"] != "voice_temporarily_unavailable" || len(h.queue.List()) != 0 || h.worker.calls.Load() != 1 {
		t.Fatalf("busy preview: %v", out)
	}
}

type testUploadStore struct {
	storage.ObjectStorage
	calls     int
	failFirst bool
}

func (s *testUploadStore) Upload(ctx context.Context, key string, b []byte, meta map[string]string) error {
	s.calls++
	if s.failFirst && s.calls == 1 {
		return errors.New("temporary storage failure")
	}
	return s.ObjectStorage.Upload(ctx, key, b, meta)
}

func TestVoiceRetryAfterUploadFailure(t *testing.T) {
	h := newVoicePlatformHarness(t)
	h.queue.WithPolicy(backoff.Policy{})
	s := &testUploadStore{ObjectStorage: h.store, failFirst: true}
	h.h.SetSigner(s)
	genID, _ := h.submitRender(t, h.onboard(t))
	ctx := context.Background()
	if err := h.queue.ProcessNext(ctx); err == nil || jobs.IsPermanent(err) {
		t.Fatalf("upload failure: %v", err)
	}
	if g := h.generation(t, genID); g.Status != "QUEUED" || g.StorageKey != "" || s.calls != 1 {
		t.Fatal("failed upload published an asset or stranded the generation")
	}
	if err := h.queue.ProcessNext(ctx); err != nil {
		t.Fatal(err)
	}
	if g := h.generation(t, genID); g.Status != "COMPLETED" || g.StorageKey == "" || s.calls != 2 {
		t.Fatal("successful upload retry did not finish the generation")
	}
}

// A zero-row transition (e.g. cancellation) and an actual SQL error are not
// interchangeable. Neither gives permission to upload, but the latter retries.
func TestVoiceRetryOnPhaseWriteError(t *testing.T) {
	for _, phase := range []string{"PROCESSING", "UPLOADING"} {
		t.Run(phase, func(t *testing.T) {
			h := newVoicePlatformHarness(t)
			h.queue.WithPolicy(backoff.Policy{})
			s := &testUploadStore{ObjectStorage: h.store}
			h.h.SetSigner(s)
			ctx := context.Background()
			genID, _ := h.submitRender(t, h.onboard(t))
			if _, err := h.db.ExecContext(ctx, `ALTER TABLE voice_generations ADD CHECK (status <> '`+phase+`')`); err != nil {
				t.Fatal(err)
			}
			if err := h.queue.ProcessNext(ctx); err == nil || jobs.IsPermanent(err) {
				t.Fatalf("phase error did not retry: %v", err)
			}
			if g := h.generation(t, genID); g.Status != "QUEUED" || s.calls != 0 {
				t.Fatalf("after failed %s transition: status=%s uploads=%d", phase, g.Status, s.calls)
			}
			if phase == "PROCESSING" && h.worker.calls.Load() != 0 {
				t.Fatal("inference started without a processing transition")
			}
		})
	}
}

// Cancel and resubmit while the original HTTP call is still rendering. A
// cancellation read followed by a separate unconditional upload would race.
func TestVoiceCancelledInFlightCannotUploadForNewJob(t *testing.T) {
	h := newVoicePlatformHarness(t)
	id := h.onboard(t)
	genID, oldJob := h.submitRender(t, id)
	s := &testUploadStore{ObjectStorage: h.store}
	h.h.SetSigner(s)
	entered, release := make(chan struct{}), make(chan struct{})
	h.worker.beforeAudio = func() { close(entered); <-release }
	done := make(chan error, 1)
	go func() { done <- h.queue.ProcessNext(context.Background()) }()
	// Always unblock the fake before the fixture shuts down, even on Fatal.
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("worker never entered synthesis")
	}
	h.must(t, 200, "POST", "/v1/audio/jobs/"+genID+"/cancel", nil, h.freeTok)
	g, newJob := h.submitRender(t, id)
	if g != genID || oldJob == newJob {
		t.Fatal("expected same generation and new job")
	}
	unblock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled worker never returned")
	}
	if s.calls != 0 || h.generation(t, genID).Status != "QUEUED" {
		t.Fatal("in-flight result wrote into the replacement submission")
	}
}
