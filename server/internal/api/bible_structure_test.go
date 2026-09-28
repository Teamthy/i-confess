package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Teamthy/i-confess/internal/bible"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

// The structure endpoint is the one Bible read that must answer before, and
// without, a catalogue: a reader draws its book list, chapter grid and verse
// bounds from it, and validates a typed reference against it. So the test
// that matters is that it answers correctly with no translation imported at
// all, and still refuses a translation it has not approved.

func TestBibleStructureServesTheCanonWithoutACatalogue(t *testing.T) {
	conn := dbtest.New(t)
	inst := newCacheInstance(t, "Bible structure", conn, nil)

	status, body := doRequest(t, inst.srv, "GET", "/v1/bible/structure", "", "")
	if status != http.StatusOK {
		t.Fatalf("GET /v1/bible/structure: got %d, want 200: %s", status, body)
	}

	var got bible.Structure
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode structure: %v", err)
	}
	if got.BookCount != 66 || got.ChapterCount != 1189 || got.VerseCount != 31102 {
		t.Fatalf("structure reports %d books / %d chapters / %d verses, want 66 / 1189 / 31102",
			got.BookCount, got.ChapterCount, got.VerseCount)
	}
	if len(got.Books) != 66 || len(got.Testaments) != 2 {
		t.Fatalf("structure carries %d books and %d testaments", len(got.Books), len(got.Testaments))
	}
	// The navigation data a client cannot draw a reader without.
	first := got.Books[0]
	if first.ID != "Gen" || first.USFM != "GEN" || first.Section != bible.SectionLaw || first.ChapterCount != 50 {
		t.Fatalf("first book is %+v, want Genesis in the Law with 50 chapters", first)
	}
	if len(first.Chapters) != 50 || first.Chapters[0] != 31 {
		t.Fatalf("Genesis chapter distribution is wrong: %v", first.Chapters)
	}
	// The alias table has to be able to resolve what a reader types.
	for spelling, want := range map[string]string{"john": "John", "1 cor": "1Cor", "psalm": "Ps", "jhn": "John"} {
		if got.Aliases[spelling] != want {
			t.Errorf("alias %q resolved to %q, want %s", spelling, got.Aliases[spelling], want)
		}
	}

	// The canon does not change, so the response must be cacheable; a reader
	// that refetches it on every navigation would spend a request on a
	// constant.
	res, err := http.Get(inst.srv.URL + "/v1/bible/structure")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if cc := res.Header.Get("Cache-Control"); cc == "" || cc == "no-store" {
		t.Errorf("Cache-Control is %q, want a public cache directive", cc)
	}
}

func TestBibleStructureRefusesAnUnapprovedTranslation(t *testing.T) {
	conn := dbtest.New(t)
	inst := newCacheInstance(t, "Bible structure rights", conn, nil)

	// An edition nobody has approved must not be described, and must not
	// produce a 500 either: it is simply not available.
	status, body := doRequest(t, inst.srv, "GET", "/v1/bible/structure?translation=not-approved", "", "")
	if status != http.StatusNotFound {
		t.Fatalf("unapproved translation: got %d, want 404: %s", status, body)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if payload["code"] != "BIBLE_NOT_FOUND" {
		t.Errorf("error code is %q, want BIBLE_NOT_FOUND", payload["code"])
	}
	if payload["error"] == "" {
		t.Error("the error body carries no reader-facing sentence")
	}
}
