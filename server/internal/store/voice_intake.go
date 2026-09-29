package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/Teamthy/i-confess/internal/voicedata"
)

// ErrNotFound is returned by intake lookups for a missing row.
var ErrIntakeNotFound = errors.New("not found")

// Recording is a licensed source recording for a minister voice.
type Recording struct {
	ID                   string          `json:"id"`
	VoiceID              string          `json:"voiceId"`
	SourceType           string          `json:"sourceType"`
	SourceURL            string          `json:"sourceUrl,omitempty"`
	Title                string          `json:"title"`
	RecordingDate        string          `json:"recordingDate,omitempty"`
	Speaker              string          `json:"speaker,omitempty"`
	RightsDocumentID     string          `json:"rightsDocumentId"`
	ProcessingPermission bool            `json:"processingPermission"`
	TrainingAllowed      bool            `json:"trainingAllowed"`
	StorageKey           string          `json:"-"`
	SHA256               string          `json:"sha256"`
	Language             string          `json:"language"`
	DurationMS           int             `json:"durationMs,omitempty"`
	Status               string          `json:"status"`
	Report               json.RawMessage `json:"report,omitempty"`
	Error                string          `json:"error,omitempty"`
	JobID                string          `json:"jobId,omitempty"`
	UploadedBy           string          `json:"uploadedBy,omitempty"`
	CreatedAt            string          `json:"createdAt"`
}

const recCols = `id, voice_id, source_type, COALESCE(source_url,''), COALESCE(recording_title,''), COALESCE(recording_date,''),
	COALESCE(speaker,''), COALESCE(rights_document_id,''), processing_permission, training_allowed, storage_key, sha256,
	COALESCE(language,''), COALESCE(duration_ms,0), status, COALESCE(ingest_report,''), COALESCE(error_message,''),
	COALESCE(job_id,''), COALESCE(uploaded_by,''), created_at`

func scanRecording(sc interface{ Scan(...any) error }) (*Recording, error) {
	var r Recording
	var pp, ta int
	var report string
	if err := sc.Scan(&r.ID, &r.VoiceID, &r.SourceType, &r.SourceURL, &r.Title, &r.RecordingDate, &r.Speaker,
		&r.RightsDocumentID, &pp, &ta, &r.StorageKey, &r.SHA256, &r.Language, &r.DurationMS, &r.Status, &report,
		&r.Error, &r.JobID, &r.UploadedBy, &r.CreatedAt); err != nil {
		return nil, err
	}
	r.ProcessingPermission, r.TrainingAllowed = pp == 1, ta == 1
	if report != "" {
		r.Report = json.RawMessage(report)
	}
	return &r, nil
}

// CreateRecording registers a recording. The rights document must belong to
// the same voice; a recording without paperwork cannot be ingested.
func (s *VoicePlatformStore) CreateRecording(ctx context.Context, r *Recording) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM voice_rights_documents WHERE id = ? AND voice_id = ?`,
		r.RightsDocumentID, r.VoiceID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("rights document %q is not on file for this voice", r.RightsDocumentID)
	}
	r.ID, r.Status, r.CreatedAt = "rec_"+uuid.NewString(), "uploaded", tsNow()
	_, err := s.db.ExecContext(ctx, `INSERT INTO voice_recordings (id, voice_id, source_type, source_url, recording_title,
		recording_date, speaker, rights_document_id, processing_permission, training_allowed, storage_key, sha256, language,
		status, uploaded_by, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.VoiceID, r.SourceType, nullIfEmpty(r.SourceURL), r.Title, nullIfEmpty(r.RecordingDate), nullIfEmpty(r.Speaker),
		r.RightsDocumentID, boolInt(r.ProcessingPermission), boolInt(r.TrainingAllowed), r.StorageKey, r.SHA256,
		nullIfEmpty(r.Language), r.Status, nullIfEmpty(r.UploadedBy), r.CreatedAt, r.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			return fmt.Errorf("this recording (same SHA-256) is already registered for the voice")
		}
		return err
	}
	return appendAudit(ctx, s.db, RightsAuditEntry{VoiceID: r.VoiceID, Actor: r.UploadedBy, Action: "RECORDING_REGISTERED",
		Decision: "allowed", Detail: r.ID + " doc=" + r.RightsDocumentID})
}

// RecordingByID loads one recording.
func (s *VoicePlatformStore) RecordingByID(ctx context.Context, id string) (*Recording, error) {
	r, err := scanRecording(s.db.QueryRowContext(ctx, `SELECT `+recCols+` FROM voice_recordings WHERE id = ? AND deleted_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrIntakeNotFound
	}
	return r, err
}

// Recordings lists a voice's recordings, newest first.
func (s *VoicePlatformStore) Recordings(ctx context.Context, voiceID string) ([]Recording, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+recCols+` FROM voice_recordings WHERE voice_id = ? AND deleted_at IS NULL
		ORDER BY created_at DESC`, voiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Recording{}
	for rows.Next() {
		r, err := scanRecording(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// SetRecordingJob records the ingestion job and moves the recording to processing.
func (s *VoicePlatformStore) SetRecordingJob(ctx context.Context, id, jobID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE voice_recordings SET job_id = ?, status = 'processing', updated_at = ?,
		row_version = row_version + 1 WHERE id = ?`, jobID, tsNow(), id)
	return err
}

// FailRecording marks ingestion failed (or rejected, for rights/content errors).
func (s *VoicePlatformStore) FailRecording(ctx context.Context, id, status, msg string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE voice_recordings SET status = ?, error_message = ?, updated_at = ?,
		row_version = row_version + 1 WHERE id = ?`, status, msg, tsNow(), id)
	return err
}

// IntakeSegment is a segment in the pool or in a frozen dataset.
type IntakeSegment struct {
	ID                 string   `json:"id"`
	VoiceID            string   `json:"voiceId"`
	RecordingID        string   `json:"recordingId"`
	DatasetID          string   `json:"datasetId,omitempty"`
	Split              string   `json:"split,omitempty"`
	StartMS            int      `json:"startMs"`
	EndMS              int      `json:"endMs"`
	AudioKey           string   `json:"-"`
	Style              string   `json:"style,omitempty"`
	RawTranscript      string   `json:"rawTranscript"`
	VerifiedTranscript string   `json:"verifiedTranscript,omitempty"`
	ASRConfidence      *float64 `json:"asrConfidence"`
	Quality            float64  `json:"quality"`
	SNRDB              *float64 `json:"snrDb"`
	SpeakerConfidence  *float64 `json:"speakerConfidence"`
	Flags              []string `json:"flags,omitempty"`
	Status             string   `json:"status"`
	RejectReason       string   `json:"rejectReason,omitempty"`
	VerifiedBy         string   `json:"verifiedBy,omitempty"`
}

const segCols = `id, COALESCE(voice_id,''), recording_id, COALESCE(dataset_id,''), COALESCE(split,''), start_ms, end_ms, audio_key,
	COALESCE(style,''), COALESCE(raw_transcript,''), COALESCE(verified_transcript,''), asr_confidence, COALESCE(quality_score,0),
	snr_db, speaker_confidence, COALESCE(screen_flags,''), status, COALESCE(reject_reason,''), COALESCE(verified_by,'')`

func scanSegment(sc interface{ Scan(...any) error }) (*IntakeSegment, error) {
	var g IntakeSegment
	var asr, snr, spk sql.NullFloat64
	var flags string
	if err := sc.Scan(&g.ID, &g.VoiceID, &g.RecordingID, &g.DatasetID, &g.Split, &g.StartMS, &g.EndMS, &g.AudioKey,
		&g.Style, &g.RawTranscript, &g.VerifiedTranscript, &asr, &g.Quality, &snr, &spk, &flags, &g.Status,
		&g.RejectReason, &g.VerifiedBy); err != nil {
		return nil, err
	}
	g.ASRConfidence, g.SNRDB, g.SpeakerConfidence = nf(asr), nf(snr), nf(spk)
	g.Flags = splitCSV(flags)
	return &g, nil
}

func nf(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	x := v.Float64
	return &x
}

func fp(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

// CompleteIngestion writes the worker's segments into the intake pool and
// marks the recording processed, in one transaction. Re-running ingestion
// replaces only the recording's still-pending pool segments; human decisions
// are never overwritten. Segments failing automated screening are stored as
// rejected with their reasons, so reviewers can see what was dropped.
func (s *VoicePlatformStore) CompleteIngestion(ctx context.Context, rec *Recording, segs []voicedata.Segment,
	th voicedata.Thresholds, durationMS int, report []byte) (kept, rejected int, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback() //nolint:errcheck
	now := tsNow()
	if _, err := tx.ExecContext(ctx, `DELETE FROM voice_dataset_segments WHERE recording_id = ? AND dataset_id IS NULL
		AND status = 'pending'`, rec.ID); err != nil {
		return 0, 0, err
	}
	for _, sg := range segs {
		flags := voicedata.Screen(sg, th)
		status, reason := "pending", ""
		if len(flags) > 0 {
			status, reason = "rejected", "auto_screen"
			rejected++
		} else {
			kept++
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO voice_dataset_segments (id, voice_id, recording_id, start_ms, end_ms,
			audio_key, raw_transcript, asr_confidence, quality_score, snr_db, speaker_confidence, screen_flags, status,
			reject_reason, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			"seg_"+uuid.NewString(), rec.VoiceID, rec.ID, sg.StartMS, sg.EndMS, sg.AudioKey, nullIfEmpty(sg.RawTranscript),
			fp(sg.ASRConfidence), sg.Quality, fp(sg.SNRDB), fp(sg.SpeakerConfidence), nullIfEmpty(strings.Join(flags, ",")),
			status, nullIfEmpty(reason), now, now); err != nil {
			return 0, 0, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE voice_recordings SET status = 'processed', duration_ms = ?, ingest_report = ?,
		error_message = NULL, updated_at = ?, row_version = row_version + 1 WHERE id = ?`,
		durationMS, string(report), now, rec.ID); err != nil {
		return 0, 0, err
	}
	if err := appendAudit(ctx, tx, RightsAuditEntry{VoiceID: rec.VoiceID, Actor: "worker", Action: "RECORDING_INGESTED",
		Decision: "n/a", Detail: fmt.Sprintf("%s kept=%d auto_rejected=%d", rec.ID, kept, rejected)}); err != nil {
		return 0, 0, err
	}
	return kept, rejected, tx.Commit()
}

// PoolSegments lists intake-pool segments for a voice, optionally by status.
func (s *VoicePlatformStore) PoolSegments(ctx context.Context, voiceID, status string, limit int) ([]IntakeSegment, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT ` + segCols + ` FROM voice_dataset_segments WHERE voice_id = ? AND dataset_id IS NULL AND deleted_at IS NULL`
	args := []any{voiceID}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY recording_id, start_ms LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []IntakeSegment{}
	for rows.Next() {
		g, err := scanSegment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}

// PoolSegmentByID loads one intake-pool segment.
func (s *VoicePlatformStore) PoolSegmentByID(ctx context.Context, id string) (*IntakeSegment, error) {
	g, err := scanSegment(s.db.QueryRowContext(ctx, `SELECT `+segCols+` FROM voice_dataset_segments WHERE id = ?
		AND dataset_id IS NULL AND deleted_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrIntakeNotFound
	}
	return g, err
}

// ReviewSegment records a human decision. Approval requires a verified
// transcript typed or confirmed by the reviewer; ASR output alone never
// enters a dataset.
func (s *VoicePlatformStore) ReviewSegment(ctx context.Context, id string, approve bool, verified, style, reason, actor string) error {
	g, err := s.PoolSegmentByID(ctx, id)
	if err != nil {
		return err
	}
	status := "rejected"
	if approve {
		if strings.TrimSpace(verified) == "" {
			return errors.New("approval requires a verified transcript")
		}
		status = "approved"
	} else if strings.TrimSpace(reason) == "" {
		return errors.New("rejection requires a reason")
	}
	_, err = s.db.ExecContext(ctx, `UPDATE voice_dataset_segments SET status = ?, verified_transcript = ?,
		normalized_transcript = ?, style = ?, reject_reason = ?, verified_by = ?, updated_at = ?, row_version = row_version + 1
		WHERE id = ? AND dataset_id IS NULL`, status, nullIfEmpty(strings.TrimSpace(verified)),
		nullIfEmpty(normalizeTranscript(verified)), nullIfEmpty(style), nullIfEmpty(reason), actor, tsNow(), id)
	if err != nil {
		return err
	}
	return appendAudit(ctx, s.db, RightsAuditEntry{VoiceID: g.VoiceID, Actor: actor, Action: "SEGMENT_" + strings.ToUpper(status),
		Decision: "n/a", Detail: id})
}

func normalizeTranscript(t string) string {
	return strings.Join(strings.Fields(strings.ToLower(t)), " ")
}

// Dataset is a frozen dataset version.
type Dataset struct {
	ID             string  `json:"id"`
	VoiceID        string  `json:"voiceId"`
	DatasetVersion string  `json:"datasetVersion"`
	ManifestKey    string  `json:"manifestKey,omitempty"`
	ManifestSHA256 string  `json:"manifestSha256"`
	TotalSegments  int     `json:"totalSegments"`
	UsableSeconds  int     `json:"usableSeconds"`
	AvgQuality     float64 `json:"avgQuality"`
	TestSegments   int     `json:"testSegments"`
	Status         string  `json:"status"`
	GrantVersion   int     `json:"grantVersion"`
	ApprovedBy     string  `json:"approvedBy,omitempty"`
	CreatedAt      string  `json:"createdAt"`
}

const dsCols = `id, voice_id, dataset_version, COALESCE(manifest_key,''), COALESCE(manifest_sha256,''), total_segments,
	usable_seconds, COALESCE(avg_quality,0), status, COALESCE(grant_version,0), COALESCE(approved_by,''), created_at,
	(SELECT COUNT(*) FROM voice_dataset_segments x WHERE x.dataset_id = voice_datasets.id AND x.split = 'test')`

func scanDataset(sc interface{ Scan(...any) error }) (*Dataset, error) {
	var d Dataset
	err := sc.Scan(&d.ID, &d.VoiceID, &d.DatasetVersion, &d.ManifestKey, &d.ManifestSHA256, &d.TotalSegments,
		&d.UsableSeconds, &d.AvgQuality, &d.Status, &d.GrantVersion, &d.ApprovedBy, &d.CreatedAt, &d.TestSegments)
	return &d, err
}

// FreezeDataset builds the next immutable dataset version from approved pool
// segments whose recordings permit training. manifestKey is where the caller
// will store the manifest bytes (returned alongside).
func (s *VoicePlatformStore) FreezeDataset(ctx context.Context, voiceID string, grantVersion int, actor string,
	manifestKey func(version string) string) (*Dataset, []byte, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	rows, err := tx.QueryContext(ctx, `SELECT s.id, s.recording_id, s.audio_key, s.verified_transcript, COALESCE(s.style,''),
		s.end_ms - s.start_ms, COALESCE(s.quality_score,0)
		FROM voice_dataset_segments s JOIN voice_recordings r ON r.id = s.recording_id
		WHERE s.voice_id = ? AND s.dataset_id IS NULL AND s.status = 'approved' AND s.deleted_at IS NULL
		AND r.training_allowed = 1 AND r.processing_permission = 1 AND r.deleted_at IS NULL`, voiceID)
	if err != nil {
		return nil, nil, err
	}
	var entries []voicedata.ManifestEntry
	for rows.Next() {
		var e voicedata.ManifestEntry
		if err := rows.Scan(&e.SegmentID, &e.RecordingID, &e.AudioKey, &e.Transcript, &e.Style, &e.DurationMS, &e.Quality); err != nil {
			rows.Close()
			return nil, nil, err
		}
		entries = append(entries, e)
	}
	rows.Close()
	if len(entries) == 0 {
		return nil, nil, errors.New("no approved segments from training-permitted recordings")
	}
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.SegmentID
	}
	split := voicedata.AssignSplit(ids, voiceID)
	for i := range entries {
		entries[i].Split = split[entries[i].SegmentID]
	}

	var versions []string
	vr, err := tx.QueryContext(ctx, `SELECT dataset_version FROM voice_datasets WHERE voice_id = ?`, voiceID)
	if err != nil {
		return nil, nil, err
	}
	for vr.Next() {
		var v string
		_ = vr.Scan(&v)
		versions = append(versions, v)
	}
	vr.Close()
	version := voicedata.NextVersion(versions)

	m := voicedata.Manifest{VoiceID: voiceID, DatasetVersion: version, GrantVersion: grantVersion, Entries: entries}
	body, sum, err := m.Encode()
	if err != nil {
		return nil, nil, err
	}
	st := voicedata.Summarize(entries)
	now := tsNow()
	d := &Dataset{ID: "ds_" + uuid.NewString(), VoiceID: voiceID, DatasetVersion: version, ManifestKey: manifestKey(version),
		ManifestSHA256: sum, TotalSegments: st.Segments, UsableSeconds: st.UsableSeconds, AvgQuality: st.AvgQuality,
		TestSegments: st.TestSegments, Status: "frozen", GrantVersion: grantVersion, ApprovedBy: actor, CreatedAt: now}
	if _, err := tx.ExecContext(ctx, `INSERT INTO voice_datasets (id, voice_id, dataset_version, manifest_key, total_segments,
		usable_seconds, avg_quality, manifest_sha256, status, approved_by, grant_version, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, d.ID, voiceID, version, d.ManifestKey, st.Segments, st.UsableSeconds,
		st.AvgQuality, sum, d.Status, actor, grantVersion, now, now); err != nil {
		return nil, nil, err
	}
	// Copy, don't move: the pool keeps its history and the frozen rows can
	// never be edited through the review endpoints (they have a dataset_id).
	for _, e := range entries {
		if _, err := tx.ExecContext(ctx, `INSERT INTO voice_dataset_segments (id, voice_id, dataset_id, recording_id, split,
			start_ms, end_ms, audio_key, style, raw_transcript, normalized_transcript, verified_transcript, asr_confidence,
			quality_score, snr_db, speaker_confidence, verified_by, status, source_segment_id, created_at, updated_at)
			SELECT ?, voice_id, ?, recording_id, ?, start_ms, end_ms, audio_key, style, raw_transcript, normalized_transcript,
			verified_transcript, asr_confidence, quality_score, snr_db, speaker_confidence, verified_by, 'approved', id, ?, ?
			FROM voice_dataset_segments WHERE id = ?`, "seg_"+uuid.NewString(), d.ID, string(e.Split), now, now, e.SegmentID); err != nil {
			return nil, nil, err
		}
	}
	if err := appendAudit(ctx, tx, RightsAuditEntry{VoiceID: voiceID, Actor: actor, Action: "DATASET_FROZEN", GrantVersion: grantVersion,
		Decision: "allowed", Detail: fmt.Sprintf("%s segments=%d sha256=%s", version, st.Segments, sum)}); err != nil {
		return nil, nil, err
	}
	return d, body, tx.Commit()
}

// Datasets lists a voice's dataset versions.
func (s *VoicePlatformStore) Datasets(ctx context.Context, voiceID string) ([]Dataset, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+dsCols+` FROM voice_datasets WHERE voice_id = ? AND deleted_at IS NULL
		ORDER BY dataset_version DESC`, voiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Dataset{}
	for rows.Next() {
		d, err := scanDataset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// DatasetByID loads one dataset.
func (s *VoicePlatformStore) DatasetByID(ctx context.Context, id string) (*Dataset, error) {
	d, err := scanDataset(s.db.QueryRowContext(ctx, `SELECT `+dsCols+` FROM voice_datasets WHERE id = ? AND deleted_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrIntakeNotFound
	}
	return d, err
}

// DatasetManifest rebuilds manifest entries for a frozen dataset from its rows.
func (s *VoicePlatformStore) DatasetManifest(ctx context.Context, datasetID string) ([]voicedata.ManifestEntry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT COALESCE(source_segment_id, id), recording_id, audio_key, verified_transcript,
		COALESCE(style,''), end_ms - start_ms, COALESCE(quality_score,0), split FROM voice_dataset_segments WHERE dataset_id = ?`, datasetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []voicedata.ManifestEntry
	for rows.Next() {
		var e voicedata.ManifestEntry
		var sp string
		if err := rows.Scan(&e.SegmentID, &e.RecordingID, &e.AudioKey, &e.Transcript, &e.Style, &e.DurationMS, &e.Quality, &sp); err != nil {
			return nil, err
		}
		e.Split = voicedata.Split(sp)
		out = append(out, e)
	}
	return out, rows.Err()
}

// TrainingRun is one fine-tuning attempt.
type TrainingRun struct {
	ID               string   `json:"id"`
	VoiceID          string   `json:"voiceId"`
	DatasetID        string   `json:"datasetId"`
	DatasetVersion   string   `json:"datasetVersion"`
	Engine           string   `json:"engine"`
	BaseModel        string   `json:"baseModel"`
	Mode             string   `json:"mode"`
	Hyperparameters  string   `json:"hyperparameters"`
	Justification    string   `json:"justification"`
	BaselineScore    *float64 `json:"baselineScore"`
	Status           string   `json:"status"`
	Progress         *float64 `json:"progress"`
	FinalLoss        *float64 `json:"finalLoss"`
	Hardware         string   `json:"hardware,omitempty"`
	DurationSeconds  int      `json:"durationSeconds,omitempty"`
	CheckpointKey    string   `json:"-"`
	CheckpointSHA256 string   `json:"checkpointSha256,omitempty"`
	Error            string   `json:"error,omitempty"`
	JobID            string   `json:"jobId,omitempty"`
	GrantVersion     int      `json:"grantVersion"`
	ModelID          string   `json:"modelId,omitempty"`
	RequestedBy      string   `json:"requestedBy"`
	CreatedAt        string   `json:"createdAt"`
	UpdatedAt        string   `json:"updatedAt"`
}

const runCols = `id, voice_id, dataset_id, dataset_version, engine, base_model, mode, hyperparameters, COALESCE(justification,''),
	baseline_score, status, progress, final_loss, COALESCE(hardware,''), COALESCE(duration_seconds,0), COALESCE(checkpoint_key,''),
	COALESCE(checkpoint_sha256,''), COALESCE(error_message,''), COALESCE(job_id,''), COALESCE(grant_version,0),
	COALESCE(model_id,''), COALESCE(requested_by,''), created_at, updated_at`

func scanRun(sc interface{ Scan(...any) error }) (*TrainingRun, error) {
	var r TrainingRun
	var base, prog, loss sql.NullFloat64
	if err := sc.Scan(&r.ID, &r.VoiceID, &r.DatasetID, &r.DatasetVersion, &r.Engine, &r.BaseModel, &r.Mode, &r.Hyperparameters,
		&r.Justification, &base, &r.Status, &prog, &loss, &r.Hardware, &r.DurationSeconds, &r.CheckpointKey, &r.CheckpointSHA256,
		&r.Error, &r.JobID, &r.GrantVersion, &r.ModelID, &r.RequestedBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}
	r.BaselineScore, r.Progress, r.FinalLoss = nf(base), nf(prog), nf(loss)
	return &r, nil
}

// CreateTrainingRun records a QUEUED run.
func (s *VoicePlatformStore) CreateTrainingRun(ctx context.Context, r *TrainingRun) error {
	r.ID, r.Status, r.CreatedAt = "run_"+uuid.NewString(), string(voicedata.RunQueued), tsNow()
	r.UpdatedAt = r.CreatedAt
	if r.Hyperparameters == "" {
		r.Hyperparameters = "{}"
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO voice_training_runs (id, voice_id, dataset_id, dataset_version, engine, base_model,
		mode, hyperparameters, justification, baseline_score, status, grant_version, requested_by, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, r.ID, r.VoiceID, r.DatasetID, r.DatasetVersion, r.Engine, r.BaseModel, r.Mode,
		r.Hyperparameters, r.Justification, fp(r.BaselineScore), r.Status, r.GrantVersion, r.RequestedBy, r.CreatedAt, r.UpdatedAt)
	if err != nil {
		return err
	}
	return appendAudit(ctx, s.db, RightsAuditEntry{VoiceID: r.VoiceID, Actor: r.RequestedBy, Action: "TRAINING_REQUESTED",
		GrantVersion: r.GrantVersion, Decision: "allowed", Detail: r.ID + " " + r.DatasetVersion + " " + r.Engine})
}

// TrainingRunByID loads one run.
func (s *VoicePlatformStore) TrainingRunByID(ctx context.Context, id string) (*TrainingRun, error) {
	r, err := scanRun(s.db.QueryRowContext(ctx, `SELECT `+runCols+` FROM voice_training_runs WHERE id = ? AND deleted_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrIntakeNotFound
	}
	return r, err
}

// TrainingRuns lists a voice's runs, newest first.
func (s *VoicePlatformStore) TrainingRuns(ctx context.Context, voiceID string) ([]TrainingRun, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+runCols+` FROM voice_training_runs WHERE voice_id = ? AND deleted_at IS NULL
		ORDER BY created_at DESC`, voiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrainingRun{}
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// RunUpdate is a partial update from the worker or an admin.
type RunUpdate struct {
	Status           voicedata.RunStatus
	Progress         *float64
	FinalLoss        *float64
	Hardware         string
	DurationSeconds  int
	CheckpointKey    string
	CheckpointSHA256 string
	Error            string
	JobID            string
	ModelID          string
}

// UpdateTrainingRun applies u, enforcing the run state machine. A same-status
// update (progress tick) is allowed.
func (s *VoicePlatformStore) UpdateTrainingRun(ctx context.Context, id string, u RunUpdate, actor string) error {
	cur, err := s.TrainingRunByID(ctx, id)
	if err != nil {
		return err
	}
	from := voicedata.RunStatus(cur.Status)
	if u.Status != "" && u.Status != from && !voicedata.CanTransitionRun(from, u.Status) {
		return fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, from, u.Status)
	}
	to := from
	if u.Status != "" {
		to = u.Status
	}
	_, err = s.db.ExecContext(ctx, `UPDATE voice_training_runs SET status = ?, progress = COALESCE(?, progress),
		final_loss = COALESCE(?, final_loss), hardware = COALESCE(?, hardware), duration_seconds = COALESCE(?, duration_seconds),
		checkpoint_key = COALESCE(?, checkpoint_key), checkpoint_sha256 = COALESCE(?, checkpoint_sha256),
		error_message = COALESCE(?, error_message), job_id = COALESCE(?, job_id), model_id = COALESCE(?, model_id),
		updated_at = ?, row_version = row_version + 1 WHERE id = ? AND status = ?`,
		string(to), fp(u.Progress), fp(u.FinalLoss), nullIfEmpty(u.Hardware), nullIntIfZero(u.DurationSeconds),
		nullIfEmpty(u.CheckpointKey), nullIfEmpty(u.CheckpointSHA256), nullIfEmpty(u.Error), nullIfEmpty(u.JobID),
		nullIfEmpty(u.ModelID), tsNow(), id, cur.Status)
	if err != nil || to == from {
		return err
	}
	return appendAudit(ctx, s.db, RightsAuditEntry{VoiceID: cur.VoiceID, Actor: actor, Action: "TRAINING_" + string(to),
		ModelID: u.ModelID, Decision: "n/a", Detail: id + " " + u.Error})
}

func nullIntIfZero(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

// BestZeroShotScore returns the highest evaluated ICF_VOICE_SCORE among the
// voice's zero-shot models, or nil when none has been evaluated.
func (s *VoicePlatformStore) BestZeroShotScore(ctx context.Context, voiceID string) (*float64, error) {
	var v sql.NullFloat64
	err := s.db.QueryRowContext(ctx, `SELECT MAX(e.icf_voice_score) FROM voice_evaluations e
		JOIN voice_models m ON m.id = e.model_id
		WHERE m.voice_id = ? AND m.mode = 'zero_shot' AND m.deleted_at IS NULL`, voiceID).Scan(&v)
	if err != nil {
		return nil, err
	}
	return nf(v), nil
}
