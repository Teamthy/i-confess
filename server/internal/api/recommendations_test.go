package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/personalization"
	"github.com/Teamthy/i-confess/internal/store"
)

func stringsOf(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func firstCategoryID(body map[string]any) string {
	cats, _ := body["categories"].([]any)
	if len(cats) == 0 {
		return ""
	}
	first, _ := cats[0].(map[string]any)
	id, _ := first["id"].(string)
	return id
}

// TestRecommendationsRankOnWhatTheListenerDid is the API-level proof of PHASE
// 43. A listener who has done nothing gets the catalogue and no claim of
// personalization; one who has finished sessions, come back to the same
// confession, skipped, and favourited gets a ranking that says so, with the
// seven signals reported as quantities.
func TestRecommendationsRankOnWhatTheListenerDid(t *testing.T) {
	f := newAudioFixture(t)
	ctx := context.Background()
	content := store.NewContentStore(f.db)

	// A second category, catalogued before Healing, with nothing listened to.
	// Without evidence it must lead; with evidence Healing must overtake it.
	peace := &models.Category{Name: "Peace", Slug: "peace", Status: "published", SortOrder: -10}
	if err := content.CreateCategory(ctx, peace); err != nil {
		t.Fatal(err)
	}
	quiet := &models.Confession{CategoryID: peace.ID, Title: "Quiet", Status: "published", Language: "en",
		ShortText: "Peace.", MediumText: "I have peace.", LongText: "I have the peace that passes understanding.",
		Variants: []models.ConfessionVariant{{Label: "1m", DurationSeconds: 60}}}
	if err := content.CreateConfession(ctx, quiet); err != nil {
		t.Fatal(err)
	}

	token, _ := f.registerWithID(t, "listener@example.com")

	status, body := f.call(t, http.MethodGet, "/recommendations", token, nil)
	if status != http.StatusOK {
		t.Fatalf("recommendations: %d %v", status, body)
	}
	if body["personalized"] != false {
		t.Errorf("no history must not claim personalization: %v", body["personalized"])
	}
	if got := firstCategoryID(body); got != peace.ID {
		t.Errorf("with no evidence the catalogue order leads; first=%s want Peace %s", got, peace.ID)
	}
	if body["suggested_duration_seconds"] != float64(0) {
		t.Errorf("no completed session must leave suggested duration 0: %v", body["suggested_duration_seconds"])
	}
	signals, _ := body["signals"].(map[string]any)
	if got := stringsOf(signals["present"]); len(got) != 0 {
		t.Errorf("present signals with no history = %v, want none", got)
	}

	// Three real completions in Healing. completeSession marks the playing
	// item completed, and the engine builds the same queue for the same
	// request, so the same confession is finished three times: a repeat.
	for i := 0; i < 3; i++ {
		f.completeASession(t, token, f.voiceStd, 120)
	}
	// One skip, recorded through the playback endpoint.
	_, sess := f.createSession(t, token, f.voiceStd, 120)
	if code, out := f.call(t, http.MethodPost, "/sessions/"+sess.ID+"/start", token, nil); code != http.StatusOK {
		t.Fatalf("start: %d %v", code, out)
	}
	skipped := f.queueItemID(t, sess.ID)
	if code, out := f.call(t, http.MethodPost, "/sessions/"+sess.ID+"/skip", token, map[string]any{"item_id": skipped}); code != http.StatusOK {
		t.Fatalf("skip: %d %v", code, out)
	}
	// A favourite in the other category, so both signals are visible at once.
	if code, out := f.call(t, http.MethodPost, "/me/favorites", token, map[string]any{
		"entity_type": "confession", "entity_id": quiet.ID}); code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("favourite: %d %v", code, out)
	}

	status, body = f.call(t, http.MethodGet, "/v1/recommendations", token, nil)
	if status != http.StatusOK {
		t.Fatalf("recommendations after listening: %d %v", status, body)
	}
	if body["personalized"] != true {
		t.Errorf("behavioural evidence must report personalized=true")
	}
	if got := firstCategoryID(body); got != f.catID {
		t.Errorf("Healing, listened to three times, must lead; first=%s", got)
	}
	signals, _ = body["signals"].(map[string]any)
	present := stringsOf(signals["present"])
	for _, want := range []personalization.Signal{
		personalization.SignalCategories, personalization.SignalCompletionRate, personalization.SignalTimeOfDay,
		personalization.SignalDuration, personalization.SignalFavourites, personalization.SignalSkips,
		personalization.SignalRepeats,
	} {
		if !contains(present, string(want)) {
			t.Errorf("signal %s should be present after real listening; present=%v", want, present)
		}
	}
	if signals["repeat_listening"] != float64(1) {
		t.Errorf("repeat_listening = %v, want 1 confession repeated (a count, not a flag)", signals["repeat_listening"])
	}
	if signals["skips"] != float64(1) {
		t.Errorf("skips = %v, want 1", signals["skips"])
	}
	if signals["completion_sample"] != float64(4) {
		t.Errorf("completion_sample = %v, want 4 (three completions and one skip)", signals["completion_sample"])
	}
	if signals["completion_rate"] != 0.75 {
		t.Errorf("completion_rate = %v, want 0.75", signals["completion_rate"])
	}
	again, _ := body["listen_again"].([]any)
	if len(again) != 1 {
		t.Fatalf("listen_again = %v, want the one repeated confession", body["listen_again"])
	}
	if first, _ := again[0].(map[string]any); first["times"] != float64(3) {
		t.Errorf("listen_again times = %v, want 3 separate sessions", first["times"])
	}
	// Typical session is 120s → nearest rung 10m; three of four items
	// finished is a 0.75 rate, which moves nothing, so 10m stays.
	if body["suggested_duration_seconds"] != float64(600) {
		t.Errorf("suggested_duration_seconds = %v, want 600", body["suggested_duration_seconds"])
	}
	reasons, _ := body["reasons"].(map[string]any)
	catReasons, _ := reasons["categories"].(map[string]any)
	healing := stringsOf(catReasons[f.catID])
	if !contains(healing, "listened 3") || !contains(healing, "repeated 3") || !contains(healing, "skipped 1") {
		t.Errorf("Healing reasons = %v, want listened 3, repeated 3 and skipped 1", healing)
	}
	confReasons, _ := reasons["confessions"].(map[string]any)
	if !contains(stringsOf(confReasons[quiet.ID]), "favourite") {
		t.Errorf("the favourited confession should carry a favourite reason: %v", confReasons[quiet.ID])
	}

	// The listener's own switch. Turning personalization off must stop the
	// behavioural signals being read at all, not merely hide them.
	if code, out := f.call(t, http.MethodPatch, "/me/preferences", token, map[string]any{"personalization_enabled": false}); code != http.StatusOK {
		t.Fatalf("preferences: %d %v", code, out)
	}
	_, body = f.call(t, http.MethodGet, "/recommendations", token, nil)
	if body["personalized"] != false {
		t.Errorf("personalization_enabled=false must report personalized=false")
	}
	signals, _ = body["signals"].(map[string]any)
	if got := stringsOf(signals["present"]); len(got) != 0 {
		t.Errorf("personalization off but signals present: %v", got)
	}
	if got := firstCategoryID(body); got != peace.ID {
		t.Errorf("with behavioural signals off the catalogue order leads again; first=%s", got)
	}
	prefs, _ := body["preferences"].(map[string]any)
	if prefs["personalization_enabled"] != false {
		t.Errorf("the response must say which switch is off: %v", prefs)
	}
}

// TestRecommendationsHonourTheRecommendationsSwitch: with recommendations off
// the endpoint still answers — a client renders whatever it gets — but ranks
// nothing and infers nothing.
func TestRecommendationsHonourTheRecommendationsSwitch(t *testing.T) {
	f := newAudioFixture(t)
	token, _ := f.registerWithID(t, "no-recs@example.com")
	if code, out := f.call(t, http.MethodPut, "/me/interests", token, map[string]any{"category_ids": []string{f.catID}}); code != http.StatusOK {
		t.Fatalf("interests: %d %v", code, out)
	}
	_, body := f.call(t, http.MethodGet, "/recommendations", token, nil)
	reasons, _ := body["reasons"].(map[string]any)
	if cats, _ := reasons["categories"].(map[string]any); len(cats) == 0 {
		t.Fatalf("an explicit interest should be a reason while recommendations are on: %v", body["reasons"])
	}
	if code, out := f.call(t, http.MethodPatch, "/me/preferences", token, map[string]any{"recommendations_enabled": false}); code != http.StatusOK {
		t.Fatalf("preferences: %d %v", code, out)
	}
	status, body := f.call(t, http.MethodGet, "/recommendations", token, nil)
	if status != http.StatusOK {
		t.Fatalf("recommendations off must still answer 200: %d", status)
	}
	reasons, _ = body["reasons"].(map[string]any)
	if cats, _ := reasons["categories"].(map[string]any); len(cats) != 0 {
		t.Errorf("recommendations off must infer nothing, got reasons %v", cats)
	}
	if body["personalized"] != false {
		t.Errorf("recommendations off must not claim personalization")
	}
}
