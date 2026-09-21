package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/Teamthy/i-confess/internal/analytics"
	"github.com/Teamthy/i-confess/internal/store"
)

// completeASession drives a session through real playback to COMPLETED, which
// is the only path that can complete a trial journey day.
func (f *audioFixture) completeASession(t *testing.T, token, voiceID string, seconds int) string {
	t.Helper()
	code, sess := f.createSession(t, token, voiceID, seconds)
	if code != http.StatusCreated || sess.ID == "" {
		t.Fatalf("create session: code %d", code)
	}
	if code, body := f.call(t, http.MethodPost, "/sessions/"+sess.ID+"/start", token, nil); code != http.StatusOK {
		t.Fatalf("start session: %d %v", code, body)
	}
	if code, body := f.call(t, http.MethodPost, "/sessions/"+sess.ID+"/complete", token, nil); code != http.StatusOK {
		t.Fatalf("complete session: %d %v", code, body)
	}
	return sess.ID
}

// TestTrialEngagementMeasuresRealCompletions is the API-level proof of the
// phase: the engagement endpoint reports days a listener actually finished, and
// the completion that produces them came through the session lifecycle rather
// than through a request body.
func TestTrialEngagementMeasuresRealCompletions(t *testing.T) {
	f := newAudioFixture(t)
	token, _ := f.registerWithID(t, "trial-engagement-api@example.com")

	if status, body := f.call(t, http.MethodPost, "/subscriptions/trial", token, nil); status != http.StatusOK {
		t.Fatalf("start trial: %d %v", status, body)
	}

	// Nothing completed yet. The clock says day one; the measurement says zero.
	status, body := f.call(t, http.MethodGet, "/subscriptions/trial/engagement", token, nil)
	if status != http.StatusOK {
		t.Fatalf("engagement: %d %v", status, body)
	}
	if body["days_completed"] != float64(0) {
		t.Fatalf("days_completed = %v, want 0 before any session finished", body["days_completed"])
	}
	if body["days_total"] != float64(7) {
		t.Fatalf("days_total = %v, want 7", body["days_total"])
	}
	if body["completion_rate"] != float64(0) {
		t.Fatalf("completion_rate = %v, want 0", body["completion_rate"])
	}
	funnel, _ := body["funnel"].(map[string]any)
	if funnel[analytics.EventTrialStarted] != float64(1) {
		t.Fatalf("funnel trial_started = %v, want 1 (funnel=%v)", funnel[analytics.EventTrialStarted], funnel)
	}

	f.completeASession(t, token, f.voiceStd, 120)

	status, body = f.call(t, http.MethodGet, "/v1/subscriptions/trial/engagement", token, nil)
	if status != http.StatusOK {
		t.Fatalf("engagement after completion: %d %v", status, body)
	}
	if body["days_completed"] != float64(1) {
		t.Fatalf("days_completed = %v, want 1 after a real completion", body["days_completed"])
	}
	if body["completion_rate"] != 1.0/7.0 {
		t.Fatalf("completion_rate = %v, want %v", body["completion_rate"], 1.0/7.0)
	}
	funnel, _ = body["funnel"].(map[string]any)
	if funnel[analytics.EventTrialDayCompleted] != float64(1) {
		t.Fatalf("funnel trial_day_completed = %v, want 1 (funnel=%v)", funnel[analytics.EventTrialDayCompleted], funnel)
	}
	completed, _ := body["completed_days"].([]any)
	if len(completed) != 1 || completed[0] != float64(1) {
		t.Fatalf("completed_days = %v, want [1]", body["completed_days"])
	}

	// A second session on the same day is a repeat, not a second day. This is
	// what keeps the completion rate a rate.
	f.completeASession(t, token, f.voiceStd, 120)
	_, body = f.call(t, http.MethodGet, "/subscriptions/trial/engagement", token, nil)
	if body["days_completed"] != float64(1) {
		t.Fatalf("days_completed after two same-day completions = %v, want 1", body["days_completed"])
	}
	if funnel, _ := body["funnel"].(map[string]any); funnel[analytics.EventTrialDayCompleted] != float64(1) {
		t.Fatalf("trial_day_completed recorded twice for one day: funnel=%v", funnel)
	}
}

// TestTrialDayCompletionCannotBeAssertedByAClient closes the obvious hole. The
// batch endpoint accepts a fixed allowlist; day completion is not on it, so an
// app cannot report a day it did not listen through.
func TestTrialDayCompletionCannotBeAssertedByAClient(t *testing.T) {
	f := newAudioFixture(t)
	token, userID := f.registerWithID(t, "trial-assert-api@example.com")

	status, body := f.call(t, http.MethodPost, "/analytics/batch", token, map[string]any{
		"events": []map[string]any{
			{"name": analytics.EventTrialDayCompleted, "props": map[string]any{"day": 7}},
		},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("posting a trial_day_completed event = %d %v, want 400", status, body)
	}

	// And the attempt wrote nothing.
	n, err := store.NewAnalyticsStore(f.db).Count(context.Background(), userID, analytics.EventTrialDayCompleted)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("a rejected client event was persisted %d times", n)
	}
}

// TestAnalyticsBatchPersistsWhatItAcknowledges is the defect this phase removes.
// The endpoint answered 202 and handed the batch to a no-op sink, so a client
// was told its event was accepted and the server then had no record of it — an
// acknowledgement for data the system did not hold.
func TestAnalyticsBatchPersistsWhatItAcknowledges(t *testing.T) {
	f := newAudioFixture(t)
	token, userID := f.registerWithID(t, "analytics-persist-api@example.com")

	status, body := f.call(t, http.MethodPost, "/analytics/batch", token, map[string]any{
		"events": []map[string]any{
			{"name": analytics.EventSearchPerformed, "props": map[string]any{"kind": "confession"}},
			{"name": analytics.EventCategoryViewed},
		},
	})
	if status != http.StatusAccepted {
		t.Fatalf("batch: %d %v", status, body)
	}
	if body["accepted"] != float64(2) {
		t.Fatalf("accepted = %v, want 2", body["accepted"])
	}

	events, err := store.NewAnalyticsStore(f.db).Recent(context.Background(), userID, 10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("persisted %d events, want the 2 that were acknowledged", len(events))
	}
	if events[0].UserID == "" {
		t.Error("the persisted event has no user id; it must be stamped from the session")
	}
}

// TestTrialJourneyResolvesDayThreeFromInterests proves the personalization day
// is actually personal. Day 3 advertises "built from what you told us you
// carry"; shipping it with a fixed category would make the copy false.
func TestTrialJourneyResolvesDayThreeFromInterests(t *testing.T) {
	f := newAudioFixture(t)
	token, _ := f.registerWithID(t, "trial-personalize-api@example.com")

	// Before any interests, day three falls back to the stored journey so the
	// day is never empty.
	status, body := f.call(t, http.MethodGet, "/subscriptions/trial", token, nil)
	if status != http.StatusOK {
		t.Fatalf("trial: %d %v", status, body)
	}
	journey, _ := body["journey"].([]any)
	if len(journey) != 7 {
		t.Fatalf("journey has %d days, want 7", len(journey))
	}
	day3, _ := journey[2].(map[string]any)
	if day3["intent"] != "personalization" {
		t.Fatalf("day 3 intent = %v, want personalization", day3["intent"])
	}
	if day3["personalized"] != true {
		t.Fatalf("day 3 is not flagged personalized: %v", day3)
	}
	fallback, _ := day3["categories"].([]any)
	if len(fallback) == 0 {
		t.Fatal("day 3 has no fallback categories, so the day could not build a session")
	}

	// The listener states an interest in the fixture's category.
	if status, body := f.call(t, http.MethodPut, "/me/interests", token, map[string]any{
		"category_ids": []string{f.catID},
	}); status != http.StatusOK {
		t.Fatalf("put interests: %d %v", status, body)
	}

	_, body = f.call(t, http.MethodGet, "/subscriptions/trial", token, nil)
	journey, _ = body["journey"].([]any)
	day3, _ = journey[2].(map[string]any)
	categories, _ := day3["categories"].([]any)
	if len(categories) != 1 || categories[0] != "healing" {
		t.Fatalf("day 3 categories = %v, want the listener's own [healing]", day3["categories"])
	}

	// Only day three varies. Every other day must stay the deterministic
	// catalogue journey, or the week is no longer testable or comparable.
	for i, raw := range journey {
		if i == 2 {
			continue
		}
		day, _ := raw.(map[string]any)
		if day["personalized"] == true {
			t.Errorf("day %d is flagged personalized; only day 3 may vary per account", i+1)
		}
	}
}
