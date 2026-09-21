package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/store"
)

// promote registers an account, grants it role through the store (which is
// what the provisioning flow does) and signs in again so the session carries
// the role. Registration is rate-limited per client address (five per ten
// minutes), and these sweeps create more accounts than that, so each
// registration arrives from its own forwarded address — the same thing seven
// administrators on seven laptops would do.
func promote(t *testing.T, srv *httptest.Server, h *Handler, role string) string {
	t.Helper()
	email := strings.ReplaceAll(role, "_", "-") + "-rbac@example.com"
	const pw = "a-strong-enough-passphrase"
	registerFrom(t, srv, email, pw, fmt.Sprintf("10.44.%d.%d", len(email)%250, len(role)%250))
	uid := userIDForEmail(t, h, email)
	if err := h.users.SetAdminRole(context.Background(), uid, role); err != nil {
		t.Fatalf("promote %s: %v", role, err)
	}
	return signIn(t, srv, email, pw)
}

func registerFrom(t *testing.T, srv *httptest.Server, email, password, addr string) {
	t.Helper()
	payload := fmt.Sprintf(`{"email":%q,"password":%q,"display_name":"Test","timezone":"UTC"}`, email, password)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/auth/register", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", addr)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode >= 300 {
		t.Fatalf("register %s: %d", email, res.StatusCode)
	}
}

func roleDenied(status int, body string) bool {
	return status == http.StatusForbidden && strings.Contains(body, "AUTH_INSUFFICIENT_PERMISSION")
}

// TestRBACMatrixIsEnforcedOnEveryAdminRoute is the PHASE 44 sweep. For each
// of the seven roles it calls every administrative route and requires the
// role gate's verdict to equal the matrix's: admitted where auth.Allowed says
// so, refused with AUTH_INSUFFICIENT_PERMISSION everywhere else. A module
// label the matrix does not know cannot be registered at all (routeAuth
// panics), so a route cannot be administrative and unmapped.
//
// "Admitted" means the role gate did not refuse; the handler may still answer
// 400 or 404 to the synthetic request, which is not this test's concern.
func TestRBACMatrixIsEnforcedOnEveryAdminRoute(t *testing.T) {
	dbConn := dbtest.New(t)
	defer dbConn.Close()

	h := NewHandler(Config{JWTSecret: "test-secret-value", TokenTTL: "24h"}, dbConn)
	h.BuildEngine()
	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	tokens := map[string]string{}
	for _, role := range auth.Roles() {
		tokens[role] = promote(t, srv, h, role)
	}

	checked := 0
	perRole := map[string]int{}
	perModule := map[auth.Module]int{}
	for _, r := range h.RouteTable() {
		if !auth.IsAdminLabel(r.Auth) || strings.HasPrefix(r.Path, "/v1/") {
			continue
		}
		module, ok := auth.ParseAdminLabel(r.Auth)
		if !ok {
			t.Errorf("%s %s carries auth %q, which names no module", r.Method, r.Path, r.Auth)
			continue
		}
		perModule[module]++
		access := auth.AccessFor(r.Method)
		path := fillPathParams(r.Path)
		for _, role := range auth.Roles() {
			want := auth.Allowed(role, module, access)
			status, body := doRequest(t, srv, r.Method, path, tokens[role], "")
			denied := roleDenied(status, body)
			switch {
			case want && denied:
				t.Errorf("%s %s [%s %s]: %s holds it but was refused: %d %s",
					r.Method, r.Path, module, access, role, status, truncateBody(body))
			case !want && !denied:
				t.Errorf("%s %s [%s %s]: %s does not hold it but got %d %s",
					r.Method, r.Path, module, access, role, status, truncateBody(body))
			}
			if want {
				perRole[role]++
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no admin routes swept")
	}
	// The reason for the phase: before it, four roles opened nothing.
	for _, role := range auth.Roles() {
		if perRole[role] == 0 {
			t.Errorf("role %s is admitted to no admin route", role)
		}
	}
	// And the matrix names no phantom module: a module with no route is a
	// door painted on a wall.
	for _, m := range auth.Modules() {
		if perModule[m] == 0 {
			t.Errorf("module %s has no route", m)
		}
	}
	t.Logf("swept %d role×route pairs; routes open per role: %v; routes per module: %v", checked, perRole, perModule)
}

// TestAdminAccessReportsTheCallersModules: the navigation endpoint says the
// same thing the gate does.
func TestAdminAccessReportsTheCallersModules(t *testing.T) {
	dbConn := dbtest.New(t)
	defer dbConn.Close()
	h := NewHandler(Config{JWTSecret: "test-secret-value", TokenTTL: "24h"}, dbConn)
	h.BuildEngine()
	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	// A plain listener has no role and no access.
	plain := registerAndSignIn(t, srv, "plain-access@example.com", "a-strong-enough-passphrase")
	if status, body := doRequest(t, srv, http.MethodGet, "/admin/access", plain, ""); !roleDenied(status, body) {
		t.Errorf("a listener asked for admin access and got %d %s", status, truncateBody(body))
	}

	support := promote(t, srv, h, auth.RoleSupportAdmin)
	status, body := doRequest(t, srv, http.MethodGet, "/admin/access", support, "")
	if status != http.StatusOK {
		t.Fatalf("/admin/access for support_admin: %d %s", status, truncateBody(body))
	}
	var out struct {
		Role       string `json:"role"`
		SuperAdmin bool   `json:"super_admin"`
		Modules    []struct {
			Module string `json:"module"`
			Access string `json:"access"`
		} `json:"modules"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if out.Role != auth.RoleSupportAdmin || out.SuperAdmin {
		t.Errorf("role reported as %q super=%v", out.Role, out.SuperAdmin)
	}
	got := map[string]string{}
	for _, m := range out.Modules {
		got[m.Module] = m.Access
	}
	if got["users"] != "write" || got["dashboard"] != "read" || got["subscriptions"] != "read" {
		t.Errorf("support_admin modules = %v; want users:write, dashboard:read, subscriptions:read", got)
	}
	for _, locked := range []string{"roles", "system", "content", "voices"} {
		if _, open := got[locked]; open {
			t.Errorf("support_admin is shown %s, which the gate refuses", locked)
		}
	}

	super := promote(t, srv, h, auth.RoleSuperAdmin)
	status, body = doRequest(t, srv, http.MethodGet, "/admin/access", super, "")
	if status != http.StatusOK {
		t.Fatalf("/admin/access for super_admin: %d", status)
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if !out.SuperAdmin || len(out.Modules) != len(auth.Modules()) {
		t.Errorf("super_admin should hold all %d modules, got %d (super=%v)", len(auth.Modules()), len(out.Modules), out.SuperAdmin)
	}
}

// TestTheologicalReviewIsRecordedByTheReviewerAndGatesProduction drives the
// review module end to end: the reviewer records the outcome, the content
// admin reads it, and the lifecycle refuses to move text into the studio that
// no reviewer has passed (directive §22).
func TestTheologicalReviewIsRecordedByTheReviewerAndGatesProduction(t *testing.T) {
	dbConn := dbtest.New(t)
	defer dbConn.Close()
	h := NewHandler(Config{JWTSecret: "test-secret-value", TokenTTL: "24h"}, dbConn)
	h.BuildEngine()
	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	ctx := context.Background()
	content := store.NewContentStore(dbConn)
	cat := &models.Category{Name: "Faith", Slug: "faith", Status: "published"}
	if err := content.CreateCategory(ctx, cat); err != nil {
		t.Fatal(err)
	}
	conf := &models.Confession{CategoryID: cat.ID, Title: "Unshaken", Status: "draft", Language: "en",
		ShortText: "I stand.", MediumText: "I stand firm in faith.", LongText: "I stand firm in faith and am not moved."}
	if err := content.CreateConfession(ctx, conf); err != nil {
		t.Fatal(err)
	}

	editor := promote(t, srv, h, auth.RoleContentAdmin)
	reviewer := promote(t, srv, h, auth.RoleTheologicalRev)
	patch := func(status string) (int, string) {
		return doRequest(t, srv, http.MethodPatch, "/admin/confessions/"+conf.ID, editor,
			fmt.Sprintf(`{"status":%q}`, status))
	}
	review := func(token, status, notes string) (int, string) {
		return doRequest(t, srv, http.MethodPost, "/admin/confessions/"+conf.ID+"/review", token,
			fmt.Sprintf(`{"status":%q,"notes":%q}`, status, notes))
	}

	for _, step := range []string{"content_review", "theological_review"} {
		if status, body := patch(step); status != http.StatusOK {
			t.Fatalf("content admin moving to %s: %d %s", step, status, truncateBody(body))
		}
	}

	// Unreviewed text cannot enter production.
	if status, body := patch("audio_production"); status != http.StatusConflict || !strings.Contains(body, "CONTENT_REVIEW_REQUIRED") {
		t.Fatalf("audio_production without a review: got %d %s, want 409 CONTENT_REVIEW_REQUIRED", status, truncateBody(body))
	}

	// The content admin cannot record the review for themselves.
	if status, body := review(editor, "reviewed", ""); !roleDenied(status, body) {
		t.Errorf("content admin recorded a theological review: %d %s", status, truncateBody(body))
	}
	// A reviewer cannot move the lifecycle.
	if status, body := doRequest(t, srv, http.MethodPatch, "/admin/confessions/"+conf.ID, reviewer, `{"status":"audio_production"}`); !roleDenied(status, body) {
		t.Errorf("reviewer moved the lifecycle: %d %s", status, truncateBody(body))
	}

	// Sending it back requires saying why.
	if status, body := review(reviewer, "needs_revision", ""); status != http.StatusBadRequest || !strings.Contains(body, "REVIEW_NOTES_REQUIRED") {
		t.Errorf("needs_revision without notes: %d %s", status, truncateBody(body))
	}
	if status, body := review(reviewer, "unreviewed", ""); status != http.StatusBadRequest {
		t.Errorf("unreviewed must not be writable: %d %s", status, truncateBody(body))
	}
	if status, body := review(reviewer, "needs_revision", "Romans 8 is quoted out of context."); status != http.StatusOK {
		t.Fatalf("needs_revision with notes: %d %s", status, truncateBody(body))
	}
	// Sent back is not passed.
	if status, _ := patch("audio_production"); status != http.StatusConflict {
		t.Errorf("audio_production after needs_revision: %d, want 409", status)
	}

	if status, body := review(reviewer, "reviewed", "Revised wording is sound."); status != http.StatusOK {
		t.Fatalf("reviewed: %d %s", status, truncateBody(body))
	}
	// The content admin reads the outcome and can now move it on.
	status, body := doRequest(t, srv, http.MethodGet, "/admin/confessions/"+conf.ID+"/review", editor, "")
	if status != http.StatusOK {
		t.Fatalf("content admin reading the review: %d %s", status, truncateBody(body))
	}
	var rec models.TheologicalReview
	if err := json.Unmarshal([]byte(body), &rec); err != nil {
		t.Fatal(err)
	}
	if rec.Status != "reviewed" || rec.Reviewer != "theological-reviewer-rbac@example.com" || rec.ReviewedAt == "" {
		t.Errorf("review record = %+v", rec)
	}
	if status, body := patch("audio_production"); status != http.StatusOK {
		t.Errorf("audio_production after a reviewed outcome: %d %s", status, truncateBody(body))
	}

	// The review is in the audit trail under the reviewer's name.
	var n int
	if err := dbConn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_logs WHERE action = 'theological_review_reviewed' AND entity_id = $1 AND admin_user_id = $2`,
		conf.ID, "theological-reviewer-rbac@example.com").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("audit rows for the review = %d, want 1", n)
	}
}

// TestAdminAnalyticsCountsThePopulation: the analytics module reports what
// the platform did, from the rows the platform wrote.
func TestAdminAnalyticsCountsThePopulation(t *testing.T) {
	f := newAudioFixture(t)
	f.completeASession(t, f.premTok, f.voiceStd, 120)
	f.createSession(t, f.freeTok, f.voiceStd, 60)

	analyst := promote(t, f.srv, f.h, auth.RoleAnalyticsAdmin)
	status, body := doRequest(t, f.srv, http.MethodGet, "/admin/analytics?days=7", analyst, "")
	if status != http.StatusOK {
		t.Fatalf("/admin/analytics: %d %s", status, truncateBody(body))
	}
	var out struct {
		Days     int                     `json:"days"`
		Overview store.AnalyticsOverview `json:"overview"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	ov := out.Overview
	if out.Days != 7 || ov.SessionsCreated < 2 || ov.SessionsCompleted < 1 || ov.ActiveListeners < 2 || ov.NewAccounts < 3 {
		t.Errorf("overview = %+v; want ≥2 sessions, ≥1 completed, ≥2 listeners, ≥3 accounts", ov)
	}
	if ov.Events == nil || ov.TrialStates == nil || ov.Subscriptions == nil {
		t.Errorf("overview maps must be present even when empty: %+v", ov)
	}
	if status, _ := doRequest(t, f.srv, http.MethodGet, "/admin/analytics?days=0", analyst, ""); status != http.StatusBadRequest {
		t.Errorf("days=0 should be refused, got %d", status)
	}
	// The support role does not hold analytics.
	support := promote(t, f.srv, f.h, auth.RoleSupportAdmin)
	if status, body := doRequest(t, f.srv, http.MethodGet, "/admin/analytics", support, ""); !roleDenied(status, body) {
		t.Errorf("support_admin read analytics: %d %s", status, truncateBody(body))
	}
}
