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
