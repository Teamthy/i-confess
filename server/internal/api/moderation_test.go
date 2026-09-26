package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

// moderationFixture is a live server with a signed-in user, a signed-in
// super-admin and the database the handlers are reading.
type moderationFixture struct {
	t      *testing.T
	srv    *httptest.Server
	conn   *db.DB
	h      *Handler
	user   string
	admin  string
	userID string
}

func newModerationFixture(t *testing.T) *moderationFixture {
	t.Helper()
	conn := dbtest.New(t)
	t.Cleanup(func() { conn.Close() })

	h := NewHandler(Config{JWTSecret: "test-secret-value", TokenTTL: "24h"}, conn)
	h.BuildEngine()
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	f := &moderationFixture{t: t, srv: srv, conn: conn, h: h}
	registerAndSignIn(t, srv, "mod-user@example.com", "a-strong-enough-passphrase")
	registerAndSignIn(t, srv, "mod-admin@example.com", "a-strong-enough-passphrase")

	f.userID = userIDForEmail(t, h, "mod-user@example.com")
	adminID := userIDForEmail(t, h, "mod-admin@example.com")
	if err := h.users.SetAdminRole(context.Background(), adminID, auth.RoleSuperAdmin); err != nil {
		t.Fatalf("promote admin: %v", err)
	}
	// Roles are read at issue time, so sign in again after promotion.
	f.user = signIn(t, srv, "mod-user@example.com", "a-strong-enough-passphrase")
	f.admin = signIn(t, srv, "mod-admin@example.com", "a-strong-enough-passphrase")
	return f
}

func (f *moderationFixture) seedPublishedConfession(id string) {
	f.t.Helper()
	if _, err := f.conn.ExecContext(context.Background(),
		`INSERT INTO categories (id,name,slug,description,icon,premium,status,sort_order,created_at,updated_at)
		 VALUES ('apicat','API','api','d','i',0,'published',1,'2026-01-01','2026-01-01')
		 ON CONFLICT (id) DO NOTHING`); err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.conn.ExecContext(context.Background(),
		`INSERT INTO confessions (id,category_id,title,short_text,intensity,language,status,author,version,published_at,created_at,updated_at)
		 VALUES (?,'apicat','t','text',1,'en','published','test',1,'2026-02-01','2026-01-01','2026-01-01')`, id); err != nil {
		f.t.Fatalf("insert confession %s: %v", id, err)
	}
}

func bodyJSON(t *testing.T, body string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("response is not JSON: %v (%s)", err, truncateBody(body))
	}
	return out
}

func TestReportingEndToEnd(t *testing.T) {
	f := newModerationFixture(t)
	f.seedPublishedConfession("rpt-c1")

	// Authentication and validation first: everything else is noise without them.
	if status, _ := doRequest(t, f.srv, http.MethodPost, "/reports", "",
		`{"entity_type":"confession","entity_id":"rpt-c1","reason":"bad"}`); status != http.StatusUnauthorized {
		t.Errorf("unauthenticated report: got %d, want 401", status)
	}
	if status, _ := doRequest(t, f.srv, http.MethodPost, "/reports", f.user,
		`{"entity_type":"voice","entity_id":"v1","reason":"this is bad"}`); status != http.StatusBadRequest {
		t.Errorf("unreportable entity type: got %d, want 400", status)
	}
	if status, _ := doRequest(t, f.srv, http.MethodPost, "/reports", f.user,
		`{"entity_type":"confession","entity_id":"rpt-c1","reason":"no"}`); status != http.StatusUnprocessableEntity {
		t.Errorf("a reason that is not actionable: got %d, want 422", status)
	}
	if status, _ := doRequest(t, f.srv, http.MethodPost, "/reports", f.user,
		`{"entity_type":"confession","entity_id":"no-such-id","reason":"this is bad"}`); status != http.StatusNotFound {
		t.Errorf("report on an unseeable id: got %d, want 404", status)
	}

	status, body := doRequest(t, f.srv, http.MethodPost, "/reports", f.user,
		`{"entity_type":"confession","entity_id":"rpt-c1","reason":"misquoted scripture","detail":"it cites Mark 3 as Mark 8"}`)
	if status != http.StatusCreated {
		t.Fatalf("file report: %d %s", status, truncateBody(body))
	}
	resp := bodyJSON(t, body)
	report, ok := resp["report"].(map[string]any)
	if !ok {
		t.Fatalf("no report object in response: %s", truncateBody(body))
	}
	if resp["already_reported"] != false {
		t.Error("first filing claimed as a duplicate")
	}
	if report["status"] != "open" {
		t.Errorf("report status = %v, want open", report["status"])
	}
	reportID, _ := report["id"].(string)
	if reportID == "" {
		t.Fatal("report id missing")
	}

	// The same complaint twice is one row and one case, told honestly.
	status, body = doRequest(t, f.srv, http.MethodPost, "/reports", f.user,
		`{"entity_type":"confession","entity_id":"rpt-c1","reason":"misquoted scripture"}`)
	if status != http.StatusOK {
		t.Errorf("duplicate report: got %d, want 200", status)
	}
	if dup := bodyJSON(t, body); dup["already_reported"] != true {
		t.Error("duplicate filing not marked already_reported")
	} else if dup["report"].(map[string]any)["id"] != reportID {
		t.Error("duplicate returned a different report row")
	}

	// The queue shows it, and only to a moderator.
	if status, _ := doRequest(t, f.srv, http.MethodGet, "/admin/moderation/queue", f.user, ""); status == http.StatusOK {
		t.Error("the queue answered a non-admin")
	}
	status, body = doRequest(t, f.srv, http.MethodGet, "/admin/moderation/queue", f.admin, "")
	if status != http.StatusOK {
		t.Fatalf("queue: %d %s", status, truncateBody(body))
	}
	q := bodyJSON(t, body)
	counts := q["counts"].(map[string]any)
	if counts["reports"] != float64(1) || counts["open_cases"] != float64(1) {
		t.Errorf("queue counts after one report: %v", counts)
	}

	// A non-decision is refused; an outcome is recorded once.
	if status, _ := doRequest(t, f.srv, http.MethodPost, "/admin/moderation/reports/"+reportID+"/decision", f.admin,
		`{"decision":"reviewed"}`); status != http.StatusBadRequest {
		t.Errorf("a non-outcome decision: got %d, want 400", status)
	}
	status, body = doRequest(t, f.srv, http.MethodPost, "/admin/moderation/reports/"+reportID+"/decision", f.admin,
		`{"decision":"dismissed","note":"quotation is correct in the NIV text"}`)
	if status != http.StatusOK {
		t.Fatalf("decide report: %d %s", status, truncateBody(body))
	}
	decided := bodyJSON(t, body)
	if decided["status"] != "dismissed" || decided["resolution_note"] == "" {
		t.Errorf("decision not visible in the response: %+v", decided)
	}
	if decided["reviewed_by"] == "" {
		t.Error("the deciding moderator is not recorded")
	}
	if status, _ := doRequest(t, f.srv, http.MethodPost, "/admin/moderation/reports/"+reportID+"/decision", f.admin,
		`{"decision":"resolved"}`); status != http.StatusConflict {
		t.Errorf("re-deciding a closed report: got %d, want 409", status)
	}

	status, body = doRequest(t, f.srv, http.MethodGet, "/admin/moderation/queue", f.admin, "")
	if status != http.StatusOK {
		t.Fatal(status)
	}
	counts = bodyJSON(t, body)["counts"].(map[string]any)
	if counts["reports"] != float64(0) || counts["open_cases"] != float64(0) {
		t.Errorf("queue counts after the decision: %v", counts)
	}
}

func TestUserConfessionSubmissionAndReview(t *testing.T) {
	f := newModerationFixture(t)

	// A private draft: created, listed with its lifecycle state, offered.
	status, body := doRequest(t, f.srv, http.MethodPost, "/me/confessions", f.user,
		`{"title":"My confession","text":"I have sinned in thought and deed.","visibility":"public"}`)
	if status != http.StatusCreated {
		t.Fatalf("create: %d %s", status, truncateBody(body))
	}
	created := bodyJSON(t, body)
	if created["status"] != "draft" || created["visibility"] != "public" {
		t.Errorf("created confession should be a public-intent draft: %+v", created)
	}
	ucID, _ := created["id"].(string)

	if status, body := doRequest(t, f.srv, http.MethodGet, "/me/confessions", f.user, ""); status != http.StatusOK ||
		!strings.Contains(body, `"status":"draft"`) {
		t.Errorf("list should expose the lifecycle state: %d %s", status, truncateBody(body))
	}

	// Only the author can offer it, and only once.
	otherToken := registerAndSignIn(t, f.srv, "mod-other@example.com", "a-strong-enough-passphrase")
	if status, _ := doRequest(t, f.srv, http.MethodPost, "/me/confessions/"+ucID+"/submit", otherToken, ""); status != http.StatusNotFound {
		t.Errorf("submitting someone else's confession: got %d, want 404 (not 403; §71)", status)
	}
	status, body = doRequest(t, f.srv, http.MethodPost, "/me/confessions/"+ucID+"/submit", f.user, "")
	if status != http.StatusOK {
		t.Fatalf("submit: %d %s", status, truncateBody(body))
	}
	if bodyJSON(t, body)["status"] != "submitted" {
		t.Errorf("after submit: %+v", bodyJSON(t, body))
	}
	if status, _ := doRequest(t, f.srv, http.MethodPost, "/me/confessions/"+ucID+"/submit", f.user, ""); status != http.StatusConflict {
		t.Errorf("double submit: got %d, want 409", status)
	}

	// Queued work is visible to the moderator.
	status, body = doRequest(t, f.srv, http.MethodGet, "/admin/moderation/queue", f.admin, "")
	if status != http.StatusOK {
		t.Fatal(status)
	}
	q := bodyJSON(t, body)
	if q["counts"].(map[string]any)["user_confessions"] != float64(1) {
		t.Errorf("queue should hold the submission: %+v", q["counts"])
	}

	// A silent rejection is refused both by status code and by effect.
	status, _ = doRequest(t, f.srv, http.MethodPost, "/admin/moderation/user-confessions/"+ucID+"/review", f.admin,
		`{"decision":"rejected"}`)
	if status != http.StatusUnprocessableEntity {
		t.Errorf("reasonless rejection: got %d, want 422", status)
	}
	status, body = doRequest(t, f.srv, http.MethodPost, "/admin/moderation/user-confessions/"+ucID+"/review", f.admin,
		`{"decision":"rejected","rejection_reason":"needs a softer opening"}`)
	if status != http.StatusOK {
		t.Fatalf("reject: %d %s", status, truncateBody(body))
	}
	rejected := bodyJSON(t, body)
	if rejected["status"] != "rejected" || rejected["rejection_reason"] != "needs a softer opening" {
		t.Errorf("rejection not recorded: %+v", rejected)
	}

	// The author reworks and re-offers; approval publishes, because that is the
	// audience the author asked for.
	if status, _ = doRequest(t, f.srv, http.MethodPost, "/me/confessions/"+ucID+"/submit", f.user, ""); status != http.StatusOK {
		t.Errorf("resubmit after rejection: got %d", status)
	}
	status, body = doRequest(t, f.srv, http.MethodPost, "/admin/moderation/user-confessions/"+ucID+"/review", f.admin,
		`{"decision":"approved","note":"welcome"}`)
	if status != http.StatusOK {
		t.Fatalf("approve: %d %s", status, truncateBody(body))
	}
	approved := bodyJSON(t, body)
	if approved["status"] != "published" {
		t.Errorf("public-intent approval = %v, want published", approved["status"])
	}
	if approved["reviewed_by"] == "" || approved["published_at"] == "" {
		t.Errorf("reviewer and publish time must be recorded: %+v", approved)
	}

	// A draft that was never offered cannot be decided on.
	status, body = doRequest(t, f.srv, http.MethodPost, "/me/confessions", f.user,
		`{"title":"Private one","text":"still drafting"}`)
	if status != http.StatusCreated {
		t.Fatal(status)
	}
	draftID, _ := bodyJSON(t, body)["id"].(string)
	if status, _ = doRequest(t, f.srv, http.MethodPost, "/admin/moderation/user-confessions/"+draftID+"/review", f.admin,
		`{"decision":"approved"}`); status != http.StatusConflict {
		t.Errorf("reviewing a draft: got %d, want 409", status)
	}
	if status, _ = doRequest(t, f.srv, http.MethodPost, "/admin/moderation/user-confessions/no-such/review", f.admin,
		`{"decision":"approved"}`); status != http.StatusNotFound {
		t.Errorf("reviewing a missing id: got %d, want 404", status)
	}
}

func TestConfessionQAGate(t *testing.T) {
	f := newModerationFixture(t)
	f.seedPublishedConfession("qa-c1")
	ctx := context.Background()

	// Move it through the line to the state the gate fires from, with a ready,
	// licensed render attached.
	if _, err := f.conn.ExecContext(ctx,
		`UPDATE confessions SET status='audio_qa' WHERE id='qa-c1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.conn.ExecContext(ctx,
		`INSERT INTO voices (id,name,type,language,premium,status,created_at,updated_at)
		 VALUES ('qa-voice','V','professional','en',0,'active','2026-01-01','2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.conn.ExecContext(ctx,
		`INSERT INTO voice_rights (id, voice_id, rights_holder, allowed_use, territories, status, created_at, updated_at)
		 VALUES ('qa-rights','qa-voice','holder','tts','GLOBAL','active','2026-01-01','2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.conn.ExecContext(ctx,
		`INSERT INTO audio_assets (id, content_id, voice_id, storage_key, duration_seconds, file_size_bytes, status, created_at, updated_at)
		 VALUES ('qa-asset','qa-c1','qa-voice','k/qa',120,4096,'processing','2026-01-01','2026-01-01')`); err != nil {
		t.Fatal(err)
	}

	// A gate against a confession with its render still in flight must not pass.
	status, body := doRequest(t, f.srv, http.MethodPost, "/admin/confessions/qa-c1/qa", f.admin, `{}`)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("QA with a render in flight: got %d, want 422: %s", status, truncateBody(body))
	}
	failed := bodyJSON(t, body)
	if failed["passed"] != false {
		t.Errorf("gate passed a render still processing: %s", truncateBody(body))
	}
	checks, _ := failed["checks"].([]any)
	if len(checks) != 4 {
		t.Errorf("checklist should report all four checks, got %v", failed["checks"])
	}

	// A failing run leaves the confession where it was, with the report kept.
	var status0, qaPassedAt, qaReport string
	if err := f.conn.QueryRowContext(ctx,
		`SELECT status, COALESCE(qa_passed_at,''), COALESCE(qa_report,'') FROM confessions WHERE id='qa-c1'`).Scan(&status0, &qaPassedAt, &qaReport); err != nil {
		t.Fatal(err)
	}
	if status0 != "audio_qa" || qaPassedAt != "" || !strings.Contains(qaReport, `"passed":false`) {
		t.Errorf("after failed gate: status=%s qa_passed_at=%s report=%s", status0, qaPassedAt, qaReport)
	}

	// The render lands; the same gate now approves and records it.
	if _, err := f.conn.ExecContext(ctx,
		`UPDATE audio_assets SET status='ready' WHERE id='qa-asset'`); err != nil {
		t.Fatal(err)
	}
	status, body = doRequest(t, f.srv, http.MethodPost, "/admin/confessions/qa-c1/qa", f.admin, `{"note":"sounds right"}`)
	if status != http.StatusOK {
		t.Fatalf("QA pass: %d %s", status, truncateBody(body))
	}
	if bodyJSON(t, body)["passed"] != true {
		t.Errorf("healthy confession should pass: %s", truncateBody(body))
	}
	if err := f.conn.QueryRowContext(ctx,
		`SELECT status, COALESCE(qa_passed_at,'') FROM confessions WHERE id='qa-c1'`).Scan(&status0, &qaPassedAt); err != nil {
		t.Fatal(err)
	}
	if status0 != "approved" || qaPassedAt == "" {
		t.Errorf("after pass: status=%s qa_passed_at=%s", status0, qaPassedAt)
	}

	// The gate fires from audio_qa only. From approved it is a conflict.
	if status, _ = doRequest(t, f.srv, http.MethodPost, "/admin/confessions/qa-c1/qa", f.admin, `{}`); status != http.StatusConflict {
		t.Errorf("re-running the gate on approved: got %d, want 409", status)
	}
	if status, _ = doRequest(t, f.srv, http.MethodPost, "/admin/confessions/missing/qa", f.admin, `{}`); status != http.StatusNotFound {
		t.Errorf("gate on a missing confession: got %d, want 404", status)
	}

	// The PATCH path now writes history and returns 404 for a phantom, not 500.
	if status, _ = doRequest(t, f.srv, http.MethodPatch, "/admin/confessions/missing", f.admin,
		`{"status":"published"}`); status != http.StatusNotFound {
		t.Errorf("PATCH on a missing confession: got %d, want 404", status)
	}
	status, _ = doRequest(t, f.srv, http.MethodPatch, "/admin/confessions/qa-c1", f.admin,
		`{"status":"published","reason":"gate cleared"}`)
	if status != http.StatusOK {
		t.Fatalf("publish: %d", status)
	}
	var hist int
	if err := f.conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM content_moderation_history WHERE confession_id='qa-c1'`).Scan(&hist); err != nil {
		t.Fatal(err)
	}
	// One row from the QA pass (audio_qa->approved), one from the PATCH
	// (approved->published). Before this phase there were no writers at all.
	if hist != 2 {
		t.Errorf("moderation history rows = %d, want 2", hist)
	}
}
