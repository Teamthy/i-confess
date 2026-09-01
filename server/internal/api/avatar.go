package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/images"
	"github.com/Teamthy/i-confess/internal/store"
)

// Avatar upload (§10, §11).
//
// The flow is: receive → validate by decoding → re-encode into variants →
// store → update profile. Re-encoding rather than passing bytes through is what
// strips EXIF (including GPS) and destroys polyglot payloads.

// avatarTTL bounds signed avatar links. Longer than audio because an avatar is
// not entitlement-gated and is fetched constantly, but still finite so a
// removed avatar stops resolving.
const avatarTTL = 24 * time.Hour

// uploadAvatar accepts a multipart image and replaces the caller's avatar.
func (h *Handler) uploadAvatar(w http.ResponseWriter, r *http.Request) {
	if h.signer == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "uploads are not configured on this server")
		return
	}
	userID := h.userID(r)

	// Bound the request body before parsing. Without this a client could stream
	// gigabytes and the limit inside the image package would never be reached.
	r.Body = http.MaxBytesReader(w, r.Body, images.MaxBytes+1<<20)

	if err := r.ParseMultipartForm(images.MaxBytes); err != nil {
		writeCode(w, http.StatusRequestEntityTooLarge, "PROFILE_INVALID",
			fmt.Sprintf("image must be %d MB or smaller", images.MaxBytes>>20))
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	file, header, err := r.FormFile("file")
	if err != nil {
		writeCode(w, http.StatusBadRequest, "PROFILE_INVALID", "a file field is required")
		return
	}
	defer file.Close()

	// The client's Content-Type is deliberately ignored: it is attacker
	// controlled. Format is decided by decoding the bytes.
	_ = header

	result, err := images.Process(file)
	if err != nil {
		writeCode(w, avatarErrorStatus(err), "PROFILE_INVALID", avatarErrorMessage(err))
		return
	}

	// Version the key so a replaced avatar gets a new URL. Reusing the key
	// would leave stale images in CDN caches for their full TTL — users would
	// change their photo and still see the old one.
	version := time.Now().UTC().Unix()
	ext := images.Extension(result.Format)
	contentType := images.ContentType(result.Format)

	urls := map[string]string{}
	for _, v := range result.Variants {
		key := fmt.Sprintf("avatars/%s/%d/%s.%s", sanitiseID(userID), version, v.Name, ext)
		if err := h.signer.Upload(r.Context(), key, v.Data, map[string]string{
			"user_id": userID, "variant": v.Name, "content_type": contentType,
		}); err != nil {
			log.Printf("avatar: failed to store %s for %s: %v", v.Name, userID, err)
			httpx.WriteError(w, http.StatusInternalServerError, "failed to store image")
			return
		}
		signed, err := h.signer.GenerateSignedURL(r.Context(), key, avatarTTL)
		if err != nil {
			log.Printf("avatar: failed to sign %s for %s: %v", v.Name, userID, err)
			httpx.WriteError(w, http.StatusInternalServerError, "failed to publish image")
			return
		}
		urls[v.Name] = signed
	}

	// The profile stores the medium key; URLs are signed per read like audio.
	mediumKey := fmt.Sprintf("avatars/%s/%d/medium.%s", sanitiseID(userID), version, ext)
	if _, err := h.profiles.UpdateProfile(r.Context(), userID, store.ProfileUpdate{
		AvatarURL: &mediumKey,
	}); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to update profile")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"variants": urls,
		"format":   result.Format,
		"note":     "Location and camera metadata are removed from uploaded images.",
	})
}

// deleteAvatar removes the caller's avatar.
//
// The stored objects are left in place rather than deleted synchronously: a
// failed delete must not block the user from removing their photo, and the
// profile no longer references them. Reaping orphaned objects belongs to a
// background sweep.
func (h *Handler) deleteAvatar(w http.ResponseWriter, r *http.Request) {
	empty := ""
	if _, err := h.profiles.UpdateProfile(r.Context(), h.userID(r), store.ProfileUpdate{
		AvatarURL: &empty,
	}); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to remove avatar")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "avatar removed"})
}

func avatarErrorStatus(err error) int {
	switch {
	case errors.Is(err, images.ErrTooLarge), errors.Is(err, images.ErrTooManyPixels):
		return http.StatusRequestEntityTooLarge
	default:
		return http.StatusUnprocessableEntity
	}
}

// avatarErrorMessage explains the refusal in terms a user can act on, without
// echoing internals.
func avatarErrorMessage(err error) string {
	switch {
	case errors.Is(err, images.ErrTooLarge):
		return fmt.Sprintf("image must be %d MB or smaller", images.MaxBytes>>20)
	case errors.Is(err, images.ErrTooManyPixels):
		return "image dimensions are too large"
	case errors.Is(err, images.ErrTooSmall):
		return fmt.Sprintf("image must be at least %dx%d pixels", images.MinDimension, images.MinDimension)
	case errors.Is(err, images.ErrUnsupported):
		return "image must be a JPEG, PNG or GIF"
	default:
		return "image could not be read"
	}
}

// sanitiseID keeps a user id safe to interpolate into a storage key.
func sanitiseID(id string) string {
	out := make([]rune, 0, len(id))
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			out = append(out, r)
		default:
			out = append(out, '-')
		}
	}
	if len(out) == 0 {
		return "unknown"
	}
	return string(out)
}
