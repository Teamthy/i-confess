package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
	"github.com/Teamthy/i-confess/internal/mfa"
	"github.com/Teamthy/i-confess/internal/oauth"
	"github.com/Teamthy/i-confess/internal/store"
)

// mfaHarness builds a handler against a fresh test database.
type mfaHarness struct {
	h      *Handler
	router http.Handler
	db     *db.DB
}

func newMFAHarness(t *testing.T) *mfaHarness {
	t.Helper()
	dbConn := setupTestDB(t)
	t.Cleanup(func() { dbConn.Close() })
	h := NewHandler(Config{JWTSecret: "test-secret", TokenTTL: "24h"}, dbConn)
	h.BuildEngine()
	return &mfaHarness{h: h, router: h.Routes(), db: dbConn}
}

func (m *mfaHarness) do(method, path, token string, body any) (int, map[string]any) {
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	m.router.ServeHTTP(rec, req)

	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

// issueAdminSession creates a user, grants an admin role, and returns a token
// bound to a session whose mfa_verified flag is set as requested.
func (m *mfaHarness) issueAdminSession(t *testing.T, role string, mfaVerified bool) (string, string) {
	t.Helper()
	users := store.NewUserStore(m.db)
	hash, err := auth.HashPassword("test-passphrase-2026")
	if err != nil {
		t.Fatal(err)
	}
	u, err := users.Create(context.Background(), "admin-mfa@test.com", hash, "Admin", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if err := users.SetAdminRole(context.Background(), u.ID, role); err != nil {
		t.Fatal(err)
	}
	sessionID, err := users.CreateAuthSession(context.Background(), u.ID, "web", "test-agent", "127.0.0.1", time.Hour, mfaVerified)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := auth.SignSessionToken(m.h.cfg.JWTSecret, "24h", u.ID, u.Email, role, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	return tok, u.ID
}

// enableMFAOn marks the user as enrolled in a second factor.
func enableMFAOn(t *testing.T, users *store.UserStore, userID string) {
	t.Helper()
	secret, err := mfa.GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	if err := users.EnableMFA(context.Background(), userID, secret); err != nil {
		t.Fatal(err)
	}
}

// fakeVerifier returns a fixed identity for any token.
type fakeVerifier struct {
	id *oauth.Identity
}

func (f *fakeVerifier) Name() string { return "google" }
func (f *fakeVerifier) Verify(_ context.Context, _ string) (*oauth.Identity, error) {
	return f.id, nil
}

// A privileged route must refuse a session that never passed a second factor,
// even when the token is valid and the role is correct.
func TestAdminRouteRefusesPasswordOnlySession(t *testing.T) {
	m := newMFAHarness(t)
	token, _ := m.issueAdminSession(t, auth.RoleSuperAdmin, false)

	code, out := m.do("GET", "/admin/stats", token, nil)
	if code != http.StatusForbidden {
		t.Fatalf("password-only admin session accepted: got %d, want 403; body=%v", code, out)
	}
	if out["code"] != "AUTH_MFA_REQUIRED" {
		t.Fatalf("wrong code: %v, want AUTH_MFA_REQUIRED", out["code"])
	}
}

// A privileged route must admit a session whose second factor was satisfied.
func TestAdminRouteAdmitsMFASession(t *testing.T) {
	m := newMFAHarness(t)
	token, _ := m.issueAdminSession(t, auth.RoleSuperAdmin, true)

	code, out := m.do("GET", "/admin/stats", token, nil)
	if code != http.StatusOK {
		t.Fatalf("MFA-verified admin session rejected: got %d, want 200; body=%v", code, out)
	}
	_ = out
}

// A non-privileged route is not gated by MFA (only admin surfaces are).
func TestUserRouteDoesNotRequireMFA(t *testing.T) {
	m := newMFAHarness(t)
	users := store.NewUserStore(m.db)
	hash, err := auth.HashPassword("test-passphrase-2026")
	if err != nil {
		t.Fatal(err)
	}
	u, err := users.Create(context.Background(), "member@test.com", hash, "Member", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	sessionID, err := users.CreateAuthSession(context.Background(), u.ID, "web", "test-agent", "127.0.0.1", time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := auth.SignSessionToken(m.h.cfg.JWTSecret, "24h", u.ID, u.Email, "", sessionID)
	if err != nil {
		t.Fatal(err)
	}

	code, _ := m.do("GET", "/me", tok, nil)
	if code != http.StatusOK {
		t.Fatalf("non-MFA session rejected on user route: got %d, want 200", code)
	}
}

// Social sign-in of an account with MFA enrolled must ask for the code
// instead of issuing a session.
func TestSocialSignInOfEnrolledUserReturnsMFARequired(t *testing.T) {
	m := newMFAHarness(t)
	users := store.NewUserStore(m.db)
	hash, err := auth.HashPassword("test-passphrase-2026")
	if err != nil {
		t.Fatal(err)
	}
	u, err := users.Create(context.Background(), "social@test.com", hash, "Social", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	enableMFAOn(t, users, u.ID)

	m.h.SetVerifiers(map[string]oauth.Verifier{
		"google": &fakeVerifier{id: &oauth.Identity{Provider: "google", Subject: "sub-1", Email: "social@test.com", EmailVerified: true}},
	})

	code, out := m.do("POST", "/auth/social/google", "", map[string]string{"id_token": "whatever"})
	if code != http.StatusOK {
		t.Fatalf("social sign-in of enrolled user did not return 200: got %d; body=%v", code, out)
	}
	if out["mfa_required"] != true {
		t.Fatalf("social sign-in of enrolled user did not request MFA: %v", out)
	}
}

// Social sign-in of an account without MFA enrolled proceeds to a session.
func TestSocialSignInWithoutMFAIssuesSession(t *testing.T) {
	m := newMFAHarness(t)
	users := store.NewUserStore(m.db)
	hash, err := auth.HashPassword("test-passphrase-2026")
	if err != nil {
		t.Fatal(err)
	}
	u, err := users.Create(context.Background(), "social2@test.com", hash, "Social2", "UTC")
	if err != nil {
		t.Fatal(err)
	}

	m.h.SetVerifiers(map[string]oauth.Verifier{
		"google": &fakeVerifier{id: &oauth.Identity{Provider: "google", Subject: "sub-2", Email: "social2@test.com", EmailVerified: true}},
	})

	code, out := m.do("POST", "/auth/social/google", "", nil)
	if code != http.StatusOK {
		t.Fatalf("social sign-in without MFA rejected: got %d; body=%v", code, out)
	}
	if out["token"] == "" {
		t.Fatal("social sign-in without MFA issued no session token")
	}
}
