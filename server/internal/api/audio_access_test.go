package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Teamthy/i-confess/internal/media"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/storage"
	"github.com/Teamthy/i-confess/internal/store"
)

// Entitlement-gated audio delivery (§11, §26, §27, §55).
//
// The property under test is commercial, not cosmetic: a free listener must not
// be able to obtain playable audio for premium material, and hiding the button
// in the client is not a control. Every assertion here is about what the server
// actually hands back.

type audioFixture struct {
	h        *Handler
	router   http.Handler
	srv      *httptest.Server
	store    storage.ObjectStorage
	catID    string
	freeTok  string
	premTok  string
	premUser string
	voiceStd string
	voicePrm string
}

// newAudioFixture builds a server with one standard and one premium voice, a
// published confession with audio for both, and a free and premium user.
func newAudioFixture(t *testing.T) *audioFixture {
	t.Helper()
	dbConn := setupTestDB(t)
	t.Cleanup(func() { dbConn.Close() })

	objStore, err := storage.New(&storage.StorageConfig{
		Provider: "local", LocalRootPath: t.TempDir(),
		CDNDomain: "/media", SigningSecret: "test-audio-secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	h := NewHandler(Config{JWTSecret: "test-secret", TokenTTL: "24h"}, dbConn)
	h.BuildEngine()
	h.SetSigner(objStore)
	if local, ok := objStore.(*storage.LocalStorage); ok {
		h.SetMediaHandler(local.Handler("/media"))
	}
	router := h.Routes()
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	ctx := context.Background()
	content := store.NewContentStore(dbConn)
	audio := store.NewAudioStore(dbConn)

	cat := &models.Category{Name: "Healing", Slug: "healing", Status: "published"}
	if err := content.CreateCategory(ctx, cat); err != nil {
		t.Fatal(err)
	}

	std := &models.Voice{Name: "Grace", Type: "professional", Language: "en", Status: "active", Premium: false}
	prm := &models.Voice{Name: "Minister", Type: "minister", Language: "en", Status: "active", Premium: true}
	if err := audio.CreateVoice(ctx, std); err != nil {
		t.Fatal(err)
	}
	if err := audio.CreateVoice(ctx, prm); err != nil {
		t.Fatal(err)
	}

	// Two confessions so a session can be packed to a useful length.
	for i, title := range []string{"Healed", "Whole"} {
		c := &models.Confession{
			CategoryID: cat.ID, Title: title, Status: "published", Language: "en",
			ShortText:  "I am healed.",
			MediumText: "I am healed by His stripes and made whole.",
			LongText:   "I am healed by His stripes and made whole in body and mind.",
			Variants: []models.ConfessionVariant{
				{Label: "1m", DurationSeconds: 60, SortOrder: i},
			},
		}
		if err := content.CreateConfession(ctx, c); err != nil {
			t.Fatal(err)
		}
		variants, err := content.Variants(ctx, c.ID)
		if err != nil || len(variants) == 0 {
			t.Fatalf("variants: %v (n=%d)", err, len(variants))
		}
		// Audio for both voices so voice choice, not availability, decides.
		for _, v := range []*models.Voice{std, prm} {
			key := storage.AudioKeyFor(c.ID, variants[0].ID, v.ID, "en", 1)
			if err := objStore.Upload(ctx, key, media.ToneBytes(2), nil); err != nil {
				t.Fatal(err)
			}
			if err := audio.UpsertAsset(ctx, &models.AudioAsset{
				ConfessionID: c.ID, VariantID: variants[0].ID, VoiceID: v.ID,
				URL: key, DurationSeconds: 60, Status: "ready",
			}); err != nil {
				t.Fatal(err)
			}
		}
	}

	f := &audioFixture{
		h: h, router: router, srv: srv, store: objStore,
		catID: cat.ID, voiceStd: std.ID, voicePrm: prm.ID,
	}
	f.freeTok = f.register(t, "free@test.com")
	f.premTok, f.premUser = f.registerWithID(t, "prem@test.com")

	// Upgrade the premium user server-side, the only place plan changes happen.
	users := store.NewUserStore(dbConn)
	if err := users.SetSubscription(ctx, f.premUser, "premium", "active"); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *audioFixture) register(t *testing.T, email string) string {
	t.Helper()
	tok, _ := f.registerWithID(t, email)
	return tok
}

func (f *audioFixture) registerWithID(t *testing.T, email string) (string, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"email": email, "password": "password123", "display_name": "T", "timezone": "UTC",
	})
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, httptest.NewRequest("POST", "/auth/register", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("register %s: %d %s", email, rec.Code, rec.Body.String())
	}
	var out struct {
		Token string `json:"token"`
		User  struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out.Token, out.User.ID
}

type sessionResp struct {
	ID                   string `json:"id"`
	VoiceID              string `json:"voice_id"`
	VoiceDowngraded      bool   `json:"voice_downgraded"`
	VoiceDowngradeReason string `json:"voice_downgrade_reason"`
	Items                []struct {
		AudioURL   string `json:"audio_url"`
		VoiceID    string `json:"voice_id"`
		Locked     bool   `json:"locked"`
		LockReason string `json:"lock_reason"`
	} `json:"items"`
}

func (f *audioFixture) createSession(t *testing.T, token, voiceID string, seconds int) (int, sessionResp) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"category_ids": []string{f.catID}, "duration_seconds": seconds, "voice_id": voiceID,
	})
	req := httptest.NewRequest("POST", "/sessions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	var out sessionResp
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

// The headline test: a free listener asking for a premium voice must never end
// up with premium audio.
//
// The session engine substitutes a free voice rather than refusing outright, so
// the guarantee is about the *audio actually served*: every item must be bound
// to the standard voice, and none may reference the premium one.
func TestFreeUserCannotObtainPremiumVoiceAudio(t *testing.T) {
	f := newAudioFixture(t)

	code, sess := f.createSession(t, f.freeTok, f.voicePrm, 120)
	if code != http.StatusCreated {
		t.Fatalf("create session: %d", code)
	}
	if len(sess.Items) == 0 {
		t.Fatal("no items")
	}

	// The engine must have downgraded the session away from the premium voice.
	if sess.VoiceID == f.voicePrm {
		t.Fatal("free user was assigned the premium voice")
	}
	// The substitution must be surfaced, not silent: a listener who picked a
	// voice and got a different one is owed an explanation (and an upgrade
	// prompt), otherwise the product looks broken rather than gated.
	if !sess.VoiceDowngraded {
		t.Fatal("premium voice was silently swapped with no voice_downgraded flag")
	}
	if sess.VoiceDowngradeReason != "premium_voice_requires_subscription" {
		t.Fatalf("voice_downgrade_reason = %q", sess.VoiceDowngradeReason)
	}

	for i, item := range sess.Items {
		if item.VoiceID == f.voicePrm {
			t.Fatalf("item %d is bound to the premium voice", i)
		}
		if item.VoiceID != f.voiceStd {
			t.Fatalf("item %d unexpected voice %q", i, item.VoiceID)
		}
		// The served URL must point at standard-voice audio.
		if strings.Contains(item.AudioURL, f.voicePrm) {
			t.Fatalf("item %d URL references premium audio: %q", i, item.AudioURL)
		}
	}
}

// A free user must not be handed a premium item even when the engine cannot
// substitute: entitlement is enforced at URL-minting time, independently of
// engine behaviour, so a future engine change cannot open a hole.
func TestEntitlementBlocksPremiumItemAtSigningTime(t *testing.T) {
	f := newAudioFixture(t)

	// Build a session as premium (so items carry the premium voice), then read
	// it back as a free user by downgrading the same account.
	code, sess := f.createSession(t, f.premTok, f.voicePrm, 120)
	if code != http.StatusCreated || len(sess.Items) == 0 {
		t.Fatalf("create session: %d", code)
	}
	if sess.Items[0].VoiceID != f.voicePrm {
		t.Fatalf("expected premium-voice items, got %q", sess.Items[0].VoiceID)
	}

	users := store.NewUserStore(f.h.usersDB())
	if err := users.SetSubscription(context.Background(), f.premUser, "free", "active"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/sessions/"+sess.ID, nil)
	req.Header.Set("Authorization", "Bearer "+f.premTok)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	var after sessionResp
	json.Unmarshal(rec.Body.Bytes(), &after)
	for i, item := range after.Items {
		if item.AudioURL != "" {
			t.Fatalf("item %d: downgraded user still received premium audio: %q", i, item.AudioURL)
		}
		if item.LockReason != "premium_voice_requires_subscription" {
			t.Fatalf("item %d: lock_reason = %q", i, item.LockReason)
		}
	}
}

func TestPremiumUserGetsPlayablePremiumAudio(t *testing.T) {
	f := newAudioFixture(t)

	code, sess := f.createSession(t, f.premTok, f.voicePrm, 120)
	if code != http.StatusCreated {
		t.Fatalf("create session: %d", code)
	}
	if len(sess.Items) == 0 {
		t.Fatal("no items")
	}
	for i, item := range sess.Items {
		if item.Locked {
			t.Fatalf("item %d locked for a premium user: %s", i, item.LockReason)
		}
		if !strings.Contains(item.AudioURL, "sig=") {
			t.Fatalf("item %d: expected a signed URL, got %q", i, item.AudioURL)
		}
	}
}

// Free must remain genuinely useful (§25).
func TestFreeUserGetsStandardVoiceAudio(t *testing.T) {
	f := newAudioFixture(t)

	code, sess := f.createSession(t, f.freeTok, f.voiceStd, 120)
	if code != http.StatusCreated {
		t.Fatalf("create session: %d", code)
	}
	for i, item := range sess.Items {
		if item.Locked || item.AudioURL == "" {
			t.Fatalf("item %d: free user refused standard audio (locked=%v reason=%s)", i, item.Locked, item.LockReason)
		}
		if !strings.Contains(item.AudioURL, "sig=") {
			t.Fatalf("item %d is not signed: %q", i, item.AudioURL)
		}
	}
}

// Entitlement is re-checked on read: a lapsed plan must stop working even for a
// session that was created while the user was premium.
func TestDowngradeRevokesAudioOnReread(t *testing.T) {
	f := newAudioFixture(t)

	code, sess := f.createSession(t, f.premTok, f.voicePrm, 120)
	if code != http.StatusCreated || len(sess.Items) == 0 {
		t.Fatalf("create session: %d", code)
	}
	if sess.Items[0].AudioURL == "" {
		t.Fatal("premium user should have had audio")
	}

	// Subscription lapses.
	users := store.NewUserStore(f.h.usersDB())
	if err := users.SetSubscription(context.Background(), f.premUser, "free", "active"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/sessions/"+sess.ID, nil)
	req.Header.Set("Authorization", "Bearer "+f.premTok)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("reload session: %d", rec.Code)
	}
	var after sessionResp
	json.Unmarshal(rec.Body.Bytes(), &after)

	for i, item := range after.Items {
		if item.AudioURL != "" {
			t.Fatalf("item %d still playable after downgrade: %q", i, item.AudioURL)
		}
		if !item.Locked {
			t.Fatalf("item %d not locked after downgrade", i)
		}
	}
}

// Session length is a plan capability, enforced server-side.
func TestFreeUserCannotCreateLongSession(t *testing.T) {
	f := newAudioFixture(t)

	code, _ := f.createSession(t, f.freeTok, f.voiceStd, 3600)
	if code != http.StatusPaymentRequired {
		t.Fatalf("free 60-minute session: got %d, want 402", code)
	}

	// A within-plan length still works.
	if code, _ := f.createSession(t, f.freeTok, f.voiceStd, 600); code != http.StatusCreated {
		t.Fatalf("free 10-minute session: got %d, want 201", code)
	}
	// Premium may go long.
	if code, _ := f.createSession(t, f.premTok, f.voiceStd, 3600); code != http.StatusCreated {
		t.Fatalf("premium 60-minute session: got %d, want 201", code)
	}
}

// The media origin must refuse unsigned and tampered requests, and the raw
// storage key must never be playable on its own.
func TestMediaOriginRequiresValidSignature(t *testing.T) {
	f := newAudioFixture(t)

	_, sess := f.createSession(t, f.freeTok, f.voiceStd, 120)
	if len(sess.Items) == 0 || sess.Items[0].AudioURL == "" {
		t.Fatal("expected a signed url")
	}
	signed := sess.Items[0].AudioURL

	get := func(path string) int {
		res, err := http.Get(f.srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		return res.StatusCode
	}

	if code := get(signed); code != http.StatusOK {
		t.Fatalf("signed request: got %d, want 200", code)
	}
	bare := signed[:strings.Index(signed, "?")]
	if code := get(bare); code != http.StatusForbidden {
		t.Fatalf("unsigned request: got %d, want 403", code)
	}
	if code := get(strings.Replace(signed, "sig=", "sig=x", 1)); code != http.StatusForbidden {
		t.Fatalf("tampered signature: got %d, want 403", code)
	}
}

// A raw storage key must never appear in a client payload.
func TestSessionNeverLeaksRawStorageKeys(t *testing.T) {
	f := newAudioFixture(t)

	body, _ := json.Marshal(map[string]any{
		"category_ids": []string{f.catID}, "duration_seconds": 120, "voice_id": f.voiceStd,
	})
	req := httptest.NewRequest("POST", "/sessions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+f.freeTok)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	payload := rec.Body.String()
	// A bare key would appear as "audio/..." with no signature attached.
	if strings.Contains(payload, `"audio_url":"audio/`) {
		t.Fatalf("raw storage key leaked into the session payload: %s", payload)
	}
}
