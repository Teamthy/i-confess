package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The API used to answer "/" and every unknown GET with an embedded HTML app,
// so a mistyped endpoint looked like a page rather than a 404.
func TestRootIsServiceInfoAndUnknownPathsAre404JSON(t *testing.T) {
	h := NewHandler(Config{JWTSecret: "test-secret", TokenTTL: "24h"}, setupTestDB(t))
	router := h.Routes()

	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}

	root := get("/")
	if root.Code != http.StatusOK || !strings.Contains(root.Body.String(), `"openapi":"/openapi.json"`) {
		t.Fatalf("GET / = %d %s", root.Code, root.Body.String())
	}
	for _, path := range []string{"/bible/capabilities-typo", "/index.html", "/admin/", "/admin"} {
		rec := get(path)
		if rec.Code != http.StatusNotFound || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
			t.Errorf("GET %s = %d (%s), want a JSON 404", path, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
	// Real admin API routes are unaffected: still routed, still refused
	// without credentials.
	if rec := get("/admin/users/admins"); rec.Code != http.StatusUnauthorized && rec.Code != http.StatusForbidden {
		t.Errorf("GET /admin/users/admins without a token = %d, want 401/403", rec.Code)
	}
}
