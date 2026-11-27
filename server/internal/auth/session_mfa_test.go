package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mfaValidator is a fake SessionValidator for testing the MFA gate.
type mfaValidator struct {
	state SessionState
	err   bool
}

func (v mfaValidator) ValidateSession(_ context.Context, _, _ string) SessionState {
	if v.err {
		return SessionState{Reason: ReasonTokenInvalid}
	}
	return v.state
}

// bearerToken signs a real access token so the request passes token parsing;
// the injected validator then decides whether the session is honoured.
func bearerToken(t *testing.T) string {
	t.Helper()
	tok, err := SignSessionToken("secret", "24h", "user-1", "a@b.com", RoleSuperAdmin, "")
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestRequireRoleWithSessionsRejectsUnverifiedMFA(t *testing.T) {
	// A valid token and an admitted role are not enough: the session must
	// also have passed a second factor.
	v := mfaValidator{state: SessionState{Valid: true, Role: RoleSuperAdmin, MFAVerified: false}}
	mw := RequireRoleWithSessions("secret", v)

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { nextCalled = true })
	handler := mw(next)

	req := httptest.NewRequest("GET", "/admin/x", nil)
	req.Header.Set("Authorization", "Bearer "+bearerToken(t))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
	if nextCalled {
		t.Fatal("next handler ran despite missing MFA")
	}
}

func TestRequireRoleWithSessionsAdmitsMFASession(t *testing.T) {
	v := mfaValidator{state: SessionState{Valid: true, Role: RoleSuperAdmin, MFAVerified: true}}
	mw := RequireRoleWithSessions("secret", v)

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { nextCalled = true })
	handler := mw(next)

	req := httptest.NewRequest("GET", "/admin/x", nil)
	req.Header.Set("Authorization", "Bearer "+bearerToken(t))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !nextCalled {
		t.Fatal("next handler did not run for MFA-verified session")
	}
}

func TestRequireRoleWithSessionsRejectsInvalidSession(t *testing.T) {
	v := mfaValidator{state: SessionState{Valid: false, Reason: ReasonSessionRevoked}}
	mw := RequireRoleWithSessions("secret", v)

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { nextCalled = true })
	handler := mw(next)

	req := httptest.NewRequest("GET", "/admin/x", nil)
	req.Header.Set("Authorization", "Bearer "+bearerToken(t))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if nextCalled {
		t.Fatal("next handler ran despite invalid session")
	}
}
