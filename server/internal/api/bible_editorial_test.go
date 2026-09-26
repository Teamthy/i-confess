package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

func TestBibleEditorialContentRequiresHumanReview(t *testing.T) {
	conn := dbtest.New(t)
	inst := newCacheInstance(t, "Bible editorial", conn, nil)
	creator := adminTokenFor(t, inst, "editorial-creator@example.com")
	reviewer := adminTokenFor(t, inst, "editorial-reviewer@example.com")

	call := func(method, path, token, body string, want int) string {
		t.Helper()
		status, response := doRequest(t, inst.srv, method, path, token, body)
		if status != want {
			t.Fatalf("%s %s: got %d, want %d: %s", method, path, status, want, response)
		}
		return response
	}
	assertNoCrossReferences := func() {
		t.Helper()
		response := call("GET", "/bible/cross-references?reference=JHN.3.16", "", "", http.StatusOK)
		var out struct {
			References []string `json:"references"`
		}
		if err := json.Unmarshal([]byte(response), &out); err != nil || len(out.References) != 0 {
			t.Fatalf("unreviewed/withdrawn cross-references exposed: %s (%v)", response, err)
		}
	}
	assertNoCrossReferences() // seeded pointers are proposals, not sign-off
	if response := call("GET", "/bible/plans", "", "", http.StatusOK); strings.Contains(response, "bible-plan-rest-week") {
		t.Fatalf("seeded unreviewed plan published: %s", response)
	}
	call("GET", "/bible/plans/psalms-for-rest", "", "", http.StatusNotFound)

	body := `{"source_reference":"John 3:16","target_reference":"Matthew 6:9","editor_note":"Editorial pointer only"}`
	response := call("POST", "/admin/bible/cross-references", creator, body, http.StatusCreated)
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(response), &created); err != nil || created.ID < 1 {
		t.Fatalf("invalid proposal: %s (%v)", response, err)
	}
	path := fmt.Sprintf("/admin/bible/cross-references/%d", created.ID)
	review := `{"decision":"approve","evidence_url":"https://example.test/editorial/review","rationale":"Second editor checked both canonical passages."}`
	call("PATCH", path, creator, review, http.StatusNotFound) // cannot self-approve
	assertNoCrossReferences()
	call("PATCH", path, reviewer, review, http.StatusOK)
	if response := call("GET", "/bible/cross-references?reference=John+3%3A16", "", "", http.StatusOK); !strings.Contains(response, "Matthew 6:9") || strings.Contains(response, "Romans 5:8") {
		t.Fatalf("approved proposal not exposed, or unreviewed seed exposed: %s", response)
	}
	call("PATCH", path, creator, `{"decision":"withdraw","evidence_url":"https://example.test/editorial/withdrawal","rationale":"Editorial pointer withdrawn by proposer."}`, http.StatusOK)
	assertNoCrossReferences()

	call("PATCH", "/admin/bible/plans/bible-plan-rest-week", reviewer,
		`{"status":"published","rationale":"Readings checked independently against canonical references."}`, http.StatusOK)
	if response := call("GET", "/bible/plans", "", "", http.StatusOK); !strings.Contains(response, "bible-plan-rest-week") {
		t.Fatalf("human-reviewed plan absent: %s", response)
	}
}

func TestBibleVerseOfDayIsGatedUntilReviewed(t *testing.T) {
	conn := dbtest.New(t)
	inst := newCacheInstance(t, "Bible verse of day", conn, nil)
	admin := adminTokenFor(t, inst, "verse-reviewer@example.com")
	ctx := context.Background()
	// Test-only source fixture; no Bible wording is generated or shipped here.
	_, err := conn.ExecContext(ctx, `INSERT INTO bible_versions
		(id,name,abbrev,language,language_name,format,coverage,licence,attribution,blob_url,sha256,created_at,updated_at,status,api_exposure_allowed)
		VALUES ('fixture','Test fixture','TST','en','English','osis','full','test terms','','test://fixture','test-sha','2026-01-01','2026-01-01','active',true)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.ExecContext(ctx, `INSERT INTO bible_verses(version_id,book_id,book_name,testament,canonical_order,chapter,verse,text,created_at,updated_at)
		VALUES('fixture','Ps','Psalms','old',19,23,1,'Test-only fixture, not Scripture.','2026-01-01','2026-01-01')`)
	if err != nil {
		t.Fatal(err)
	}
	path := "/bible/verse-of-day?translation=fixture"
	if status, body := doRequest(t, inst.srv, http.MethodGet, path, "", ""); status != http.StatusServiceUnavailable || strings.Contains(body, "Test-only fixture") {
		t.Fatalf("unreviewed verse exposed: %d %s", status, body)
	}
	day := (time.Now().UTC().YearDay()-1)%7 + 1
	adminPath := fmt.Sprintf("/admin/bible/verse-of-day/%d", day)
	if status, _ := doRequest(t, inst.srv, http.MethodPut, adminPath, admin,
		`{"reference":"PSA.23.1-2","editor_note":"Test","evidence":"https://example.test/review"}`); status != http.StatusBadRequest {
		t.Fatalf("verse-of-day must reject ranges, got %d", status)
	}
	if status, body := doRequest(t, inst.srv, http.MethodPut, adminPath, admin,
		`{"reference":"PSA.23.1","editor_note":"Test editorial selection","evidence":"https://example.test/review"}`); status != http.StatusOK {
		t.Fatalf("review verse of day: %d %s", status, body)
	}
	if status, body := doRequest(t, inst.srv, http.MethodGet, path, "", ""); status != http.StatusOK || !strings.Contains(body, "PSA.23.1") {
		t.Fatalf("reviewed verse not exposed from fixture: %d %s", status, body)
	}
}
