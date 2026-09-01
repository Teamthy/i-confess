package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The spec is generated from the route table, so these tests check that the
// table is complete and that the description stays useful to a client author.

// Every API route must be described. This is the test that stops the spec
// drifting: adding an endpoint without going through h.route fails here.
func TestSpecCoversEveryAPIRoute(t *testing.T) {
	a := newAuthHarness(t)
	spec := a.h.OpenAPISpec("http://localhost")
	paths, _ := spec["paths"].(map[string]any)

	var missing []string
	for _, r := range a.h.RouteTable() {
		if isNonAPIRoute(r.Path) {
			continue
		}
		item, ok := paths[r.Path].(map[string]any)
		if !ok {
			missing = append(missing, r.Method+" "+r.Path)
			continue
		}
		if _, ok := item[strings.ToLower(r.Method)]; !ok {
			missing = append(missing, r.Method+" "+r.Path)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("routes are served but not described: %v", missing)
	}
}

// A route table with only a handful of entries would mean registration is
// silently not recording, and the coverage test above would pass vacuously.
func TestRouteTableIsPopulated(t *testing.T) {
	a := newAuthHarness(t)
	if n := len(a.h.RouteTable()); n < 60 {
		t.Fatalf("route table has %d entries; registration is not recording", n)
	}
}

// Authenticated endpoints must be marked as such, or a generated client will
// omit the Authorization header and every call will 401.
func TestProtectedRoutesDeclareSecurity(t *testing.T) {
	a := newAuthHarness(t)
	spec := a.h.OpenAPISpec("http://localhost")
	paths, _ := spec["paths"].(map[string]any)

	for _, r := range a.h.RouteTable() {
		if isNonAPIRoute(r.Path) || r.Auth == "public" {
			continue
		}
		item, _ := paths[r.Path].(map[string]any)
		op, _ := item[strings.ToLower(r.Method)].(map[string]any)
		if op == nil {
			continue
		}
		if _, ok := op["security"]; !ok {
			t.Fatalf("%s %s requires auth but the spec does not say so", r.Method, r.Path)
		}
	}
}

// Public endpoints must NOT demand auth, or a client will refuse to call them
// before sign-in and the login screen cannot load content.
func TestPublicRoutesDeclareNoSecurity(t *testing.T) {
	a := newAuthHarness(t)
	spec := a.h.OpenAPISpec("http://localhost")
	paths, _ := spec["paths"].(map[string]any)

	for _, r := range a.h.RouteTable() {
		if isNonAPIRoute(r.Path) || r.Auth != "public" {
			continue
		}
		item, _ := paths[r.Path].(map[string]any)
		op, _ := item[strings.ToLower(r.Method)].(map[string]any)
		if op == nil {
			continue
		}
		if _, ok := op["security"]; ok {
			t.Fatalf("%s %s is public but the spec marks it authenticated", r.Method, r.Path)
		}
	}
}

// Operation ids become method names in every generated client, so a collision
// silently overwrites one endpoint with another.
func TestOperationIDsAreUnique(t *testing.T) {
	a := newAuthHarness(t)
	seen := map[string]string{}

	for _, r := range a.h.RouteTable() {
		if isNonAPIRoute(r.Path) {
			continue
		}
		id := operationID(r.Method, r.Path)
		if prev, dup := seen[id]; dup {
			t.Fatalf("operation id %q is produced by both %s and %s %s", id, prev, r.Method, r.Path)
		}
		seen[id] = r.Method + " " + r.Path
	}
}

// Every endpoint needs a summary, or the generated client is undocumented.
func TestEveryRouteHasASummary(t *testing.T) {
	a := newAuthHarness(t)
	var bare []string
	for _, r := range a.h.RouteTable() {
		if isNonAPIRoute(r.Path) {
			continue
		}
		if strings.TrimSpace(r.Summary) == "" {
			bare = append(bare, r.Method+" "+r.Path)
		}
	}
	if len(bare) > 0 {
		t.Fatalf("routes with no summary: %v", bare)
	}
}

// Path placeholders must be declared, or a generated client cannot build a URL.
func TestPathParametersAreDeclared(t *testing.T) {
	a := newAuthHarness(t)
	spec := a.h.OpenAPISpec("http://localhost")
	paths, _ := spec["paths"].(map[string]any)

	for path, v := range paths {
		if !strings.Contains(path, "{") {
			continue
		}
		item, _ := v.(map[string]any)
		for method, opv := range item {
			op, _ := opv.(map[string]any)
			params, ok := op["parameters"].([]any)
			if !ok || len(params) == 0 {
				t.Fatalf("%s %s has placeholders but declares no parameters", method, path)
			}
		}
	}
}

// A client that only knows about 200 turns a 402 into "something went wrong"
// instead of "this is a Premium feature".
func TestErrorResponsesAreDocumented(t *testing.T) {
	a := newAuthHarness(t)
	spec := a.h.OpenAPISpec("http://localhost")
	paths, _ := spec["paths"].(map[string]any)

	item, _ := paths["/me/downloads"].(map[string]any)
	if item == nil {
		t.Skip("downloads route not registered in this build")
	}
	post, _ := item["post"].(map[string]any)
	res, _ := post["responses"].(map[string]any)

	for _, code := range []string{"401", "402", "429"} {
		if _, ok := res[code]; !ok {
			t.Fatalf("POST /me/downloads does not document a %s response", code)
		}
	}
}

// The served document must be valid JSON and structurally complete.
func TestServedSpecIsValid(t *testing.T) {
	a := newAuthHarness(t)

	req := httptest.NewRequest("GET", "/openapi.json", nil)
	rec := httptest.NewRecorder()
	a.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("openapi.json: %d", rec.Code)
	}
	var spec map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &spec); err != nil {
		t.Fatalf("served spec is not valid JSON: %v", err)
	}
	if spec["openapi"] != OpenAPIVersion {
		t.Fatalf("openapi version = %v", spec["openapi"])
	}
	comps, _ := spec["components"].(map[string]any)
	schemes, _ := comps["securitySchemes"].(map[string]any)
	if _, ok := schemes["bearerAuth"]; !ok {
		t.Fatal("spec does not define the bearer security scheme")
	}
}
