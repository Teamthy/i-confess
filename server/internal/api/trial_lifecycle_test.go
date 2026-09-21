package api

import (
	"net/http"
	"testing"
)

// §36 as an API surface (G-3). The state machine and its CAS writes are
// tested in internal/billing and internal/store; what this file pins is what
// the three endpoints owe the client: an honest "you are eligible", a claim
// that says "started" with a day, a second claim refused with the state that
// made it impossible, a legacy PATCH that cannot jump backwards, and a
// verified receipt that closes the trial as converted without the client
// being trusted to say so.

func TestTrialLifecycleAPI(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "trial-api@test.com")

	if code, _ := a.do("GET", "/subscriptions/trial/lifecycle", "", nil); code != http.StatusUnauthorized {
		t.Errorf("unauthenticated lifecycle: got %d, want 401", code)
	}

	// First read materialises the record. Before PHASE 35 there was no
	// record to read: the "trial" was arithmetic on created_at.
	code, body := a.do("GET", "/subscriptions/trial/lifecycle", token, nil)
	if code != http.StatusOK {
		t.Fatalf("lifecycle: %d %v", code, body)
	}
	if body["status"] != "eligible" || body["day"].(float64) != 0 {
		t.Fatalf("fresh lifecycle = %v, want eligible day 0", body)
	}
	if _, ok := body["id"].(string); !ok || body["id"] == "" {
		t.Errorf("lifecycle must name the record it read: %v", body)
	}

	// The legacy journey endpoint now answers from the record: an account
	// that has not claimed is on no day. (Age-derived days made every new
	// account a trialist, which is the illusion G-3 named.)
	code, body = a.do("GET", "/subscriptions/trial", token, nil)
	if code != http.StatusOK || body["current_day"].(float64) != 0 {
		t.Fatalf("journey before claim: %d %v, want day 0", code, body)
	}
	if journey, ok := body["journey"].([]any); !ok || len(journey) != 7 {
		t.Errorf("journey should still list all seven days: %v", body["journey"])
	}

	// The claim.
	code, body = a.do("POST", "/subscriptions/trial/start", token, nil)
	if code != http.StatusOK {
		t.Fatalf("start: %d %v", code, body)
	}
	if s := body["status"]; s != "started" && s != "active" {
		t.Errorf("after start status = %v, want started (or active, if anything read it first)", s)
	}
	if body["day"].(float64) != 1 {
		t.Errorf("day right after the claim = %v, want 1", body["day"])
	}
	if body["ends_at"] == "" || body["ends_at"] == nil {
		t.Errorf("the claim must hand back the clock it set: %v", body)
	}

	// A second claim: refused, and told why in terms a client can show.
	code, body = a.do("POST", "/subscriptions/trial/start", token, nil)
	if code != http.StatusConflict {
		t.Fatalf("second start: %d %v", code, body)
	}
	if body["code"] != "TRIAL_ALREADY_USED" || body["status"] == "" {
		t.Errorf("second start refusal = %v, want TRIAL_ALREADY_USED naming the state", body)
	}

	// The legacy PATCH: the graph binds it in both directions.
	if code, body := a.do("PATCH", "/subscriptions/trial", token,
		map[string]any{"status": "eligible"}); code != http.StatusConflict {
		t.Errorf("backwards PATCH: %d %v, want 409", code, body)
	} else if body["status"] == "" || body["allowed"] == nil {
		t.Errorf("409 must say where the trial is and where it may go: %v", body)
	}
	if code, _ := a.do("PATCH", "/subscriptions/trial", token,
		map[string]any{"status": "not-a-state"}); code != http.StatusBadRequest {
		t.Errorf("unknown status: %d, want 400", code)
	}
	code, body = a.do("PATCH", "/subscriptions/trial", token, map[string]any{"status": "expiring"})
	if code != http.StatusOK || body["status"] != "expiring" {
		t.Errorf("forward PATCH: %d %v, want 200 expiring", code, body)
	}

	// A fresh journey read after the claim shows the clock, not the age.
	if code, body = a.do("GET", "/subscriptions/trial", token, nil); code != http.StatusOK ||
		body["current_day"].(float64) != 1 {
		t.Errorf("journey after claim: %d %v, want day 1", code, body)
	}
}

func TestTrialConversionFollowsVerifiedReceipt(t *testing.T) {
	a := newAuthHarness(t)

	// Buyer one claims, then pays: the receipt is what closes the trial.
	token, _ := a.register(t, "trial-convert@test.com")
	if code, body := a.do("POST", "/subscriptions/trial/start", token, nil); code != http.StatusOK {
		t.Fatalf("start: %d %v", code, body)
	}
	code, body := a.do("POST", "/subscriptions/verify", token,
		map[string]any{"provider": "apple", "receipt": "valid_monthly_trialconvert"})
	if code != http.StatusOK || body["verified"] != true {
		t.Fatalf("verify: %d %v", code, body)
	}
	code, body = a.do("GET", "/subscriptions/trial/lifecycle", token, nil)
	if code != http.StatusOK {
		t.Fatalf("lifecycle after purchase: %d", code)
	}
	if body["status"] != "converted" || body["converted_at"] == "" || body["converted_at"] == nil {
		t.Fatalf("after purchase lifecycle = %v, want converted with a stamp", body)
	}
	// The journey is over: day 0, and the paywall has nothing left to claim.
	if code, body := a.do("GET", "/subscriptions/trial", token, nil); code != http.StatusOK ||
		body["current_day"].(float64) != 0 {
		t.Errorf("journey after conversion: %d %v, want day 0", code, body)
	}

	// Buyer two pays without ever claiming: no trial is marked converted,
	// because none converted — but the account is premium, so the claim is
	// refused for the ordinary reason.
	token2, _ := a.register(t, "trial-direct@test.com")
	if code, body := a.do("POST", "/subscriptions/verify", token2,
		map[string]any{"provider": "google", "receipt": "valid_annual_traildirect"}); code != http.StatusOK ||
		body["verified"] != true {
		t.Fatalf("verify direct buyer: %d %v", code, body)
	}
	if code, body := a.do("GET", "/subscriptions/trial/lifecycle", token2, nil); code != http.StatusOK ||
		body["status"] != "eligible" {
		t.Errorf("unclaimed trial after a direct purchase: %d %v, want still eligible", code, body)
	}
	if code, body := a.do("POST", "/subscriptions/trial/start", token2, nil); code != http.StatusConflict ||
		body["code"] != "TRIAL_UNAVAILABLE" {
		t.Errorf("premium start: %d %v, want 409 TRIAL_UNAVAILABLE", code, body)
	}
}
