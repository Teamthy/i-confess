package api

import (
	"context"
	"net/http"
	"testing"
)

// registerSecond returns a signed-in token and the account id for a second
// listener, so a test has a pair to block between.
func (f *moderationFixture) registerSecond(t *testing.T, email string) (string, string) {
	t.Helper()
	registerAndSignIn(t, f.srv, email, "a-strong-enough-passphrase")
	// Sign in again for a token issued through the same path the fixture uses,
	// so a role or claim change in registration cannot make this diverge.
	return signIn(t, f.srv, email, "a-strong-enough-passphrase"), userIDForEmail(t, f.h, email)
}

// TestBlockingEndToEnd is the API semantics half of the blocking trio.
func TestBlockingEndToEnd(t *testing.T) {
	f := newModerationFixture(t)
	otherTok, otherID := f.registerSecond(t, "blocked-party@example.com")

	// Unauthenticated.
	if status, _ := doRequest(t, f.srv, http.MethodGet, "/me/blocks", "", ""); status != http.StatusUnauthorized {
		t.Errorf("unauthenticated list: got %d, want 401", status)
	}

	// A self-block is a 400. A listener who reaches for "block" on their own
	// account is confused about the button, and silently filtering their own
	// content from their own feed would be the worst available answer.
	if status, body := doRequest(t, f.srv, http.MethodPost, "/me/blocks", f.user,
		`{"user_id":"`+f.userID+`"}`); status != http.StatusBadRequest {
		t.Errorf("self-block: got %d %s, want 400", status, truncateBody(body))
	}

	// A block against an account that does not exist is a 404, not a row that
	// drives a filter matching nobody.
	if status, _ := doRequest(t, f.srv, http.MethodPost, "/me/blocks", f.user,
		`{"user_id":"no-such-account"}`); status != http.StatusNotFound {
		t.Errorf("blocking a nonexistent account: got %d, want 404", status)
	}

	status, body := doRequest(t, f.srv, http.MethodPost, "/me/blocks", f.user,
		`{"user_id":"`+otherID+`","reason":"keeps messaging me"}`)
	if status != http.StatusCreated {
		t.Fatalf("block: %d %s", status, truncateBody(body))
	}

	// Idempotent, and reported as such rather than as a second creation.
	if status, _ := doRequest(t, f.srv, http.MethodPost, "/me/blocks", f.user,
		`{"user_id":"`+otherID+`"}`); status != http.StatusOK {
		t.Errorf("repeat block: got %d, want 200 (idempotent)", status)
	}

	status, body = doRequest(t, f.srv, http.MethodGet, "/me/blocks", f.user, "")
	if status != http.StatusOK {
		t.Fatalf("list: %d", status)
	}
	list := bodyJSON(t, body)
	if list["count"] != float64(1) {
		t.Errorf("block count = %v, want 1", list["count"])
	}

	// The blocked account sees nothing: a block is not shown to them.
	if status, body := doRequest(t, f.srv, http.MethodGet, "/v1/me/blocks", otherTok, ""); status != http.StatusOK {
		t.Fatalf("blocked party list: %d", status)
	} else if got := bodyJSON(t, body)["count"]; got != float64(0) {
		t.Errorf("the blocked account sees %v blocks, want 0", got)
	}

	// Reversible.
	if status, _ := doRequest(t, f.srv, http.MethodDelete, "/me/blocks/"+otherID, f.user, ""); status != http.StatusNoContent {
		t.Errorf("unblock: got %d, want 204", status)
	}
	// And unblocking something never blocked is not an error.
	if status, _ := doRequest(t, f.srv, http.MethodDelete, "/me/blocks/"+otherID, f.user, ""); status != http.StatusNoContent {
		t.Errorf("second unblock: got %d, want 204", status)
	}
	if _, body := doRequest(t, f.srv, http.MethodGet, "/me/blocks", f.user, ""); true {
		if got := bodyJSON(t, body)["count"]; got != float64(0) {
			t.Errorf("count after unblock = %v, want 0", got)
		}
	}
}

// TestBlockingFiltersThePublicReader proves the effect rather than the record:
// a blocked author's published testimony disappears from the blocker's feed and
// stays in everyone else's.
func TestBlockingFiltersThePublicReader(t *testing.T) {
	f := newModerationFixture(t)
	authorTok, authorID := f.registerSecond(t, "testimony-author@example.com")
	readerTok, _ := f.registerSecond(t, "testimony-reader@example.com")

	// Two published public testimonies by two different authors.
	for _, tok := range []string{authorTok, f.user} {
		status, body := doRequest(t, f.srv, http.MethodPost, "/me/confessions", tok,
			`{"title":"Public testimony","text":"a public testimony long enough to pass",`+
				`"visibility":"public"}`)
		if status != http.StatusCreated {
			t.Fatalf("create confession: %d %s", status, truncateBody(body))
		}
	}
	// Publish both through the moderator path.
	if err := publishAllUserConfessions(t, f); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// Anonymous sees both.
	if status, body := doRequest(t, f.srv, http.MethodGet, "/community/confessions", "", ""); status != http.StatusOK {
		t.Fatalf("anonymous feed: %d", status)
	} else if got := len(bodyJSON(t, body)["confessions"].([]any)); got != 2 {
		t.Fatalf("anonymous feed has %d confessions, want 2", got)
	}

	// The reader blocks the first author.
	if status, body := doRequest(t, f.srv, http.MethodPost, "/me/blocks", readerTok,
		`{"user_id":"`+authorID+`"}`); status != http.StatusCreated {
		t.Fatalf("block: %d %s", status, truncateBody(body))
	}

	if status, body := doRequest(t, f.srv, http.MethodGet, "/community/confessions", readerTok, ""); status != http.StatusOK {
		t.Fatalf("reader feed: %d", status)
	} else if got := len(bodyJSON(t, body)["confessions"].([]any)); got != 1 {
		t.Errorf("the blocker's feed has %d confessions, want the blocked author's excluded", got)
	}

	// Unblocking restores it.
	if _, _ = doRequest(t, f.srv, http.MethodDelete, "/me/blocks/"+authorID, readerTok, ""); true {
	}
	if status, body := doRequest(t, f.srv, http.MethodGet, "/community/confessions", readerTok, ""); status != http.StatusOK {
		t.Fatalf("reader feed after unblock: %d", status)
	} else if got := len(bodyJSON(t, body)["confessions"].([]any)); got != 2 {
		t.Errorf("the feed has %d confessions after an unblock, want 2", got)
	}
}

// publishAllUserConfessions moves every submitted user confession to published
// through the store, so the public reader has something to serve.
func publishAllUserConfessions(t *testing.T, f *moderationFixture) error {
	t.Helper()
	_, err := f.conn.ExecContext(context.Background(),
		`UPDATE user_confessions SET status='published', published_at='2026-02-01'
		 WHERE status IN ('draft','submitted')`)
	return err
}

// TestAppealEndToEnd is the API semantics half of the appeal trio: the
// refusals, the filing, and the moderator's decision.
func TestAppealEndToEnd(t *testing.T) {
	f := newModerationFixture(t)
	f.seedPublishedConfession("appeal-conf")

	// Nothing appealable yet.
	if status, _ := doRequest(t, f.srv, http.MethodPost, "/me/appeals", f.user,
		`{"decision_type":"report","decision_id":"nope","statement":"a statement long enough to read"}`); status != http.StatusNotFound {
		t.Errorf("appealing an unknown decision: got %d, want 404", status)
	}

	// File a report and have the moderator dismiss it.
	status, body := doRequest(t, f.srv, http.MethodPost, "/reports", f.user,
		`{"entity_type":"confession","entity_id":"appeal-conf","reason":"misquoted scripture"}`)
	if status != http.StatusCreated {
		t.Fatalf("report: %d %s", status, truncateBody(body))
	}
	reportID := bodyJSON(t, body)["report"].(map[string]any)["id"].(string)

	// A report that is still open has not been decided, so it cannot be appealed.
	if status, _ := doRequest(t, f.srv, http.MethodPost, "/me/appeals", f.user,
		`{"decision_type":"report","decision_id":"`+reportID+`","statement":"a statement long enough to read"}`); status != http.StatusConflict {
		t.Errorf("appealing an undecided report: got %d, want 409", status)
	}

	if status, body := doRequest(t, f.srv, http.MethodPost, "/admin/moderation/reports/"+reportID+"/decision", f.admin,
		`{"decision":"dismissed","note":"no action needed"}`); status != http.StatusOK {
		t.Fatalf("dismiss report: %d %s", status, truncateBody(body))
	}

	// A statement too short to read is a 400, not a 500.
	if status, _ := doRequest(t, f.srv, http.MethodPost, "/me/appeals", f.user,
		`{"decision_type":"report","decision_id":"`+reportID+`","statement":"no"}`); status != http.StatusBadRequest {
		t.Errorf("a two-character statement: got %d, want 400", status)
	}

	status, body = doRequest(t, f.srv, http.MethodPost, "/me/appeals", f.user,
		`{"decision_type":"report","decision_id":"`+reportID+`","statement":"the misquote is still there and the dismissal does not address it"}`)
	if status != http.StatusCreated {
		t.Fatalf("file appeal: %d %s", status, truncateBody(body))
	}
	appeal := bodyJSON(t, body)
	if appeal["status"] != "submitted" {
		t.Errorf("appeal status = %v, want submitted", appeal["status"])
	}
	appealID := appeal["id"].(string)

	// Somebody else's appeal is not visible and not decidable by them.
	otherTok, _ := f.registerSecond(t, "appeal-bystander@example.com")
	if status, body := doRequest(t, f.srv, http.MethodGet, "/me/appeals", otherTok, ""); status != http.StatusOK {
		t.Fatalf("bystander list: %d", status)
	} else if got := bodyJSON(t, body)["count"]; got != float64(0) {
		t.Errorf("a bystander sees %v appeals, want 0", got)
	}

	// A non-admin cannot decide.
	if status, _ := doRequest(t, f.srv, http.MethodPost, "/admin/moderation/appeals/"+appealID+"/decision", f.user,
		`{"decision":"overturned"}`); status != http.StatusForbidden {
		t.Errorf("non-admin deciding an appeal: got %d, want 403", status)
	}

	// An unknown outcome is refused before anything is written.
	if status, _ := doRequest(t, f.srv, http.MethodPost, "/admin/moderation/appeals/"+appealID+"/decision", f.admin,
		`{"decision":"approved"}`); status != http.StatusBadRequest {
		t.Errorf("an unknown decision: got %d, want 400", status)
	}

	// Overturning reopens the report. It does not resolve it: an appeal
	// succeeding means the first decision was wrong, not that the opposite one
	// is right.
	status, body = doRequest(t, f.srv, http.MethodPost, "/v1/admin/moderation/appeals/"+appealID+"/decision", f.admin,
		`{"decision":"overturned","note":"the misquote is real"}`)
	if status != http.StatusOK {
		t.Fatalf("decide appeal: %d %s", status, truncateBody(body))
	}
	if got := bodyJSON(t, body)["status"]; got != "overturned" {
		t.Errorf("decision status = %v, want overturned", got)
	}

	// The report is open again.
	status, body = doRequest(t, f.srv, http.MethodGet, "/admin/moderation/queue", f.admin, "")
	if status != http.StatusOK {
		t.Fatalf("queue: %d", status)
	}
	queue := bodyJSON(t, body)
	counts, _ := queue["counts"].(map[string]any)
	if counts["reports"] != float64(1) {
		t.Errorf("reopened reports in queue = %v, want 1 (counts=%v)", counts["reports"], counts)
	}
	if counts["appeals"] != float64(0) {
		t.Errorf("decided appeals still in queue = %v, want 0", counts["appeals"])
	}

	// A decided appeal is final.
	if status, _ := doRequest(t, f.srv, http.MethodPost, "/admin/moderation/appeals/"+appealID+"/decision", f.admin,
		`{"decision":"upheld"}`); status != http.StatusConflict {
		t.Errorf("re-deciding a closed appeal: got %d, want 409", status)
	}

	// And the appellant can see the outcome and the reasoning.
	if status, body := doRequest(t, f.srv, http.MethodGet, "/me/appeals", f.user, ""); status != http.StatusOK {
		t.Fatalf("my appeals: %d", status)
	} else {
		appeals, _ := bodyJSON(t, body)["appeals"].([]any)
		if len(appeals) != 1 {
			t.Fatalf("appeals = %d, want 1", len(appeals))
		}
		got := appeals[0].(map[string]any)
		if got["status"] != "overturned" || got["decision_note"] != "the misquote is real" {
			t.Errorf("the appellant sees %+v, want the outcome and the reasoning", got)
		}
	}
}

// TestAppealOfARejectedConfessionDoesNotPublish is the invariant that keeps an
// appeal from becoming a publication back door.
func TestAppealOfARejectedConfessionDoesNotPublish(t *testing.T) {
	f := newModerationFixture(t)

	status, body := doRequest(t, f.srv, http.MethodPost, "/me/confessions", f.user,
		`{"title":"My testimony","text":"a testimony long enough to submit for review"}`)
	if status != http.StatusCreated {
		t.Fatalf("create confession: %d %s", status, truncateBody(body))
	}
	// The create endpoint returns the confession itself, not a wrapper.
	ucID, _ := bodyJSON(t, body)["id"].(string)
	if ucID == "" {
		t.Fatalf("no confession id in response: %s", truncateBody(body))
	}

	if status, body := doRequest(t, f.srv, http.MethodPost, "/me/confessions/"+ucID+"/submit", f.user, ""); status != http.StatusOK {
		t.Fatalf("submit: %d %s", status, truncateBody(body))
	}
	if status, body := doRequest(t, f.srv, http.MethodPost, "/admin/moderation/user-confessions/"+ucID+"/review", f.admin,
		`{"decision":"rejected","rejection_reason":"it names another person"}`); status != http.StatusOK {
		t.Fatalf("reject: %d %s", status, truncateBody(body))
	}

	status, body = doRequest(t, f.srv, http.MethodPost, "/me/appeals", f.user,
		`{"decision_type":"confession","decision_id":"`+ucID+`","statement":"I removed the name and the reason no longer applies"}`)
	if status != http.StatusCreated {
		t.Fatalf("appeal: %d %s", status, truncateBody(body))
	}
	appealID := bodyJSON(t, body)["id"].(string)

	if status, _ := doRequest(t, f.srv, http.MethodPost, "/admin/moderation/appeals/"+appealID+"/decision", f.admin,
		`{"decision":"overturned","note":"fair enough"}`); status != http.StatusOK {
		t.Fatalf("decide: %d", status)
	}

	// Back into the review queue - not published, and not resolved.
	var status2 string
	if err := f.conn.QueryRowContext(context.Background(),
		`SELECT status FROM user_confessions WHERE id=$1`, ucID).Scan(&status2); err != nil {
		t.Fatalf("read confession: %v", err)
	}
	if status2 != "submitted" {
		t.Errorf("an overturned appeal left the confession %q, want submitted - an appeal must not publish", status2)
	}
}
