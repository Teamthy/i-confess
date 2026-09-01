package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Observability and audit (§50, §51, §83, §85).

// Liveness must not depend on anything: a probe that fails on a database blip
// makes the orchestrator restart a healthy process and turns an outage into a
// crash loop.
func TestLivenessIsDependencyFree(t *testing.T) {
	a := newAuthHarness(t)
	for _, path := range []string{"/healthz", "/health/live"} {
		if code, _ := a.do("GET", path, "", nil); code != http.StatusOK {
			t.Fatalf("%s: got %d, want 200", path, code)
		}
	}
}

// Readiness checks the database, because without it nothing works, and reports
// degraded subsystems without failing over them.
func TestReadinessReportsSubsystems(t *testing.T) {
	a := newAuthHarness(t)

	code, out := a.do("GET", "/health/ready", "", nil)
	if code != http.StatusOK {
		t.Fatalf("ready: %d %v", code, out)
	}
	subs, _ := out["subsystems"].(map[string]any)
	if subs["database"] != true {
		t.Fatal("readiness does not confirm the database")
	}
	// Email and push are unconfigured in this harness; readiness must still
	// pass, because those degrade gracefully rather than stopping service.
	if out["status"] != "ready" {
		t.Fatalf("status = %v with only optional subsystems missing", out["status"])
	}
}

// Failure counts reveal whether an attack is landing, which is exactly what an
// attacker wants to know.
func TestMetricsAndAuditRequireAdmin(t *testing.T) {
	a := newAuthHarness(t)
	user, _ := a.register(t, "nosy-metrics@test.com")

	for _, path := range []string{"/admin/metrics", "/admin/audit"} {
		if code, _ := a.do("GET", path, "", nil); code != http.StatusForbidden {
			t.Fatalf("%s anonymous: got %d, want 403", path, code)
		}
		if code, _ := a.do("GET", path, user, nil); code != http.StatusForbidden {
			t.Fatalf("%s as ordinary user: got %d, want 403", path, code)
		}
	}
}

// Counters must actually move, or the dashboard is decorative.
func TestLoginMetricsAreRecorded(t *testing.T) {
	a := newAuthHarness(t)
	a.register(t, "metrics@test.com")

	before := a.h.metrics.Snapshot()

	a.do("POST", "/auth/login", "", map[string]string{
		"email": "metrics@test.com", "password": "password123",
	})
	a.do("POST", "/auth/login", "", map[string]string{
		"email": "metrics@test.com", "password": "wrong",
	})

	after := a.h.metrics.Snapshot()
	if after[MetricLoginSuccess] <= before[MetricLoginSuccess] {
		t.Fatal("successful login was not counted")
	}
	if after[MetricLoginFailure] <= before[MetricLoginFailure] {
		t.Fatal("failed login was not counted")
	}
}

// A rights change must leave a queryable record, not just a log line: log
// retention is days, a licence decision matters for years.
func TestRightsChangeIsPersistedToAudit(t *testing.T) {
	a := newAuthHarness(t)

	// Record an audit entry through the same path the rights handler uses.
	req := mustRequest(t, "PUT", "/admin/voices/v1/rights", "", nil)
	a.h.recordAudit(req, "voice_rights_ai_generation_granted", "voice", "v1",
		"Signed agreement 2026-01-15 clause 4.", "success")

	entries, err := a.h.audio.AuditTrail(req.Context(), "voice", "v1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("the rights change left no audit record")
	}
	e := entries[0]
	if e.Action != "voice_rights_ai_generation_granted" {
		t.Fatalf("action = %q", e.Action)
	}
	// The attestation is the whole point: it records the authority the grant
	// was made under.
	if e.Detail == "" {
		t.Fatal("audit record lost the attestation")
	}
	if e.Result != "success" {
		t.Fatalf("result = %q", e.Result)
	}
}

// The trail must be filterable, or it is unusable at any real volume.
func TestAuditTrailFiltersByEntity(t *testing.T) {
	a := newAuthHarness(t)
	req := mustRequest(t, "GET", "/admin/audit", "", nil)

	a.h.recordAudit(req, "voice_rights_updated", "voice", "v1", "x", "success")
	a.h.recordAudit(req, "user_suspended", "user", "u9", "y", "success")

	voices, _ := a.h.audio.AuditTrail(req.Context(), "voice", "", 50)
	for _, e := range voices {
		if e.Entity != "voice" {
			t.Fatalf("filter leaked a %q entry", e.Entity)
		}
	}
	all, _ := a.h.audio.AuditTrail(req.Context(), "", "", 50)
	if len(all) < 2 {
		t.Fatalf("unfiltered trail returned %d entries, want at least 2", len(all))
	}
}

func mustRequest(t *testing.T, method, path, token string, body any) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}
