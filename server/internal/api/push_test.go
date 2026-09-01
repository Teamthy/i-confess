package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/push"
)

// Push registration and end-to-end scheduled delivery (§45, §47, §19).

// The bug this closes: the API accepted push_token and silently discarded it,
// so a schedule had nowhere to deliver.
func TestPushTokenIsPersisted(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "push1@test.com")

	code, out := a.do("POST", "/me/devices", token, map[string]any{
		"device_id": "d1", "platform": "ios",
		"device_name": "iPhone", "push_token": "apns-token-abc",
	})
	if code != http.StatusOK {
		t.Fatalf("register device: %d %v", code, out)
	}
	if out["push_enabled"] != true {
		t.Fatalf("server did not acknowledge the push token: %v", out)
	}

	// It must be readable by the delivery path...
	targets, err := a.h.library.PushTargetsFor(context.Background(), currentUserID(t, a, token))
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].Token != "apns-token-abc" {
		t.Fatalf("token not stored for delivery: %+v", targets)
	}

	// ...and still never echoed to a client.
	req := httptest.NewRequest("GET", "/me/devices", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	a.router.ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), "apns-token-abc") {
		t.Fatalf("push token echoed to the client: %s", rec.Body.String())
	}
}

// The whole point of the slice: a schedule reaches a real device.
func TestScheduleDeliversToRegisteredDevice(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "push2@test.com")

	sender := push.NewMemorySender()
	a.h.SetPushSender(sender)

	a.do("POST", "/me/devices", token, map[string]any{
		"device_id": "phone", "platform": "ios", "push_token": "tok-live",
	})

	// A schedule one minute from now, in the user's own timezone.
	loc, err := time.LoadLocation("Africa/Lagos")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	fire := time.Now().In(loc).Add(time.Minute).Truncate(time.Minute)
	code, out := a.do("POST", "/schedules", token, map[string]any{
		"label": "Morning declaration", "time": fire.Format("15:04"),
		"days_of_week":     []int{1, 2, 3, 4, 5, 6, 7},
		"timezone":         "Africa/Lagos",
		"duration_seconds": 900,
	})
	if code != http.StatusCreated {
		t.Fatalf("create schedule: %d %v", code, out)
	}

	// Drive the sweep deterministically rather than waiting on the ticker.
	a.h.dispatcher.Now = func() time.Time { return fire.Add(10 * time.Second).UTC() }
	rep, err := a.h.RunScheduleSweep(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Sent != 1 {
		t.Fatalf("schedule did not deliver: %+v", rep)
	}

	msgs := sender.Sent()
	if len(msgs) != 1 {
		t.Fatalf("delivered %d notifications, want 1", len(msgs))
	}
	if msgs[0].Token != "tok-live" {
		t.Fatalf("wrong token: %q", msgs[0].Token)
	}
	if !strings.Contains(msgs[0].Title, "Morning declaration") {
		t.Fatalf("notification does not carry the schedule label: %q", msgs[0].Title)
	}
	// Tapping must open the session.
	if !strings.Contains(msgs[0].Data["deeplink"], "/start") {
		t.Fatalf("no actionable deep link: %v", msgs[0].Data)
	}
	// Nothing sensitive belongs in a payload that surfaces on a lock screen.
	if strings.Contains(strings.ToLower(msgs[0].Body), "push2@test.com") {
		t.Fatal("notification body leaks the user's email")
	}
}

// Turning schedule notifications off must actually stop them.
func TestDisablingScheduleNotificationsStopsDelivery(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "push3@test.com")

	sender := push.NewMemorySender()
	a.h.SetPushSender(sender)

	a.do("POST", "/me/devices", token, map[string]any{
		"device_id": "phone", "platform": "android", "push_token": "tok-x",
	})
	a.do("PATCH", "/me/notifications", token, map[string]any{"scheduled_sessions": false})

	loc, err := time.LoadLocation("Africa/Lagos")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	fire := time.Now().In(loc).Add(time.Minute).Truncate(time.Minute)
	a.do("POST", "/schedules", token, map[string]any{
		"label": "Quiet", "time": fire.Format("15:04"),
		"days_of_week": []int{1, 2, 3, 4, 5, 6, 7}, "timezone": "Africa/Lagos",
		"duration_seconds": 600,
	})

	a.h.dispatcher.Now = func() time.Time { return fire.Add(10 * time.Second).UTC() }
	rep, _ := a.h.RunScheduleSweep(context.Background())

	if len(sender.Sent()) != 0 {
		t.Fatal("notified a user who turned schedule reminders off")
	}
	if rep.Skipped != 1 {
		t.Fatalf("opt-out not recorded as skipped: %+v", rep)
	}
}

// A sweep that runs twice must not wake someone twice.
func TestRepeatedSweepDoesNotDoubleNotify(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "push4@test.com")

	sender := push.NewMemorySender()
	a.h.SetPushSender(sender)
	a.do("POST", "/me/devices", token, map[string]any{
		"device_id": "phone", "platform": "ios", "push_token": "tok-1",
	})

	loc, err := time.LoadLocation("Africa/Lagos")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	fire := time.Now().In(loc).Add(time.Minute).Truncate(time.Minute)
	a.do("POST", "/schedules", token, map[string]any{
		"label": "Daily", "time": fire.Format("15:04"),
		"days_of_week": []int{1, 2, 3, 4, 5, 6, 7}, "timezone": "Africa/Lagos",
		"duration_seconds": 600,
	})

	at := fire.Add(10 * time.Second).UTC()
	a.h.dispatcher.Now = func() time.Time { return at }

	a.h.RunScheduleSweep(context.Background())
	// Reset the window as a restart or a second replica would.
	a.h.dispatcher.ResetWindow(fire.Add(-time.Minute).UTC())
	a.h.RunScheduleSweep(context.Background())

	if n := len(sender.Sent()); n != 1 {
		t.Fatalf("user was notified %d times for one occurrence", n)
	}
}

// A device with no push token must not break the sweep for others.
func TestDeviceWithoutTokenIsSkipped(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "push5@test.com")

	sender := push.NewMemorySender()
	a.h.SetPushSender(sender)
	a.do("POST", "/me/devices", token, map[string]any{
		"device_id": "browser", "platform": "web",
	})

	targets, _ := a.h.library.PushTargetsFor(context.Background(), currentUserID(t, a, token))
	if len(targets) != 0 {
		t.Fatalf("a device with no token was listed as deliverable: %+v", targets)
	}
}

// currentUserID reads the caller's id from /me.
func currentUserID(t *testing.T, a *authHarness, token string) string {
	t.Helper()
	_, out := a.do("GET", "/me", token, nil)
	if u, ok := out["user"].(map[string]any); ok {
		if id, ok := u["id"].(string); ok {
			return id
		}
	}
	if id, ok := out["id"].(string); ok {
		return id
	}
	t.Fatalf("could not read user id from /me: %v", out)
	return ""
}
