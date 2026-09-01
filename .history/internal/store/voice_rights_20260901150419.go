package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/models"
	"github.com/google/uuid"
)

// VoiceRightsStore manages voice authorization and legal metadata.
type VoiceRightsStore struct {
	db *sql.DB
}

func NewVoiceRightsStore(db *sql.DB) *VoiceRightsStore {
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

	terr := strings.Join(vr.Territories, ",")
	langs := strings.Join(vr.Languages, ",")

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO voice_licenses (id, voice_id, owner_name, license_status, commercial_use, ai_generation_allowed, 
		                              territories, languages, marketing_allowed, license_expiry, revocation_terms, 
		                              provider, provider_voice_id, notes, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		vr.ID, vr.VoiceID, vr.OwnerName, vr.LicenseStatus, boolInt(vr.CommercialUse), boolInt(vr.AIGenerationAllowed),
		terr, langs, boolInt(vr.MarketingAllowed), nullIfEmpty(vr.ExpirationDate), vr.RevocationTerms,
		vr.Provider, vr.ProviderVoiceID, vr.Notes, vr.CreatedAt, vr.UpdatedAt)
	return err
}

// ByVoiceID retrieves voice rights for a specific voice.
func (s *VoiceRightsStore) ByVoiceID(ctx context.Context, voiceID string) (*models.VoiceRights, error) {
	var vr models.VoiceRights
	var terr, langs sql.NullString
	var expDate sql.NullString

	err := s.db.QueryRowContext(ctx,
		`SELECT id, voice_id, owner_name, license_status, commercial_use, ai_generation_allowed, 
		        COALESCE(territories, ''), COALESCE(languages, ''), marketing_allowed, COALESCE(license_expiry, ''), 
		        COALESCE(revocation_terms, ''), COALESCE(provider, ''), COALESCE(provider_voice_id, ''), 
		        COALESCE(notes, ''), created_at, updated_at
		 FROM voice_licenses WHERE voice_id = ?`, voiceID).
		Scan(&vr.ID, &vr.VoiceID, &vr.OwnerName, &vr.LicenseStatus, &vr.CommercialUse, &vr.AIGenerationAllowed,
			&terr, &langs, &vr.MarketingAllowed, &expDate, &vr.RevocationTerms, &vr.Provider, &vr.ProviderVoiceID,
			&vr.Notes, &vr.CreatedAt, &vr.UpdatedAt)
	if err != nil {
		return nil, err
	}

	if terr.Valid && terr.String != "" {
		vr.Territories = strings.Split(terr.String, ",")
	}
	if langs.Valid && langs.String != "" {
		vr.Languages = strings.Split(langs.String, ",")
	}
	if expDate.Valid {
		vr.ExpirationDate = expDate.String
	}
	return &vr, nil
}

// Update modifies voice rights.
func (s *VoiceRightsStore) Update(ctx context.Context, vr *models.VoiceRights) error {
	vr.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	terr := strings.Join(vr.Territories, ",")
	langs := strings.Join(vr.Languages, ",")

	_, err := s.db.ExecContext(ctx,
		`UPDATE voice_rights SET owner_name=?, license_status=?, commercial_use=?, ai_generation_allowed=?, 
		                          territories=?, languages=?, marketing_allowed=?, expiration_date=?, 
		                          revocation_terms=?, provider=?, provider_voice_id=?, notes=?, updated_at=?
		 WHERE voice_id = ?`,
		vr.OwnerName, vr.LicenseStatus, boolInt(vr.CommercialUse), boolInt(vr.AIGenerationAllowed),
		terr, langs, boolInt(vr.MarketingAllowed), nullIfEmpty(vr.ExpirationDate), vr.RevocationTerms,
		vr.Provider, vr.ProviderVoiceID, vr.Notes, vr.UpdatedAt, vr.VoiceID)
	return err
}
