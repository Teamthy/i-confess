package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/voiceengine"
	"github.com/Teamthy/i-confess/internal/voicegov"
	"github.com/google/uuid"
)

// VoicePlatformStore persists the licensed-voice platform: minister voices,
// granular rights, models, references, pronunciations and generations.
//
// Grants are always read fresh. Nothing here caches a rights decision, so a
// revocation written by one process is honoured by the next generation in any
// other process (§67).
type VoicePlatformStore struct{ db *db.DB }

// NewVoicePlatformStore constructs the store.
func NewVoicePlatformStore(d *db.DB) *VoicePlatformStore { return &VoicePlatformStore{db: d} }

// ErrVoiceNotFound is returned for unknown voices.
var ErrVoiceNotFound = errors.New("voice not found")

// ErrIllegalTransition is returned for a lifecycle change voicegov forbids.
var ErrIllegalTransition = errors.New("illegal rights status transition")

// tsLayout is fixed-width so stored timestamps sort lexicographically, which
// the SQL expiry comparison relies on. RFC3339Nano trims trailing zeros and
// would sort "12:00:00Z" after "12:00:00.5Z".
const tsLayout = "2006-01-02T15:04:05.000000Z"

func tsNow() string { return time.Now().UTC().Format(tsLayout) }

// MinisterVoice is the public-facing voice identity (§5). It deliberately
// carries no legal detail; rights live in the grant.
type MinisterVoice struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	DisplayName        string `json:"displayName"`
	Description        string `json:"description,omitempty"`
	Language           string `json:"language"`
	Locale             string `json:"locale,omitempty"`
	Accent             string `json:"accent,omitempty"`
	GenderPresentation string `json:"genderPresentation,omitempty"`
	Status             string `json:"status"`
	CloneEnabled       bool   `json:"cloneEnabled"`
	TrainingEnabled    bool   `json:"trainingEnabled"`
	GenerationEnabled  bool   `json:"generationEnabled"`
	Synthetic          bool   `json:"synthetic"`
	SampleURL          string `json:"sampleUrl,omitempty"`
}

const voiceCols = `id, name, COALESCE(display_name, name), COALESCE(description,''), language,
	COALESCE(locale,''), COALESCE(accent,''), COALESCE(gender_presentation,''), status,
	clone_enabled, training_enabled, generation_enabled, synthetic, COALESCE(sample_url,'')`

func scanVoice(sc interface{ Scan(...any) error }) (*MinisterVoice, error) {
	var v MinisterVoice
	var clone, train, gen, syn int
	if err := sc.Scan(&v.ID, &v.Name, &v.DisplayName, &v.Description, &v.Language, &v.Locale, &v.Accent,
		&v.GenderPresentation, &v.Status, &clone, &train, &gen, &syn, &v.SampleURL); err != nil {
		return nil, err
	}
	v.CloneEnabled, v.TrainingEnabled, v.GenerationEnabled, v.Synthetic = clone == 1, train == 1, gen == 1, syn == 1
	return &v, nil
}

// CreateMinisterVoice registers a synthetic minister voice. It starts with
// every capability off and a PENDING grant: registering a voice authorizes
// nothing.
func (s *VoicePlatformStore) CreateMinisterVoice(ctx context.Context, v *MinisterVoice, rightsHolder, actor string) error {
	if v.ID == "" {
		v.ID = "voice_" + uuid.NewString()[:8]
	}
	if v.Language == "" {
		v.Language = "en"
	}
	v.Status, v.Synthetic = "active", true
	v.CloneEnabled, v.TrainingEnabled, v.GenerationEnabled = false, false, false
	now := tsNow()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.ExecContext(ctx, `INSERT INTO voices (id, name, display_name, description, type, provider, gender,
		language, locale, accent, gender_presentation, status, synthetic, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'minister', 'voice-engine', ?, ?, ?, ?, ?, 'active', 1, ?, ?)`,
		v.ID, v.Name, v.DisplayName, nullIfEmpty(v.Description), nullIfEmpty(v.GenderPresentation),
		v.Language, nullIfEmpty(v.Locale), nullIfEmpty(v.Accent), nullIfEmpty(v.GenderPresentation), now, now); err != nil {
		return fmt.Errorf("insert voice: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO voice_rights_grants (id, voice_id, rights_holder, status, created_at, updated_at)
		VALUES (?, ?, ?, 'PENDING', ?, ?)`, uuid.NewString(), v.ID, rightsHolder, now, now); err != nil {
		return fmt.Errorf("insert grant: %w", err)
	}
	if err := appendAudit(ctx, tx, RightsAuditEntry{VoiceID: v.ID, Actor: actor, Action: "VOICE_REGISTERED", Decision: "n/a", GrantVersion: 1}); err != nil {
		return err
	}
	return tx.Commit()
}

// ListMinisterVoices returns synthetic minister voices. When publicOnly is
// set, only voices whose grant currently authorizes are returned, so the
// library never advertises a voice that cannot be played (§45).
func (s *VoicePlatformStore) ListMinisterVoices(ctx context.Context, publicOnly bool) ([]MinisterVoice, error) {
	q := `SELECT ` + voiceCols + ` FROM voices v WHERE synthetic = 1 AND deleted_at IS NULL`
	if publicOnly {
		q += ` AND status = 'active' AND generation_enabled = 1 AND EXISTS (
			SELECT 1 FROM voice_rights_grants g WHERE g.voice_id = v.id
			AND g.status IN ('APPROVED','RESTRICTED') AND g.deleted_at IS NULL
			AND (g.expires_at IS NULL OR g.expires_at > ?))`
	}
	q += ` ORDER BY name`
	var rows *sql.Rows
	var err error
	if publicOnly {
		rows, err = s.db.QueryContext(ctx, q, tsNow())
	} else {
		rows, err = s.db.QueryContext(ctx, q)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MinisterVoice
	for rows.Next() {
		v, err := scanVoice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

// MinisterVoiceByID returns one voice.
func (s *VoicePlatformStore) MinisterVoiceByID(ctx context.Context, id string) (*MinisterVoice, error) {
	v, err := scanVoice(s.db.QueryRowContext(ctx, `SELECT `+voiceCols+` FROM voices WHERE id = ? AND synthetic = 1 AND deleted_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrVoiceNotFound
	}
	return v, err
}

// ---------------------------------------------------------------- grants

// Grant loads the live grant with its capability rows.
func (s *VoicePlatformStore) Grant(ctx context.Context, voiceID string) (*voicegov.Grant, error) {
	var g voicegov.Grant
	var status, from, exp, restr, terr, langs, post string
	err := s.db.QueryRowContext(ctx, `SELECT voice_id, rights_holder, status, COALESCE(effective_from,''), COALESCE(expires_at,''),
		restrictions, territories, languages, post_termination, grant_version
		FROM voice_rights_grants WHERE voice_id = ? AND deleted_at IS NULL`, voiceID).
		Scan(&g.VoiceID, &g.RightsHolder, &status, &from, &exp, &restr, &terr, &langs, &post, &g.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	g.Status, g.PostTermination = voicegov.Status(status), voicegov.AssetPolicy(post)
	if g.EffectiveFrom, err = parseOptTime(from); err != nil {
		return nil, err
	}
	if g.ExpiresAt, err = parseOptTime(exp); err != nil {
		return nil, err
	}
	for _, p := range splitCSV(restr) {
		g.Restrictions = append(g.Restrictions, voicegov.ContentPurpose(p))
	}
	g.Territories, g.Languages = splitCSV(terr), splitCSV(langs)

	rows, err := s.db.QueryContext(ctx, `SELECT capability, granted FROM voice_usage_permissions WHERE voice_id = ? AND deleted_at IS NULL`, voiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	g.Capabilities = map[voicegov.Capability]bool{}
	for rows.Next() {
		var c string
		var granted int
		if err := rows.Scan(&c, &granted); err != nil {
			return nil, err
		}
		// Unknown capability strings in the database are ignored rather than
		// honoured: only capabilities the code understands can authorize.
		if voicegov.ValidCapability(voicegov.Capability(c)) {
			g.Capabilities[voicegov.Capability(c)] = granted == 1
		}
	}
	return &g, rows.Err()
}

// GrantTerms is the editable part of a grant. Status changes go through
// TransitionGrant so they are validated and separately audited.
type GrantTerms struct {
	RightsHolder    string
	EffectiveFrom   *time.Time
	ExpiresAt       *time.Time
	Capabilities    map[voicegov.Capability]bool
	Restrictions    []voicegov.ContentPurpose
	Territories     []string
	Languages       []string
	PostTermination voicegov.AssetPolicy
	Notes           string
}

// UpdateGrantTerms replaces the grant's terms, bumps its version and writes one
// permission row per known capability (explicit false included), all in one
// transaction with an audit entry.
func (s *VoicePlatformStore) UpdateGrantTerms(ctx context.Context, voiceID string, t GrantTerms, actor, remote string) (int, error) {
	for c := range t.Capabilities {
		if !voicegov.ValidCapability(c) {
			return 0, fmt.Errorf("unknown capability %q", c)
		}
	}
	for _, p := range t.Restrictions {
		if !voicegov.ValidPurpose(p) {
			return 0, fmt.Errorf("unknown purpose %q", p)
		}
	}
	if t.PostTermination == "" {
		t.PostTermination = voicegov.AssetUnpublish
	}
	if !voicegov.ValidAssetPolicy(t.PostTermination) {
		return 0, fmt.Errorf("unknown post-termination policy %q", t.PostTermination)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck
	now := tsNow()
	var version int
	var status string
	err = tx.QueryRowContext(ctx, `UPDATE voice_rights_grants SET rights_holder = COALESCE(NULLIF(?, ''), rights_holder),
		effective_from = ?, expires_at = ?, restrictions = ?, territories = ?, languages = ?, post_termination = ?,
		notes = ?, grant_version = grant_version + 1, row_version = row_version + 1, updated_at = ?
		WHERE voice_id = ? AND deleted_at IS NULL RETURNING grant_version, status`,
		t.RightsHolder, fmtOptTime(t.EffectiveFrom), fmtOptTime(t.ExpiresAt), joinPurposes(t.Restrictions),
		strings.Join(t.Territories, ","), strings.Join(t.Languages, ","), string(t.PostTermination),
		nullIfEmpty(t.Notes), now, voiceID).Scan(&version, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrVoiceNotFound
	}
	if err != nil {
		return 0, err
	}
	if voicegov.Status(status) == voicegov.StatusRevoked {
		return 0, fmt.Errorf("%w: a revoked grant cannot be edited; register a new agreement", ErrIllegalTransition)
	}
	for _, c := range voicegov.AllCapabilities {
		if _, err := tx.ExecContext(ctx, `INSERT INTO voice_usage_permissions (id, voice_id, capability, granted, grant_version, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (voice_id, capability) DO UPDATE SET granted = EXCLUDED.granted, grant_version = EXCLUDED.grant_version,
			updated_at = EXCLUDED.updated_at, row_version = voice_usage_permissions.row_version + 1`,
			uuid.NewString(), voiceID, string(c), boolInt(t.Capabilities[c]), version, now, now); err != nil {
			return 0, fmt.Errorf("write capability %s: %w", c, err)
		}
	}
	if err := syncVoiceFlags(ctx, tx, voiceID, voicegov.Status(status), t.Capabilities); err != nil {
		return 0, err
	}
	granted := make([]string, 0)
	for _, c := range voicegov.AllCapabilities {
		if t.Capabilities[c] {
			granted = append(granted, string(c))
		}
	}
	if err := appendAudit(ctx, tx, RightsAuditEntry{VoiceID: voiceID, Actor: actor, Action: "RIGHTS_TERMS_UPDATED",
		Decision: "n/a", GrantVersion: version, Detail: "granted: " + strings.Join(granted, ","), RemoteAddr: remote}); err != nil {
		return 0, err
	}
	return version, tx.Commit()
}

// TransitionGrant moves a grant through its lifecycle. Suspension, expiry and
// revocation immediately clear the voice's generation/training flags in the
// same transaction.
func (s *VoicePlatformStore) TransitionGrant(ctx context.Context, voiceID string, to voicegov.Status, actor, reason, remote string) error {
	if !to.Valid() {
		return fmt.Errorf("%w: unknown status %q", ErrIllegalTransition, to)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	var from string
	var version int
	err = tx.QueryRowContext(ctx, `SELECT status, grant_version FROM voice_rights_grants WHERE voice_id = ? AND deleted_at IS NULL FOR UPDATE`, voiceID).Scan(&from, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrVoiceNotFound
	}
	if err != nil {
		return err
	}
	if !voicegov.CanTransition(voicegov.Status(from), to) {
		return fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, from, to)
	}
	if to.Authorizing() {
		// Approval requires at least one signed rights document on file.
		var docs int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM voice_rights_documents WHERE voice_id = ? AND deleted_at IS NULL`, voiceID).Scan(&docs); err != nil {
			return err
		}
		if docs == 0 {
			return fmt.Errorf("%w: a rights document must be on file before %s", ErrIllegalTransition, to)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE voice_rights_grants SET status = ?, grant_version = grant_version + 1,
		row_version = row_version + 1, updated_at = ? WHERE voice_id = ?`, string(to), tsNow(), voiceID); err != nil {
		return err
	}
	caps := map[voicegov.Capability]bool{}
	rows, err := tx.QueryContext(ctx, `SELECT capability, granted FROM voice_usage_permissions WHERE voice_id = ?`, voiceID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var c string
		var g int
		if err := rows.Scan(&c, &g); err != nil {
			rows.Close()
			return err
		}
		caps[voicegov.Capability(c)] = g == 1
	}
	rows.Close()
	if err := syncVoiceFlags(ctx, tx, voiceID, to, caps); err != nil {
		return err
	}
	if err := appendAudit(ctx, tx, RightsAuditEntry{VoiceID: voiceID, Actor: actor, Action: "RIGHTS_STATUS_" + string(to),
		Decision: "n/a", GrantVersion: version + 1, Reason: from + "->" + string(to), Detail: reason, RemoteAddr: remote}); err != nil {
		return err
	}
	return tx.Commit()
}

// syncVoiceFlags keeps the denormalised voice flags consistent with the grant.
// They are a display convenience; Authorize remains the gate.
func syncVoiceFlags(ctx context.Context, tx *db.Tx, voiceID string, st voicegov.Status, caps map[voicegov.Capability]bool) error {
	on := st.Authorizing()
	_, err := tx.ExecContext(ctx, `UPDATE voices SET clone_enabled = ?, training_enabled = ?, generation_enabled = ?, updated_at = ? WHERE id = ?`,
		boolInt(on && caps[voicegov.CanClone]), boolInt(on && caps[voicegov.CanTrain]),
		boolInt(on && caps[voicegov.CanGenerate] && caps[voicegov.CanClone]), tsNow(), voiceID)
	return err
}

// AddRightsDocument records a licence document (the file itself lives in a
// private bucket).
func (s *VoicePlatformStore) AddRightsDocument(ctx context.Context, voiceID, docType, title, storageKey, sha, actor string) (string, error) {
	if storageKey == "" || sha == "" {
		return "", errors.New("storage key and sha256 are required")
	}
	id := uuid.NewString()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.ExecContext(ctx, `INSERT INTO voice_rights_documents (id, voice_id, document_type, title, storage_key, sha256, uploaded_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, id, voiceID, docType, title, storageKey, sha, actor, tsNow()); err != nil {
		return "", err
	}
	if err := appendAudit(ctx, tx, RightsAuditEntry{VoiceID: voiceID, Actor: actor, Action: "RIGHTS_DOCUMENT_ADDED", Decision: "n/a", Detail: docType + ":" + sha}); err != nil {
		return "", err
	}
	return id, tx.Commit()
}

// ---------------------------------------------------------------- audit

// RightsAuditEntry is one row of the append-only voice audit log (§66).
type RightsAuditEntry struct {
	ID           string `json:"id"`
	VoiceID      string `json:"voiceId"`
	Actor        string `json:"actor"`
	Action       string `json:"action"`
	ModelID      string `json:"modelId,omitempty"`
	GenerationID string `json:"generationId,omitempty"`
	GrantVersion int    `json:"grantVersion,omitempty"`
	Decision     string `json:"decision"`
	Reason       string `json:"reason,omitempty"`
	Detail       string `json:"detail,omitempty"`
	RemoteAddr   string `json:"remoteAddr,omitempty"`
	CreatedAt    string `json:"timestamp"`
}

type execer interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
}

func appendAudit(ctx context.Context, ex execer, e RightsAuditEntry) error {
	_, err := ex.ExecContext(ctx, `INSERT INTO voice_rights_audit_logs (id, voice_id, actor, action, model_id, generation_id,
		grant_version, decision, reason, detail, remote_addr, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), e.VoiceID, e.Actor, e.Action, nullIfEmpty(e.ModelID), nullIfEmpty(e.GenerationID),
		e.GrantVersion, e.Decision, nullIfEmpty(e.Reason), nullIfEmpty(e.Detail), nullIfEmpty(e.RemoteAddr), tsNow())
	if err != nil {
		return fmt.Errorf("voice audit: %w", err)
	}
	return nil
}

// AppendRightsAudit writes an audit entry outside any other transaction, e.g.
// for generation decisions.
func (s *VoicePlatformStore) AppendRightsAudit(ctx context.Context, e RightsAuditEntry) error {
	return appendAudit(ctx, s.db, e)
}

// RightsAudit returns the newest entries for a voice.
func (s *VoicePlatformStore) RightsAudit(ctx context.Context, voiceID string, limit int) ([]RightsAuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, voice_id, actor, action, COALESCE(model_id,''), COALESCE(generation_id,''),
		COALESCE(grant_version,0), COALESCE(decision,''), COALESCE(reason,''), COALESCE(detail,''), COALESCE(remote_addr,''), created_at
		FROM voice_rights_audit_logs WHERE voice_id = ? ORDER BY created_at DESC LIMIT ?`, voiceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RightsAuditEntry
	for rows.Next() {
		var e RightsAuditEntry
		if err := rows.Scan(&e.ID, &e.VoiceID, &e.Actor, &e.Action, &e.ModelID, &e.GenerationID, &e.GrantVersion,
			&e.Decision, &e.Reason, &e.Detail, &e.RemoteAddr, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- models

// CreateModel registers a new model version. Versions are immutable: an
// existing (voice, version) pair is a conflict, never an overwrite (§61).
func (s *VoicePlatformStore) CreateModel(ctx context.Context, m *voiceengine.Model, modelName, language, actor string) error {
	if m.ID == "" {
		m.ID = "model_" + uuid.NewString()[:8]
	}
	if m.Mode == "" {
		m.Mode = "zero_shot"
	}
	m.Status = voiceengine.ModelCandidate
	now := tsNow()
	_, err := s.db.ExecContext(ctx, `INSERT INTO voice_models (id, voice_id, engine, engine_version, model_name, model_version, mode,
		checkpoint_key, dataset_version, training_run_id, language, status, license_reviewed, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'candidate', ?, ?, ?)`,
		m.ID, m.VoiceID, string(m.Engine), m.EngineVersion, modelName, m.ModelVersion, m.Mode,
		nullIfEmpty(m.CheckpointURI), nullIfEmpty(m.DatasetVersion), nullIfEmpty(m.TrainingRunID), language,
		boolInt(m.LicenseReviewed), now, now)
	if err != nil {
		return fmt.Errorf("create model: %w", err)
	}
	return appendAudit(ctx, s.db, RightsAuditEntry{VoiceID: m.VoiceID, Actor: actor, Action: "MODEL_REGISTERED", ModelID: m.ID, Decision: "n/a"})
}

// Models returns all live models for a voice.
func (s *VoicePlatformStore) Models(ctx context.Context, voiceID string) ([]voiceengine.Model, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, voice_id, engine, engine_version, model_version, mode, COALESCE(dataset_version,''),
		COALESCE(training_run_id,''), COALESCE(checkpoint_key,''), status, icf_voice_score, fallback_approved, license_reviewed, COALESCE(promoted_at,'')
		FROM voice_models WHERE voice_id = ? AND deleted_at IS NULL ORDER BY created_at`, voiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []voiceengine.Model
	for rows.Next() {
		var m voiceengine.Model
		var engine, status, promoted string
		var fb, lic int
		if err := rows.Scan(&m.ID, &m.VoiceID, &engine, &m.EngineVersion, &m.ModelVersion, &m.Mode, &m.DatasetVersion,
			&m.TrainingRunID, &m.CheckpointURI, &status, &m.ICFVoiceScore, &fb, &lic, &promoted); err != nil {
			return nil, err
		}
		m.Engine, m.Status, m.FallbackApproved, m.LicenseReviewed = voiceengine.Engine(engine), voiceengine.ModelStatus(status), fb == 1, lic == 1
		if t, err := parseOptTime(promoted); err == nil && t != nil {
			m.PromotedAt = *t
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SetModelEvaluation records a gate outcome on a model. PASS moves it to
// approved; FAIL to rejected; NEEDS_REVIEW to evaluating.
func (s *VoicePlatformStore) SetModelEvaluation(ctx context.Context, modelID string, score float64, verdict string, fallbackApproved bool, actor string) error {
	status := voiceengine.ModelEvaluating
	switch verdict {
	case "PASS":
		status = voiceengine.ModelApproved
	case "FAIL":
		status = voiceengine.ModelRejected
	}
	var voiceID string
	err := s.db.QueryRowContext(ctx, `UPDATE voice_models SET icf_voice_score = ?, status = ?, fallback_approved = ?, approved_by = ?,
		updated_at = ?, row_version = row_version + 1 WHERE id = ? AND status IN ('candidate','evaluating','approved') RETURNING voice_id`,
		score, string(status), boolInt(fallbackApproved && status == voiceengine.ModelApproved), nullIfEmpty(actor), tsNow(), modelID).Scan(&voiceID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("model %s not found or not in an evaluable state", modelID)
	}
	if err != nil {
		return err
	}
	return appendAudit(ctx, s.db, RightsAuditEntry{VoiceID: voiceID, Actor: actor, Action: "MODEL_EVALUATED_" + verdict, ModelID: modelID, Decision: "n/a"})
}

// ApplyModelChanges writes the output of voiceengine.Promote/Rollback in one
// transaction. Retirements are applied before the promotion so the
// one-production-per-voice index never sees two.
func (s *VoicePlatformStore) ApplyModelChanges(ctx context.Context, voiceID string, changes map[string]voiceengine.ModelStatus, actor, action string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	now := tsNow()
	var promoted string
	for id, st := range changes {
		if st == voiceengine.ModelProduction {
			promoted = id
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE voice_models SET status = ?, updated_at = ?, row_version = row_version + 1 WHERE id = ? AND voice_id = ?`,
			string(st), now, id, voiceID); err != nil {
			return err
		}
	}
	if promoted != "" {
		res, err := tx.ExecContext(ctx, `UPDATE voice_models SET status = 'production', promoted_at = ?, approved_by = ?, updated_at = ?,
			row_version = row_version + 1 WHERE id = ? AND voice_id = ?`, now, actor, now, promoted, voiceID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return fmt.Errorf("model %s not found for voice %s", promoted, voiceID)
		}
	}
	if err := appendAudit(ctx, tx, RightsAuditEntry{VoiceID: voiceID, Actor: actor, Action: action, ModelID: promoted, Decision: "n/a"}); err != nil {
		return err
	}
	return tx.Commit()
}

// SaveEvaluation stores an evaluation report.
func (s *VoicePlatformStore) SaveEvaluation(ctx context.Context, voiceID, modelID, goldenSet string, metrics, weights, report any, score, coverage float64, verdict, actor string) (string, error) {
	mj, _ := json.Marshal(metrics)
	wj, _ := json.Marshal(weights)
	rj, _ := json.Marshal(report)
	id := uuid.NewString()
	_, err := s.db.ExecContext(ctx, `INSERT INTO voice_evaluations (id, voice_id, model_id, golden_set, metrics, weights, icf_voice_score,
		coverage, verdict, report, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, voiceID, modelID, goldenSet, string(mj), string(wj), score, coverage, verdict, string(rj), actor, tsNow())
	return id, err
}

// ---------------------------------------------------------------- references & dictionary

// AddReference records a reference clip.
func (s *VoicePlatformStore) AddReference(ctx context.Context, r voiceengine.Reference, actor string) (string, error) {
	if !voiceengine.ValidStyle(r.Style) {
		return "", fmt.Errorf("unknown style %q", r.Style)
	}
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	now := tsNow()
	_, err := s.db.ExecContext(ctx, `INSERT INTO voice_references (id, voice_id, style, audio_key, transcript, duration_ms, quality_score,
		rights_ok, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.VoiceID, r.Style, r.URI, r.Transcript, r.DurationMS, r.Quality, boolInt(r.RightsOK), now, now)
	if err != nil {
		return "", err
	}
	return r.ID, appendAudit(ctx, s.db, RightsAuditEntry{VoiceID: r.VoiceID, Actor: actor, Action: "REFERENCE_ADDED", Decision: "n/a", Detail: r.Style})
}

// References returns active references for a voice.
func (s *VoicePlatformStore) References(ctx context.Context, voiceID string) ([]voiceengine.Reference, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, voice_id, style, audio_key, transcript, duration_ms, quality_score, rights_ok
		FROM voice_references WHERE voice_id = ? AND status = 'active' AND deleted_at IS NULL`, voiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []voiceengine.Reference
	for rows.Next() {
		var r voiceengine.Reference
		var ok int
		if err := rows.Scan(&r.ID, &r.VoiceID, &r.Style, &r.URI, &r.Transcript, &r.DurationMS, &r.Quality, &ok); err != nil {
			return nil, err
		}
		r.RightsOK = ok == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

// Pronunciations loads the dictionary and a version string (latest update
// time) that feeds the content hash, so editing an entry re-renders affected
// audio instead of serving stale pronunciations.
func (s *VoicePlatformStore) Pronunciations(ctx context.Context) ([]voiceengine.PronunciationEntry, string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT term, locale, COALESCE(respelling,''), COALESCE(ipa,''), aliases, provider_overrides,
		COALESCE(category,''), updated_at FROM pronunciation_dictionary WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	var out []voiceengine.PronunciationEntry
	version := ""
	for rows.Next() {
		var e voiceengine.PronunciationEntry
		var aliases, overrides, updated string
		if err := rows.Scan(&e.Term, &e.Locale, &e.Respelling, &e.IPA, &aliases, &overrides, &e.Category, &updated); err != nil {
			return nil, "", err
		}
		e.Aliases = splitCSV(aliases)
		_ = json.Unmarshal([]byte(overrides), &e.ProviderOverrides)
		if updated > version {
			version = updated
		}
		out = append(out, e)
	}
	return out, version, rows.Err()
}

// UpsertPronunciation creates or replaces a dictionary entry.
func (s *VoicePlatformStore) UpsertPronunciation(ctx context.Context, e voiceengine.PronunciationEntry) error {
	if strings.TrimSpace(e.Term) == "" || (e.Respelling == "" && e.IPA == "") {
		return errors.New("term and a respelling or IPA are required")
	}
	ov, _ := json.Marshal(e.ProviderOverrides)
	if e.ProviderOverrides == nil {
		ov = []byte("{}")
	}
	now := tsNow()
	_, err := s.db.ExecContext(ctx, `INSERT INTO pronunciation_dictionary (id, term, locale, respelling, ipa, aliases, provider_overrides, category, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (term, locale) DO UPDATE SET respelling = EXCLUDED.respelling, ipa = EXCLUDED.ipa, aliases = EXCLUDED.aliases,
		provider_overrides = EXCLUDED.provider_overrides, category = EXCLUDED.category, updated_at = EXCLUDED.updated_at,
		row_version = pronunciation_dictionary.row_version + 1, deleted_at = NULL`,
		uuid.NewString(), e.Term, e.Locale, nullIfEmpty(e.Respelling), nullIfEmpty(e.IPA), strings.Join(e.Aliases, ","), string(ov),
		nullIfEmpty(e.Category), now, now)
	return err
}

// ---------------------------------------------------------------- generations

// Generation is one synthetic render record (§55).
type Generation struct {
	ID             string `json:"generationId"`
	ContentHash    string `json:"contentHash"`
	VoiceID        string `json:"voiceId"`
	ModelID        string `json:"modelId,omitempty"`
	Engine         string `json:"engine,omitempty"`
	EngineVersion  string `json:"engineVersion,omitempty"`
	Purpose        string `json:"purpose"`
	Style          string `json:"style"`
	Language       string `json:"language"`
	Locale         string `json:"locale,omitempty"`
	TextSHA256     string `json:"textHash"`
	AudioSHA256    string `json:"audioHash,omitempty"`
	StorageKey     string `json:"-"`
	DurationMS     int    `json:"durationMs,omitempty"`
	Synthetic      bool   `json:"synthetic"`
	FellBack       bool   `json:"fellBack"`
	FallbackReason string `json:"fallbackReason,omitempty"`
	GrantVersion   int    `json:"grantVersion,omitempty"`
	Queue          string `json:"queue"`
	JobID          string `json:"jobId,omitempty"`
	OwnerUserID    string `json:"-"`
	Visibility     string `json:"visibility"`
	Status         string `json:"status"`
	ErrorClass     string `json:"errorClass,omitempty"`
	ErrorMessage   string `json:"error,omitempty"`
	RequestedBy    string `json:"-"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

const genCols = `id, content_hash, voice_id, COALESCE(model_id,''), COALESCE(engine,''), COALESCE(engine_version,''), purpose, style,
	language, COALESCE(locale,''), text_sha256, COALESCE(audio_sha256,''), COALESCE(storage_key,''), COALESCE(duration_ms,0), synthetic,
	fell_back, COALESCE(fallback_reason,''), COALESCE(grant_version,0), queue, COALESCE(job_id,''), COALESCE(owner_user_id,''), visibility,
	status, COALESCE(error_class,''), COALESCE(error_message,''), COALESCE(requested_by,''), created_at, updated_at`

func scanGen(sc interface{ Scan(...any) error }) (*Generation, error) {
	var g Generation
	var syn, fb int
	err := sc.Scan(&g.ID, &g.ContentHash, &g.VoiceID, &g.ModelID, &g.Engine, &g.EngineVersion, &g.Purpose, &g.Style, &g.Language,
		&g.Locale, &g.TextSHA256, &g.AudioSHA256, &g.StorageKey, &g.DurationMS, &syn, &fb, &g.FallbackReason, &g.GrantVersion,
		&g.Queue, &g.JobID, &g.OwnerUserID, &g.Visibility, &g.Status, &g.ErrorClass, &g.ErrorMessage, &g.RequestedBy,
		&g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return nil, err
	}
	g.Synthetic, g.FellBack = syn == 1, fb == 1
	return &g, nil
}

// CreateOrGetGeneration inserts a generation keyed by content hash. If one
// already exists (and is not failed/cancelled) it is returned with
// created=false - the dedup that keeps identical renders from being paid for
// twice (§29, §60). Failed rows are reset to QUEUED for another attempt.
func (s *VoicePlatformStore) CreateOrGetGeneration(ctx context.Context, g *Generation) (*Generation, bool, error) {
	if g.ID == "" {
		g.ID = "gen_" + uuid.NewString()
	}
	now := tsNow()
	row := s.db.QueryRowContext(ctx, `INSERT INTO voice_generations (id, content_hash, voice_id, model_id, engine, engine_version, purpose, style,
		language, locale, text_sha256, grant_version, queue, owner_user_id, visibility, status, requested_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'QUEUED', ?, ?, ?)
		ON CONFLICT (content_hash) DO NOTHING RETURNING `+genCols,
		g.ID, g.ContentHash, g.VoiceID, nullIfEmpty(g.ModelID), nullIfEmpty(g.Engine), nullIfEmpty(g.EngineVersion), g.Purpose, g.Style,
		g.Language, nullIfEmpty(g.Locale), g.TextSHA256, g.GrantVersion, g.Queue, nullIfEmpty(g.OwnerUserID), g.Visibility,
		nullIfEmpty(g.RequestedBy), now, now)
	created, err := scanGen(row)
	if err == nil {
		return created, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	existing, err := scanGen(s.db.QueryRowContext(ctx, `SELECT `+genCols+` FROM voice_generations WHERE content_hash = ?`, g.ContentHash))
	if err != nil {
		return nil, false, err
	}
	if existing.Status == string(voiceengine.GenFailed) || existing.Status == string(voiceengine.GenCancelled) {
		reset, err := scanGen(s.db.QueryRowContext(ctx, `UPDATE voice_generations SET status = 'QUEUED', error_class = NULL, error_message = NULL,
			grant_version = ?, updated_at = ?, row_version = row_version + 1 WHERE id = ? AND status IN ('FAILED','CANCELLED') RETURNING `+genCols,
			g.GrantVersion, now, existing.ID))
		if err == nil {
			return reset, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, false, err
		}
	}
	return existing, false, nil
}

// SetGenerationJob links a queued job.
func (s *VoicePlatformStore) SetGenerationJob(ctx context.Context, id, jobID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE voice_generations SET job_id = ?, updated_at = ? WHERE id = ?`, jobID, tsNow(), id)
	return err
}

// GenerationByID returns one generation.
func (s *VoicePlatformStore) GenerationByID(ctx context.Context, id string) (*Generation, error) {
	g, err := scanGen(s.db.QueryRowContext(ctx, `SELECT `+genCols+` FROM voice_generations WHERE id = ? AND deleted_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return g, err
}

// AdvanceGeneration moves a generation to status, refusing to leave a
// terminal state.
func (s *VoicePlatformStore) AdvanceGeneration(ctx context.Context, id string, st voiceengine.GenerationStatus) error {
	res, err := s.db.ExecContext(ctx, `UPDATE voice_generations SET status = ?, updated_at = ?, row_version = row_version + 1
		WHERE id = ? AND status NOT IN ('COMPLETED','FAILED','CANCELLED')`, string(st), tsNow(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("generation %s is terminal or missing", id)
	}
	return nil
}

// CompleteGeneration records the produced asset.
func (s *VoicePlatformStore) CompleteGeneration(ctx context.Context, g *Generation) error {
	_, err := s.db.ExecContext(ctx, `UPDATE voice_generations SET status = 'COMPLETED', model_id = ?, engine = ?, engine_version = ?,
		audio_sha256 = ?, storage_key = ?, duration_ms = ?, fell_back = ?, fallback_reason = ?, grant_version = ?, updated_at = ?,
		row_version = row_version + 1 WHERE id = ? AND status NOT IN ('COMPLETED','CANCELLED')`,
		g.ModelID, g.Engine, g.EngineVersion, g.AudioSHA256, g.StorageKey, g.DurationMS, boolInt(g.FellBack),
		nullIfEmpty(g.FallbackReason), g.GrantVersion, tsNow(), g.ID)
	return err
}

// FailGeneration records a classified failure.
func (s *VoicePlatformStore) FailGeneration(ctx context.Context, id, class, msg string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE voice_generations SET status = 'FAILED', error_class = ?, error_message = ?, updated_at = ?,
		row_version = row_version + 1 WHERE id = ? AND status NOT IN ('COMPLETED','CANCELLED')`, class, msg, tsNow(), id)
	return err
}

// CancelGeneration cancels if still cancellable. It returns false if the job
// had progressed too far.
func (s *VoicePlatformStore) CancelGeneration(ctx context.Context, id string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE voice_generations SET status = 'CANCELLED', updated_at = ?, row_version = row_version + 1
		WHERE id = ? AND status IN ('QUEUED','PROCESSING','GENERATED','POST_PROCESSING')`, tsNow(), id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ---------------------------------------------------------------- helpers

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func joinPurposes(ps []voicegov.ContentPurpose) string {
	s := make([]string, len(ps))
	for i, p := range ps {
		s[i] = string(p)
	}
	return strings.Join(s, ",")
}

func parseOptTime(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return nil, fmt.Errorf("parse time %q: %w", s, err)
	}
	return &t, nil
}

func fmtOptTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(tsLayout)
}
