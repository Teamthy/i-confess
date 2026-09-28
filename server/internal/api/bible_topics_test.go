package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

// Topics are derived from reviewed content rather than authored twice, so the
// tests that matter are about what the derivation refuses to do: publish a
// topic from unpublished content, and publish a citation the canon does not
// contain.

func seedTopicCitation(t *testing.T, conn *db.DB, slug, name, confessionID, status, book string, chapter int, verse string) {
	t.Helper()
	ctx := context.Background()
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO categories (id,name,slug,description,icon,premium,status,sort_order,created_at,updated_at)
		 VALUES (?,?,?,'Topic seed','i',0,'published',1,'2026-01-01','2026-01-01')
		 ON CONFLICT (id) DO NOTHING`,
		"cat-"+slug, name, slug); err != nil {
		t.Fatalf("seed category: %v", err)
	}
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO confessions (id,category_id,title,short_text,intensity,language,status,author,version,created_at,updated_at)
		 VALUES (?,?,?,'short',1,'en',?,'test',1,'2026-01-01','2026-01-01')
		 ON CONFLICT (id) DO NOTHING`,
		confessionID, "cat-"+slug, "Confession "+confessionID, status); err != nil {
		t.Fatalf("seed confession: %v", err)
	}
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO scripture_references (id,confession_id,book,chapter,verse,translation,is_direct_quote,notes,sort_order)
		 VALUES (?,?,?,?,?,'KJV',1,'',0)
		 ON CONFLICT (id) DO NOTHING`,
		"scr-"+confessionID+"-"+book, confessionID, book, chapter, verse); err != nil {
		t.Fatalf("seed citation: %v", err)
	}
}

func TestBibleTopicsAreBuiltFromReviewedCitationsOnly(t *testing.T) {
	conn := dbtest.New(t)
	inst := newCacheInstance(t, "Bible topics", conn, nil)

	// Two published confessions citing the same verse, one citing another,
	// and a draft whose citation must not reach the topic.
	seedTopicCitation(t, conn, "trust", "Trust", "conf-topic-1", "published", "Prov", 3, "5")
	seedTopicCitation(t, conn, "trust", "Trust", "conf-topic-2", "published", "Prov", 3, "5")
	seedTopicCitation(t, conn, "trust", "Trust", "conf-topic-3", "published", "Rom", 8, "28")
	seedTopicCitation(t, conn, "trust", "Trust", "conf-topic-draft", "draft", "Ps", 23, "1")

	status, body := doRequest(t, inst.srv, "GET", "/v1/bible/topics", "", "")
	if status != http.StatusOK {
		t.Fatalf("GET /v1/bible/topics: got %d, want 200: %s", status, body)
	}
	var list struct {
		Topics []struct {
			Slug         string `json:"slug"`
			Name         string `json:"name"`
			PassageCount int    `json:"passage_count"`
		} `json:"topics"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatalf("decode topics: %v", err)
	}
	var found bool
	for _, topic := range list.Topics {
		if topic.Slug == "trust" {
			found = true
			if topic.PassageCount != 2 {
				t.Errorf("topic carries %d passages, want 2 (the draft's citation must not count)", topic.PassageCount)
			}
		}
	}
	if !found {
		t.Fatalf("the seeded topic is missing from %s", body)
	}

	status, body = doRequest(t, inst.srv, "GET", "/v1/bible/topics/trust", "", "")
	if status != http.StatusOK {
		t.Fatalf("GET topic: got %d, want 200: %s", status, body)
	}
	var detail struct {
		Topic struct {
			Slug     string `json:"slug"`
			Passages []struct {
				Reference       string `json:"reference"`
				BookID          string `json:"book_id"`
				CanonicalID     string `json:"canonical_id"`
				ConfessionCount int    `json:"confession_count"`
			} `json:"passages"`
		} `json:"topic"`
	}
	if err := json.Unmarshal([]byte(body), &detail); err != nil {
		t.Fatalf("decode topic: %v", err)
	}
	if len(detail.Topic.Passages) != 2 {
		t.Fatalf("topic detail carries %d passages, want 2: %s", len(detail.Topic.Passages), body)
	}
	// Most-cited first, and spelled the way the rest of the platform spells it.
	first := detail.Topic.Passages[0]
	if first.Reference != "Proverbs 3:5" || first.ConfessionCount != 2 {
		t.Errorf("leading passage is %q cited %dx, want Proverbs 3:5 cited 2x", first.Reference, first.ConfessionCount)
	}
	if first.CanonicalID != "PRO.3.5" {
		t.Errorf("canonical ID is %q, want PRO.3.5", first.CanonicalID)
	}
	// A topic nobody has reviewed content for is absent, not empty.
	if status, _ := doRequest(t, inst.srv, "GET", "/v1/bible/topics/not-a-topic", "", ""); status != http.StatusNotFound {
		t.Errorf("unknown topic: got %d, want 404", status)
	}
}

func TestBibleTopicsDropCitationsTheCanonDoesNotContain(t *testing.T) {
	conn := dbtest.New(t)
	inst := newCacheInstance(t, "Bible topics canon", conn, nil)

	// John has 21 chapters; a citation to John 99 is corrupt and must never
	// become a link a reader can follow into nothing.
	seedTopicCitation(t, conn, "canon-guard", "Canon guard", "conf-bad", "published", "John", 99, "1")
	seedTopicCitation(t, conn, "canon-guard", "Canon guard", "conf-good", "published", "John", 3, "16")

	status, body := doRequest(t, inst.srv, "GET", "/v1/bible/topics/canon-guard", "", "")
	if status != http.StatusOK {
		t.Fatalf("GET topic: got %d, want 200: %s", status, body)
	}
	var detail struct {
		Topic struct {
			Passages []struct {
				Reference string `json:"reference"`
			} `json:"passages"`
		} `json:"topic"`
	}
	if err := json.Unmarshal([]byte(body), &detail); err != nil {
		t.Fatalf("decode topic: %v", err)
	}
	if len(detail.Topic.Passages) != 1 || detail.Topic.Passages[0].Reference != "John 3:16" {
		t.Fatalf("topic kept a non-canonical citation: %s", body)
	}
}

func TestBibleRandomRequiresAnApprovedTranslation(t *testing.T) {
	conn := dbtest.New(t)
	inst := newCacheInstance(t, "Bible random", conn, nil)
	seedTopicCitation(t, conn, "random-pool", "Random pool", "conf-random", "published", "Ps", 23, "1")

	// No translation named: the reader is asked for one rather than being
	// given an arbitrary edition.
	status, body := doRequest(t, inst.srv, "GET", "/v1/bible/random", "", "")
	if status != http.StatusBadRequest {
		t.Fatalf("no translation: got %d, want 400: %s", status, body)
	}
	// An edition nobody approved cannot be read, randomly or otherwise.
	status, body = doRequest(t, inst.srv, "GET", "/v1/bible/random?translation=not-approved", "", "")
	if status != http.StatusNotFound {
		t.Fatalf("unapproved translation: got %d, want 404: %s", status, body)
	}
}
