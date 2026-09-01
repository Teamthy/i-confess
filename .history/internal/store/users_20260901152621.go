package store

import (
	"context"
	"database/sql"
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
