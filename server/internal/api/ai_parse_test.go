package api

import (
	"net/http"
	"testing"
)

// TestAIParseReturnsRealCategoryIDs is the regression test for an engine
// request that carried category slugs ("healing") where the engine selects by
// database id, so no session built from it could ever find content.
func TestAIParseReturnsRealCategoryIDs(t *testing.T) {
	f := newAudioFixture(t)

	status, body := f.call(t, http.MethodPost, "/ai/parse", f.freeTok,
		map[string]any{"utterance": "I need healing before surgery", "voice_id": f.voiceStd})
	if status != http.StatusOK {
		t.Fatalf("parse: %d %v", status, body)
	}
	parsed := body["parsed"].(map[string]any)
	if cats := parsed["categories"].([]any); len(cats) != 1 || cats[0] != "healing" {
		t.Fatalf("parsed categories = %v, want [healing]", cats)
	}
	if parsed["voice_id"] != f.voiceStd {
		t.Errorf("voice_id = %v, want the caller's", parsed["voice_id"])
	}
	eng := body["engine_request"].(map[string]any)
	if ids := eng["CategoryIDs"].([]any); len(ids) != 1 || ids[0] != f.catID {
		t.Fatalf("engine CategoryIDs = %v, want [%s]", ids, f.catID)
	}

	// Explicit categories may be given by id or slug; unknown ones are refused.
	status, body = f.call(t, http.MethodPost, "/v1/ai/parse", f.freeTok,
		map[string]any{"categories": []string{f.catID}, "duration_seconds": 300})
	if status != http.StatusOK {
		t.Fatalf("explicit id: %d %v", status, body)
	}
	status, _ = f.call(t, http.MethodPost, "/ai/parse", f.freeTok,
		map[string]any{"categories": []string{"no-such-category"}})
	if status != http.StatusBadRequest {
		t.Fatalf("unknown category: status %d, want 400", status)
	}
}
