package api

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Teamthy/i-confess/internal/entitlements"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/store"
)

// Profile, preferences and interests API (PRD §5, §14, §20, §51, §56).
//
// Two rules run through all of it:
//
//  1. The authenticated user is the only source of identity. No handler reads a
//     user id from a path, query or body (§72, §99).
//  2. Responses are explicit DTOs. Private fields cannot leak by being added to
//     a struct that happens to be serialised somewhere public (§6).

// getProfile returns the caller's own profile.
func (h *Handler) getProfile(w http.ResponseWriter, r *http.Request) {
	p, err := h.profiles.Profile(r.Context(), h.userID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load profile")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p)
}

// updateProfile applies a partial update.
//
// Every field is a pointer so the handler can tell "not supplied" from
// "cleared". Sending only {"bio": ""} must clear the bio and leave the display
// name alone, which a value-typed struct cannot express.
func (h *Handler) updateProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DisplayName *string `json:"display_name"`
		Username    *string `json:"username"`
		Bio         *string `json:"bio"`
		AvatarURL   *string `json:"avatar_url"`
		Timezone    *string `json:"timezone"`
		Locale      *string `json:"locale"`
		Language    *string `json:"language"`
		CountryCode *string `json:"country_code"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeCode(w, http.StatusBadRequest, "PROFILE_INVALID", "invalid request body")
		return
	}

	update := store.ProfileUpdate{}

	if req.DisplayName != nil {
		// Validate the raw value first: normalisation collapses whitespace and
		// would hide control characters from the check.
		if err := validateDisplayName(*req.DisplayName); err != nil {
			writeCode(w, http.StatusBadRequest, "PROFILE_INVALID", err.Error())
			return
		}
		name := normaliseSpace(*req.DisplayName)
		if err := validateDisplayName(name); err != nil {
			writeCode(w, http.StatusBadRequest, "PROFILE_INVALID", err.Error())
			return
		}
		update.DisplayName = &name
	}
	if req.Username != nil {
		uname := strings.ToLower(strings.TrimSpace(*req.Username))
		if uname != "" {
			if err := validateUsername(uname); err != nil {
				writeCode(w, http.StatusBadRequest, "USERNAME_INVALID", err.Error())
				return
			}
		}
		update.Username = &uname
	}
	if req.Bio != nil {
		bio := strings.TrimSpace(*req.Bio)
		if utf8.RuneCountInString(bio) > 500 {
			writeCode(w, http.StatusBadRequest, "PROFILE_INVALID", "bio must be 500 characters or fewer")
			return
		}
		// Stored as plain text. Escaping happens at render time; storing markup
		// would make every future consumer responsible for sanitising it (§9).
		update.Bio = &bio
	}
	if req.AvatarURL != nil {
		if err := validateHTTPSURL(*req.AvatarURL); err != nil {
			writeCode(w, http.StatusBadRequest, "PROFILE_INVALID", err.Error())
			return
		}
		update.AvatarURL = req.AvatarURL
	}
	if req.Timezone != nil {
		// Validated against the IANA database, not a regex: scheduling depends
		// on real daylight-saving rules (§18, §34). Empty is rejected too,
		// because time.LoadLocation("") silently resolves to UTC and would
		// quietly move a user's 6 AM to another continent.
		if strings.TrimSpace(*req.Timezone) == "" {
			writeCode(w, http.StatusBadRequest, "INVALID_TIMEZONE", "timezone is required")
			return
		}
		if _, err := time.LoadLocation(*req.Timezone); err != nil {
			writeCode(w, http.StatusBadRequest, "INVALID_TIMEZONE",
				"timezone must be a valid IANA name such as Africa/Lagos")
			return
		}
		update.Timezone = req.Timezone
	}
	if req.Locale != nil {
		if !validLocale(*req.Locale) {
			writeCode(w, http.StatusBadRequest, "INVALID_LOCALE", "locale must look like en or en-NG")
			return
		}
		update.Locale = req.Locale
	}
	if req.Language != nil {
		if !validLocale(*req.Language) {
			writeCode(w, http.StatusBadRequest, "INVALID_LOCALE", "language must look like en or en-GB")
			return
		}
		update.Language = req.Language
	}
	if req.CountryCode != nil {
		cc := strings.ToUpper(strings.TrimSpace(*req.CountryCode))
		if cc != "" && len(cc) != 2 {
			writeCode(w, http.StatusBadRequest, "PROFILE_INVALID", "country_code must be a 2-letter ISO code")
			return
		}
		update.CountryCode = &cc
	}

	p, err := h.profiles.UpdateProfile(r.Context(), h.userID(r), update)
	if errors.Is(err, store.ErrUsernameTaken) {
		writeCode(w, http.StatusConflict, "USERNAME_TAKEN", "that username is already taken")
		return
	}
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "PROFILE_UPDATE_FAILED", "failed to update profile")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p)
}

// getPreferences returns the caller's preferences.
func (h *Handler) getPreferences(w http.ResponseWriter, r *http.Request) {
	p, err := h.profiles.Preferences(r.Context(), h.userID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load preferences")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p)
}

// updatePreferences applies a partial preference update.
func (h *Handler) updatePreferences(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DefaultDuration        *int    `json:"default_duration"`
		DefaultVoiceID         *string `json:"default_voice_id"`
		Autoplay               *bool   `json:"autoplay"`
		PreferredQuality       *string `json:"preferred_quality"`
		DownloadOverWifi       *bool   `json:"download_over_wifi"`
		NotificationsEnabled   *bool   `json:"notifications_enabled"`
		RecommendationsEnabled *bool   `json:"recommendations_enabled"`
		PersonalizationEnabled *bool   `json:"personalization_enabled"`
		Language               *string `json:"language"`
		Theme                  *string `json:"theme"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeCode(w, http.StatusBadRequest, "PROFILE_INVALID", "invalid request body")
		return
	}

	userID := h.userID(r)
	update := store.PreferencesUpdate{
		Autoplay:               req.Autoplay,
		DownloadOverWifi:       req.DownloadOverWifi,
		NotificationsEnabled:   req.NotificationsEnabled,
		RecommendationsEnabled: req.RecommendationsEnabled,
		PersonalizationEnabled: req.PersonalizationEnabled,
	}

	if req.DefaultDuration != nil {
		if *req.DefaultDuration < 60 || *req.DefaultDuration > 3*3600 {
			writeCode(w, http.StatusBadRequest, "PROFILE_INVALID",
				"default_duration must be between 60 seconds and 3 hours")
			return
		}
		update.DefaultDuration = req.DefaultDuration
	}
	if req.DefaultVoiceID != nil && *req.DefaultVoiceID != "" {
		// The backend decides which voices a user may have, never the client
		// (§16). An unknown or inactive voice is refused outright.
		v, err := h.audio.VoiceByID(r.Context(), *req.DefaultVoiceID)
		if err != nil || v.Status != "active" {
			writeCode(w, http.StatusUnprocessableEntity, "VOICE_UNAVAILABLE", "that voice is not available")
			return
		}
		// Premium voices require entitlement. Storing one a free user cannot
		// use would produce silent downgrades on every future session.
		if v.Premium {
			ent := h.entitlementsFor(r.Context(), userID)
			if !ent.CanAccessPremiumVoices {
				writeCode(w, http.StatusPaymentRequired, "ENTITLEMENT_REQUIRED",
					"that voice is available on Premium")
				return
			}
		}
		update.DefaultVoiceID = req.DefaultVoiceID
	} else if req.DefaultVoiceID != nil {
		update.DefaultVoiceID = req.DefaultVoiceID
	}
	if req.PreferredQuality != nil {
		if !oneOf(*req.PreferredQuality, "low", "standard", "high") {
			writeCode(w, http.StatusBadRequest, "PROFILE_INVALID",
				"preferred_quality must be low, standard or high")
			return
		}
		update.PreferredQuality = req.PreferredQuality
	}
	if req.Language != nil {
		if !validLocale(*req.Language) {
			writeCode(w, http.StatusBadRequest, "INVALID_LOCALE", "language must look like en or en-GB")
			return
		}
		update.Language = req.Language
	}
	if req.Theme != nil {
		if !oneOf(*req.Theme, "light", "dark", "system") {
			writeCode(w, http.StatusBadRequest, "PROFILE_INVALID", "theme must be light, dark or system")
			return
		}
		update.Theme = req.Theme
	}

	p, err := h.profiles.UpdatePreferences(r.Context(), userID, update)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "PROFILE_UPDATE_FAILED", "failed to update preferences")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p)
}

// getInterests returns the caller's interests, split by provenance.
//
// Explicit and inferred interests are returned in separate lists rather than
// one merged array, so a client cannot accidentally present a machine guess as
// something the user said (§15).
func (h *Handler) getInterests(w http.ResponseWriter, r *http.Request) {
	all, err := h.profiles.Interests(r.Context(), h.userID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load interests")
		return
	}
	explicit := []any{}
	inferred := []any{}
	for _, i := range all {
		if i.Explicit {
			explicit = append(explicit, i)
		} else {
			inferred = append(inferred, i)
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"explicit": explicit,
		"inferred": inferred,
		"note":     "Explicit interests were chosen by you. Inferred interests are derived from listening and can be disabled in personalization settings.",
	})
}

// putInterests replaces the caller's explicitly chosen interests.
func (h *Handler) putInterests(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CategoryIDs []string `json:"category_ids"`
		Source      string   `json:"source"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeCode(w, http.StatusBadRequest, "PROFILE_INVALID", "invalid request body")
		return
	}
	// Only explicit sources are settable through this endpoint. A client must
	// not be able to write AI_INFERENCE rows and have them read back as the
	// user's own stated preferences.
	source := req.Source
	if source == "" {
		source = store.SourceExplicitSelection
	}
	if !store.IsExplicit(source) {
		writeCode(w, http.StatusBadRequest, "PROFILE_INVALID",
			"source must be ONBOARDING or EXPLICIT_SELECTION")
		return
	}
	if len(req.CategoryIDs) > 38 {
		writeCode(w, http.StatusBadRequest, "PROFILE_INVALID", "too many categories selected")
		return
	}

	// Categories are configured in the database, never hard-coded (§13), so
	// validity is a lookup rather than a constant list.
	valid, err := h.profiles.ValidCategoryIDs(r.Context(), req.CategoryIDs)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to validate categories")
		return
	}
	for _, id := range req.CategoryIDs {
		if !valid[id] {
			writeCode(w, http.StatusUnprocessableEntity, "CATEGORY_NOT_FOUND", "unknown category: "+id)
			return
		}
	}

	if err := h.profiles.ReplaceExplicitInterests(r.Context(), h.userID(r), req.CategoryIDs, source); err != nil {
		writeCode(w, http.StatusInternalServerError, "PROFILE_UPDATE_FAILED", "failed to save interests")
		return
	}
	h.getInterests(w, r)
}

// bootstrap returns exactly what the app needs to start (PRD §56).
//
// Deliberately excludes history, favourites and library contents: those are
// paginated surfaces the app fetches when a user navigates to them. Loading
// them here would make cold start scale with account age (§95).
func (h *Handler) bootstrap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := h.userID(r)

	u, err := h.users.ByID(ctx, userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load account")
		return
	}
	profile, err := h.profiles.Profile(ctx, userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load profile")
		return
	}
	prefs, err := h.profiles.Preferences(ctx, userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load preferences")
		return
	}
	interests, err := h.profiles.Interests(ctx, userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load interests")
		return
	}

	ent := h.entitlementsFor(ctx, userID)

	explicit := []string{}
	for _, i := range interests {
		if i.Explicit {
			explicit = append(explicit, i.CategoryID)
		}
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"user":        u,
		"profile":     profile,
		"preferences": prefs,
		// Only explicit interests are sent at startup; inferences belong to the
		// personalization surface, not the boot payload.
		"interests":    explicit,
		"subscription": map[string]any{"plan": ent.Plan},
		"entitlements": entitlementView(ent),
		// Profile completion drives an optional prompt, never a gate (§12).
		"profile_completion": profileCompletion(profile, prefs, explicit),
		"server_time":        time.Now().UTC().Format(time.RFC3339),
	})
}

// entitlementView is the explicit client projection of a plan's capabilities.
func entitlementView(e entitlements.Entitlements) map[string]any {
	return map[string]any{
		"plan":                     e.Plan,
		"premium_voices":           e.CanAccessPremiumVoices,
		"premium_content":          e.CanAccessPremiumContent,
		"offline_downloads":        e.CanDownload,
		"personal_confessions":     e.CanCreatePersonalConfessions,
		"max_session_seconds":      e.MaxSessionSeconds(),
		"max_concurrent_downloads": e.MaxConcurrentDownloads,
	}
}

// profileCompletion reports optional steps. Nothing here blocks use of the app:
// it exists to suggest, not to nag or gate (§12).
func profileCompletion(p *models.UserProfile, prefs *models.UserPreferences, interests []string) map[string]any {
	steps := []map[string]any{
		{"key": "display_name", "label": "Add your name", "done": p.DisplayName != ""},
		{"key": "interests", "label": "Choose what to focus on", "done": len(interests) > 0},
		{"key": "voice", "label": "Pick a preferred voice", "done": prefs.DefaultVoiceID != ""},
		{"key": "avatar", "label": "Add a profile photo", "done": p.AvatarURL != ""},
	}
	done := 0
	for _, s := range steps {
		if s["done"].(bool) {
			done++
		}
	}
	return map[string]any{
		"completed": done,
		"total":     len(steps),
		"steps":     steps,
	}
}

// ---------------------------------------------------------------------------
// Validation helpers (PRD §7, §8, §73, §88)
// ---------------------------------------------------------------------------

// reservedUsernames cannot be claimed: they would let a user impersonate the
// platform or collide with a route.
var reservedUsernames = map[string]bool{
	"admin": true, "administrator": true, "root": true, "system": true,
	"support": true, "help": true, "api": true, "auth": true, "login": true,
	"logout": true, "register": true, "settings": true, "me": true,
	"iconfess": true, "i-confess": true, "official": true, "staff": true,
	"moderator": true, "security": true, "billing": true, "null": true,
}

func validateUsername(u string) error {
	if n := utf8.RuneCountInString(u); n < 3 || n > 30 {
		return errors.New("username must be between 3 and 30 characters")
	}
	if reservedUsernames[u] {
		return errors.New("that username is reserved")
	}
	// ASCII-only, which keeps usernames unambiguous in URLs and prevents
	// homograph impersonation using lookalike Unicode characters.
	for _, r := range u {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '.') {
			return errors.New("username may contain only letters, numbers, underscore and period")
		}
	}
	if strings.HasPrefix(u, ".") || strings.HasSuffix(u, ".") || strings.Contains(u, "..") {
		return errors.New("username may not start or end with a period, or contain '..'")
	}
	return nil
}

// validateDisplayName is deliberately permissive about script: a name is not
// invalid for being written in Yoruba, Arabic or Chinese (§8, §86).
func validateDisplayName(name string) error {
	if name == "" {
		return nil
	}
	if n := utf8.RuneCountInString(name); n > 60 {
		return errors.New("display name must be 60 characters or fewer")
	}
	for _, r := range name {
		// Control characters are refused outright.
		if unicode.IsControl(r) {
			return errors.New("display name contains invalid characters")
		}
		// Zero-width and bidirectional formatting characters are refused
		// because they let a name render as something other than what is
		// stored — the classic display-spoofing trick. The whole U+202A..202E
		// and U+2066..2069 range is covered, not a sample of it.
		switch {
		case r == '\u200b', r == '\u200c', r == '\u200d', r == '\ufeff',
			r >= '\u200e' && r <= '\u200f',
			r >= '\u202a' && r <= '\u202e',
			r >= '\u2066' && r <= '\u2069':
			return errors.New("display name contains invalid characters")
		}
	}
	return nil
}

// validateHTTPSURL refuses anything that is not a plain https URL, which blocks
// javascript: and data: payloads from reaching a client that renders them.
func validateHTTPSURL(raw string) error {
	if raw == "" {
		return nil
	}
	if !strings.HasPrefix(raw, "https://") {
		return errors.New("url must start with https://")
	}
	if len(raw) > 2048 {
		return errors.New("url is too long")
	}
	if strings.ContainsAny(raw, " \t\n\r<>\"") {
		return errors.New("url contains invalid characters")
	}
	return nil
}

// validLocale accepts a BCP-47-ish tag: "en" or "en-NG".
func validLocale(l string) bool {
	if l == "" {
		return false
	}
	parts := strings.Split(l, "-")
	if len(parts) > 2 {
		return false
	}
	if len(parts[0]) < 2 || len(parts[0]) > 3 {
		return false
	}
	for _, r := range parts[0] {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	if len(parts) == 2 {
		if len(parts[1]) != 2 {
			return false
		}
		for _, r := range parts[1] {
			if r < 'A' || r > 'Z' {
				return false
			}
		}
	}
	return true
}

func normaliseSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

// writeCode emits a stable machine-readable error code alongside the message,
// so clients localise their own text rather than parsing ours (§49, §88).
func writeCode(w http.ResponseWriter, status int, code, message string) {
	httpx.WriteJSON(w, status, map[string]string{"error": message, "code": code})
}
