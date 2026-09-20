package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/playapi"
)

// Tests for Play real-time developer notifications (IC-003, PR B).
//
// The property under test is that a notification is a reason to ask Google, not
// an instruction to believe. Nothing in the Pub/Sub payload is authenticated to
// this server beyond the push token and the audience, and the payload itself
// carries no state at all - so every test here arranges for the API's answer to
// disagree with what the message implies, and asserts the API wins.

func newGoogleNotificationVerifier(t *testing.T, client PlayPurchaseClient, mutate func(*GoogleNotificationConfig)) *GoogleNotificationVerifier {
	t.Helper()

	cfg := GoogleNotificationConfig{
		PackageName: testPlayPackage,
		Plans: map[string]string{
			testPlayMonthly: "monthly",
			testPlayAnnual:  "annual",
		},
		Purchases: client,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	verifier, err := NewGoogleNotificationVerifier(cfg)
	if err != nil {
		t.Fatalf("build notification resolver: %v", err)
	}
	return verifier
}

func googleInput(mutate func(*GoogleNotificationInput)) GoogleNotificationInput {
	in := GoogleNotificationInput{
		NotificationID: "pubsub-message-1",
		PackageName:    testPlayPackage,
		PurchaseToken:  testPlayToken,
		Type:           playapi.NotificationRenewed,
		EventTime:      time.Now().UTC(),
	}
	if mutate != nil {
		mutate(&in)
	}
	return in
}

// TestGoogleNotificationBelievesGoogleNotTheMessage is the core property.
//
// The message says a subscription was renewed. Google's API says it expired a
// week ago. A resolver that trusted the message grants premium to anyone who
// can post to the endpoint; one that asks Google does not.
func TestGoogleNotificationBelievesGoogleNotTheMessage(t *testing.T) {
	client := &fakePlay{purchase: purchase(playapi.StateExpired, testPlayMonthly,
		time.Now().Add(-7*24*time.Hour), nil)}

	note, err := newGoogleNotificationVerifier(t, client, nil).Resolve(context.Background(),
		googleInput(func(in *GoogleNotificationInput) { in.Type = playapi.NotificationRenewed }))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if note.Verification.Valid {
		t.Fatal("a renewal message about an expired subscription granted entitlement")
	}
	if note.Verification.State != "expired" {
		t.Errorf("state = %q, want expired", note.Verification.State)
	}
	if got := client.tokens(); len(got) != 1 || got[0] != testPlayToken {
		t.Errorf("asked Google about %v, want the notification's purchase token", got)
	}
}

// TestGoogleNotificationAppliesRevocationEvenWhenTheAPILags: Google documents
// that the subscription state can still read active when a refund is
// announced. Waiting for the API to catch up leaves a refunded subscription
// entitled - which is a customer keeping access they were refunded for.
func TestGoogleNotificationAppliesRevocationEvenWhenTheAPILags(t *testing.T) {
	client := &fakePlay{purchase: purchase(playapi.StateActive, testPlayMonthly,
		time.Now().Add(20*24*time.Hour), nil)}

	note, err := newGoogleNotificationVerifier(t, client, nil).Resolve(context.Background(),
		googleInput(func(in *GoogleNotificationInput) { in.Type = playapi.NotificationRevoked }))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if note.Verification.Valid {
		t.Fatal("a revoked purchase is still reported as valid")
	}
	if note.Verification.State != "refunded" {
		t.Errorf("state = %q, want refunded", note.Verification.State)
	}
}

func TestGoogleNotificationAcceptsARenewal(t *testing.T) {
	expires := time.Now().Add(25 * 24 * time.Hour)
	client := &fakePlay{purchase: purchase(playapi.StateActive, testPlayAnnual, expires, nil)}

	note, err := newGoogleNotificationVerifier(t, client, nil).Resolve(context.Background(), googleInput(nil))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !note.Applies || !note.Verification.Valid {
		t.Fatalf("note = %+v, want an applied, entitling renewal", note)
	}
	if note.PurchaseToken != testPlayToken {
		t.Errorf("purchase token = %q", note.PurchaseToken)
	}
	if note.TypeName != "SUBSCRIPTION_RENEWED" {
		t.Errorf("type name = %q", note.TypeName)
	}
	if note.Verification.ExpiresAt == "" {
		t.Error("no expiry was carried through")
	}
}

// TestGoogleNotificationNeverWritesAnotherApplication: the endpoint is open to
// the internet, so a notification about a different package must not resolve.
func TestGoogleNotificationNeverWritesAnotherApplication(t *testing.T) {
	client := &fakePlay{purchase: purchase(playapi.StateActive, testPlayMonthly, time.Now().Add(time.Hour), nil)}

	_, err := newGoogleNotificationVerifier(t, client, nil).Resolve(context.Background(),
		googleInput(func(in *GoogleNotificationInput) { in.PackageName = "com.attacker.app" }))
	if !errors.Is(err, ErrInvalidReceipt) {
		t.Fatalf("err = %v, want ErrInvalidReceipt", err)
	}
	if client.calls() != 0 {
		t.Error("a notification for another package was still sent to Google")
	}
}

// TestGoogleNotificationIgnoresWhatItCannotActOn: a console test notification
// carries no purchase, and an unknown numeric type is a type this server has
// never seen. Neither may change entitlement.
func TestGoogleNotificationIgnoresWhatItCannotActOn(t *testing.T) {
	client := &fakePlay{purchase: purchase(playapi.StateActive, testPlayMonthly, time.Now().Add(time.Hour), nil)}
	verifier := newGoogleNotificationVerifier(t, client, nil)

	testNote, err := verifier.Resolve(context.Background(), googleInput(func(in *GoogleNotificationInput) {
		in.Test = true
		in.PurchaseToken = ""
	}))
	if err != nil {
		t.Fatalf("test notification: %v", err)
	}
	if testNote.Applies {
		t.Error("a console test notification was applied to entitlement")
	}

	unknown, err := verifier.Resolve(context.Background(), googleInput(func(in *GoogleNotificationInput) {
		in.Type = 999
	}))
	if err != nil {
		t.Fatalf("unknown type: %v", err)
	}
	if unknown.Applies {
		t.Error("an unknown notification type was applied to entitlement")
	}
	if unknown.Detail == "" {
		t.Error("no detail explaining why nothing changed")
	}
	if client.calls() != 0 {
		t.Error("an informational notification still called Google")
	}
}

// TestGoogleNotificationKeepsTheCallersFaultApartFromOurs: a bad token is the
// user's problem and answers 400; a broken deployment is ours and answers 503,
// so a missing service account does not look like a stream of fraudulent users.
func TestGoogleNotificationKeepsTheCallersFaultApartFromOurs(t *testing.T) {
	cases := map[string]struct {
		err  error
		want error
	}{
		"bad token":     {err: playapi.ErrTokenInvalid, want: ErrInvalidReceipt},
		"unauthorised":  {err: playapi.ErrUnauthorized, want: ErrProviderError},
		"unavailable":   {err: playapi.ErrUnavailable, want: ErrProviderError},
		"wrapped token": {err: errors.New("play: " + playapi.ErrTokenInvalid.Error()), want: ErrProviderError},
	}
	for name, tc := range cases {
		client := &fakePlay{err: tc.err}
		_, err := newGoogleNotificationVerifier(t, client, nil).Resolve(context.Background(), googleInput(nil))
		if tc.want == ErrInvalidReceipt {
			if !errors.Is(err, ErrInvalidReceipt) {
				t.Errorf("%s: err = %v, want ErrInvalidReceipt", name, err)
			}
			continue
		}
		if errors.Is(err, ErrInvalidReceipt) {
			t.Errorf("%s: err = %v reported the caller's receipt as bad", name, err)
		}
	}
}

func TestGoogleNotificationRequiresAToken(t *testing.T) {
	client := &fakePlay{purchase: purchase(playapi.StateActive, testPlayMonthly, time.Now().Add(time.Hour), nil)}
	_, err := newGoogleNotificationVerifier(t, client, nil).Resolve(context.Background(),
		googleInput(func(in *GoogleNotificationInput) { in.PurchaseToken = "" }))
	if !errors.Is(err, ErrInvalidReceipt) {
		t.Fatalf("err = %v, want ErrInvalidReceipt", err)
	}
}

func TestGoogleNotificationDecodesKnownTypes(t *testing.T) {
	names := map[int]string{
		playapi.NotificationPurchased:     "SUBSCRIPTION_PURCHASED",
		playapi.NotificationInGracePeriod: "SUBSCRIPTION_IN_GRACE_PERIOD",
		playapi.NotificationOnHold:        "SUBSCRIPTION_ON_HOLD",
		playapi.NotificationExpired:       "SUBSCRIPTION_EXPIRED",
		playapi.NotificationPaused:        "SUBSCRIPTION_PAUSED",
	}
	for kind, want := range names {
		if got := playapi.NotificationTypeName(kind); got != want {
			t.Errorf("type %d = %q, want %q", kind, got, want)
		}
	}
}

func TestNewGoogleNotificationVerifierRefusesIncompleteConfiguration(t *testing.T) {
	client := &fakePlay{purchase: purchase(playapi.StateActive, testPlayMonthly, time.Now().Add(time.Hour), nil)}

	if _, err := NewGoogleNotificationVerifier(GoogleNotificationConfig{
		Plans: map[string]string{testPlayMonthly: "monthly"}, Purchases: client,
	}); !errors.Is(err, ErrUnconfigured) {
		t.Errorf("missing package: err = %v, want ErrUnconfigured", err)
	}
	if _, err := NewGoogleNotificationVerifier(GoogleNotificationConfig{
		PackageName: testPlayPackage, Purchases: client,
	}); !errors.Is(err, ErrUnconfigured) {
		t.Errorf("missing product map: err = %v, want ErrUnconfigured", err)
	}
	if _, err := NewGoogleNotificationVerifier(GoogleNotificationConfig{
		PackageName: testPlayPackage, Plans: map[string]string{testPlayMonthly: "monthly"},
	}); !errors.Is(err, ErrUnconfigured) {
		t.Errorf("missing client: err = %v, want ErrUnconfigured", err)
	}
}

// TestPlayAcknowledgerFromEnvWithoutCredentials: development has no store
// credentials by design, and the caller must be able to tell "nothing to
// acknowledge with" apart from "the store refused it".
func TestPlayAcknowledgerFromEnvWithoutCredentials(t *testing.T) {
	t.Setenv("ENV", "development")
	t.Setenv(EnvGoogleKeyPath, "")
	t.Setenv(EnvFCMServiceAccount, "")
	t.Setenv(EnvGooglePackage, "")

	ack, err := PlayAcknowledgerFromEnv()
	if err != nil {
		t.Fatalf("development without credentials: err = %v, want nil", err)
	}
	if ack != nil {
		t.Error("an acknowledger was built without a service account")
	}
}

// Outside development and test, a missing acknowledger is an error rather than
// a silent no-op: unacknowledged Play purchases are refunded three days later,
// and a log line does not stop that.
func TestPlayAcknowledgerFromEnvIsAnErrorInProduction(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv(EnvGoogleKeyPath, "")
	t.Setenv(EnvFCMServiceAccount, "")
	t.Setenv(EnvGooglePackage, "")

	if _, err := PlayAcknowledgerFromEnv(); !errors.Is(err, ErrUnconfigured) {
		t.Fatalf("err = %v, want ErrUnconfigured", err)
	}
}
