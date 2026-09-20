package playapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Tests for Play acknowledgement (IC-003, PR B).
//
// Play refunds a purchase that is not acknowledged within three days. Detecting
// that condition and logging it - which is what the server did before - does
// not stop the refund: the money goes back and the customer keeps the
// entitlement. These tests cover the call that stops it, and the two failure
// modes that must be told apart: "the store refused this purchase" and "our
// credentials are wrong".

func acknowledgeServer(t *testing.T, status int, body string, wantPath string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if wantPath != "" && r.URL.Path != wantPath {
			t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q", got)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func acknowledgeClient(srv *httptest.Server) *Client {
	return &Client{
		PackageName: "app.iconfess",
		BaseURL:     srv.URL,
		TokenSource: func(context.Context) (string, error) { return "test-token", nil },
	}
}

func TestAcknowledgePostsToThePurchaseEndpoint(t *testing.T) {
	// The acknowledge endpoint is addressed by product and token; the v2 read
	// is addressed by token alone. Getting this wrong is a 404 in production,
	// three days after the sale.
	want := "/androidpublisher/v3/applications/app.iconfess/purchases/subscriptions/premium_monthly/tokens/purchase-token-abc:acknowledge"
	srv := acknowledgeServer(t, http.StatusNoContent, "", want)

	if err := acknowledgeClient(srv).Acknowledge(context.Background(), "premium_monthly", "purchase-token-abc"); err != nil {
		t.Fatalf("Acknowledge: %v", err)
	}
}

func TestAcknowledgeTreatsAnAlreadyAcknowledgedPurchaseAsSuccess(t *testing.T) {
	srv := acknowledgeServer(t, http.StatusBadRequest,
		`{"error":{"code":400,"message":"The purchase has already been acknowledged."}}`, "")

	if err := acknowledgeClient(srv).Acknowledge(context.Background(), "premium_monthly", "token"); err != nil {
		t.Fatalf("err = %v, want nil: an already-acknowledged purchase is not a failure", err)
	}
}

func TestAcknowledgeSeparatesOurFaultFromThePurchase(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
		want   error
	}{
		"bad token":       {status: http.StatusBadRequest, body: `{"error":{"message":"Invalid token."}}`, want: ErrTokenInvalid},
		"gone":            {status: http.StatusGone, body: ``, want: ErrTokenInvalid},
		"not found":       {status: http.StatusNotFound, body: ``, want: ErrTokenInvalid},
		"credentials":     {status: http.StatusForbidden, body: `{"error":{"message":"insufficient permissions"}}`, want: ErrUnauthorized},
		"unauthenticated": {status: http.StatusUnauthorized, body: ``, want: ErrUnauthorized},
		"outage":          {status: http.StatusInternalServerError, body: ``, want: ErrUnavailable},
		"rate limited":    {status: http.StatusTooManyRequests, body: ``, want: ErrUnavailable},
	}
	for name, tc := range cases {
		srv := acknowledgeServer(t, tc.status, tc.body, "")
		err := acknowledgeClient(srv).Acknowledge(context.Background(), "premium_monthly", "token")
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
}

func TestAcknowledgeRefusesIncompleteArguments(t *testing.T) {
	srv := acknowledgeServer(t, http.StatusNoContent, "", "")
	client := acknowledgeClient(srv)

	if err := client.Acknowledge(context.Background(), "", "token"); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("no product id: err = %v, want ErrTokenInvalid", err)
	}
	if err := client.Acknowledge(context.Background(), "premium_monthly", ""); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("no token: err = %v, want ErrTokenInvalid", err)
	}
}
