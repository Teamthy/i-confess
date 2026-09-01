package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Teamthy/i-confess/internal/models"
	"github.com/google/uuid"
)

var ErrNotFound = errors.New("not found")

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func newID() string { return uuid.NewString() }

// UserStore manages users, subscriptions, and admin roles.
type UserStore struct{ db *sql.DB }

func NewUserStore(db *sql.DB) *UserStore { return &UserStore{db: db} }

func (s *UserStore) Create(ctx context.Context, email, hash, name, tz string) (*models.User, error) {
	u := &models.User{ID: newID(), Email: email, DisplayName: name, Timezone: tz, Status: "active", CreatedAt: now()}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, display_name, timezone, status, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?)`,
		u.ID, u.Email, hash, u.DisplayName, u.Timezone, u.Status, u.CreatedAt, u.CreatedAt)
	if err != nil {
		return nil, err
	}
	// Every user gets a free subscription and default session preferences.
	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO subscriptions (id, user_id, plan, status, created_at) VALUES (?,?,?,?,?)`,
		newID(), u.ID, "free", "active", now())
	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO session_preferences (user_id, default_duration_seconds, updated_at) VALUES (?,1800,?)`,
		u.ID, now())
	return u, nil
}

func (s *UserStore) ByEmail(ctx context.Context, email string) (*models.User, string, error) {
	var u models.User
	var hash string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, COALESCE(display_name,''), timezone, status, created_at
		 FROM users WHERE email = ?`, email).
		Scan(&u.ID, &u.Email, &hash, &u.DisplayName, &u.Timezone, &u.Status, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	return &u, hash, nil
}

func (s *UserStore) ByID(ctx context.Context, id string) (*models.User, error) {
	var u models.User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, COALESCE(display_name,''), timezone, status, created_at FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Email, &u.DisplayName, &u.Timezone, &u.Status, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *UserStore) AdminRole(ctx context.Context, userID string) (string, error) {
	var role string
	err := s.db.QueryRowContext(ctx,
		`SELECT role FROM admin_users WHERE user_id = ?`, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return role, err
}

func (s *UserStore) SetAdminRole(ctx context.Context, userID, role string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO admin_users (id, user_id, role, created_at) VALUES (?,?,?,?)
		 ON CONFLICT(user_id) DO UPDATE SET role = excluded.role`,
		newID(), userID, role, now())
	return err
}

func (s *UserStore) Subscription(ctx context.Context, userID string) (string, error) {
	var plan string
	err := s.db.QueryRowContext(ctx,
		`SELECT plan FROM subscriptions WHERE user_id = ? AND status = 'active' ORDER BY created_at DESC LIMIT 1`,
		userID).Scan(&plan)
	if errors.Is(err, sql.ErrNoRows) {
		return "free", nil
	}
	return plan, err
}

func (s *UserStore) SetSubscription(ctx context.Context, userID, plan, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE subscriptions SET plan = ?, status = ? WHERE user_id = ?`, plan, status, userID)
	return err
}

// UpdatePassword updates a user's password hash.
func (s *UserStore) UpdatePassword(ctx context.Context, userID, newHash string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`,
		newHash, now(), userID)
	return err
}

// SetStatus updates a user's account status (active, suspended, deleted, etc).
func (s *UserStore) SetStatus(ctx context.Context, userID, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET status = ?, updated_at = ? WHERE id = ?`,
		status, now(), userID)
	return err
}

// RemoveAdminRole removes a user's admin privileges.
func (s *UserStore) RemoveAdminRole(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM admin_users WHERE user_id = ?`, userID)
	return err
}

// ListAdminsByRole returns all admins with a specific role.
func (s *UserStore) ListAdminsByRole(ctx context.Context, role string, limit int) ([]*models.User, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT u.id, u.email, COALESCE(u.display_name,''), u.timezone, u.status, u.created_at
		 FROM users u INNER JOIN admin_users a ON u.id = a.user_id
		 WHERE a.role = ? LIMIT ?`, role, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Email, &u.DisplayName, &u.Timezone, &u.Status, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, &u)
	}
	return users, rows.Err()
}

// ListAllAdmins returns all admin users.
func (s *UserStore) ListAllAdmins(ctx context.Context, limit int) ([]*models.User, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT u.id, u.email, COALESCE(u.display_name,''), u.timezone, u.status, u.created_at
		 FROM users u INNER JOIN admin_users a ON u.id = a.user_id
		 LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Email, &u.DisplayName, &u.Timezone, &u.Status, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, &u)
	}
	return users, rows.Err()
}

func (s *UserStore) UpsertProfile(ctx context.Context, userID, displayName, username, bio, avatarURL, timezone, locale, countryCode string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_profiles (id, user_id, display_name, username, bio, avatar_url, timezone, locale, country_code, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET display_name = excluded.display_name, username = excluded.username, bio = excluded.bio, avatar_url = excluded.avatar_url, timezone = excluded.timezone, locale = excluded.locale, country_code = excluded.country_code, updated_at = excluded.updated_at`,
		newID(), userID, displayName, username, bio, avatarURL, timezone, locale, countryCode, now(), now())
	return err
}

func (s *UserStore) GetProfile(ctx context.Context, userID string) (map[string]any, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT display_name, username, bio, avatar_url, timezone, locale, country_code, created_at, updated_at FROM user_profiles WHERE user_id = ?`, userID)
	var displayName, username, bio, avatarURL, timezone, locale, countryCode, createdAt, updatedAt string
	if err := row.Scan(&displayName, &username, &bio, &avatarURL, &timezone, &locale, &countryCode, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	return map[string]any{
		"user_id":      userID,
		"display_name": displayName,
		"username":     username,
		"bio":          bio,
		"avatar_url":   avatarURL,
		"timezone":     timezone,
		"locale":       locale,
		"country_code": countryCode,
		"created_at":   createdAt,
		"updated_at":   updatedAt,
	}, nil
}

func (s *UserStore) UpsertPreference(ctx context.Context, userID, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_preferences (id, user_id, default_duration, default_voice_id, autoplay, preferred_quality, download_over_wifi, notifications_enabled, recommendations_enabled, personalization_enabled, language, theme, updated_at)
		 VALUES (?, ?, COALESCE((SELECT default_duration FROM user_preferences WHERE user_id = ?), 1800), COALESCE((SELECT default_voice_id FROM user_preferences WHERE user_id = ?), ''), COALESCE((SELECT autoplay FROM user_preferences WHERE user_id = ?), 1), COALESCE((SELECT preferred_quality FROM user_preferences WHERE user_id = ?), 'standard'), COALESCE((SELECT download_over_wifi FROM user_preferences WHERE user_id = ?), 1), COALESCE((SELECT notifications_enabled FROM user_preferences WHERE user_id = ?), 1), COALESCE((SELECT recommendations_enabled FROM user_preferences WHERE user_id = ?), 1), COALESCE((SELECT personalization_enabled FROM user_preferences WHERE user_id = ?), 1), COALESCE((SELECT language FROM user_preferences WHERE user_id = ?), 'en'), COALESCE((SELECT theme FROM user_preferences WHERE user_id = ?), 'system'), ?)
		 ON CONFLICT(user_id) DO UPDATE SET `+key+` = excluded.`+key+`, updated_at = excluded.updated_at`,
		newID(), userID, userID, userID, userID, userID, userID, userID, userID, userID, userID, userID, now())
	return err
}

func (s *UserStore) GetPreferences(ctx context.Context, userID string) (map[string]any, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT default_duration, default_voice_id, autoplay, preferred_quality, download_over_wifi, notifications_enabled, recommendations_enabled, personalization_enabled, language, theme, updated_at FROM user_preferences WHERE user_id = ?`, userID)
	var defaultDuration int
	var defaultVoiceID, preferredQuality, language, theme, updatedAt sql.NullString
	var autoplay, downloadOverWifi, notificationsEnabled, recommendationsEnabled, personalizationEnabled int
	if err := row.Scan(&defaultDuration, &defaultVoiceID, &autoplay, &preferredQuality, &downloadOverWifi, &notificationsEnabled, &recommendationsEnabled, &personalizationEnabled, &language, &theme, &updatedAt); err != nil {
		return nil, err
	}
	return map[string]any{
		"user_id":                 userID,
		"default_duration":        defaultDuration,
		"default_voice_id":        defaultVoiceID.String,
		"autoplay":                autoplay == 1,
		"preferred_quality":       preferredQuality.String,
		"download_over_wifi":      downloadOverWifi == 1,
		"notifications_enabled":   notificationsEnabled == 1,
		"recommendations_enabled": recommendationsEnabled == 1,
		"personalization_enabled": personalizationEnabled == 1,
		"language":                language.String,
		"theme":                   theme.String,
		"updated_at":              updatedAt.String,
	}, nil
}

func (s *UserStore) UpsertInterest(ctx context.Context, userID, categoryID string, weight float64, source string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_interests (id, user_id, category_id, weight, source, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id, category_id, source) DO UPDATE SET weight = excluded.weight, updated_at = excluded.updated_at`,
		newID(), userID, categoryID, weight, source, now(), now())
	return err
}

func (s *UserStore) ListInterests(ctx context.Context, userID string) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT category_id, weight, source, created_at, updated_at FROM user_interests WHERE user_id = ? ORDER BY updated_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var categoryID, source, createdAt, updatedAt string
		var weight float64
		if err := rows.Scan(&categoryID, &weight, &source, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"category_id": categoryID, "weight": weight, "source": source, "created_at": createdAt, "updated_at": updatedAt})
	}
	return out, rows.Err()
}

func (s *UserStore) SetDefaultVoicePreference(ctx context.Context, userID, voiceID string) error {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO user_voice_preferences (id, user_id, voice_id, is_default, created_at) VALUES (?, ?, ?, 1, ?) ON CONFLICT(user_id, voice_id) DO UPDATE SET is_default = 1`, newID(), userID, voiceID, now()); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE user_voice_preferences SET is_default = CASE WHEN voice_id = ? THEN 1 ELSE 0 END WHERE user_id = ?`, voiceID, userID)
	return err
}

func (s *UserStore) GetDefaultVoicePreference(ctx context.Context, userID string) (string, error) {
	var voiceID string
	err := s.db.QueryRowContext(ctx,
		`SELECT voice_id FROM user_voice_preferences WHERE user_id = ? AND is_default = 1 LIMIT 1`, userID).Scan(&voiceID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return voiceID, err
}

func (s *UserStore) SetEmailVerified(ctx context.Context, userID string, verified bool) error {
	v := 0
	if verified {
		v = 1
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET email_verified = ?, updated_at = ? WHERE id = ?`, v, now(), userID)
	return err
}

func (s *UserStore) CreateVerificationToken(ctx context.Context, userID, purpose, token string, minutes int) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO email_verification_tokens (id, user_id, purpose, token, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`, newID(), userID, purpose, token, time.Now().Add(time.Duration(minutes)*time.Minute).UTC().Format(time.RFC3339), now())
	return err
}

func (s *UserStore) VerifyEmailToken(ctx context.Context, userID, token string) (bool, error) {
	var expiresAt string
	var usedAt sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT expires_at, used_at FROM email_verification_tokens WHERE user_id = ? AND token = ? AND (used_at IS NULL)`, userID, token).
		Scan(&expiresAt, &usedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if time.Now().UTC().After(func() time.Time { t, _ := time.Parse(time.RFC3339, expiresAt); return t }()) {
		return false, nil
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE email_verification_tokens SET used_at = ? WHERE user_id = ? AND token = ?`, now(), userID, token); err != nil {
		return false, err
	}
	if err := s.SetEmailVerified(ctx, userID, true); err != nil {
		return false, err
	}
	return true, nil
}

func (s *UserStore) CreatePasswordReset(ctx context.Context, userID, token string, minutes int) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO password_reset_tokens (id, user_id, token, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?)`, newID(), userID, token, time.Now().Add(time.Duration(minutes)*time.Minute).UTC().Format(time.RFC3339), now())
	return err
}

func (s *UserStore) ResetPassword(ctx context.Context, userID, token, newHash string) error {
	var expiresAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT expires_at FROM password_reset_tokens WHERE user_id = ? AND token = ? AND used_at IS NULL`, userID, token).Scan(&expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if time.Now().UTC().After(func() time.Time { t, _ := time.Parse(time.RFC3339, expiresAt); return t }()) {
		return errors.New("reset token expired")
	}
	if err := s.UpdatePassword(ctx, userID, newHash); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE password_reset_tokens SET used_at = ? WHERE user_id = ? AND token = ?`, now(), userID, token)
	return err
}

func (s *UserStore) UserIDByPasswordResetToken(ctx context.Context, token string) (string, error) {
	var userID string
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id FROM password_reset_tokens WHERE token = ? AND used_at IS NULL`, token).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return userID, err
}

func (s *UserStore) UserIDByVerificationToken(ctx context.Context, token string) (string, error) {
	var userID string
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id FROM email_verification_tokens WHERE token = ? AND used_at IS NULL`, token).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return userID, err
}

func (s *UserStore) CreateRefreshToken(ctx context.Context, token, userID, deviceID, platform string) (string, error) {
	hash := token
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO refresh_tokens (id, user_id, token_hash, device_id, platform, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`, newID(), userID, hash, deviceID, platform, time.Now().Add(30*24*time.Hour).UTC().Format(time.RFC3339), now())
	return hash, err
}

func (s *UserStore) RotateRefreshToken(ctx context.Context, currentToken, userID, deviceID, platform string) (string, error) {
	var rowID string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM refresh_tokens WHERE user_id = ? AND token_hash = ? AND revoked_at IS NULL`, userID, currentToken).Scan(&rowID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	newToken := uuid.NewString()
	_, err = s.db.ExecContext(ctx,
		`UPDATE refresh_tokens SET revoked_at = ?, replaced_by = ? WHERE id = ?`, now(), newToken, rowID)
	if err != nil {
		return "", err
	}
	_, err = s.CreateRefreshToken(ctx, newToken, userID, deviceID, platform)
	if err != nil {
		return "", err
	}
	return newToken, nil
}

func (s *UserStore) RevokeRefreshToken(ctx context.Context, userID, token string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE refresh_tokens SET revoked_at = ? WHERE user_id = ? AND token_hash = ? AND revoked_at IS NULL`, now(), userID, token)
	return err
}

func (s *UserStore) RevokeAllSessions(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE refresh_tokens SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL`, now(), userID)
	return err
}

func (s *UserStore) TrackDevice(ctx context.Context, userID, deviceID, platform, userAgent string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_devices (id, user_id, device_id, platform, user_agent, last_seen_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id, device_id) DO UPDATE SET platform = excluded.platform, user_agent = excluded.user_agent, last_seen_at = excluded.last_seen_at`,
		newID(), userID, deviceID, platform, userAgent, now(), now())
	return err
}

func (s *UserStore) ListDevices(ctx context.Context, userID string) ([]map[string]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT device_id, platform, user_agent, last_seen_at, revoked_at FROM user_devices WHERE user_id = ? ORDER BY last_seen_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]string
	for rows.Next() {
		var deviceID, platform, userAgent, lastSeen, revoked sql.NullString
		if err := rows.Scan(&deviceID, &platform, &userAgent, &lastSeen, &revoked); err != nil {
			return nil, err
		}
		entry := map[string]string{
			"device_id":    deviceID.String,
			"platform":     platform.String,
			"user_agent":   userAgent.String,
			"last_seen_at": lastSeen.String,
			"revoked_at":   revoked.String,
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

func (s *UserStore) RecordSecurityEvent(ctx context.Context, userID, eventType, ip, userAgent string, metadata map[string]string) error {
	jsonMeta := "{}"
	if len(metadata) > 0 {
		b, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		jsonMeta = string(b)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO security_events (id, user_id, event_type, ip_address, user_agent, metadata, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		newID(), userID, eventType, ip, userAgent, jsonMeta, now())
	return err
}

func (s *UserStore) RecordConsent(ctx context.Context, userID, category, version, ip, userAgent string, granted bool) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO consent_records (id, user_id, category, granted, version, ip_address, user_agent, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, newID(), userID, category, boolToInt(granted), version, ip, userAgent, now())
	return err
}

func (s *UserStore) EnableMFA(ctx context.Context, userID, secret string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO mfa_secrets (user_id, secret, enabled, updated_at) VALUES (?, ?, 1, ?)
		 ON CONFLICT(user_id) DO UPDATE SET secret = excluded.secret, enabled = 1, updated_at = excluded.updated_at`,
		userID, secret, now())
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE users SET mfa_enabled = 1, updated_at = ? WHERE id = ?`, now(), userID)
	return err
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
