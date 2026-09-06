package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/Teamthy/i-confess/internal/db"

	"github.com/Teamthy/i-confess/internal/models"
	"github.com/google/uuid"
)

// ProfileStore owns the user profile, preferences and interests (PRD §5, §14, §20).
//
// These are deliberately separate tables rather than columns on `users`: the
// identity record is security-critical and rarely written, while preferences
// change constantly. Keeping them apart means a preference toggle never
// contends with authentication reads (§113).
type ProfileStore struct{ db *db.DB }

func NewProfileStore(db *db.DB) *ProfileStore { return &ProfileStore{db: db} }

// ---------------------------------------------------------------------------
// Profile
// ---------------------------------------------------------------------------

// Profile returns a user's profile, creating an empty one on first read so
// callers never have to handle a missing row.
func (s *ProfileStore) Profile(ctx context.Context, userID string) (*models.UserProfile, error) {
	p := &models.UserProfile{UserID: userID}
	var username, bio, avatar, cover, country sql.NullString

	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, COALESCE(display_name,''), username, bio, avatar_url, cover_url,
		        timezone, locale, country_code, language, created_at, updated_at
		 FROM user_profiles WHERE user_id = ?`, userID).
		Scan(&p.ID, &p.UserID, &p.DisplayName, &username, &bio, &avatar, &cover,
			&p.Timezone, &p.Locale, &country, &p.Language, &p.CreatedAt, &p.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return s.createProfile(ctx, userID)
	}
	if err != nil {
		return nil, err
	}
	p.Username, p.Bio = username.String, bio.String
	p.AvatarURL, p.CoverURL = avatar.String, cover.String
	p.CountryCode = country.String
	return p, nil
}

func (s *ProfileStore) createProfile(ctx context.Context, userID string) (*models.UserProfile, error) {
	p := &models.UserProfile{
		ID: uuid.New().String(), UserID: userID,
		Timezone: "UTC", Locale: "en-US", Language: "en",
		CreatedAt: now(), UpdatedAt: now(),
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_profiles (id,user_id,timezone,locale,language,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?)
		 ON CONFLICT(user_id) DO NOTHING`,
		p.ID, p.UserID, p.Timezone, p.Locale, p.Language, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// ProfileUpdate carries only the fields a caller wants changed. Pointers
// distinguish "set to empty" from "leave alone", so a partial PATCH cannot
// silently blank a field the client did not mention.
type ProfileUpdate struct {
	DisplayName *string
	Username    *string
	Bio         *string
	AvatarURL   *string
	Timezone    *string
	Locale      *string
	Language    *string
	CountryCode *string
}

// ErrUsernameTaken is returned when a username is already in use.
var ErrUsernameTaken = errors.New("username is already taken")

// UpdateProfile applies a partial update.
func (s *ProfileStore) UpdateProfile(ctx context.Context, userID string, u ProfileUpdate) (*models.UserProfile, error) {
	if _, err := s.Profile(ctx, userID); err != nil {
		return nil, err
	}

	sets := []string{}
	args := []any{}
	add := func(col string, val any) {
		sets = append(sets, col+" = ?")
		args = append(args, val)
	}

	if u.DisplayName != nil {
		add("display_name", *u.DisplayName)
	}
	if u.Username != nil {
		// Usernames are compared case-insensitively, so store the normalised
		// form and reject collisions before writing (§7).
		normalised := strings.ToLower(strings.TrimSpace(*u.Username))
		if normalised != "" {
			taken, err := s.usernameTaken(ctx, normalised, userID)
			if err != nil {
				return nil, err
			}
			if taken {
				return nil, ErrUsernameTaken
			}
		}
		add("username", nullIfEmpty(normalised))
	}
	if u.Bio != nil {
		add("bio", nullIfEmpty(*u.Bio))
	}
	if u.AvatarURL != nil {
		add("avatar_url", nullIfEmpty(*u.AvatarURL))
	}
	if u.Timezone != nil {
		add("timezone", *u.Timezone)
	}
	if u.Locale != nil {
		add("locale", *u.Locale)
	}
	if u.Language != nil {
		add("language", *u.Language)
	}
	if u.CountryCode != nil {
		add("country_code", nullIfEmpty(*u.CountryCode))
	}

	if len(sets) > 0 {
		add("updated_at", now())
		args = append(args, userID)
		// Columns are fixed identifiers from this function, never user input;
		// every value is a bound parameter (§69).
		q := "UPDATE user_profiles SET " + strings.Join(sets, ", ") + " WHERE user_id = ?"
		if _, err := s.db.ExecContext(ctx, q, args...); err != nil {
			return nil, err
		}
	}
	return s.Profile(ctx, userID)
}

func (s *ProfileStore) usernameTaken(ctx context.Context, username, excludeUserID string) (bool, error) {
	var found string
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id FROM user_profiles WHERE LOWER(username) = ? AND user_id != ? LIMIT 1`,
		username, excludeUserID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ---------------------------------------------------------------------------
// Preferences
// ---------------------------------------------------------------------------

// Preferences returns a user's preferences, creating defaults on first read.
func (s *ProfileStore) Preferences(ctx context.Context, userID string) (*models.UserPreferences, error) {
	p := &models.UserPreferences{UserID: userID}
	var voiceID sql.NullString

	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, default_duration, default_voice_id, autoplay, preferred_quality,
		        download_over_wifi, notifications_enabled, recommendations_enabled,
		        personalization_enabled, language, theme, updated_at
		 FROM user_preferences WHERE user_id = ?`, userID).
		Scan(&p.ID, &p.UserID, &p.DefaultDuration, &voiceID, &p.Autoplay, &p.PreferredQuality,
			&p.DownloadOverWifi, &p.NotificationsEnabled, &p.RecommendationsEnabled,
			&p.PersonalizationEnabled, &p.Language, &p.Theme, &p.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return s.createPreferences(ctx, userID)
	}
	if err != nil {
		return nil, err
	}
	p.DefaultVoiceID = voiceID.String
	return p, nil
}

func (s *ProfileStore) createPreferences(ctx context.Context, userID string) (*models.UserPreferences, error) {
	p := &models.UserPreferences{
		ID: uuid.New().String(), UserID: userID,
		DefaultDuration: 1800, Autoplay: true, PreferredQuality: "standard",
		DownloadOverWifi: true, NotificationsEnabled: true,
		RecommendationsEnabled: true, PersonalizationEnabled: true,
		Language: "en", Theme: "system", UpdatedAt: now(),
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_preferences (id,user_id,updated_at) VALUES (?,?,?)
		 ON CONFLICT(user_id) DO NOTHING`,
		p.ID, p.UserID, p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// PreferencesUpdate is a partial preference update.
type PreferencesUpdate struct {
	DefaultDuration        *int
	DefaultVoiceID         *string
	Autoplay               *bool
	PreferredQuality       *string
	DownloadOverWifi       *bool
	NotificationsEnabled   *bool
	RecommendationsEnabled *bool
	PersonalizationEnabled *bool
	Language               *string
	Theme                  *string
}

// UpdatePreferences applies a partial update.
func (s *ProfileStore) UpdatePreferences(ctx context.Context, userID string, u PreferencesUpdate) (*models.UserPreferences, error) {
	if _, err := s.Preferences(ctx, userID); err != nil {
		return nil, err
	}

	sets := []string{}
	args := []any{}
	add := func(col string, val any) {
		sets = append(sets, col+" = ?")
		args = append(args, val)
	}

	if u.DefaultDuration != nil {
		add("default_duration", *u.DefaultDuration)
	}
	if u.DefaultVoiceID != nil {
		add("default_voice_id", nullIfEmpty(*u.DefaultVoiceID))
	}
	if u.Autoplay != nil {
		add("autoplay", boolInt(*u.Autoplay))
	}
	if u.PreferredQuality != nil {
		add("preferred_quality", *u.PreferredQuality)
	}
	if u.DownloadOverWifi != nil {
		add("download_over_wifi", boolInt(*u.DownloadOverWifi))
	}
	if u.NotificationsEnabled != nil {
		add("notifications_enabled", boolInt(*u.NotificationsEnabled))
	}
	if u.RecommendationsEnabled != nil {
		add("recommendations_enabled", boolInt(*u.RecommendationsEnabled))
	}
	if u.PersonalizationEnabled != nil {
		add("personalization_enabled", boolInt(*u.PersonalizationEnabled))
	}
	if u.Language != nil {
		add("language", *u.Language)
	}
	if u.Theme != nil {
		add("theme", *u.Theme)
	}

	if len(sets) > 0 {
		add("updated_at", now())
		args = append(args, userID)
		q := "UPDATE user_preferences SET " + strings.Join(sets, ", ") + " WHERE user_id = ?"
		if _, err := s.db.ExecContext(ctx, q, args...); err != nil {
			return nil, err
		}
	}
	return s.Preferences(ctx, userID)
}

// ---------------------------------------------------------------------------
// Interests (PRD §14, §15)
// ---------------------------------------------------------------------------

// Interest sources. Explicit choices and machine inferences are stored with
// distinct sources and must never be conflated: presenting an inference as the
// user's own stated preference is dishonest (§15).
const (
	SourceOnboarding        = "ONBOARDING"
	SourceExplicitSelection = "EXPLICIT_SELECTION"
	SourceFavorite          = "FAVORITE"
	SourceListeningBehavior = "LISTENING_BEHAVIOR"
	SourceRecommendation    = "RECOMMENDATION"
	SourceAIInference       = "AI_INFERENCE"
)

// IsExplicit reports whether a source represents a choice the user actually made.
func IsExplicit(source string) bool {
	return source == SourceOnboarding || source == SourceExplicitSelection
}

// Interests lists a user's interests, newest first.
func (s *ProfileStore) Interests(ctx context.Context, userID string) ([]models.UserInterest, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, category_id, weight, source, created_at, updated_at
		 FROM user_interests WHERE user_id = ? ORDER BY weight DESC, created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.UserInterest{}
	for rows.Next() {
		var i models.UserInterest
		if err := rows.Scan(&i.ID, &i.UserID, &i.CategoryID, &i.Weight, &i.Source, &i.CreatedAt, &i.UpdatedAt); err != nil {
			return nil, err
		}
		i.Explicit = IsExplicit(i.Source)
		out = append(out, i)
	}
	return out, rows.Err()
}

// ReplaceExplicitInterests sets the user's stated interests in one transaction.
//
// Only explicit rows are replaced. Inferred interests are left untouched,
// because a user editing their stated preferences is not asking the system to
// forget what it has observed — and conversely, observation must never
// overwrite what they stated.
func (s *ProfileStore) ReplaceExplicitInterests(ctx context.Context, userID string, categoryIDs []string, source string) error {
	if !IsExplicit(source) {
		return errors.New("ReplaceExplicitInterests requires an explicit source")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM user_interests WHERE user_id = ? AND source IN (?,?)`,
		userID, SourceOnboarding, SourceExplicitSelection); err != nil {
		return err
	}
	for _, catID := range categoryIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO user_interests (id,user_id,category_id,weight,source,created_at,updated_at)
			 VALUES (?,?,?,?,?,?,?)
			 ON CONFLICT(user_id, category_id, source) DO UPDATE SET weight = excluded.weight, updated_at = excluded.updated_at`,
			uuid.New().String(), userID, catID, 1.0, source, now(), now()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ValidCategoryIDs filters a list down to categories that actually exist, so an
// invalid id is rejected rather than stored as a dangling reference (§73).
func (s *ProfileStore) ValidCategoryIDs(ctx context.Context, ids []string) (map[string]bool, error) {
	valid := map[string]bool{}
	if len(ids) == 0 {
		return valid, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM categories WHERE id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		valid[id] = true
	}
	return valid, rows.Err()
}
