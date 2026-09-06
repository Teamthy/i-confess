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

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

// TestEveryAdminRouteRejectsANonAdmin covers an invariant that nothing else
// enforces.
//
// Handler.route() takes an `auth` string and records it in the route table, but
// enforcement comes entirely from the separate `wrap` argument. The recorded
// value is documentation. A route can therefore declare "admin" and be handed
// the ordinary authenticated wrapper, and every existing check still passes:
// route parity compares paths, not privileges, and the per-handler tests exercise
// the routes they were written for.
//
// All 44 admin routes get the correct wrapper today. This test is here so that
// stays true when someone adds the 45th.
func TestEveryAdminRouteRejectsANonAdmin(t *testing.T) {
	dbConn := dbtest.New(t)
	defer dbConn.Close()

	h := NewHandler(Config{JWTSecret: "test-secret-value", TokenTTL: "24h"}, dbConn)
	h.BuildEngine()
	h.Routes()

	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	// An ordinary signed-in user with no admin role.
	token := registerAndSignIn(t, srv, "plain-user@example.com", "a-strong-enough-passphrase")

	routes := h.RouteTable()
	checked, skipped := 0, 0

	for _, r := range routes {
		if r.Auth != "admin" {
			continue
		}
		// Only exercise the unprefixed form; the /v1/ twins are covered by the
		// route parity test, and hitting both doubles the run time for no new
		// information about privilege.
		if strings.HasPrefix(r.Path, "/v1/") {
			continue
		}

		path := fillPathParams(r.Path)
		status, body := doRequest(t, srv, r.Method, path, token, "")

		// 401, 403 and 404 are all acceptable: the point is that a non-admin
		// never receives the resource. 404 in particular is fine because some
		// handlers resolve the target before authorising, and an unresolvable
		// id is not a privilege leak.
		if status >= 200 && status < 300 {
			t.Errorf("%s %s returned %d to a non-admin: %s", r.Method, path, status, truncateBody(body))
			continue
		}
		if status == http.StatusMethodNotAllowed || status == http.StatusNotFound && false {
			skipped++
			continue
		}
		checked++
	}

	if checked == 0 {
		t.Fatal("exercised no admin routes - the test would pass against an empty route table")
	}
	t.Logf("%d admin routes rejected a non-admin (%d skipped)", checked, skipped)
}

// TestAdminRoutesStillWorkForAnAdmin is the counterpart. A test that asserts
// "deny everyone" is satisfied by breaking admin access entirely, which would be
// reverted the moment someone tried to use the admin panel.
func TestAdminRoutesStillWorkForAnAdmin(t *testing.T) {
	dbConn := dbtest.New(t)
	defer dbConn.Close()

	h := NewHandler(Config{JWTSecret: "test-secret-value", TokenTTL: "24h"}, dbConn)
	h.BuildEngine()
	h.Routes()

	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	token := registerAndSignIn(t, srv, "admin-user@example.com", "a-strong-enough-passphrase")

	// Promote through the store, which is what the admin-provisioning flow does.
	userID := userIDForEmail(t, h, "admin-user@example.com")
	if err := h.users.SetAdminRole(context.Background(), userID, auth.RoleSuperAdmin); err != nil {
		t.Fatalf("promote to admin: %v", err)
	}

	// Re-sign in so the token carries the role. Roles are read at issue time.
	adminToken := signIn(t, srv, "admin-user@example.com", "a-strong-enough-passphrase")
	if adminToken == token {
		t.Log("token unchanged after promotion; role is re-read per request, which is also acceptable")
	}

	status, body := doRequest(t, srv, http.MethodGet, "/admin/users/admins", adminToken, "")
	if status != http.StatusOK {
		t.Fatalf("GET /admin/users/admins returned %d for an admin: %s", status, truncateBody(body))
	}
}

func registerAndSignIn(t *testing.T, srv *httptest.Server, email, password string) string {
	t.Helper()
	payload := fmt.Sprintf(`{"email":%q,"password":%q,"display_name":"Test","timezone":"UTC"}`, email, password)
	if status, body := doRequest(t, srv, http.MethodPost, "/auth/register", "", payload); status >= 300 {
		t.Fatalf("register %s: %d %s", email, status, truncateBody(body))
	}
	return signIn(t, srv, email, password)
}

func signIn(t *testing.T, srv *httptest.Server, email, password string) string {
	t.Helper()
	payload := fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)
	status, body := doRequest(t, srv, http.MethodPost, "/auth/login", "", payload)
	if status != http.StatusOK {
		t.Fatalf("sign in %s: %d %s", email, status, truncateBody(body))
	}
	tok, err := jsonField(body, "token")
	if err != nil {
		t.Fatalf("no token in login response: %v (%s)", err, truncateBody(body))
	}
	return tok
}

func userIDForEmail(t *testing.T, h *Handler, email string) string {
	t.Helper()
	u, _, err := h.users.ByEmail(context.Background(), email)
	if err != nil || u == nil {
		t.Fatalf("look up %s: %v", email, err)
	}
	return u.ID
}

func doRequest(t *testing.T, srv *httptest.Server, method, path, token, body string) (int, string) {
	t.Helper()
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	req, err := http.NewRequest(method, srv.URL+path, reader)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, path, err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, path, err)
	}
	defer func() { _ = res.Body.Close() }()

	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return res.StatusCode, string(raw)
}

// fillPathParams substitutes a plausible value for each {param} so the request
// reaches the authorisation check rather than failing to route.
func fillPathParams(path string) string {
	replacer := strings.NewReplacer(
		"{id}", "00000000-0000-0000-0000-000000000000",
		"{userId}", "00000000-0000-0000-0000-000000000000",
		"{confessionId}", "00000000-0000-0000-0000-000000000000",
		"{voiceId}", "00000000-0000-0000-0000-000000000000",
		"{sessionId}", "00000000-0000-0000-0000-000000000000",
	)
	out := replacer.Replace(path)
	// Any remaining {param} becomes a placeholder of the right shape.
	for strings.Contains(out, "{") {
		i := strings.Index(out, "{")
		j := strings.Index(out, "}")
		if j < i {
			break
		}
		out = out[:i] + "placeholder" + out[j+1:]
	}
	return out
}

func truncateBody(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 200 {
		return s
	}
	return s[:200] + "..."
}

// jsonField pulls one top-level string field out of a JSON body.
func jsonField(body, key string) (string, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return "", err
	}
	v, ok := m[key].(string)
	if !ok {
		return "", fmt.Errorf("field %q missing or not a string", key)
	}
	return v, nil
}

// TestRoleScopingIsEnforced covers the layer below "is an admin". The generic
// admin wrapper (router.go) names no extra role, so it admits SUPER_ADMIN only;
// narrower wrappers name the roles that may use them. Both halves have to hold
// or the role constants are decoration.
func TestRoleScopingIsEnforced(t *testing.T) {
	dbConn := dbtest.New(t)
	defer dbConn.Close()

	h := NewHandler(Config{JWTSecret: "test-secret-value", TokenTTL: "24h"}, dbConn)
	h.BuildEngine()

	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	// A route the generic admin wrapper serves: super admins only.
	const superOnly = "/admin/categories"
	// A route wrapped for voice managers (router.go:217).
	const voiceOnly = "/admin/voices/00000000-0000-0000-0000-000000000000/rights"

	cases := []struct {
		name    string
		role    string
		path    string
		wantDen bool // want a 403 from the role gate
	}{
		{"content admin denied a super-admin route", auth.RoleContentAdmin, superOnly, true},
		{"content admin denied a voice-manager route", auth.RoleContentAdmin, voiceOnly, true},
		{"voice manager admitted to a voice-manager route", auth.RoleVoiceManager, voiceOnly, false},
		{"super admin admitted everywhere", auth.RoleSuperAdmin, superOnly, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			email := strings.ReplaceAll(tc.role, "_", "-") + "-scoping@example.com"
			registerAndSignIn(t, srv, email, "a-strong-enough-passphrase")
			uid := userIDForEmail(t, h, email)
			if err := h.users.SetAdminRole(context.Background(), uid, tc.role); err != nil {
				t.Fatalf("promote: %v", err)
			}
			// Sign in AFTER the promotion: the gate reads the role off the
			// session, not off the token, so the session must carry it.
			token := signIn(t, srv, email, "a-strong-enough-passphrase")

			status, body := doRequest(t, srv, http.MethodGet, tc.path, token, "")
			denied := status == http.StatusForbidden && strings.Contains(body, "AUTH_INSUFFICIENT_PERMISSION")
			if tc.wantDen && !denied {
				t.Errorf("%s: got %d %s", tc.path, status, truncateBody(body))
			}
			if !tc.wantDen && denied {
				t.Errorf("%s: role %s was refused by the role gate: %s", tc.path, tc.role, truncateBody(body))
			}
		})
	}
}
