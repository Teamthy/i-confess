package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

// GET /entitlements and GET /subscription answered 501 with a comment claiming
// their packages had "no constructors". Both packages existed and were already
// used elsewhere in this package. These tests pin the behaviour the handlers now
// have, and in particular the boundary between plans, which is the thing a
// client gets wrong if the response is ambiguous.

func getJSON(t *testing.T, f *audioFixture, path, token string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, f.srv.URL+path, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = res.Body.Close() }()

	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return res.StatusCode, body
}

func TestEntitlementsEndpointReportsTheFreePlan(t *testing.T) {
	f := newAudioFixture(t)

	status, body := getJSON(t, f, "/entitlements", f.freeTok)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %v)", status, body)
	}

	if body["plan"] != "free" {
		t.Errorf("plan = %v, want free", body["plan"])
	}
	if body["premium_voices"] != false {
		t.Errorf("premium_voices = %v, want false on the free plan", body["premium_voices"])
	}

	// Free is 15 minutes. Not zero - the free tier is meant to be usable - and
	// not the premium ceiling either.
	maxSec, ok := body["max_session_seconds"].(float64)
	if !ok {
		t.Fatalf("max_session_seconds missing or not a number: %v", body["max_session_seconds"])
	}
	if maxSec != 15*60 {
		t.Errorf("max_session_seconds = %v, want 900", maxSec)
	}
	if _, ok := body["playback_ttl_seconds"]; !ok {
		t.Error("playback_ttl_seconds missing - the client needs it to know when to re-request a signed URL")
	}
}

func TestEntitlementsEndpointReportsThePremiumPlan(t *testing.T) {
	f := newAudioFixture(t)

	status, body := getJSON(t, f, "/entitlements", f.premTok)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %v)", status, body)
	}
	if body["plan"] != "premium" {
		t.Errorf("plan = %v, want premium", body["plan"])
	}
	if body["premium_voices"] != true {
		t.Errorf("premium_voices = %v, want true on the premium plan", body["premium_voices"])
	}
	if got := body["max_session_seconds"].(float64); got != 3*3600 {
		t.Errorf("max_session_seconds = %v, want 10800", got)
	}
}

func TestEntitlementsRequiresAuthentication(t *testing.T) {
	f := newAudioFixture(t)

	// Without a token the route is rejected before the handler runs, so a
	// handler that forgot to check would still be safe. The assertion is that
	// the endpoint never leaks entitlements anonymously.
	status, _ := getJSON(t, f, "/entitlements", "")
	if status == http.StatusOK {
		t.Error("GET /entitlements returned 200 without a token")
	}
}

func TestSubscriptionEndpointReportsPlanAndStatus(t *testing.T) {
	f := newAudioFixture(t)

	status, body := getJSON(t, f, "/subscription", f.premTok)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %v)", status, body)
	}

	if body["plan"] != "premium" {
		t.Errorf("plan = %v, want premium", body["plan"])
	}
	// Status is reported separately from plan because a row can exist with an
	// expired status. A client that reads only "plan" would show Premium for a
	// lapsed subscription, which the user reports as a bug.
	if body["status"] != "active" {
		t.Errorf("status = %v, want active", body["status"])
	}
	if body["active"] != true {
		t.Errorf("active = %v, want true", body["active"])
	}
}

func TestSubscriptionReportsFreeForAUserWithNoRow(t *testing.T) {
	f := newAudioFixture(t)

	status, body := getJSON(t, f, "/subscription", f.freeTok)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %v)", status, body)
	}
	if body["plan"] != "free" {
		t.Errorf("plan = %v, want free for a user with no subscription row", body["plan"])
	}
	if body["active"] != false {
		t.Errorf("active = %v, want false", body["active"])
	}
}

// TestBothPrefixesServeTheNewHandlers covers the G-8 failure mode: an endpoint
// implemented under one prefix and left as a stub under the other.
func TestBothPrefixesServeTheNewHandlers(t *testing.T) {
	f := newAudioFixture(t)

	for _, path := range []string{"/entitlements", "/v1/entitlements", "/subscription", "/v1/subscription"} {
		status, body := getJSON(t, f, path, f.premTok)
		if status != http.StatusOK {
			t.Errorf("%s: status = %d, want 200 (body: %v)", path, status, body)
			continue
		}
		if code, _ := body["code"].(string); code == "NOT_IMPLEMENTED" {
			t.Errorf("%s still returns NOT_IMPLEMENTED", path)
		}
	}
}
