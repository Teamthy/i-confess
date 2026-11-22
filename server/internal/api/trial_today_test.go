package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/media"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/storage"
	"github.com/Teamthy/i-confess/internal/store"
)

// addPlayableCategory adds a published category with one confession that has
// audio for both fixture voices, so journey days that use it can be built.
func (f *audioFixture) addPlayableCategory(t *testing.T, name, slug string) {
	t.Helper()
	ctx := context.Background()
	content := store.NewContentStore(f.db)
	audio := store.NewAudioStore(f.db)
	cat := &models.Category{Name: name, Slug: slug, Status: "published"}
	if err := content.CreateCategory(ctx, cat); err != nil {
		t.Fatal(err)
	}
	for i, title := range []string{name + " One", name + " Two"} {
		c := &models.Confession{
			CategoryID: cat.ID, Title: title, Status: "published", Language: "en",
			ShortText: "I declare it.", MediumText: "I declare it and believe it.", LongText: "I declare it, believe it and live it.",
			Variants: []models.ConfessionVariant{{Label: "1m", DurationSeconds: 60, SortOrder: i}},
		}
		if err := content.CreateConfession(ctx, c); err != nil {
			t.Fatal(err)
		}
		variants, err := content.Variants(ctx, c.ID)
		if err != nil || len(variants) == 0 {
			t.Fatalf("variants: %v", err)
		}
		for _, vid := range []string{f.voiceStd, f.voicePrm} {
			key := storage.AudioKeyFor(c.ID, variants[0].ID, vid, "en", 1)
			if err := f.store.Upload(ctx, key, media.ToneBytes(2), nil); err != nil {
				t.Fatal(err)
			}
			if err := audio.UpsertAsset(ctx, &models.AudioAsset{
				ConfessionID: c.ID, VariantID: variants[0].ID, VoiceID: vid,
				URL: key, DurationSeconds: 60, Status: "ready",
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// setTrialDay moves a running trial's clock so that today is journey day n.
func (f *audioFixture) setTrialDay(t *testing.T, userID string, n int) {
	t.Helper()
	start := time.Now().UTC().Add(-time.Duration(n-1)*24*time.Hour - time.Hour)
	if _, err := f.db.ExecContext(context.Background(),
		`UPDATE trials SET started_at=?, expires_at=? WHERE user_id=?`,
		start.Format(time.RFC3339), start.Add(7*24*time.Hour).Format(time.RFC3339), userID); err != nil {
		t.Fatal(err)
	}
}

func sessionIDs(t *testing.T, body map[string]any) []string {
	t.Helper()
	raw, _ := body["sessions"].([]any)
	ids := make([]string, 0, len(raw))
	for _, s := range raw {
		sess := s.(map[string]any)["session"].(map[string]any)
		ids = append(ids, sess["id"].(string))
	}
	return ids
}

// TestTrialTodayBuildsWhatEachDayDeclares is the G-48 regression test: the
// journey's flags used to be advertised and never acted on.
func TestTrialTodayBuildsWhatEachDayDeclares(t *testing.T) {
	f := newAudioFixture(t)
	for _, c := range [][2]string{{"Peace", "peace"}, {"Gratitude", "gratitude"}, {"Faith", "faith"}, {"Purpose", "purpose"}, {"Hope", "hope"}} {
		f.addPlayableCategory(t, c[0], c[1])
	}
	token, userID := f.registerWithID(t, "journey@example.com")

	// No trial: nothing to start.
	status, body := f.call(t, http.MethodPost, "/subscriptions/trial/today", token, nil)
	if status != http.StatusConflict || body["code"] != "TRIAL_NOT_ACTIVE" {
		t.Fatalf("before trial: %d %v", status, body)
	}
	if status, body := f.call(t, http.MethodPost, "/subscriptions/trial", token, nil); status != http.StatusOK {
		t.Fatalf("start trial: %d %v", status, body)
	}

	// Day 1: one session, built once; a retry returns the same one.
	status, body = f.call(t, http.MethodPost, "/subscriptions/trial/today", token, nil)
	if status != http.StatusOK || body["action"] != "play" || body["day"].(float64) != 1 {
		t.Fatalf("day 1: %d %v", status, body)
	}
	first := sessionIDs(t, body)
	if len(first) != 1 {
		t.Fatalf("day 1 built %d sessions, want 1", len(first))
	}
	_, again := f.call(t, http.MethodPost, "/v1/subscriptions/trial/today", token, nil)
	if ids := sessionIDs(t, again); len(ids) != 1 || ids[0] != first[0] {
		t.Fatalf("retry built a new session: %v vs %v", ids, first)
	}

	// Day 2: two sessions, morning and evening.
	f.setTrialDay(t, userID, 2)
	status, body = f.call(t, http.MethodPost, "/subscriptions/trial/today", token, nil)
	if status != http.StatusOK {
		t.Fatalf("day 2: %d %v", status, body)
	}
	two := body["sessions"].([]any)
	if len(two) != 2 || two[0].(map[string]any)["label"] != "Morning" || two[1].(map[string]any)["label"] != "Evening" {
		t.Fatalf("day 2 sessions = %v", two)
	}

	// Day 5: a Premium voice is selected.
	f.setTrialDay(t, userID, 5)
	status, body = f.call(t, http.MethodPost, "/subscriptions/trial/today", token, nil)
	if status != http.StatusOK || body["premium_voice_available"] != true {
		t.Fatalf("day 5: %d %v", status, body)
	}
	sess := body["sessions"].([]any)[0].(map[string]any)["session"].(map[string]any)
	if sess["voice_id"] != f.voicePrm {
		t.Fatalf("day 5 voice = %v, want the Premium voice %s", sess["voice_id"], f.voicePrm)
	}

	// Day 6: the builder, prefilled; nothing is built.
	f.setTrialDay(t, userID, 6)
	status, body = f.call(t, http.MethodPost, "/subscriptions/trial/today", token, nil)
	if status != http.StatusOK || body["action"] != "open_builder" || len(body["sessions"].([]any)) != 0 {
		t.Fatalf("day 6: %d %v", status, body)
	}
	if ids := body["builder"].(map[string]any)["category_ids"].([]any); len(ids) != 2 {
		t.Fatalf("day 6 builder categories = %v", ids)
	}
}
