package store

import (
	"context"
	"database/sql"
	"github.com/Teamthy/i-confess/internal/db"
	"time"

	"github.com/Teamthy/i-confess/internal/models"
	"github.com/google/uuid"
)

// VoiceRightsStore manages voice authorization and legal metadata.
type VoiceRightsStore struct {
	db *db.DB
}

func NewVoiceRightsStore(db *db.DB) *VoiceRightsStore {
	return &VoiceRightsStore{db: db}
}

// Create inserts a new voice rights record.
func (s *VoiceRightsStore) Create(ctx context.Context, vr *models.VoiceRights) error {
	if vr.ID == "" {
		vr.ID = uuid.New().String()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	vr.CreatedAt = now
	vr.UpdatedAt = now

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO voice_rights (id, voice_id, rights_holder, authorization_reference, allowed_use, 
		                            territories, start_date, expiry_date, status, metadata, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		vr.ID, vr.VoiceID, vr.RightsHolder, nullIfEmpty(vr.AuthorizationReference), vr.AllowedUse,
		vr.Territories, nullIfEmpty(vr.StartDate), nullIfEmpty(vr.ExpiryDate), vr.Status,
		nullIfEmpty(vr.Metadata), vr.CreatedAt, vr.UpdatedAt)
	return err
}

// ByVoiceID retrieves voice rights for a specific voice.
func (s *VoiceRightsStore) ByVoiceID(ctx context.Context, voiceID string) (*models.VoiceRights, error) {
	var vr models.VoiceRights
	var authRef, startDate, expiryDate, metadata sql.NullString

	err := s.db.QueryRowContext(ctx,
		`SELECT id, voice_id, rights_holder, COALESCE(authorization_reference, ''), allowed_use, 
		        territories, COALESCE(start_date, ''), COALESCE(expiry_date, ''), status, 
		        COALESCE(metadata, ''), created_at, updated_at
		 FROM voice_rights WHERE voice_id = ?`, voiceID).
		Scan(&vr.ID, &vr.VoiceID, &vr.RightsHolder, &authRef, &vr.AllowedUse,
			&vr.Territories, &startDate, &expiryDate, &vr.Status,
			&metadata, &vr.CreatedAt, &vr.UpdatedAt)
	if err != nil {
		return nil, err
	}

	if authRef.Valid {
		vr.AuthorizationReference = authRef.String
	}
	if startDate.Valid {
		vr.StartDate = startDate.String
	}
	if expiryDate.Valid {
		vr.ExpiryDate = expiryDate.String
	}
	if metadata.Valid {
		vr.Metadata = metadata.String
	}

	return &vr, nil
}

// Update modifies voice rights.
func (s *VoiceRightsStore) Update(ctx context.Context, vr *models.VoiceRights) error {
	vr.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx,
		`UPDATE voice_rights SET rights_holder=?, authorization_reference=?, allowed_use=?, 
		                         territories=?, start_date=?, expiry_date=?, status=?, metadata=?, updated_at=?
		 WHERE voice_id = ?`,
		vr.RightsHolder, nullIfEmpty(vr.AuthorizationReference), vr.AllowedUse,
		vr.Territories, nullIfEmpty(vr.StartDate), nullIfEmpty(vr.ExpiryDate), vr.Status,
		nullIfEmpty(vr.Metadata), vr.UpdatedAt, vr.VoiceID)
	return err
}
