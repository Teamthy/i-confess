package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Collections, devices, notifications and export (§35–§37, §45, §46, §49, §71).

func collectionID(t *testing.T, out map[string]any) string {
	t.Helper()
	id, _ := out["id"].(string)
	if id == "" {
		t.Fatalf("no collection id in response: %v", out)
	}
	return id
}

// A personal collection must never be public because a field was omitted (§37).
func TestCollectionDefaultsToPrivate(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "col1@test.com")

	code, out := a.do("POST", "/me/collections", token, map[string]any{"name": "Morning"})
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, out)
	}
	if out["visibility"] != "private" {
		t.Fatalf("visibility = %v, want private", out["visibility"])
	}

	// An unrecognised value must also fall back to private, not be stored.
	_, out = a.do("POST", "/me/collections", token, map[string]any{
		"name": "Odd", "visibility": "everyone",
	})
	if out["visibility"] != "private" {
		t.Fatalf("unknown visibility became %v", out["visibility"])
	}
}

func TestCollectionCRUD(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "col2@test.com")

	_, created := a.do("POST", "/me/collections", token, map[string]any{
		"name": "Evening", "description": "Before bed",
	})
	id := collectionID(t, created)

	code, out := a.do("PATCH", "/me/collections/"+id, token, map[string]any{
		"name": "Late Evening", "visibility": "unlisted",
	})
	if code != http.StatusOK {
		t.Fatalf("update: %d", code)
	}
	if out["name"] != "Late Evening" || out["visibility"] != "unlisted" {
		t.Fatalf("update did not apply: %v", out)
	}

	if code, _ := a.do("DELETE", "/me/collections/"+id, token, nil); code != http.StatusOK {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := a.do("GET", "/me/collections/"+id, token, nil); code != http.StatusNotFound {
		t.Fatalf("deleted collection still readable: %d", code)
	}
}

// The ownership test that matters: another user's collection must be
// unreachable, and reported as 404 rather than 403 so the id is not confirmed.
func TestCannotAccessAnotherUsersCollection(t *testing.T) {
	a := newAuthHarness(t)
	alice, _ := a.register(t, "alice-col@test.com")
	bob, _ := a.register(t, "bob-col@test.com")

	_, created := a.do("POST", "/me/collections", alice, map[string]any{"name": "Private"})
	id := collectionID(t, created)

	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{"GET", "/me/collections/" + id, nil},
		{"PATCH", "/me/collections/" + id, map[string]any{"name": "Hijacked"}},
		{"DELETE", "/me/collections/" + id, nil},
		{"POST", "/me/collections/" + id + "/items", map[string]any{"confession_id": "x"}},
	} {
		code, _ := a.do(tc.method, tc.path, bob, tc.body)
		if code == http.StatusOK || code == http.StatusCreated {
			t.Fatalf("%s %s: bob reached alice's collection (%d)", tc.method, tc.path, code)
		}
		if code == http.StatusForbidden {
			t.Fatalf("%s %s returned 403, which confirms the collection exists", tc.method, tc.path)
		}
	}

	// Alice's collection is intact.
	if code, _ := a.do("GET", "/me/collections/"+id, alice, nil); code != http.StatusOK {
		t.Fatalf("alice lost access to her own collection: %d", code)
	}
	// And Bob's list does not include it.
	_, list := a.doList("GET", "/me/collections", bob)
	if len(list) != 0 {
		t.Fatalf("bob's collection list contains %d entries", len(list))
	}
}

// Adding a nonexistent confession must be refused, or the collection fills with
// dangling references that break every later read.
func TestCollectionRejectsUnknownConfession(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "col3@test.com")
	_, created := a.do("POST", "/me/collections", token, map[string]any{"name": "X"})
	id := collectionID(t, created)

	code, out := a.do("POST", "/me/collections/"+id+"/items", token, map[string]any{
		"confession_id": "does-not-exist",
	})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422", code)
	}
	if out["code"] != "RESOURCE_NOT_FOUND" {
		t.Fatalf("code = %v", out["code"])
	}
}

func TestCollectionValidation(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "col4@test.com")

	for _, body := range []map[string]any{
		{"name": ""},
		{"name": "   "},
		{"name": strings.Repeat("x", 200)},
		{"name": "ok", "description": strings.Repeat("d", 600)},
	} {
		if code, _ := a.do("POST", "/me/collections", token, body); code == http.StatusCreated {
			t.Fatalf("invalid collection accepted: %v", body)
		}
	}
}

// ---------------------------------------------------------------------------
// Devices
// ---------------------------------------------------------------------------

func TestDeviceRegistrationIsIdempotent(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "dev1@test.com")

	body := map[string]any{
		"device_id": "device-abc", "platform": "ios",
		"device_name": "iPhone", "app_version": "1.2.0",
	}
	for i := 0; i < 3; i++ {
		if code, _ := a.do("POST", "/me/devices", token, body); code != http.StatusOK {
			t.Fatalf("register %d: %d", i, code)
		}
	}

	_, devices := a.doList("GET", "/me/devices", token)
	if len(devices) != 1 {
		t.Fatalf("re-registering created %d rows, want 1", len(devices))
	}
}

// A push token is a delivery credential and must not be echoed back.
func TestDeviceResponseDoesNotEchoPushToken(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "dev2@test.com")

	const push = "super-secret-push-token"
	a.do("POST", "/me/devices", token, map[string]any{
		"device_id": "d1", "platform": "android", "push_token": push,
	})

	req := httptest.NewRequest("GET", "/me/devices", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	a.router.ServeHTTP(rec, req)

	if strings.Contains(rec.Body.String(), push) {
		t.Fatalf("push token echoed back to the client: %s", rec.Body.String())
	}
}

func TestDeviceValidationAndScoping(t *testing.T) {
	a := newAuthHarness(t)
	alice, _ := a.register(t, "alice-dev@test.com")
	bob, _ := a.register(t, "bob-dev@test.com")

	if code, _ := a.do("POST", "/me/devices", alice, map[string]any{"platform": "ios"}); code != http.StatusBadRequest {
		t.Fatalf("missing device_id accepted: %d", code)
	}
	if code, _ := a.do("POST", "/me/devices", alice, map[string]any{
		"device_id": "d", "platform": "toaster",
	}); code != http.StatusBadRequest {
		t.Fatalf("bogus platform accepted: %d", code)
	}

	a.do("POST", "/me/devices", alice, map[string]any{"device_id": "alice-phone", "platform": "ios"})

	// Bob cannot revoke Alice's device.
	if code, _ := a.do("DELETE", "/me/devices/alice-phone", bob, nil); code == http.StatusOK {
		t.Fatal("bob revoked alice's device")
	}
	_, devices := a.doList("GET", "/me/devices", alice)
	if len(devices) != 1 {
		t.Fatalf("alice's device was removed: %d remain", len(devices))
	}
}

// ---------------------------------------------------------------------------
// Notification preferences
// ---------------------------------------------------------------------------

// Marketing is opt-in; functional notifications are on by default (§46).
func TestNotificationDefaultsAreOptInForMarketing(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "notif1@test.com")

	code, out := a.do("GET", "/me/notifications", token, nil)
	if code != http.StatusOK {
		t.Fatalf("get: %d", code)
	}
	prefs, _ := out["preferences"].(map[string]any)
	if prefs["product_updates"] != false {
		t.Fatalf("product_updates defaulted to %v; marketing must be opt-in", prefs["product_updates"])
	}
	if prefs["scheduled_sessions"] != true {
		t.Fatal("functional notifications should default on")
	}
}

func TestNotificationPreferencesPersist(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "notif2@test.com")

	code, out := a.do("PATCH", "/me/notifications", token, map[string]any{
		"recommendations": false, "product_updates": true,
	})
	if code != http.StatusOK {
		t.Fatalf("patch: %d %v", code, out)
	}
	if out["recommendations"] != false || out["product_updates"] != true {
		t.Fatalf("update not applied: %v", out)
	}

	// A partial update must not reset the untouched field.
	_, out2 := a.do("GET", "/me/notifications", token, nil)
	prefs, _ := out2["preferences"].(map[string]any)
	if prefs["recommendations"] != false {
		t.Fatal("setting did not persist")
	}
	if prefs["scheduled_sessions"] != true {
		t.Fatal("an untouched preference was reset")
	}
}

// ---------------------------------------------------------------------------
// Data export
// ---------------------------------------------------------------------------

// An export must contain the user's data and none of the security material
// protecting it: a leaked export must not be a credential (§49).
func TestExportContainsDataButNoSecrets(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "export@test.com")
	a.do("POST", "/me/collections", token, map[string]any{"name": "Exported"})
	a.do("POST", "/me/devices", token, map[string]any{"device_id": "d1", "platform": "ios"})

	req := httptest.NewRequest("GET", "/me/export", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	a.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("export: %d", rec.Code)
	}
	// Served as a download rather than rendered inline.
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Fatalf("Content-Disposition = %q, want an attachment", cd)
	}

	body := rec.Body.String()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"account", "profile", "preferences", "collections", "devices", "sign_in_methods"} {
		if _, ok := out[key]; !ok {
			t.Fatalf("export missing %q", key)
		}
	}

	// The credential material must be absent.
	for _, secret := range []string{"password_hash", "token_hash", "refresh_token", token} {
		if strings.Contains(body, secret) {
			t.Fatalf("export leaked %q", secret)
		}
	}
	// bcrypt hashes start with $2; none should appear anywhere.
	if strings.Contains(body, "$2a$") || strings.Contains(body, "$2b$") {
		t.Fatal("export contains a bcrypt hash")
	}
}

func TestExportRequiresAuth(t *testing.T) {
	a := newAuthHarness(t)
	if code, _ := a.do("GET", "/me/export", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated export: %d", code)
	}
}

// One user's export must never contain another user's data.
func TestExportIsScopedToCaller(t *testing.T) {
	a := newAuthHarness(t)
	alice, _ := a.register(t, "alice-exp@test.com")
	bob, _ := a.register(t, "bob-exp@test.com")

	a.do("POST", "/me/collections", alice, map[string]any{"name": "AliceOnlyCollection"})

	req := httptest.NewRequest("GET", "/me/export", nil)
	req.Header.Set("Authorization", "Bearer "+bob)
	rec := httptest.NewRecorder()
	a.router.ServeHTTP(rec, req)

	if strings.Contains(rec.Body.String(), "AliceOnlyCollection") {
		t.Fatal("bob's export contains alice's collection")
	}
	if strings.Contains(rec.Body.String(), "alice-exp@test.com") {
		t.Fatal("bob's export contains alice's email")
	}
}

// doList issues a request expecting a JSON array.
func (a *authHarness) doList(method, path, token string) (int, []any) {
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	a.router.ServeHTTP(rec, req)

	var out []any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}
