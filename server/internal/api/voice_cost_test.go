package api

// Cost telemetry and the batch budget ceiling (audit VE-017), tested through the
// real endpoints rather than around them: the finding was that spend was
// neither measured nor bounded, so these tests pin both halves - what a worker
// reports has to survive into the row and the metrics, and a batch that cannot
// fit the ceiling has to be refused *before* it queues anything.

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// metricCounter reads one line of the Prometheus exposition.
func metricCounter(t *testing.T, body, name string) float64 {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, name+" ") {
			v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(line, name+" ")), 64)
			if err != nil {
				t.Fatalf("metric %s is not a number: %q", name, line)
			}
			return v
		}
	}
	t.Fatalf("metric %s missing from exposition", name)
	return 0
}

func TestRenderCostIsMeasuredStoredAndPriced(t *testing.T) {
	t.Setenv("VOICE_GPU_SECOND_COST_USD", "0.0004")
	h := newVoicePlatformHarness(t)
	id := h.onboard(t)

	prom := func() string {
		rec := h.do(t, "GET", "/v1/admin/voice-metrics?format=prometheus", nil, h.admin)
		if rec.Code != http.StatusOK {
			t.Fatalf("prometheus metrics: %d %s", rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}
	beforeInf, beforeAudio, beforeMetered := metricCounter(t, prom(), "icf_voice_inference_seconds_total"),
		metricCounter(t, prom(), "icf_voice_audio_seconds_total"), metricCounter(t, prom(), "icf_voice_metered_renders_total")

	out := h.must(t, 202, "POST", "/v1/voices/generate", map[string]any{"voiceId": id, "text": "Grace and peace.",
		"style": "reflection", "purpose": "reflection"}, h.admin)
	genID := out["generation"].(map[string]any)["generationId"].(string)
	h.drain(t)

	job := h.must(t, 200, "GET", "/v1/audio/jobs/"+genID, nil, h.admin)
	g := job["generation"].(map[string]any)
	// The worker said 2.5 s of inference and 3.25 s of request; the API must
	// report exactly that, not a rounded echo of its own wall clock.
	if g["inferenceSeconds"] != 2.5 || g["workerSeconds"] != 3.25 {
		t.Fatalf("cost headers did not reach the generation: %v", g)
	}
	// 2.5 s x $0.0004/s = $0.001 = 1000 microseconds, stored as an integer
	// because money in a float column is how rounding arguments start.
	if g["costUsdMicros"] != float64(1000) {
		t.Fatalf("costUsdMicros = %v, want 1000", g["costUsdMicros"])
	}
	var inf, work sql.NullFloat64
	var micros sql.NullInt64
	if err := h.db.QueryRowContext(context.Background(),
		`SELECT inference_seconds, worker_seconds, cost_usd_micros FROM voice_generations WHERE id = ?`, genID).
		Scan(&inf, &work, &micros); err != nil {
		t.Fatal(err)
	}
	if !inf.Valid || inf.Float64 != 2.5 || !work.Valid || work.Float64 != 3.25 || !micros.Valid || micros.Int64 != 1000 {
		t.Fatalf("stored cost row: %+v %+v %+v", inf, work, micros)
	}

	// Deltas, because these counters are per-process globals shared by every
	// test in the package: an absolute number here would be a scheduling accident.
	if got := metricCounter(t, prom(), "icf_voice_inference_seconds_total") - beforeInf; math.Abs(got-2.5) > 1e-6 {
		t.Fatalf("metrics gained %v inference-seconds for a 2.5 s render", got)
	}
	if got := metricCounter(t, prom(), "icf_voice_audio_seconds_total") - beforeAudio; math.Abs(got-3) > 1e-6 {
		t.Fatalf("metrics gained %v audio-seconds for a 3 s render", got)
	}
	if got := metricCounter(t, prom(), "icf_voice_metered_renders_total") - beforeMetered; got != 1 {
		t.Fatalf("metered renders grew by %v, want 1", got)
	}
	// The JSON view the admin UI reads has to say the same thing, and say
	// whether a price exists at all.
	snap := h.must(t, 200, "GET", "/v1/admin/voice-metrics", nil, h.admin)["metrics"].(map[string]any)
	cost, _ := snap["cost"].(map[string]any)
	if cost == nil || cost["priced"] != true {
		t.Fatalf("metrics cost block: %v", snap["cost"])
	}
	if cost["secondsPerAudioSecond"] != 0.8333 {
		t.Errorf("secondsPerAudioSecond = %v, want the measured 2.5/3.0 = 0.8333", cost["secondsPerAudioSecond"])
	}
	// $0.001 over 3 s of audio is $0.02 per audio-minute.
	if cost["costPerAudioMinuteUSD"] != 0.02 {
		t.Errorf("costPerAudioMinuteUSD = %v, want 0.02", cost["costPerAudioMinuteUSD"])
	}
}

func TestUnreportedWorkerCostStaysUnmeasuredNotFree(t *testing.T) {
	// The other half of the contract. A worker that reports nothing must leave
	// the columns NULL: 0 would read as "this render was free", which is the
	// exact misreading a cost system exists to prevent.
	t.Setenv("VOICE_GPU_SECOND_COST_USD", "0.0004")
	h := newVoicePlatformHarness(t)
	h.worker.reportCosts = false
	id := h.onboard(t)

	out := h.must(t, 202, "POST", "/v1/voices/generate", map[string]any{"voiceId": id, "text": "Grace and peace.",
		"style": "reflection", "purpose": "reflection"}, h.admin)
	genID := out["generation"].(map[string]any)["generationId"].(string)
	h.drain(t)

	h.must(t, 200, "GET", "/v1/audio/jobs/"+genID, nil, h.admin)
	var n sql.NullFloat64
	var micros sql.NullInt64
	if err := h.db.QueryRowContext(context.Background(),
		`SELECT inference_seconds, cost_usd_micros FROM voice_generations WHERE id = ?`, genID).Scan(&n, &micros); err != nil {
		t.Fatal(err)
	}
	if n.Valid || micros.Valid {
		t.Fatalf("unreported cost was written as %v/%v; unmeasured must stay NULL", n, micros)
	}
}

func TestBatchOverGpuBudgetIsRefusedBeforeAnythingIsQueued(t *testing.T) {
	// The finding, precisely: a 10,000-item batch was an unbounded GPU spend.
	// The fix is only real if nothing is queued on the refusal path, so that is
	// what this asserts - not just the status code.
	t.Setenv("VOICE_BATCH_GPU_BUDGET_SECONDS", "1")
	h := newVoicePlatformHarness(t)
	id := h.onboard(t)

	long := "Blessed is the one who walks not in the counsel of the wicked, nor stands in the way of sinners, nor sits in the seat of the mockers. "
	items := make([]map[string]any, 0, 3)
	for i := 0; i < 3; i++ {
		items = append(items, map[string]any{"label": "Psalm " + strconv.Itoa(i+1), "text": long})
	}
	rec := h.do(t, "POST", "/v1/admin/voices/"+id+"/batches", map[string]any{
		"title": "Psalms", "purpose": "reflection", "style": "reflection", "items": items}, h.admin)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d, want 422: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["code"] != "voice_budget_exceeded" {
		t.Fatalf("code = %v, want voice_budget_exceeded; body %v", body["code"], body)
	}
	// The refusal is only actionable if it carries the plan it refused and the
	// ceiling it was measured against.
	est, _ := body["estimate"].(map[string]any)
	if est == nil || est["items"].(float64) != 3 {
		t.Fatalf("refusal must say what was planned: %v", body)
	}
	if basis, _ := est["basis"].(string); !strings.HasPrefix(basis, "assumed:") {
		t.Fatalf("with no completed renders the basis must be the stated assumption, got %q", basis)
	}
	if bud, _ := body["budget"].(map[string]any); bud == nil || bud["batch_ceiling_seconds"].(float64) != 1 {
		t.Fatalf("refusal must name the ceiling: %v", body["budget"])
	}
	if body["items_fit"].(float64) != 0 {
		t.Fatalf("items_fit should be 0 against a 1 s ceiling, got %v", body["items_fit"])
	}
	if calls := h.worker.calls.Load(); calls != 0 {
		t.Fatalf("%d renders reached the worker after a budget refusal", calls)
	}
	var gens, batches int
	if err := h.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM voice_generations`).Scan(&gens); err != nil {
		t.Fatal(err)
	}
	if err := h.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM voice_batches`).Scan(&batches); err != nil {
		t.Fatal(err)
	}
	if gens != 0 || batches != 0 {
		t.Fatalf("refusal left work behind: %d generations, %d batches", gens, batches)
	}
	// The refusal is on the rights audit trail, not only in the response.
	audit := h.must(t, 200, "GET", "/v1/admin/voices/"+id+"/audit", nil, h.admin)
	found := false
	for _, e := range audit["entries"].([]any) {
		if m, _ := e.(map[string]any); m["action"] == "VOICE_BATCH_REFUSED_BUDGET" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no budget refusal in the audit log: %v", audit["entries"])
	}
}

func TestBatchRecordsPlannedSpendThenActualSpend(t *testing.T) {
	t.Setenv("VOICE_BATCH_GPU_BUDGET_SECONDS", "100000")
	t.Setenv("VOICE_GPU_SECOND_COST_USD", "0.0004")
	h := newVoicePlatformHarness(t)
	id := h.onboard(t)

	items := []map[string]any{{"label": "a", "text": "Grace and peace."}, {"label": "b", "text": "Be still and know."}}
	out := h.must(t, 202, "POST", "/v1/admin/voices/"+id+"/batches", map[string]any{
		"title": "Two", "purpose": "reflection", "style": "reflection", "items": items}, h.admin)
	b := out["batch"].(map[string]any)
	batchID := b["id"].(string)
	// The plan is recorded next to the batch: a ceiling you cannot audit after
	// the fact is a ceiling nobody can argue with later.
	est, ok := b["estInferenceSeconds"].(float64)
	if !ok || est <= 0 {
		t.Fatalf("batch has no planned spend: %v", b)
	}
	if basis, _ := b["estimateBasis"].(string); !strings.HasPrefix(basis, "assumed:") {
		t.Fatalf("estimate basis: %q", basis)
	}
	if micro, ok := b["estCostUsdMicros"].(float64); !ok || micro <= 0 {
		t.Fatalf("a priced deployment must record a planned cost, got %v", b["estCostUsdMicros"])
	}

	h.drain(t)
	got := h.must(t, 200, "GET", "/v1/admin/batches/"+batchID, nil, h.admin)["batch"].(map[string]any)
	actual, ok := got["actualSpend"].(map[string]any)
	if !ok {
		t.Fatalf("no measured spend on the batch: %v", got)
	}
	if actual["metered"].(float64) != 2 || actual["completed"].(float64) != 2 {
		t.Fatalf("expected 2 metered completed renders: %v", actual)
	}
	// Both renders reported 2.5 s, so the measured figure is 5 s of inference -
	// and the comparison an operator wants is the one this row now makes
	// possible: planned against actual.
	if actual["inferenceSeconds"].(float64) != 5 {
		t.Fatalf("measured inference total: %v", actual["inferenceSeconds"])
	}
	if actual["costUsdMicros"].(float64) != 2000 {
		t.Fatalf("measured cost: %v", actual)
	}
}

func TestBatchEstimateUsesMeasuredHistoryWhenThereIsAny(t *testing.T) {
	// An estimate from nothing is an assumption; the point of recording
	// inference seconds is that after the first render there is no need to
	// assume. The basis string is the assertion here, because a number that
	// silently changes source is how estimates get trusted wrongly.
	t.Setenv("VOICE_BATCH_GPU_BUDGET_SECONDS", "100000")
	h := newVoicePlatformHarness(t)
	id := h.onboard(t)
	out := h.must(t, 202, "POST", "/v1/voices/generate", map[string]any{"voiceId": id, "text": "Grace and peace.",
		"style": "reflection", "purpose": "reflection"}, h.admin)
	_ = out
	h.drain(t)

	items := []map[string]any{{"label": "a", "text": "Grace and peace."}}
	got := h.must(t, 202, "POST", "/v1/admin/voices/"+id+"/batches", map[string]any{
		"title": "One", "purpose": "reflection", "style": "reflection", "items": items}, h.admin)["batch"].(map[string]any)
	basis, _ := got["estimateBasis"].(string)
	if !strings.HasPrefix(basis, "measured:") {
		t.Fatalf("estimate basis after one completed render: %q", basis)
	}
	// The fake worker reported 2.5 s for 3.0 s of audio.
	if !strings.Contains(basis, "0.833") {
		t.Fatalf("ratio should be the measured 2.5/3.0, got %q", basis)
	}
}

func TestDailyVoiceBudgetRefusesAfterTheCeilingIsSpent(t *testing.T) {
	// The per-batch ceiling bounds one request; this bounds a day. It is
	// measured rather than estimated because spend is what has happened.
	t.Setenv("VOICE_BATCH_GPU_BUDGET_SECONDS", "0")
	t.Setenv("VOICE_GPU_DAILY_BUDGET_SECONDS", "4")
	h := newVoicePlatformHarness(t)
	id := h.onboard(t)

	// One completed render costs 2.5 s of the 4 s day.
	h.must(t, 202, "POST", "/v1/voices/generate", map[string]any{"voiceId": id, "text": "Grace and peace.",
		"style": "reflection", "purpose": "reflection"}, h.admin)
	h.drain(t)

	long := "Blessed is the one who walks not in the counsel of the wicked, nor stands in the way of sinners. "
	items := []map[string]any{{"label": "a", "text": long}, {"label": "b", "text": long}}
	rec := h.do(t, "POST", "/v1/admin/voices/"+id+"/batches", map[string]any{
		"title": "Over day", "purpose": "reflection", "style": "reflection", "items": items}, h.admin)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d, want 422: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "daily ceiling") {
		t.Fatalf("refusal must name the daily ceiling: %s", rec.Body.String())
	}
	if calls := h.worker.calls.Load(); calls != 1 {
		t.Fatalf("the day's own render plus nothing else may reach the worker, got %d calls", calls)
	}
}
