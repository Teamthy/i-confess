package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

// TestEveryNonPublicRouteRejectsUnauthenticatedCaller is the regression test for IC-004.
// It sweeps the live route table and asserts that every endpoint registered with Auth != "public"
// strictly rejects requests that supply no credentials or an invalid token.
func TestEveryNonPublicRouteRejectsUnauthenticatedCaller(t *testing.T) {
	dbConn := dbtest.New(t)
	defer dbConn.Close()

	h := NewHandler(Config{JWTSecret: "test-sweep-secret", TokenTTL: "24h"}, dbConn)
	h.BuildEngine()
	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	routes := h.RouteTable()
	checked := 0

	for _, r := range routes {
		if r.Auth == "public" {
			continue
		}
		// Test unprefixed routes
		if strings.HasPrefix(r.Path, "/v1/") {
			continue
		}

		path := fillPathParams(r.Path)

		// 1. Unauthenticated request (no header)
		status, body := doRequest(t, srv, r.Method, path, "", `{"test":"payload"}`)
		if status >= 200 && status < 300 {
			t.Errorf("IC-004 violation: %s %s (auth=%q) returned %d to unauthenticated caller: %s",
				r.Method, path, r.Auth, status, truncateBody(body))
			continue
		}

		// 2. Request with invalid/forged token
		statusInvalid, _ := doRequest(t, srv, r.Method, path, "invalid.jwt.token", `{"test":"payload"}`)
		if statusInvalid >= 200 && statusInvalid < 300 {
			t.Errorf("%s %s (auth=%q) returned %d to invalid token caller",
				r.Method, path, r.Auth, statusInvalid)
			continue
		}

		checked++
	}

	if checked == 0 {
		t.Fatal("tested 0 non-public routes; route table was empty")
	}
	t.Logf("verified %d non-public routes reject unauthenticated/invalid requests", checked)
}

// TestRevokedSessionRejection asserts that revoking a session immediately causes
// subsequent authenticated calls to fail with 401.
func TestRevokedSessionRejection(t *testing.T) {
	dbConn := dbtest.New(t)
	defer dbConn.Close()

	h := NewHandler(Config{JWTSecret: "test-sweep-secret", TokenTTL: "24h"}, dbConn)
	h.BuildEngine()
	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	token := registerAndSignIn(t, srv, "user-revoked@example.com", "a-strong-enough-passphrase")

	// Call /me -> should be 200
	status, _ := doRequest(t, srv, http.MethodGet, "/me", token, "")
	if status != http.StatusOK {
		t.Fatalf("GET /me before logout returned %d, want 200", status)
	}

	// Sign out
	status, _ = doRequest(t, srv, http.MethodPost, "/auth/logout", token, "")
	if status != http.StatusOK {
		t.Fatalf("POST /auth/logout returned %d", status)
	}

	// Give immediate tick
	time.Sleep(10 * time.Millisecond)

	// Call /me again -> must be 401
	status, body := doRequest(t, srv, http.MethodGet, "/me", token, "")
	if status != http.StatusUnauthorized {
		t.Fatalf("GET /me with revoked session returned %d, want 401: %s", status, truncateBody(body))
	}

	// Call /community/posts -> must be 401
	status, body = doRequest(t, srv, http.MethodPost, "/community/posts", token, `{"body":"hello"}`)
	if status != http.StatusUnauthorized {
		t.Fatalf("POST /community/posts with revoked session returned %d, want 401: %s", status, truncateBody(body))
	}

	// Call /analytics/batch -> must be 401
	status, body = doRequest(t, srv, http.MethodPost, "/analytics/batch", token, `{"events":[{"name":"app_opened"}]}`)
	if status != http.StatusUnauthorized {
		t.Fatalf("POST /analytics/batch with revoked session returned %d, want 401: %s", status, truncateBody(body))
	}
}
