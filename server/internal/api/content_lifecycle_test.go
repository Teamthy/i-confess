package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Teamthy/i-confess/internal/content"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
	"github.com/Teamthy/i-confess/internal/models"
)

// adminClient returns a handler and a super-admin bearer token.
func adminClient(t *testing.T) (*Handler, *httptest.Server, string) {
	t.Helper()
	conn := dbtest.New(t)
	t.Cleanup(func() { conn.Close() })

	h := NewHandler(Config{JWTSecret: "test-secret-value", TokenTTL: "24h"}, conn)
	h.BuildEngine()
	srv := httptest.NewServer(h.Routes())
	t.Cleanup(srv.Close)

	email := "lifecycle-admin@example.com"
	registerAndSignIn(t, srv, email, "a-strong-enough-passphrase")
	uid := userIDForEmail(t, h, email)
	if err := h.users.SetAdminRole(context.Background(), uid, "super_admin"); err != nil {
		t.Fatalf("promote: %v", err)
	}
	return h, srv, signIn(t, srv, email, "a-strong-enough-passphrase")
}

// TestAdminCanMoveAConfessionThroughTheWholeLifecycle is the regression test
// for G-5. Before this phase, five of the eight documented states were refused
// by the database and the handler reported them as a 500 "failed to update
// confession", so the section 22 review workflow could not be executed at all.
func TestAdminCanMoveAConfessionThroughTheWholeLifecycle(t *testing.T) {
	h, srv, token := adminClient(t)

	catID := newTestCategory(t, h, "lifecycle")
	confID := newTestConfession(t, h, catID, "Lifecycle Probe")

	for _, s := range content.All() {
		body := fmt.Sprintf(`{"status":%q}`, string(s))
		status, resp := adminPatch(t, srv, "/admin/confessions/"+confID, token, body)
		if status != http.StatusOK {
			t.Errorf("moving a confession to %q returned %d, want 200: %s", s, status, resp)
		}

		var got struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal([]byte(resp), &got); err == nil && got.Status != string(s) {
			t.Errorf("after setting %q the API reported %q", s, got.Status)
		}
	}
}

// TestAdminRejectsAnUnknownStatusWithABadRequest checks the handler fails
// cleanly. A vocabulary violation is a client error; before this phase the
// ones the database caught surfaced as 500s.
func TestAdminRejectsAnUnknownStatusWithABadRequest(t *testing.T) {
	h, srv, token := adminClient(t)

	catID := newTestCategory(t, h, "lifecycle")
	confID := newTestConfession(t, h, catID, "Lifecycle Probe 2")

	for _, bad := range []string{"publishedd", "PUBLISHED", "pending_deletion", "deleted", "rejected", "under_review"} {
		body := fmt.Sprintf(`{"status":%q}`, bad)
		status, resp := adminPatch(t, srv, "/admin/confessions/"+confID, token, body)
		if status != http.StatusBadRequest {
			t.Errorf("status %q returned %d, want 400: %s", bad, status, resp)
		}
		if strings.Contains(resp, "500") {
			t.Errorf("status %q produced a server error: %s", bad, resp)
		}
	}
}

// TestAdminCreateRejectsAnUnknownStatus covers the creation path, which took
// req.Status raw with no validation at all: an invalid value reached the
// constraint and came back as a 500 "failed to create confession".
func TestAdminCreateRejectsAnUnknownStatus(t *testing.T) {
	h, srv, token := adminClient(t)
	catID := newTestCategory(t, h, "lifecycle")

	body := fmt.Sprintf(
		`{"category_id":%q,"title":"Bad Status","short_text":"a","medium_text":"ab","long_text":"abc","status":"not-a-real-status"}`,
		catID)
	status, resp := adminRequest(t, srv, http.MethodPost, "/admin/confessions", token, body)
	if status != http.StatusBadRequest {
		t.Errorf("creating a confession with a bad status returned %d, want 400: %s", status, resp)
	}
}

func adminRequest(t *testing.T, srv *httptest.Server, method, path, token, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return res.StatusCode, string(raw)
}

func adminPatch(t *testing.T, srv *httptest.Server, path, token, body string) (int, string) {
	t.Helper()
	return adminRequest(t, srv, http.MethodPatch, path, token, body)
}

func newTestCategory(t *testing.T, h *Handler, slug string) string {
	t.Helper()
	c := &models.Category{Name: "Lifecycle " + slug, Slug: slug, Status: "published", SortOrder: 1}
	if err := h.cont.CreateCategory(context.Background(), c); err != nil {
		t.Fatalf("create category: %v", err)
	}
	return c.ID
}

func newTestConfession(t *testing.T, h *Handler, catID, title string) string {
	t.Helper()
	c := &models.Confession{
		CategoryID: catID, Title: title, ShortText: "a", MediumText: "ab", LongText: "abc",
		Status: "published", Language: "en", Intensity: 1,
	}
	if err := h.cont.CreateConfession(context.Background(), c); err != nil {
		t.Fatalf("create confession: %v", err)
	}
	return c.ID
}
