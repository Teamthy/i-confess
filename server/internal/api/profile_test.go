package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Teamthy/i-confess/internal/models"
)

// Profile domain tests (PRD §6, §7, §8, §12, §15, §16, §56, §73, §89, §90).

func TestProfileCreatedLazilyWithSaneDefaults(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "prof@test.com")

	code, out := a.do("GET", "/me/profile", token, nil)
	if code != http.StatusOK {
		t.Fatalf("get profile: %d", code)
	}
	if out["timezone"] == "" || out["language"] == "" {
		t.Fatalf("profile missing defaults: %v", out)
	}
}

func TestProfilePartialUpdateDoesNotClobber(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "partial@test.com")

	if code, _ := a.do("PATCH", "/me/profile", token, map[string]any{
		"display_name": "Adaeze Okonkwo", "bio": "Walking in peace.",
	}); code != http.StatusOK {
		t.Fatal("initial update failed")
	}

	// Updating only the timezone must leave the name and bio intact.
	code, out := a.do("PATCH", "/me/profile", token, map[string]any{"timezone": "Africa/Lagos"})
	if code != http.StatusOK {
		t.Fatalf("timezone update: %d %v", code, out)
	}
	if out["display_name"] != "Adaeze Okonkwo" {
		t.Fatalf("display_name was clobbered: %v", out["display_name"])
	}
	if out["bio"] != "Walking in peace." {
		t.Fatalf("bio was clobbered: %v", out["bio"])
	}

	// An explicit empty string must clear the field.
	_, out = a.do("PATCH", "/me/profile", token, map[string]any{"bio": ""})
	if out["bio"] != nil && out["bio"] != "" {
		t.Fatalf("explicit clear ignored: %v", out["bio"])
	}
}

// Timezone must be validated against the real IANA database, because schedules
// depend on daylight-saving rules (§18, §34).
func TestTimezoneValidation(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "tz@test.com")

	for _, good := range []string{"Africa/Lagos", "America/New_York", "Europe/Berlin", "Asia/Manila", "UTC"} {
		if code, _ := a.do("PATCH", "/me/profile", token, map[string]any{"timezone": good}); code != http.StatusOK {
			t.Fatalf("valid timezone %q rejected: %d", good, code)
		}
	}
	for _, bad := range []string{"UTC+1", "GMT+0100", "Mars/Olympus", "", "Africa/Lagosss"} {
		code, out := a.do("PATCH", "/me/profile", token, map[string]any{"timezone": bad})
		if code == http.StatusOK {
			t.Fatalf("invalid timezone %q accepted", bad)
		}
		if bad != "" && out["code"] != "INVALID_TIMEZONE" {
			t.Fatalf("timezone %q: code = %v", bad, out["code"])
		}
	}
}

func TestUsernameRules(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "uname@test.com")

	if code, out := a.do("PATCH", "/me/profile", token, map[string]any{"username": "Grace_1"}); code != http.StatusOK {
		t.Fatalf("valid username rejected: %d %v", code, out)
	} else if out["username"] != "grace_1" {
		// Normalised to lowercase so uniqueness is genuinely case-insensitive.
		t.Fatalf("username not normalised: %v", out["username"])
	}

	bad := []string{"ab", "admin", "support", "has space", "emoji😀", "..dots", "dots..", "trailing."}
	for _, u := range bad {
		if code, _ := a.do("PATCH", "/me/profile", token, map[string]any{"username": u}); code == http.StatusOK {
			t.Fatalf("invalid username %q accepted", u)
		}
	}
}

// Uniqueness must be case-insensitive, or "Grace" and "grace" become two users
// who can impersonate each other.
func TestUsernameUniquenessIsCaseInsensitive(t *testing.T) {
	a := newAuthHarness(t)
	first, _ := a.register(t, "first@test.com")
	second, _ := a.register(t, "second@test.com")

	if code, _ := a.do("PATCH", "/me/profile", first, map[string]any{"username": "shiloh"}); code != http.StatusOK {
		t.Fatal("first claim failed")
	}
	code, out := a.do("PATCH", "/me/profile", second, map[string]any{"username": "SHILOH"})
	if code != http.StatusConflict {
		t.Fatalf("case-variant username accepted: %d", code)
	}
	if out["code"] != "USERNAME_TAKEN" {
		t.Fatalf("code = %v, want USERNAME_TAKEN", out["code"])
	}

	// Re-saving one's own username must not conflict with oneself.
	if code, _ := a.do("PATCH", "/me/profile", first, map[string]any{"username": "shiloh"}); code != http.StatusOK {
		t.Fatalf("user could not keep their own username: %d", code)
	}
}

// Names in any script must be accepted; only control characters are refused (§8, §86).
func TestDisplayNameAcceptsGlobalScripts(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "intl@test.com")

	for _, name := range []string{"Adaeze Okonkwo", "Müller", "Иван", "李伟", "عبد الله", "Ọlá"} {
		if code, _ := a.do("PATCH", "/me/profile", token, map[string]any{"display_name": name}); code != http.StatusOK {
			t.Fatalf("legitimate name %q rejected", name)
		}
	}
	// Control and bidi-override characters are refused.
	if code, _ := a.do("PATCH", "/me/profile", token, map[string]any{"display_name": "bad\u202ename"}); code == http.StatusOK {
		t.Fatal("bidi control character accepted in display name")
	}
}

// Avatar URLs must not be able to carry script payloads (§10, §73).
func TestAvatarURLRejectsNonHTTPS(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "avatar@test.com")

	for _, bad := range []string{
		"javascript:alert(1)", "data:text/html;base64,PHNjcmlwdD4=",
		"http://insecure.example.com/a.png", "https://x.com/a.png\"><script>",
	} {
		if code, _ := a.do("PATCH", "/me/profile", token, map[string]any{"avatar_url": bad}); code == http.StatusOK {
			t.Fatalf("dangerous avatar url accepted: %q", bad)
		}
	}
	if code, _ := a.do("PATCH", "/me/profile", token, map[string]any{
		"avatar_url": "https://cdn.example.com/a.png",
	}); code != http.StatusOK {
		t.Fatal("valid https avatar url rejected")
	}
}

// The public projection must never carry private fields. This is the test that
// stops a future field addition from quietly publishing personal data (§6, §63).
func TestPublicProfileOmitsPrivateFields(t *testing.T) {
	p := &models.UserProfile{
		ID: "id-1", UserID: "user-1", DisplayName: "Grace", Username: "grace",
		Bio: "Peace", AvatarURL: "https://cdn/x.png",
		Timezone: "Africa/Lagos", Locale: "en-NG", CountryCode: "NG", Language: "en",
	}
	b, err := json.Marshal(p.Public())
	if err != nil {
		t.Fatal(err)
	}
	payload := string(b)

	for _, leaked := range []string{"user-1", "id-1", "Africa/Lagos", "en-NG", "NG", "user_id", "timezone", "locale", "country"} {
		if containsField(payload, leaked) {
			t.Fatalf("public profile leaked %q: %s", leaked, payload)
		}
	}
	// And it must still carry what a public profile is for.
	if !containsField(payload, "grace") || !containsField(payload, "Grace") {
		t.Fatalf("public profile is missing its public fields: %s", payload)
	}
}

func containsField(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		func() bool {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
			return false
		}()
}

// Preferences must reject values the rest of the system cannot honour (§73).
func TestPreferenceValidation(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "prefs@test.com")

	cases := []struct {
		name string
		body map[string]any
		code string
	}{
		{"duration too short", map[string]any{"default_duration": 5}, "PROFILE_INVALID"},
		{"duration too long", map[string]any{"default_duration": 99999}, "PROFILE_INVALID"},
		{"bogus quality", map[string]any{"preferred_quality": "ultra"}, "PROFILE_INVALID"},
		{"bogus theme", map[string]any{"theme": "neon"}, "PROFILE_INVALID"},
		{"bogus language", map[string]any{"language": "english"}, "INVALID_LOCALE"},
		{"unknown voice", map[string]any{"default_voice_id": "no-such-voice"}, "VOICE_UNAVAILABLE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out := a.do("PATCH", "/me/preferences", token, tc.body)
			if code == http.StatusOK {
				t.Fatalf("invalid preference accepted: %v", tc.body)
			}
			if out["code"] != tc.code {
				t.Fatalf("code = %v, want %v", out["code"], tc.code)
			}
		})
	}

	// A valid update still works.
	if code, _ := a.do("PATCH", "/me/preferences", token, map[string]any{
		"default_duration": 900, "preferred_quality": "high", "theme": "dark", "autoplay": false,
	}); code != http.StatusOK {
		t.Fatal("valid preference update rejected")
	}
}

// Personalization must be genuinely switchable off (§48, §68).
func TestPersonalizationCanBeDisabled(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "personal@test.com")

	code, out := a.do("PATCH", "/me/preferences", token, map[string]any{
		"personalization_enabled": false, "recommendations_enabled": false,
	})
	if code != http.StatusOK {
		t.Fatalf("update: %d", code)
	}
	if out["personalization_enabled"] != false || out["recommendations_enabled"] != false {
		t.Fatalf("personalization not disabled: %v", out)
	}

	// And it must persist.
	_, out = a.do("GET", "/me/preferences", token, nil)
	if out["personalization_enabled"] != false {
		t.Fatal("personalization setting did not persist")
	}
}

// Interests must be validated against real categories, never hard-coded (§13, §73).
func TestInterestsRejectUnknownCategories(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "interests@test.com")

	code, out := a.do("PUT", "/me/interests", token, map[string]any{
		"category_ids": []string{"not-a-real-category"},
	})
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown category accepted: %d", code)
	}
	if out["code"] != "CATEGORY_NOT_FOUND" {
		t.Fatalf("code = %v", out["code"])
	}
}

// A client must not be able to write inferences and have them read back as the
// user's own stated preferences (§15).
func TestInterestsRejectInferredSourceFromClient(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "infer@test.com")

	for _, src := range []string{"AI_INFERENCE", "LISTENING_BEHAVIOR", "RECOMMENDATION"} {
		code, _ := a.do("PUT", "/me/interests", token, map[string]any{
			"category_ids": []string{}, "source": src,
		})
		if code == http.StatusOK {
			t.Fatalf("client was allowed to write %s interests", src)
		}
	}
}

// The interests response must keep stated and inferred preferences apart.
func TestInterestsSeparateExplicitFromInferred(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "split@test.com")

	code, out := a.do("GET", "/me/interests", token, nil)
	if code != http.StatusOK {
		t.Fatalf("get interests: %d", code)
	}
	if _, ok := out["explicit"]; !ok {
		t.Fatal("response has no explicit list")
	}
	if _, ok := out["inferred"]; !ok {
		t.Fatal("response has no inferred list")
	}
}

// Bootstrap must return what the app needs and nothing heavyweight (§56, §95).
func TestBootstrapReturnsStartupPayloadOnly(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "boot@test.com")

	code, out := a.do("GET", "/me/bootstrap", token, nil)
	if code != http.StatusOK {
		t.Fatalf("bootstrap: %d", code)
	}
	for _, key := range []string{"user", "profile", "preferences", "interests", "subscription", "entitlements", "profile_completion"} {
		if _, ok := out[key]; !ok {
			t.Fatalf("bootstrap missing %q", key)
		}
	}
	// History and favourites are paginated surfaces; loading them at startup
	// would make cold start scale with account age.
	for _, key := range []string{"history", "favorites", "sessions", "library"} {
		if _, ok := out[key]; ok {
			t.Fatalf("bootstrap should not include %q", key)
		}
	}

	// Entitlements must be present and authoritative, not client-assertable.
	ent, _ := out["entitlements"].(map[string]any)
	if ent["plan"] != "free" {
		t.Fatalf("plan = %v, want free", ent["plan"])
	}
	if ent["premium_voices"] != false {
		t.Fatalf("free user shown premium_voices = %v", ent["premium_voices"])
	}
}

// Profile completion must be informational, never a gate (§12).
func TestProfileCompletionIsAdvisory(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "complete@test.com")

	_, out := a.do("GET", "/me/bootstrap", token, nil)
	pc, _ := out["profile_completion"].(map[string]any)
	if pc["total"] == nil || pc["steps"] == nil {
		t.Fatalf("profile_completion malformed: %v", pc)
	}

	// An incomplete profile must not block ordinary use.
	if code, _ := a.do("GET", "/me/preferences", token, nil); code != http.StatusOK {
		t.Fatal("incomplete profile blocked access to preferences")
	}
}

// Profile endpoints must require authentication (§99).
func TestProfileEndpointsRequireAuth(t *testing.T) {
	a := newAuthHarness(t)
	for _, path := range []string{"/me/profile", "/me/preferences", "/me/interests", "/me/bootstrap"} {
		if code, _ := a.do("GET", path, "", nil); code != http.StatusUnauthorized {
			t.Fatalf("%s without a token: got %d, want 401", path, code)
		}
	}
}

// One user's profile writes must never affect another's (§71, §72).
func TestProfileUpdatesAreScopedToCaller(t *testing.T) {
	a := newAuthHarness(t)
	alice, aliceID := a.register(t, "alice3@test.com")
	bob, _ := a.register(t, "bob3@test.com")

	a.do("PATCH", "/me/profile", alice, map[string]any{"display_name": "Alice"})

	// Bob supplies Alice's user id in the body. Identity must come from the
	// token, so this must change Bob's profile, not Alice's.
	a.do("PATCH", "/me/profile", bob, map[string]any{
		"display_name": "Hacked", "user_id": aliceID,
	})

	_, out := a.do("GET", "/me/profile", alice, nil)
	if out["display_name"] != "Alice" {
		t.Fatalf("alice's profile was modified by bob: %v", out["display_name"])
	}
}
