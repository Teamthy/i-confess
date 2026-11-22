package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Teamthy/i-confess/internal/seed"
)

var testCatalog = []Category{
	{Slug: "healing", Name: "Healing"},
	{Slug: "peace", Name: "Peace"},
	{Slug: "rest", Name: "Rest"},
	{Slug: "hope", Name: "Hope"},
	{Slug: "overcoming-fear", Name: "Overcoming Fear"},
}

// The keyword parser used to emit "sleep" and "future", which are not
// categories, so the engine found nothing for them.
func TestKeywordTargetsAreCanonical(t *testing.T) {
	canonical := map[string]bool{}
	for _, s := range seed.CategorySlugsInCanonicalOrder() {
		canonical[s] = true
	}
	for kw, slug := range keywordToCategory {
		if !canonical[slug] {
			t.Errorf("keyword %q maps to %q, which is not a canonical category", kw, slug)
		}
	}
	if !canonical[defaultCategory] {
		t.Errorf("default category %q is not canonical", defaultCategory)
	}
}

func TestKeywordParserStaysInCatalogAndIsDeterministic(t *testing.T) {
	req := Request{Utterance: "I can't sleep, I'm afraid about surgery and my future"}
	first, err := KeywordParser{}.Parse(context.Background(), req, testCatalog)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"rest", "overcoming-fear", "healing"} // order of appearance, capped at 3
	if strings.Join(first.Categories, ",") != strings.Join(want, ",") {
		t.Fatalf("categories = %v, want %v", first.Categories, want)
	}
	for i := 0; i < 20; i++ {
		again, _ := KeywordParser{}.Parse(context.Background(), req, testCatalog)
		if strings.Join(again.Categories, ",") != strings.Join(first.Categories, ",") {
			t.Fatalf("non-deterministic result: %v vs %v", again.Categories, first.Categories)
		}
	}

	// A keyword whose category is not published is dropped, not passed on.
	got, _ := KeywordParser{}.Parse(context.Background(), Request{Utterance: "joy"}, testCatalog)
	if strings.Join(got.Categories, ",") != "peace" {
		t.Fatalf("unpublished category leaked: %v", got.Categories)
	}
}

func TestToEngineRequestUsesCategoryIDs(t *testing.T) {
	p := Parsed{Categories: []string{"healing"}, Duration: 600}
	er, err := p.ToEngineRequest(map[string]string{"healing": "cat-123"})
	if err != nil {
		t.Fatal(err)
	}
	if len(er.CategoryIDs) != 1 || er.CategoryIDs[0] != "cat-123" {
		t.Fatalf("CategoryIDs = %v, want [cat-123]", er.CategoryIDs)
	}
	if _, err := p.ToEngineRequest(map[string]string{}); err == nil {
		t.Fatal("unknown slug should be rejected")
	}
}

func TestNewParserRejectsUnusableProviders(t *testing.T) {
	if _, err := NewParser(Options{Provider: "openai"}); err == nil {
		t.Error("openai is not implemented and must not silently become keyword")
	}
	if _, err := NewParser(Options{Provider: "anthropic"}); err == nil {
		t.Error("anthropic without a key must be rejected")
	}
	if p, err := NewParser(Options{}); err != nil {
		t.Error(err)
	} else if _, ok := p.(KeywordParser); !ok {
		t.Errorf("default parser = %T, want KeywordParser", p)
	}
}

// fakeClaude serves one canned Messages API response and records the request.
func fakeClaude(t *testing.T, status int, stopReason, text string) (*httptest.Server, *map[string]any) {
	t.Helper()
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status != http.StatusOK {
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"boom"}}`))
			return
		}
		resp := map[string]any{
			"id": "msg_test", "type": "message", "role": "assistant", "model": DefaultModel,
			"content":     []map[string]any{{"type": "text", "text": text}},
			"stop_reason": stopReason,
			"usage":       map[string]any{"input_tokens": 10, "output_tokens": 10},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func TestClaudeParserConstrainsAndValidatesOutput(t *testing.T) {
	// The model names one real category, one invented one and a duplicate.
	srv, got := fakeClaude(t, http.StatusOK, "end_turn",
		`{"categories":["healing","made-up","healing","peace"],"duration_minutes":15,"reason":"Surgery worries."}`)
	p := NewClaudeParser(Options{APIKey: "test-key", BaseURL: srv.URL})

	parsed, err := p.Parse(context.Background(),
		Request{Utterance: "peace before my surgery, about 15 minutes", VoiceID: "voice-1"}, testCatalog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(parsed.Categories, ",") != "healing,peace" {
		t.Errorf("categories = %v, want [healing peace]", parsed.Categories)
	}
	if parsed.Duration != 900 {
		t.Errorf("duration = %d, want 900", parsed.Duration)
	}
	if parsed.VoiceID != "voice-1" || parsed.Parser != "claude" {
		t.Errorf("voice/parser = %q/%q", parsed.VoiceID, parsed.Parser)
	}

	// The request must pin the output to the catalogue's slugs.
	req := *got
	if req["model"] != DefaultModel {
		t.Errorf("model = %v", req["model"])
	}
	enc, _ := json.Marshal(req["output_config"])
	for _, c := range testCatalog {
		if !strings.Contains(string(enc), `"`+c.Slug+`"`) {
			t.Errorf("schema enum missing %q: %s", c.Slug, enc)
		}
	}
	if !strings.Contains(string(enc), `"json_schema"`) {
		t.Errorf("no structured output format: %s", enc)
	}
	var msgs strings.Builder
	enc2 := json.NewEncoder(&msgs)
	enc2.SetEscapeHTML(false)
	_ = enc2.Encode(req["messages"])
	if !strings.Contains(msgs.String(), "<utterance>") {
		t.Errorf("utterance not delimited: %s", msgs.String())
	}
}

func TestClaudeParserFailuresFallBackToKeyword(t *testing.T) {
	cases := map[string]*httptest.Server{}
	cases["refusal"], _ = fakeClaude(t, http.StatusOK, "refusal", "")
	cases["no valid categories"], _ = fakeClaude(t, http.StatusOK, "end_turn", `{"categories":["nope"],"duration_minutes":0,"reason":""}`)
	cases["malformed"], _ = fakeClaude(t, http.StatusOK, "end_turn", `not json`)
	cases["server error"], _ = fakeClaude(t, http.StatusBadRequest, "", "")

	for name, srv := range cases {
		t.Run(name, func(t *testing.T) {
			claude := NewClaudeParser(Options{APIKey: "test-key", BaseURL: srv.URL})
			if _, err := claude.Parse(context.Background(), Request{Utterance: "I am anxious"}, testCatalog); err == nil {
				t.Fatal("claude parser should report the failure")
			}
			fp := &FallbackParser{Primary: claude, Fallback: KeywordParser{}}
			parsed, err := fp.Parse(context.Background(), Request{Utterance: "I am anxious"}, testCatalog)
			if err != nil {
				t.Fatal(err)
			}
			if parsed.Parser != "keyword-fallback" || strings.Join(parsed.Categories, ",") != "peace" {
				t.Fatalf("fallback = %+v", parsed)
			}
		})
	}
}

// The keyword parser (the fallback whenever no model is configured) used to
// read only "30" or "long": "15 minutes" came back as the 10-minute default.
func TestKeywordParserReadsSpokenDurations(t *testing.T) {
	cases := map[string]int{
		"15 minutes on fear":                    900,
		"Fifteen minutes before a hard meeting": 900,
		"give me an hour of peace":              3600,
		"half an hour on hope":                  1800,
		"20m of faith":                          1200,
		"2 hrs of prayer":                       7200,
		"forty-five minutes on healing":         2700,
		"I am anxious about money":              DefaultDurationSeconds, // "a m" is not a length
		"Psalm 30 for my family":                DefaultDurationSeconds, // a bare number is not a length
		"something long on gratitude":           1800,
	}
	catalog := []Category{{Slug: "peace", Name: "Peace"}}
	for utterance, want := range cases {
		got, err := (KeywordParser{}).Parse(context.Background(), Request{Utterance: utterance}, catalog)
		if err != nil {
			t.Fatalf("%q: %v", utterance, err)
		}
		if got.Duration != want {
			t.Errorf("%q: duration %d, want %d", utterance, got.Duration, want)
		}
	}
}
