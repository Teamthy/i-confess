package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/store"
)

// Offline downloads (§28, §44).

// downloadFixture reuses the audio fixture, which already seeds a category,
// two voices and published assets.
func downloadFixture(t *testing.T) *audioFixture {
	t.Helper()
	return newAudioFixture(t)
}

// confessionID returns a confession that has audio.
func (f *audioFixture) confessionID(t *testing.T) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/categories/"+f.catID+"/confessions", nil)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)

	var confs []struct {
		ID string `json:"id"`
	}
	decodeInto(t, rec.Body.Bytes(), &confs)
	if len(confs) == 0 {
		t.Fatal("fixture has no confessions")
	}
	return confs[0].ID
}

// The commercial rule: a free user cannot take content offline.
func TestFreeUserCannotDownload(t *testing.T) {
	f := downloadFixture(t)
	conf := f.confessionID(t)

	code, out := f.call(t, "POST", "/me/downloads", f.freeTok, map[string]any{
		"confession_id": conf, "voice_id": f.voiceStd,
	})
	if code != http.StatusPaymentRequired {
		t.Fatalf("free user was allowed to download: %d %v", code, out)
	}
	if out["code"] != "ENTITLEMENT_REQUIRED" {
		t.Fatalf("code = %v", out["code"])
	}
}

func TestPremiumUserCanDownload(t *testing.T) {
	f := downloadFixture(t)
	conf := f.confessionID(t)

	code, out := f.call(t, "POST", "/me/downloads", f.premTok, map[string]any{
		"confession_id": conf, "voice_id": f.voiceStd,
	})
	if code != http.StatusCreated {
		t.Fatalf("premium download refused: %d %v", code, out)
	}

	url, _ := out["download_url"].(string)
	if !strings.Contains(url, "sig=") {
		t.Fatalf("download url is not signed: %q", url)
	}
	if out["expires_at"] == nil || out["expires_at"] == "" {
		t.Fatal("licence has no expiry: it would never lapse")
	}

	// Fetching the signed URL must actually work.
	res, err := http.Get(f.srv.URL + url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("signed download url returned %d", res.StatusCode)
	}
}

// The storage key must never reach a client: a licence that has lapsed would
// otherwise still be redeemable from a record cached on the device.
func TestDownloadResponseHidesStorageKey(t *testing.T) {
	f := downloadFixture(t)
	conf := f.confessionID(t)

	body := f.raw(t, "POST", "/me/downloads", f.premTok, map[string]any{
		"confession_id": conf, "voice_id": f.voiceStd,
	})
	// A bare key looks like "audio/..." with no signature attached.
	if strings.Contains(body, `"storage_key"`) || strings.Contains(body, `"url":"audio/`) {
		t.Fatalf("storage key leaked: %s", body)
	}

	list := f.raw(t, "GET", "/me/downloads", f.premTok, nil)
	if strings.Contains(list, `"storage_key"`) {
		t.Fatalf("storage key leaked in the download list: %s", list)
	}
}

// A lapsed subscription must eventually stop offline playback. The server
// cannot delete files from a phone, so it works by refusing to renew.
func TestDowngradeBlocksRenewal(t *testing.T) {
	f := downloadFixture(t)
	conf := f.confessionID(t)

	code, out := f.call(t, "POST", "/me/downloads", f.premTok, map[string]any{
		"confession_id": conf, "voice_id": f.voiceStd,
	})
	if code != http.StatusCreated {
		t.Fatalf("setup download: %d", code)
	}
	dl, _ := out["download"].(map[string]any)
	id, _ := dl["id"].(string)

	users := store.NewUserStore(f.h.db)
	if err := users.SetSubscription(context.Background(), f.premUser, "free", "active"); err != nil {
		t.Fatal(err)
	}

	code, out = f.call(t, "POST", "/me/downloads/"+id+"/refresh", f.premTok, nil)
	if code != http.StatusPaymentRequired {
		t.Fatalf("a downgraded user renewed an offline licence: %d %v", code, out)
	}
	// The response must explain what happens next rather than just refusing.
	if out["note"] == nil {
		t.Fatal("refusal does not tell the user their download will expire")
	}
}

// The limit counts live licences, so removing one frees a slot. Counting
// lifetime downloads would punish someone for managing their storage.
func TestRemovingADownloadFreesASlot(t *testing.T) {
	f := downloadFixture(t)
	conf := f.confessionID(t)

	code, out := f.call(t, "POST", "/me/downloads", f.premTok, map[string]any{
		"confession_id": conf, "voice_id": f.voiceStd,
	})
	if code != http.StatusCreated {
		t.Fatalf("create: %d", code)
	}
	dl, _ := out["download"].(map[string]any)
	id, _ := dl["id"].(string)

	_, list := f.call(t, "GET", "/me/downloads", f.premTok, nil)
	if used, _ := list["used"].(float64); used != 1 {
		t.Fatalf("used = %v, want 1", list["used"])
	}

	if code, _ := f.call(t, "DELETE", "/me/downloads/"+id, f.premTok, nil); code != http.StatusOK {
		t.Fatalf("delete: %d", code)
	}
	_, list = f.call(t, "GET", "/me/downloads", f.premTok, nil)
	if used, _ := list["used"].(float64); used != 0 {
		t.Fatalf("used = %v after removal, want 0", list["used"])
	}
}

// Re-requesting a download the user already holds must renew, not fail: a
// client retrying after a dropped connection should not be punished.
func TestRepeatDownloadIsIdempotent(t *testing.T) {
	f := downloadFixture(t)
	conf := f.confessionID(t)

	for i := 0; i < 3; i++ {
		if code, out := f.call(t, "POST", "/me/downloads", f.premTok, map[string]any{
			"confession_id": conf, "voice_id": f.voiceStd,
		}); code != http.StatusCreated {
			t.Fatalf("attempt %d: %d %v", i, code, out)
		}
	}
	_, list := f.call(t, "GET", "/me/downloads", f.premTok, nil)
	if used, _ := list["used"].(float64); used != 1 {
		t.Fatalf("three identical requests created %v licences, want 1", list["used"])
	}
}

// One user must not reach another's downloads.
func TestDownloadsAreScopedToOwner(t *testing.T) {
	f := downloadFixture(t)
	conf := f.confessionID(t)

	_, out := f.call(t, "POST", "/me/downloads", f.premTok, map[string]any{
		"confession_id": conf, "voice_id": f.voiceStd,
	})
	dl, _ := out["download"].(map[string]any)
	id, _ := dl["id"].(string)

	// The free user cannot delete or refresh it.
	if code, _ := f.call(t, "DELETE", "/me/downloads/"+id, f.freeTok, nil); code == http.StatusOK {
		t.Fatal("another user deleted this download")
	}
	// And it is still there.
	_, list := f.call(t, "GET", "/me/downloads", f.premTok, nil)
	if used, _ := list["used"].(float64); used != 1 {
		t.Fatalf("owner lost their download: used = %v", list["used"])
	}
	// The other user's library is empty.
	_, otherList := f.call(t, "GET", "/me/downloads", f.freeTok, nil)
	if used, _ := otherList["used"].(float64); used != 0 {
		t.Fatalf("another user sees %v downloads", otherList["used"])
	}
}

func TestDownloadUnknownConfession(t *testing.T) {
	f := downloadFixture(t)
	code, _ := f.call(t, "POST", "/me/downloads", f.premTok, map[string]any{
		"confession_id": "does-not-exist",
	})
	if code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", code)
	}
}

func TestDownloadEndpointsRequireAuth(t *testing.T) {
	f := downloadFixture(t)
	if code, _ := f.call(t, "GET", "/me/downloads", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("list without auth: %d", code)
	}
	if code, _ := f.call(t, "POST", "/me/downloads", "", map[string]any{"confession_id": "x"}); code != http.StatusUnauthorized {
		t.Fatalf("create without auth: %d", code)
	}
}

// The licence expiry must reflect the plan, so a client can warn before
// content stops working.
func TestLicenceExpiryMatchesPlan(t *testing.T) {
	f := downloadFixture(t)
	conf := f.confessionID(t)

	_, out := f.call(t, "POST", "/me/downloads", f.premTok, map[string]any{
		"confession_id": conf, "voice_id": f.voiceStd,
	})
	expires, _ := out["expires_at"].(string)
	at, err := time.Parse(time.RFC3339, expires)
	if err != nil {
		t.Fatalf("expiry is not RFC3339: %q", expires)
	}
	if !at.After(time.Now()) {
		t.Fatal("licence is already expired on issue")
	}
	// Premium allows 7 days; anything wildly longer means the plan value is
	// not being honoured.
	if at.After(time.Now().Add(30 * 24 * time.Hour)) {
		t.Fatalf("licence lasts until %s, far beyond the plan allowance", at)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func (f *audioFixture) call(t *testing.T, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	rec := f.request(t, method, path, token, body)
	var out map[string]any
	decodeInto(t, rec.Body.Bytes(), &out)
	return rec.Code, out
}

func (f *audioFixture) raw(t *testing.T, method, path, token string, body any) string {
	t.Helper()
	return f.request(t, method, path, token, body).Body.String()
}

func (f *audioFixture) request(t *testing.T, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *strings.Reader
	if body != nil {
		rdr = strings.NewReader(encodeJSON(t, body))
	} else {
		rdr = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func decodeInto(t *testing.T, data []byte, v any) {
	t.Helper()
	_ = json.Unmarshal(data, v)
}

func encodeJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
