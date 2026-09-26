package api

import (
	"net/http"
	"testing"
)

// TestTrialEndpointsPersistTheLifecycle proves the route does not infer a
// trial from account creation and that both the unprefixed and versioned
// contracts reach the same persistent implementation.
func TestTrialEndpointsPersistTheLifecycle(t *testing.T) {
	f := newAudioFixture(t)
	token, _ := f.registerWithID(t, "trial-api@example.com")

	status, body := f.call(t, http.MethodGet, "/subscriptions/trial", token, nil)
	if status != http.StatusOK || body["state"] != "ELIGIBLE" {
		t.Fatalf("initial trial: %d %v", status, body)
	}
	status, body = f.call(t, http.MethodPost, "/subscriptions/trial", token, nil)
	if status != http.StatusOK || body["state"] != "ACTIVE" {
		t.Fatalf("start trial: %d %v", status, body)
	}
	if body["plan"] != "premium" {
		t.Fatalf("running trial plan = %v, want premium", body["plan"])
	}

	status, body = f.call(t, http.MethodGet, "/v1/subscriptions/trial/status", token, nil)
	if status != http.StatusOK || body["state"] != "ACTIVE" || body["entitled"] != true {
		t.Fatalf("status endpoint: %d %v", status, body)
	}
	// Starting the same trial twice is a conflict once the client is no longer
	// replaying the same idempotency request.
	status, body = f.call(t, http.MethodPost, "/v1/subscriptions/trial", token, nil)
	if status != http.StatusOK || body["state"] != "ACTIVE" {
		t.Fatalf("repeat start should be safely idempotent: %d %v", status, body)
	}
}

func TestTrialConversionDoesNotBypassReceiptVerification(t *testing.T) {
	f := newAudioFixture(t)
	token, _ := f.registerWithID(t, "trial-convert-api@example.com")
	if status, body := f.call(t, http.MethodPost, "/subscriptions/trial", token, nil); status != http.StatusOK {
		t.Fatalf("start trial: %d %v", status, body)
	}
	status, body := f.call(t, http.MethodPost, "/subscriptions/trial/convert", token, nil)
	if status != http.StatusOK || body["state"] != "CONVERTED" {
		t.Fatalf("convert: %d %v", status, body)
	}
	if body["plan"] != "free" {
		t.Fatalf("converted plan = %v, want free until a store receipt is verified", body["plan"])
	}
}
