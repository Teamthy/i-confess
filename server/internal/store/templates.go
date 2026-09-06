package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/db"
)

// Template is a saved custom session configuration.
type Template struct {
	ID          string   `json:"id"`
	UserID      string   `json:"user_id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	CategoryIDs []string `json:"category_ids"`
	Weights     string   `json:"weights,omitempty"` // JSON map
	VoiceID     string   `json:"voice_id,omitempty"`
	VoiceRules  string   `json:"voice_rules,omitempty"` // JSON
	Ordering    string   `json:"ordering,omitempty"`    // JSON e.g. {"strategy":"BALANCED"}
	IsPublic    bool     `json:"is_public"`
	ShareToken  string   `json:"share_token,omitempty"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

type TemplateStore struct{ db *db.DB }

func NewTemplateStore(db *db.DB) *TemplateStore { return &TemplateStore{db: db} }

func (s *TemplateStore) Create(ctx context.Context, t *Template) error {
	if t.ID == "" {
		t.ID = newID()
	}
	if t.ShareToken == "" {
		t.ShareToken = newShareToken()
	}
	t.CreatedAt, t.UpdatedAt = now(), now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_templates (id,user_id,name,description,category_ids,weights,voice_id,voice_rules,ordering,is_public,share_token,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.UserID, t.Name, nullIfEmpty(t.Description), strings.Join(t.CategoryIDs, ","),
		nullIfEmpty(t.Weights), nullIfEmpty(t.VoiceID), nullIfEmpty(t.VoiceRules), nullIfEmpty(t.Ordering),
		boolInt(t.IsPublic), nullIfEmpty(t.ShareToken), t.CreatedAt, t.UpdatedAt)
	return err
}

func (s *TemplateStore) ByID(ctx context.Context, id string) (*Template, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,user_id,name,COALESCE(description,''),category_ids,COALESCE(weights,''),COALESCE(voice_id,''),COALESCE(voice_rules,''),COALESCE(ordering,''),is_public,COALESCE(share_token,''),created_at,updated_at
		 FROM user_templates WHERE id = ?`, id)
	return scanTemplate(row)
}

func (s *TemplateStore) ByShareToken(ctx context.Context, token string) (*Template, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,user_id,name,COALESCE(description,''),category_ids,COALESCE(weights,''),COALESCE(voice_id,''),COALESCE(voice_rules,''),COALESCE(ordering,''),is_public,COALESCE(share_token,''),created_at,updated_at
		 FROM user_templates WHERE share_token = ?`, token)
	return scanTemplate(row)
}

func (s *TemplateStore) ListByUser(ctx context.Context, userID string) ([]Template, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,user_id,name,COALESCE(description,''),category_ids,COALESCE(weights,''),COALESCE(voice_id,''),COALESCE(voice_rules,''),COALESCE(ordering,''),is_public,COALESCE(share_token,''),created_at,updated_at
		 FROM user_templates WHERE user_id = ? ORDER BY updated_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Template
	for rows.Next() {
		var t Template
		var cats string
		var weights, voiceID, voiceRules, ordering, shareToken sql.NullString
		var desc sql.NullString
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &desc, &cats, &weights, &voiceID, &voiceRules, &ordering, &t.IsPublic, &shareToken, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.Description = desc.String
		t.CategoryIDs = strings.Split(cats, ",")
		if cats == "" {
			t.CategoryIDs = []string{}
		}
		t.Weights = weights.String
		t.VoiceID = voiceID.String
		t.VoiceRules = voiceRules.String
		t.Ordering = ordering.String
		t.ShareToken = shareToken.String
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *TemplateStore) Update(ctx context.Context, t *Template) error {
	t.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`UPDATE user_templates SET name=?, description=?, category_ids=?, weights=?, voice_id=?, voice_rules=?, ordering=?, is_public=?, updated_at=? WHERE id=? AND user_id=?`,
		t.Name, nullIfEmpty(t.Description), strings.Join(t.CategoryIDs, ","), nullIfEmpty(t.Weights),
		nullIfEmpty(t.VoiceID), nullIfEmpty(t.VoiceRules), nullIfEmpty(t.Ordering), boolInt(t.IsPublic), t.UpdatedAt, t.ID, t.UserID)
	return err
}

func (s *TemplateStore) Delete(ctx context.Context, id, userID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM user_templates WHERE id=? AND user_id=?`, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanTemplate(row *sql.Row) (*Template, error) {
	var t Template
	var cats string
	var weights, voiceID, voiceRules, ordering, shareToken sql.NullString
	var desc sql.NullString
	if err := row.Scan(&t.ID, &t.UserID, &t.Name, &desc, &cats, &weights, &voiceID, &voiceRules, &ordering, &t.IsPublic, &shareToken, &t.CreatedAt, &t.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	t.Description = desc.String
	if cats != "" {
		t.CategoryIDs = strings.Split(cats, ",")
	}
	t.Weights = weights.String
	t.VoiceID = voiceID.String
	t.VoiceRules = voiceRules.String
	t.Ordering = ordering.String
	t.ShareToken = shareToken.String
	return &t, nil
}

func newShareToken() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("060102150405")))
	}
	return hex.EncodeToString(b) // 12 hex chars, e.g. a1b2c3d4e5f6
}
