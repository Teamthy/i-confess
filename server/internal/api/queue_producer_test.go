package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/jobs"
	"github.com/Teamthy/i-confess/internal/voice"
	"github.com/Teamthy/i-confess/internal/workers"
)

// failingSynth is a provider that refuses every request. It exists to make the
// retry distinction observable: one kind of fault is worth another attempt and
// the other is not, and the queue is only useful if the two are told apart.
type failingSynth struct{ err error }

func (s *failingSynth) Name() string { return "failing" }

func (s *failingSynth) Synthesize(context.Context, voice.SynthesisRequest) (*voice.SynthesisResult, error) {
	return nil, s.err
}

// queueOf reaches the in-process queue the test fixture runs on.
func queueOf(t *testing.T, h *qaHarness) *jobs.MemoryQueue {
	t.Helper()
	q, ok := h.h.GetQueue().(*jobs.MemoryQueue)
	if !ok {
		t.Fatal("the fixture is not running the in-memory queue this test inspects")
	}
	return q
}

// TestRetryableProviderFaultQueuesARetry covers the producer side of the queue.
//
// A transient provider fault is exactly what a queue is for: the request was
// well-formed, the rights were in order and the operator did everything right.
// Before this the render simply stayed failed until somebody noticed and
// resubmitted it by hand, and the confession had no audio in the meantime.
func TestRetryableProviderFaultQueuesARetry(t *testing.T) {
	h := newQAHarness(t)
	h.h.SetPipeline(voice.NewPipeline(
		&failingSynth{err: voice.RetryableError("provider returned 503")}, h.store))
	h.grantRights(t)

	conf := h.confessionID(t)
	rec := h.do(t, "POST", "/admin/audio/generate", map[string]any{
		"confession_id": conf, "voice_id": h.voiceStd,
	}, h.admin)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 for a retryable provider fault", rec.Code)
	}

	queued := queueOf(t, h).List()
	if len(queued) != 1 {
		t.Fatalf("%d jobs queued, want 1: %+v", len(queued), queued)
	}
	j := queued[0]
	if j.Type != workers.TypeAudioGenerate {
		t.Errorf("job type = %q, want %q", j.Type, workers.TypeAudioGenerate)
	}
	if got, _ := j.Payload["confession_id"].(string); got != conf {
		t.Errorf("queued confession_id = %q, want %q", got, conf)
	}
	if got, _ := j.Payload["voice_id"].(string); got != h.voiceStd {
		t.Errorf("queued voice_id = %q, want %q", got, h.voiceStd)
	}
	if j.IdempotencyKey == "" {
		t.Error("the retry job has no idempotency key, so repeated failures pile up")
	}
	if j.MaxAttempts != jobs.DefaultMaxAttempts {
		t.Errorf("max attempts = %d, want %d", j.MaxAttempts, jobs.DefaultMaxAttempts)
	}
}

// TestPermanentProviderFaultIsNotQueued is the other half. Queueing a fault that
// will recur means paying for attempts that cannot succeed and burying the real
// signal - a rejected voice id - under retries.
func TestPermanentProviderFaultIsNotQueued(t *testing.T) {
	h := newQAHarness(t)
	h.h.SetPipeline(voice.NewPipeline(
		&failingSynth{err: voice.PermanentError("voice id is not valid on this account")}, h.store))
	h.grantRights(t)

	rec := h.do(t, "POST", "/admin/audio/generate", map[string]any{
		"confession_id": h.confessionID(t), "voice_id": h.voiceStd,
	}, h.admin)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 for a permanent provider fault", rec.Code)
	}
	if n := len(queueOf(t, h).List()); n != 0 {
		t.Errorf("%d jobs queued for a fault that will recur, want 0", n)
	}
}

// TestRepeatedFailureDoesNotQueueTwice is why the retry carries an idempotency
// key: an operator clicking generate three times against a provider that is down
// must produce one retry, not three.
func TestRepeatedFailureDoesNotQueueTwice(t *testing.T) {
	h := newQAHarness(t)
	h.h.SetPipeline(voice.NewPipeline(
		&failingSynth{err: voice.RetryableError("provider returned 429")}, h.store))
	h.grantRights(t)

	body := map[string]any{"confession_id": h.confessionID(t), "voice_id": h.voiceStd}
	for i := 0; i < 3; i++ {
		if rec := h.do(t, "POST", "/admin/audio/generate", body, h.admin); rec.Code != http.StatusBadGateway {
			t.Fatalf("attempt %d: status = %d, want 502", i, rec.Code)
		}
	}
	if n := len(queueOf(t, h).List()); n != 1 {
		t.Errorf("%d retry jobs queued for one render, want 1", n)
	}
}

// TestQueuedGenerationRunsThroughTheWorker closes the loop: a job the producer
// queued is performed by the registered handler, using the same pipeline the
// synchronous endpoint uses.
func TestQueuedGenerationRunsThroughTheWorker(t *testing.T) {
	h := newQAHarness(t)
	h.grantRights(t)
	conf := h.confessionID(t)

	// First the provider is down, which queues the retry.
	h.h.SetPipeline(voice.NewPipeline(
		&failingSynth{err: voice.RetryableError("provider returned 503")}, h.store))
	if rec := h.do(t, "POST", "/admin/audio/generate", map[string]any{
		"confession_id": conf, "voice_id": h.voiceStd,
	}, h.admin); rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}

	// Then it recovers. The worker, not the operator, finishes the render.
	synth := &stubSynth{}
	h.h.SetPipeline(voice.NewPipeline(synth, h.store))

	q := queueOf(t, h)
	workers.Register(q, workers.Services{Generate: h.h})
	w := jobs.NewWorker(q, 1)
	w.PollInterval = 5 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	defer func() { cancel(); w.Stop() }()

	deadline := 200
	for i := 0; i < deadline; i++ {
		stats, err := q.Stats(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if stats.Completed > 0 {
			break
		}
		if stats.DeadLetter > 0 {
			for _, j := range q.List() {
				t.Fatalf("the queued retry was parked: %q (%s)", j.DeadLetterReason, j.LastError)
			}
		}
		time.Sleep(5 * time.Millisecond)
	}

	stats, err := q.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Completed != 1 {
		t.Fatalf("stats = %+v, want the queued retry to have completed", stats)
	}
	if synth.calls == 0 {
		t.Error("the worker completed the job without the provider being called")
	}
}
