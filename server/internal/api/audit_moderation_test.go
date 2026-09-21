package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// G-41: audit_logs must cover content and moderation actions.
//
// Until this phase the admin-wide sink was wired only where earlier phases
// wired it (rights changes, audio QA transitions), while moderation wrote
// its own trails (moderation_cases, content_moderation_history). An operator
// asking "who touched this content and when" had to query three tables and
// guess which workflow was involved. These tests hold the promise made in
// the PHASE 31 conditions: every moderation and content status action the
// API performs is visible in audit_logs with a real actor.

type auditRow struct {
	action, entity, entityID, actor, detail, result string
}

func auditRowsFor(t *testing.T, f *moderationFixture, action string) []auditRow {
	t.Helper()
	rows, err := f.conn.QueryContext(context.Background(),
		`SELECT action, entity, COALESCE(entity_id,''), COALESCE(actor,''),
		        COALESCE(detail,''), COALESCE(result,'')
		 FROM audit_logs WHERE action = $1 ORDER BY created_at ASC`, action)
	if err != nil {
		t.Fatalf("query audit_logs: %v", err)
	}
	defer rows.Close()
	var out []auditRow
	for rows.Next() {
		var a auditRow
		if err := rows.Scan(&a.action, &a.entity, &a.entityID, &a.actor, &a.detail, &a.result); err != nil {
			t.Fatal(err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// mustAudit asserts the action was recorded exactly want times, against
// entity, and that every line names a real actor.
func mustAudit(t *testing.T, f *moderationFixture, want int, action, entity, entityID string) []auditRow {
	t.Helper()
	got := auditRowsFor(t, f, action)
	if len(got) != want {
		t.Fatalf("audit %s: %d rows, want %d", action, len(got), want)
	}
	for _, row := range got {
		if row.entity != entity {
			t.Errorf("audit %s: entity = %q, want %q", action, row.entity, entity)
		}
		if entityID != "" && row.entityID != entityID {
			t.Errorf("audit %s: entity_id = %q, want %q", action, row.entityID, entityID)
		}
		if row.actor == "" {
			t.Errorf("audit %s: actor is empty; every moderation action has a subject", action)
		}
	}
	return got
}

func TestAuditLogsCoverContentAndModeration(t *testing.T) {
	f := newModerationFixture(t)
	ctx := context.Background()

	// ---- report_created, once per real filing, not per retry ----
	f.seedPublishedConfession("aud-c1")
	status, body := doRequest(t, f.srv, http.MethodPost, "/reports", f.user,
		`{"entity_type":"confession","entity_id":"aud-c1","reason":"misquoted scripture"}`)
	if status != http.StatusCreated {
		t.Fatalf("file report: %d %s", status, truncateBody(body))
	}
	reportID := bodyJSON(t, body)["report"].(map[string]any)["id"].(string)
	if _, body := doRequest(t, f.srv, http.MethodPost, "/reports", f.user,
		`{"entity_type":"confession","entity_id":"aud-c1","reason":"misquoted scripture"}`); !strings.Contains(body, `"already_reported":true`) {
		t.Fatalf("duplicate filing should be flagged: %s", truncateBody(body))
	}

	created := mustAudit(t, f, 1, "report_created", "report", reportID)
	if !strings.Contains(created[0].detail, "misquoted scripture") || !strings.Contains(created[0].detail, "aud-c1") {
		t.Errorf("report_created detail should name the reason and target: %q", created[0].detail)
	}
	if created[0].actor != "mod-user@example.com" {
		t.Errorf("report_created actor = %q, want the reporter", created[0].actor)
	}
	if created[0].result != "open" {
		t.Errorf("report_created result = %q, want open", created[0].result)
	}

	// ---- report_dismissed ----
	if status, body := doRequest(t, f.srv, http.MethodPost,
		"/admin/moderation/reports/"+reportID+"/decision", f.admin,
		`{"decision":"dismissed","note":"quotation is correct in the NIV text"}`); status != http.StatusOK {
		t.Fatalf("dismiss report: %d %s", status, truncateBody(body))
	}
	dismissed := mustAudit(t, f, 1, "report_dismissed", "report", reportID)
	if dismissed[0].detail != "quotation is correct in the NIV text" {
		t.Errorf("report_dismissed detail = %q, want the note", dismissed[0].detail)
	}
	if dismissed[0].actor != "mod-admin@example.com" {
		t.Errorf("report_dismissed actor = %q, want the deciding moderator", dismissed[0].actor)
	}

	// ---- report_resolved: a second report, closed the other way ----
	f.seedPublishedConfession("aud-c2")
	status, body = doRequest(t, f.srv, http.MethodPost, "/reports", f.user,
		`{"entity_type":"confession","entity_id":"aud-c2","reason":"offensive language","detail":"needs editorial eyes"}`)
	if status != http.StatusCreated {
		t.Fatalf("file second report: %d %s", status, truncateBody(body))
	}
	report2ID := bodyJSON(t, body)["report"].(map[string]any)["id"].(string)
	if status, body := doRequest(t, f.srv, http.MethodPost,
		"/admin/moderation/reports/"+report2ID+"/decision", f.admin,
		`{"decision":"resolved","note":"confession unpublished"}`); status != http.StatusOK {
		t.Fatalf("resolve report: %d %s", status, truncateBody(body))
	}
	resolved := mustAudit(t, f, 1, "report_resolved", "report", report2ID)
	if resolved[0].result != "resolved" {
		t.Errorf("report_resolved result = %q, want resolved", resolved[0].result)
	}

	// ---- user_confession_submitted / _rejected / _approved ----
	status, body = doRequest(t, f.srv, http.MethodPost, "/me/confessions", f.user,
		`{"title":"Audit me","text":"I confess.","visibility":"public"}`)
	if status != http.StatusCreated {
		t.Fatalf("create confession: %d %s", status, truncateBody(body))
	}
	ucID := bodyJSON(t, body)["id"].(string)
	if status, body := doRequest(t, f.srv, http.MethodPost,
		"/me/confessions/"+ucID+"/submit", f.user, ""); status != http.StatusOK {
		t.Fatalf("submit: %d %s", status, truncateBody(body))
	}
	submitted := mustAudit(t, f, 1, "user_confession_submitted", "user_confession", ucID)
	if submitted[0].detail != "visibility=public" || submitted[0].actor != "mod-user@example.com" {
		t.Errorf("user_confession_submitted row = %+v", submitted[0])
	}

	if status, body := doRequest(t, f.srv, http.MethodPost,
		"/admin/moderation/user-confessions/"+ucID+"/review", f.admin,
		`{"decision":"rejected","rejection_reason":"needs a softer opening"}`); status != http.StatusOK {
		t.Fatalf("reject: %d %s", status, truncateBody(body))
	}
	rejected := mustAudit(t, f, 1, "user_confession_rejected", "user_confession", ucID)
	if rejected[0].detail != "needs a softer opening" || rejected[0].result != "rejected" {
		t.Errorf("user_confession_rejected row = %+v", rejected[0])
	}

	// The rejection must not look like the end of the story: resubmission is
	// a second audit line, and approval of a public-intent confession is
	// recorded under the state it actually reached.
	if status, _ := doRequest(t, f.srv, http.MethodPost,
		"/me/confessions/"+ucID+"/submit", f.user, ""); status != http.StatusOK {
		t.Fatalf("resubmit: %d", status)
	}
	mustAudit(t, f, 2, "user_confession_submitted", "user_confession", ucID)
	if status, body := doRequest(t, f.srv, http.MethodPost,
		"/admin/moderation/user-confessions/"+ucID+"/review", f.admin,
		`{"decision":"approved","note":"welcome"}`); status != http.StatusOK {
		t.Fatalf("approve: %d %s", status, truncateBody(body))
	}
	approved := mustAudit(t, f, 1, "user_confession_approved", "user_confession", ucID)
	if approved[0].result != "published" {
		t.Errorf("user_confession_approved result = %q, want published (public intent)", approved[0].result)
	}

	// ---- confession_qa_failed / confession_qa_passed ----
	if _, err := f.conn.ExecContext(ctx,
		`UPDATE confessions SET status='audio_qa' WHERE id='aud-c1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.conn.ExecContext(ctx,
		`INSERT INTO voices (id,name,type,language,premium,status,created_at,updated_at)
		 VALUES ('aud-voice','V','professional','en',0,'active','2026-01-01','2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	// A ready render with no licence: audio_ready passes, voices_licensed does
	// not, and the failed gate must say which.
	if _, err := f.conn.ExecContext(ctx,
		`INSERT INTO audio_assets (id, content_id, voice_id, storage_key, duration_seconds, file_size_bytes, status, created_at, updated_at)
		 VALUES ('aud-asset','aud-c1','aud-voice','k/aud',120,4096,'ready','2026-01-01','2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	if status, body := doRequest(t, f.srv, http.MethodPost, "/admin/confessions/aud-c1/qa", f.admin, `{}`); status != http.StatusUnprocessableEntity {
		t.Fatalf("QA against unlicensed voice: %d %s", status, truncateBody(body))
	}
	failed := mustAudit(t, f, 1, "confession_qa_failed", "confession", "aud-c1")
	if failed[0].detail != "voices_licensed" {
		t.Errorf("confession_qa_failed detail = %q, want the failing check name", failed[0].detail)
	}
	if failed[0].result != "audio_qa" {
		t.Errorf("confession_qa_failed result = %q, want the state it was held in", failed[0].result)
	}

	if _, err := f.conn.ExecContext(ctx,
		`INSERT INTO voice_rights (id, voice_id, rights_holder, allowed_use, territories, status, created_at, updated_at)
		 VALUES ('aud-rights','aud-voice','holder','tts','GLOBAL','active','2026-01-01','2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	if status, body := doRequest(t, f.srv, http.MethodPost, "/admin/confessions/aud-c1/qa", f.admin,
		`{"note":"licence landed"}`); status != http.StatusOK {
		t.Fatalf("QA pass: %d %s", status, truncateBody(body))
	}
	passed := mustAudit(t, f, 1, "confession_qa_passed", "confession", "aud-c1")
	if passed[0].detail != "licence landed" || passed[0].result != "approved" {
		t.Errorf("confession_qa_passed row = %+v", passed[0])
	}

	// A 409 or 404 refusal of the gate is not an event on the content, so it
	// must not grow the trail.
	if status, _ := doRequest(t, f.srv, http.MethodPost, "/admin/confessions/aud-c1/qa", f.admin, `{}`); status != http.StatusConflict {
		t.Fatalf("re-running the gate: %d", status)
	}
	if status, _ := doRequest(t, f.srv, http.MethodPost, "/admin/confessions/missing/qa", f.admin, `{}`); status != http.StatusNotFound {
		t.Fatalf("gate on a phantom: %d", status)
	}
	mustAudit(t, f, 1, "confession_qa_failed", "confession", "aud-c1")
	mustAudit(t, f, 1, "confession_qa_passed", "confession", "aud-c1")

	// ---- confession_status_{status}: the canonical PATCH ----
	if status, body := doRequest(t, f.srv, http.MethodPatch, "/admin/confessions/aud-c1", f.admin,
		`{"status":"published","reason":"gate cleared"}`); status != http.StatusOK {
		t.Fatalf("publish: %d %s", status, truncateBody(body))
	}
	pub := mustAudit(t, f, 1, "confession_status_published", "confession", "aud-c1")
	if pub[0].detail != "gate cleared" || pub[0].result != "ok" {
		t.Errorf("confession_status_published row = %+v", pub[0])
	}
	// A no-op PATCH moved nothing: it is still an admin action, but it is
	// marked as one so the trail cannot be mistaken for a transition.
	if status, _ := doRequest(t, f.srv, http.MethodPatch, "/admin/confessions/aud-c1", f.admin,
		`{"status":"published"}`); status != http.StatusOK {
		t.Fatalf("no-op publish: %d", status)
	}
	noop := mustAudit(t, f, 2, "confession_status_published", "confession", "aud-c1")
	if noop[1].result != "unchanged" {
		t.Errorf("no-op PATCH result = %q, want unchanged", noop[1].result)
	}
	// A refused PATCH (bad body, phantom id) records nothing.
	if status, _ := doRequest(t, f.srv, http.MethodPatch, "/admin/confessions/missing", f.admin,
		`{"status":"published"}`); status != http.StatusNotFound {
		t.Fatalf("phantom PATCH: %d", status)
	}
	var refused int
	if err := f.conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_logs WHERE entity = 'confession' AND entity_id = 'missing'`).Scan(&refused); err != nil {
		t.Fatal(err)
	}
	if refused != 0 {
		t.Errorf("a 404 PATCH wrote %d audit rows; refused actions must leave no trail", refused)
	}

	// The whole trail answers "who touched this confession" from the one
	// admin-visible surface, without naming the other two tables at all.
	status, body = doRequest(t, f.srv, http.MethodGet, "/admin/audit?entity=confession&entity_id=aud-c1", f.admin, "")
	if status != http.StatusOK {
		t.Fatalf("admin audit: %d %s", status, truncateBody(body))
	}
	trail := body
	for _, want := range []string{"confession_qa_failed", "confession_qa_passed", "confession_status_published"} {
		if !strings.Contains(trail, want) {
			t.Errorf("GET /admin/audit?entity=confession missing %s: %s", want, truncateBody(trail))
		}
	}
}
