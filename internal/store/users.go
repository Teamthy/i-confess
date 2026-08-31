package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/Teamthy/i-confess/internal/models"
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
