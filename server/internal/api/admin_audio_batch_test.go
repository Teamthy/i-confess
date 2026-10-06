package api

// The legacy bulk endpoint, POST /admin/audio/generate/batch (audit finding
// VE-021, found while working VE-017).
//
// It used to call CreateJob without a content_version_id, which the store
// refuses - a job must point at the exact text a render was made from. Every
// item failed, the loop logged and continued, and the endpoint answered
// "Created 0 generation jobs" with a 202. Callers could not tell a queue that
// was busy from a queue that had never been written to, and the response read
// like a receipt for work nobody had done.
//
// The single-item endpoint beside it had already been repaired for exactly this
// bug; the batch path was missed. These two tests are the reason it cannot be
// missed again: one proves the work is real, the other proves an empty result is
// reported as an empty result.

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/Teamthy/i-confess/internal/jobs"

	"github.com/Teamthy/i-confess/internal/workers"
)

// durableQueue wires the queue the same way production startup does. The
// handler separates "a queue exists in this process" from "a queue survives a
// restart", because a bulk render accepted onto a queue that is lost on restart
// is the original bug wearing a different hat; the fixture runs the in-memory
// queue for its own reasons, so this opts the test into the path that enqueues.
func durableQueue(t *testing.T, h *qaHarness) {
	t.Helper()
	h.h.SetQueue(jobs.NewMemoryQueue())
}

func TestLegacyBatchEndpointRefusesWhenNoDurableQueueIsWired(t *testing.T) {
	// The fixture's default: no durable queue. Answering 202 here would be the
	// old behaviour exactly - a receipt for work that cannot happen.
	h := newQAHarness(t)
	h.grantRights(t)
	ids := h.confessionIDs(t)
	rec := h.do(t, "POST", "/admin/audio/generate/batch", map[string]any{
		"confession_ids": ids, "voice_id": h.voiceStd}, h.admin)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "durable background queue") {
		t.Fatalf("the refusal must name what to do about it: %s", rec.Body.String())
	}
	var rowCount int
	if err := h.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM audio_generation_jobs`).Scan(&rowCount); err != nil {
		t.Fatal(err)
	}
	if rowCount != 0 {
		t.Fatalf("a refusal that records %d job rows is not a refusal", rowCount)
	}
}

func TestLegacyBatchEndpointQueuesWorkThatExists(t *testing.T) {
	h := newQAHarness(t)
	h.grantRights(t)
	durableQueue(t, h)
	ids := h.confessionIDs(t)
	if len(ids) == 0 {
		t.Fatal("the fixture has no confessions to queue")
	}

	rec := h.do(t, "POST", "/admin/audio/generate/batch", map[string]any{
		"confession_ids": ids, "voice_id": h.voiceStd}, h.admin)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status %d, want 202: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if got := int(body["queued"].(float64)); got != len(ids) {
		t.Fatalf("queued = %d, want %d: %v", got, len(ids), body)
	}
	if !strings.HasPrefix(body["message"].(string), strconv.Itoa(len(ids))+" queued") {
		t.Fatalf("message must state the counts it means: %v", body["message"])
	}

	// A job row only counts if it points at snapshotted text: the missing
	// content_version_id is the whole reason this endpoint used to no-op.
	var total, withVersion int
	if err := h.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM audio_generation_jobs`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if err := h.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM audio_generation_jobs WHERE content_version_id IS NOT NULL`).Scan(&withVersion); err != nil {
		t.Fatal(err)
	}
	if total != len(ids) || withVersion != len(ids) {
		t.Fatalf("jobs recorded=%d with a text version=%d, want %d of each", total, withVersion, len(ids))
	}

	// And someone has to do the work. A row that no queue owns is the bug in a
	// different table.
	queued := queueOf(t, h).List()
	found := 0
	for _, j := range queued {
		if j.Type == workers.TypeAudioGenerate {
			found++
		}
	}
	if found != len(ids) {
		t.Fatalf("%d TypeAudioGenerate jobs on the queue, want %d (all jobs: %d)", found, len(ids), len(queued))
	}

	// Re-submitting the same list queues nothing new: every item comes back
	// "reused" pointing at the job that already exists.
	again := h.do(t, "POST", "/admin/audio/generate/batch", map[string]any{
		"confession_ids": ids, "voice_id": h.voiceStd}, h.admin)
	if again.Code != http.StatusAccepted {
		t.Fatalf("re-submit status %d, want 202: %s", again.Code, again.Body.String())
	}
	var reBody map[string]any
	_ = json.Unmarshal(again.Body.Bytes(), &reBody)
	if reBody["queued"].(float64) != 0 || reBody["reused"].(float64) != float64(len(ids)) {
		t.Fatalf("re-submit must reuse, not re-render: %v", reBody)
	}
	if got := len(queueOf(t, h).List()); got != len(ids) {
		t.Fatalf("re-submit put %d jobs on the queue, want %d", got, len(ids))
	}
}

func TestLegacyBatchEndpointReportsRefusalsAsRefusals(t *testing.T) {
	h := newQAHarness(t)
	h.grantRights(t)
	durableQueue(t, h)

	rec := h.do(t, "POST", "/admin/audio/generate/batch", map[string]any{
		"confession_ids": []string{"missing-1", "missing-2"}, "voice_id": h.voiceStd}, h.admin)
	if rec.Code == http.StatusAccepted {
		t.Fatalf("a batch that queued nothing must not be accepted: %s", rec.Body.String())
	}
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d, want 422: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["queued"].(float64) != 0 || body["refused"].(float64) != 2 {
		t.Fatalf("counts: %v", body)
	}
	items, _ := body["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("the caller needs to know which id failed, got %d item results", len(items))
	}
	for _, raw := range items {
		it, _ := raw.(map[string]any)
		if it["outcome"] != "refused" || !strings.Contains(it["error"].(string), "not found") {
			t.Fatalf("item result must name its reason: %v", it)
		}
	}
	var rowCount int
	if err := h.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM audio_generation_jobs`).Scan(&rowCount); err != nil {
		t.Fatal(err)
	}
	if rowCount != 0 {
		t.Fatalf("refused items must not leave job rows behind, found %d", rowCount)
	}
}
