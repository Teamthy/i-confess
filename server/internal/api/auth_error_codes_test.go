package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The auth endpoints are the ones a client cannot recover from by guessing.
//
// contracts/openapi.json describes the error envelope as `{error, code}` with
// "Do not parse; use code", and clients/dart states the same rule in
// api_error.dart. Until PHASE 19 the six public auth handlers answered with
// `httpx.WriteError`, which writes `{error}` and nothing else — so every client
// branching on `code` fell through to a status-based guess, and a 401 from
// /auth/login ("wrong password") was indistinguishable from a 401 from an
// authenticated call ("your session ended"). The mobile app shipped a
// status-based mapping to work around it; these tests are what keep the real
// contract honest.
//
// 5xx responses are deliberately excluded. A client does not branch on why the
// server failed; it retries or reports an outage.
func TestAuthErrorsCarryStableCodes(t *testing.T) {
	dbConn := setupTestDB(t)
	defer dbConn.Close()

	h := NewHandler(Config{JWTSecret: "test-secret", TokenTTL: "24h"}, dbConn)
	h.BuildEngine()
	router := h.Routes()

	// A real account, so "wrong password" is a genuine mismatch rather than an
	// unknown address. The server must not distinguish the two, but the test
	// should be exercising the case a listener actually hits.
	if rec := postJSON(router, "/auth/register", map[string]string{
		"email":    "codes@example.com",
		"password": "correct-passphrase-2026",
	}); rec.Code != http.StatusOK {
		t.Fatalf("setup registration failed: %d %s", rec.Code, rec.Body.String())
	}

	tests := []struct {
		name   string
		path   string
		body   map[string]string
		status int
		code   string
	}{
		{
			"a wrong password is AUTH_INVALID_CREDENTIALS",
			"/auth/login",
			map[string]string{"email": "codes@example.com", "password": "wrong-passphrase-2026"},
			http.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS",
		},
		{
			"an unknown address answers identically to a wrong password",
			"/auth/login",
			map[string]string{"email": "nobody-here@example.com", "password": "wrong-passphrase-2026"},
			http.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS",
		},
		{
			"a malformed registration body is a validation failure",
			"/auth/register",
			map[string]string{"email": "x@example.com", "password": "correct-passphrase-2026", "unexpected": "field"},
			http.StatusBadRequest, "AUTH_VALIDATION_FAILED",
		},
		{
			"a breached password is a validation failure",
			"/auth/register",
			map[string]string{"email": "breach@example.com", "password": "password"},
			http.StatusBadRequest, "AUTH_VALIDATION_FAILED",
		},
		{
			"verification without a token names the token, not the input",
			"/auth/verify-email",
			map[string]string{},
			http.StatusBadRequest, "AUTH_TOKEN_INVALID",
		},
		{
			"an unknown verification token is AUTH_TOKEN_INVALID",
			"/auth/verify-email",
			map[string]string{"token": "not-a-real-token"},
			http.StatusUnauthorized, "AUTH_TOKEN_INVALID",
		},
		{
			"resending without an address is a validation failure",
			"/auth/resend-verification",
			map[string]string{},
			http.StatusBadRequest, "AUTH_VALIDATION_FAILED",
		},
		{
			"a reset request without an address is a validation failure",
			"/auth/request-password-reset",
			map[string]string{},
			http.StatusBadRequest, "AUTH_VALIDATION_FAILED",
		},
		{
			"a reset without a token names the token",
			"/auth/reset-password",
			map[string]string{"password": "a-completely-new-passphrase"},
			http.StatusBadRequest, "AUTH_TOKEN_INVALID",
		},
		{
			"a weak new password is a validation failure, not a token failure",
			"/auth/reset-password",
			map[string]string{"token": "not-a-real-token", "password": "password"},
			http.StatusBadRequest, "AUTH_VALIDATION_FAILED",
		},
		{
			"an unknown reset token is AUTH_TOKEN_INVALID",
			"/auth/reset-password",
			map[string]string{"token": "not-a-real-token", "password": "a-completely-new-passphrase"},
			http.StatusUnauthorized, "AUTH_TOKEN_INVALID",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := postJSON(router, tc.path, tc.body)

			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tc.status, rec.Body.String())
			}

			var payload map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatalf("response is not JSON: %v (%s)", err, rec.Body.String())
			}
			if payload["code"] != tc.code {
				t.Errorf("code = %v, want %q — a client cannot branch on a status alone, "+
					"because 401 means a wrong password here and an ended session elsewhere",
					payload["code"], tc.code)
			}
			if msg, _ := payload["error"].(string); msg == "" {
				t.Error("the human message must survive alongside the code")
			}
		})
	}
}

// A refused account is coded, and coded vaguely.
//
// The handler's message has been "this account is not available" since it was
// written, because moderation state is not the caller's business (S80). The code
// has to carry the same restraint: AUTH_ACCOUNT_SUSPENDED already exists and
// would tell a caller exactly what the message refuses to.
func TestRefusedAccountIsCodedWithoutLeakingWhy(t *testing.T) {
	dbConn := setupTestDB(t)
	defer dbConn.Close()

	h := NewHandler(Config{JWTSecret: "test-secret", TokenTTL: "24h"}, dbConn)
	h.BuildEngine()
	router := h.Routes()

	if rec := postJSON(router, "/auth/register", map[string]string{
		"email":    "restricted@example.com",
		"password": "correct-passphrase-2026",
	}); rec.Code != http.StatusOK {
		t.Fatalf("setup registration failed: %d %s", rec.Code, rec.Body.String())
	}

	if _, err := dbConn.ExecContext(context.Background(),
		`UPDATE users SET status='suspended' WHERE email=$1`, "restricted@example.com"); err != nil {
		t.Fatalf("could not restrict the account: %v", err)
	}

	rec := postJSON(router, "/auth/login", map[string]string{
		"email":    "restricted@example.com",
		"password": "correct-passphrase-2026",
	})

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (%s)", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if payload["code"] != "AUTH_ACCOUNT_UNAVAILABLE" {
		t.Errorf("code = %v, want AUTH_ACCOUNT_UNAVAILABLE", payload["code"])
	}
	if payload["code"] == "AUTH_ACCOUNT_SUSPENDED" {
		t.Error("the code must not disclose moderation state that the message withholds (S80)")
	}
}

// The /v1 twins carry the same codes, because a client that pins the versioned
// prefix must not get a different contract for asking politely.
func TestVersionedAuthErrorsCarryTheSameCodes(t *testing.T) {
	dbConn := setupTestDB(t)
	defer dbConn.Close()

	h := NewHandler(Config{JWTSecret: "test-secret", TokenTTL: "24h"}, dbConn)
	h.BuildEngine()
	router := h.Routes()

	rec := postJSON(router, "/v1/auth/verify-email", map[string]string{"token": "not-a-real-token"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (%s)", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if payload["code"] != "AUTH_TOKEN_INVALID" {
		t.Errorf("code = %v, want AUTH_TOKEN_INVALID", payload["code"])
	}
}

func postJSON(router http.Handler, path string, body map[string]string) *httptest.ResponseRecorder {
	encoded, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}
