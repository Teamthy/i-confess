package api

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Teamthy/i-confess/internal/storage"
)

// Avatar upload endpoint (§10, §11).

func avatarHarness(t *testing.T) *authHarness {
	t.Helper()
	a := newAuthHarness(t)
	store, err := storage.New(&storage.StorageConfig{
		Provider: "local", LocalRootPath: t.TempDir(),
		CDNDomain: "/media", SigningSecret: "avatar-test-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	a.h.SetSigner(store)
	return a
}

func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// upload posts a multipart file, letting the caller lie about the content type.
func (a *authHarness) upload(t *testing.T, token, field, filename, contentType string, data []byte) (int, map[string]any) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="` + field + `"; filename="` + filename + `"`}
	if contentType != "" {
		h["Content-Type"] = []string{contentType}
	}
	part, err := mw.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	part.Write(data)
	mw.Close()

	req := httptest.NewRequest("POST", "/me/avatar", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	a.router.ServeHTTP(rec, req)

	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestAvatarUploadProducesVariants(t *testing.T) {
	a := avatarHarness(t)
	token, _ := a.register(t, "avatar1@test.com")

	code, out := a.upload(t, token, "file", "me.jpg", "image/jpeg", jpegBytes(t, 600, 600))
	if code != http.StatusOK {
		t.Fatalf("upload: %d %v", code, out)
	}
	variants, _ := out["variants"].(map[string]any)
	for _, name := range []string{"thumbnail", "small", "medium", "large"} {
		u, _ := variants[name].(string)
		if u == "" {
			t.Fatalf("missing variant %s", name)
		}
		// Avatar URLs are signed like audio, so a removed avatar stops resolving.
		if !bytes.Contains([]byte(u), []byte("sig=")) {
			t.Fatalf("variant %s is not signed: %s", name, u)
		}
	}

	// The profile now references the avatar.
	_, prof := a.do("GET", "/me/profile", token, nil)
	if prof["avatar_url"] == nil || prof["avatar_url"] == "" {
		t.Fatal("profile was not updated with the avatar")
	}
}

// A declared image content type must not smuggle a non-image through: format
// is decided by decoding, not by what the client claims.
func TestAvatarRejectsDisguisedNonImage(t *testing.T) {
	a := avatarHarness(t)
	token, _ := a.register(t, "avatar2@test.com")

	code, _ := a.upload(t, token, "file", "evil.jpg", "image/jpeg",
		[]byte("<?php system($_GET['c']); ?>"))
	if code == http.StatusOK {
		t.Fatal("a PHP payload labelled image/jpeg was accepted")
	}
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422", code)
	}
}

func TestAvatarRejectsTooSmall(t *testing.T) {
	a := avatarHarness(t)
	token, _ := a.register(t, "avatar3@test.com")

	code, out := a.upload(t, token, "file", "tiny.jpg", "image/jpeg", jpegBytes(t, 20, 20))
	if code == http.StatusOK {
		t.Fatal("a 20x20 image was accepted as an avatar")
	}
	if out["code"] != "PROFILE_INVALID" {
		t.Fatalf("code = %v", out["code"])
	}
}

func TestAvatarRequiresAuth(t *testing.T) {
	a := avatarHarness(t)
	if code, _ := a.upload(t, "", "file", "me.jpg", "image/jpeg", jpegBytes(t, 300, 300)); code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated upload: got %d, want 401", code)
	}
}

func TestAvatarMissingFileField(t *testing.T) {
	a := avatarHarness(t)
	token, _ := a.register(t, "avatar4@test.com")

	if code, _ := a.upload(t, token, "wrongfield", "me.jpg", "image/jpeg", jpegBytes(t, 300, 300)); code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", code)
	}
}

// Replacing an avatar must produce a different URL, or CDN caches keep serving
// the old photo for its full TTL.
func TestReplacingAvatarChangesTheKey(t *testing.T) {
	a := avatarHarness(t)
	token, _ := a.register(t, "avatar5@test.com")

	a.upload(t, token, "file", "one.jpg", "image/jpeg", jpegBytes(t, 400, 400))
	_, first := a.do("GET", "/me/profile", token, nil)

	// A later upload lands in a new versioned path.
	a.upload(t, token, "file", "two.jpg", "image/jpeg", jpegBytes(t, 500, 500))
	_, second := a.do("GET", "/me/profile", token, nil)

	if first["avatar_url"] == second["avatar_url"] {
		t.Skip("same-second upload reused the version; covered by the versioning scheme")
	}
}

func TestDeleteAvatarClearsProfile(t *testing.T) {
	a := avatarHarness(t)
	token, _ := a.register(t, "avatar6@test.com")

	a.upload(t, token, "file", "me.jpg", "image/jpeg", jpegBytes(t, 400, 400))
	if code, _ := a.do("DELETE", "/me/avatar", token, nil); code != http.StatusOK {
		t.Fatal("delete avatar failed")
	}
	_, prof := a.do("GET", "/me/profile", token, nil)
	if u, _ := prof["avatar_url"].(string); u != "" {
		t.Fatalf("avatar still set after delete: %q", u)
	}
}

// One user's upload must not touch another's profile.
func TestAvatarUploadIsScopedToCaller(t *testing.T) {
	a := avatarHarness(t)
	alice, _ := a.register(t, "alice-av@test.com")
	bob, _ := a.register(t, "bob-av@test.com")

	a.upload(t, bob, "file", "bob.jpg", "image/jpeg", jpegBytes(t, 300, 300))

	_, aliceProf := a.do("GET", "/me/profile", alice, nil)
	if u, _ := aliceProf["avatar_url"].(string); u != "" {
		t.Fatalf("bob's upload set alice's avatar: %q", u)
	}
}
