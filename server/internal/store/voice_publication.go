package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/voiceengine"
	"github.com/Teamthy/i-confess/internal/voicegov"
)

// ErrRenderCommitUnknown means the transaction might already be committed.
// Never compensate by deleting an object that might back a committed asset.
var ErrRenderCommitUnknown = errors.New("render metadata commit outcome unknown")

func confirmRenderGrant(ctx context.Context, q rowQuerier, use voicegov.RenderUse) error {
	g, err := readGrant(ctx, q, use.VoiceID)
	if err != nil {
		return err // a DB outage is not a permanent rights refusal
	}
	if d := use.Recheck(g, time.Now().UTC()); !d.Allowed {
		return &voiceengine.ErrRightsDenied{Decision: d}
	}
	return nil
}

// CheckRenderGrant is for admission to external work only. Publication uses
// withRenderGrant rather than a check followed by an independent update.
func (s *VoicePlatformStore) CheckRenderGrant(ctx context.Context, use voicegov.RenderUse) error {
	return confirmRenderGrant(ctx, s.db, use)
}

// Stage the DB change, then confirm its grant under a shared row lock just
// before commit. A concurrent rights writer either wins (we roll back) or
// follows the published change. Never keep this transaction across inference,
// storage, encoding or client I/O. A stale job cannot change another voice's row.
func (s *VoicePlatformStore) withRenderGrant(ctx context.Context, use voicegov.RenderUse, stage func(*db.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if err := stage(tx); err != nil {
		return err
	}
	// Both terms and lifecycle writers lock/update this parent before writing
	// permissions. Read the terms in a NEW statement after any lock wait so
	// READ COMMITTED sees the winning writer's capabilities as well as header.
	var one int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM voice_rights_grants WHERE voice_id = ? AND deleted_at IS NULL FOR SHARE`, use.VoiceID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return &voiceengine.ErrRightsDenied{Decision: use.Recheck(nil, time.Now().UTC())}
	}
	if err != nil {
		return err
	}
	if err := confirmRenderGrant(ctx, tx, use); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return errors.Join(ErrRenderCommitUnknown, err)
	}
	return nil
}

// BeginGenerationUpload is the handoff from verified inference to private
// storage. A cancellation/owner change or stale grant discards the bytes.
func (s *VoicePlatformStore) BeginGenerationUpload(ctx context.Context, id, jobID string, use voicegov.RenderUse) error {
	return s.withRenderGrant(ctx, use, func(tx *db.Tx) error {
		return generationUpdated(tx.ExecContext(ctx, `UPDATE voice_generations SET status = 'UPLOADING', grant_version = ?, updated_at = ?, row_version = row_version + 1
			WHERE id = ? AND job_id = ? AND voice_id = ? AND status = 'PROCESSING' AND deleted_at IS NULL`, use.GrantVersion, tsNow(), id, jobID, use.VoiceID))
	})
}

// CompleteGeneration publishes the pointer, cache identity and allowed audit
// together. An upload by itself is not a completed generation.
func (s *VoicePlatformStore) CompleteGeneration(ctx context.Context, g *Generation, use voicegov.RenderUse) error {
	return s.withRenderGrant(ctx, use, func(tx *db.Tx) error {
		err := generationUpdated(tx.ExecContext(ctx, `UPDATE voice_generations SET status = 'COMPLETED', content_hash = ?, model_id = ?, engine = ?, engine_version = ?,
			audio_sha256 = ?, storage_key = ?, duration_ms = ?, fell_back = ?, fallback_reason = ?, updated_at = ?,
			inference_seconds = ?, worker_seconds = ?, cost_usd_micros = ?, row_version = row_version + 1
			WHERE id = ? AND job_id = ? AND voice_id = ? AND grant_version = ? AND deleted_at IS NULL AND status = 'UPLOADING'`,
			g.ContentHash, g.ModelID, g.Engine, g.EngineVersion, g.AudioSHA256, g.StorageKey, g.DurationMS, boolInt(g.FellBack),
			nullIfEmpty(g.FallbackReason), tsNow(), g.InferenceSeconds, g.WorkerSeconds, g.CostUSDMicros, g.ID, g.JobID, use.VoiceID, use.GrantVersion))
		if err != nil {
			return err
		}
		action := "VOICE_GENERATED"
		if g.FellBack {
			action = "VOICE_GENERATED_FALLBACK"
		}
		return appendAudit(ctx, tx, RightsAuditEntry{VoiceID: use.VoiceID, Actor: "worker", Action: action, ModelID: g.ModelID, GenerationID: g.ID,
			GrantVersion: use.GrantVersion, Decision: "allowed", Detail: g.FallbackReason})
	})
}

// RefuseGeneration records a terminal rights refusal with its audit. A losing
// delivery does not write a denial against a newer owner of the same generation.
func (s *VoicePlatformStore) RefuseGeneration(ctx context.Context, id, jobID, msg string, d voicegov.Decision) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	voiceID, err := failGenerationRow(ctx, tx, id, jobID, string(voiceengine.ClassRights), msg)
	if err != nil {
		return err
	}
	if err := appendAudit(ctx, tx, RightsAuditEntry{VoiceID: voiceID, Actor: "worker", Action: "GENERATION_REFUSED_RIGHTS", GenerationID: id,
		GrantVersion: d.GrantVersion, Decision: "denied", Reason: string(d.Reason)}); err != nil {
		return err
	}
	return tx.Commit()
}

// SetGenerationVariants attaches new encodings only to this still-live master.
func (s *VoicePlatformStore) SetGenerationVariants(ctx context.Context, g *Generation, use voicegov.RenderUse, v map[string]AudioVariant) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.withRenderGrant(ctx, use, func(tx *db.Tx) error {
		return generationUpdated(tx.ExecContext(ctx, `UPDATE voice_generations SET variants = ?, updated_at = ?, row_version = row_version + 1 WHERE id = ? AND voice_id = ?
			AND storage_key = ? AND grant_version = ? AND status = 'COMPLETED' AND deleted_at IS NULL`, string(b), tsNow(), g.ID, use.VoiceID, g.StorageKey, use.GrantVersion))
	})
}

// RecordStreamStart authorizes the first-byte handoff, not ongoing playback.
func (s *VoicePlatformStore) RecordStreamStart(ctx context.Context, use voicegov.RenderUse, e RightsAuditEntry) error {
	use.Request.Action = voicegov.ActionStream // also rechecks generation permission
	e.VoiceID, e.GrantVersion = use.VoiceID, use.GrantVersion
	e.Action, e.Decision = "VOICE_STREAMED", "allowed"
	return s.withRenderGrant(ctx, use, func(tx *db.Tx) error { return appendAudit(ctx, tx, e) })
}
