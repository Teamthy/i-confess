package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/moderation"
	"github.com/lib/pq"
)

// ModerationStore owns the ugc moderation surface: user reports, moderation
// cases, the queue a moderator works from, user-confession submission and
// review, and the §75 audio QA gate.
//
// The tables it reads have existed since the baseline schema; until PHASE 31
// nothing wrote to them. The vocabularies live in internal/moderation and are
// parity-tested against the live CHECK constraints, the same contract
// internal/content keeps for the editorial lifecycle.
type ModerationStore struct {
	db *db.DB
}

func NewModerationStore(db *db.DB) *ModerationStore { return &ModerationStore{db: db} }

const ucColumns = `id, user_id, title, text, COALESCE(category_id,''), is_private,
	status, visibility, COALESCE(review_notes,''), COALESCE(reviewed_by,''),
	COALESCE(reviewed_at,''), COALESCE(rejection_reason,''), COALESCE(published_at,''),
	version, created_at, updated_at`

func scanUserConfession(row interface{ Scan(...any) error }) (models.UserConfession, error) {
	var uc models.UserConfession
	err := row.Scan(&uc.ID, &uc.UserID, &uc.Title, &uc.Text, &uc.CategoryID, &uc.IsPrivate,
		&uc.Status, &uc.Visibility, &uc.ReviewNotes, &uc.ReviewedBy, &uc.ReviewedAt,
		&uc.RejectionReason, &uc.PublishedAt, &uc.Version, &uc.CreatedAt, &uc.UpdatedAt)
	return uc, err
}

// ---------- Reports ----------

// ReportableEntityExists backs report validation: a report may only name an
// entity the reporter could actually have seen. A report against an id that
// does not exist is garbage the queue would otherwise carry forever; a report
// against a draft confession names something the reporter cannot have read.
func (s *ModerationStore) ReportableEntityExists(ctx context.Context, entityType, entityID string) (bool, error) {
	switch entityType {
	case "confession":
		var n int
		// Deprecated content is still playable in sessions that already hold
		// it, so a listener can legitimately report it.
		err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM confessions WHERE id = ? AND status IN ('published','deprecated')`,
			entityID).Scan(&n)
		return n > 0, err
	case "community_post":
		var n int
		err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM community_posts WHERE id = ?`, entityID).Scan(&n)
		return n > 0, err
	default:
		return false, fmt.Errorf("entity type %q is not reportable", entityType)
	}
}

const reportColumns = `id, reporter_id, entity_type, entity_id, reason, COALESCE(detail,''),
	status, COALESCE(reviewed_by,''), COALESCE(reviewed_at,''), COALESCE(resolution_note,''), created_at`

func scanReport(row interface{ Scan(...any) error }) (models.Report, error) {
	var r models.Report
	err := row.Scan(&r.ID, &r.ReporterID, &r.EntityType, &r.EntityID, &r.Reason, &r.Detail,
		&r.Status, &r.ReviewedBy, &r.ReviewedAt, &r.ResolutionNote, &r.CreatedAt)
	return r, err
}

// CreateReport files a report and opens a moderation case for the entity.
//
// One open report per reporter per entity is enforced by the partial unique
// index from migration 0009. A duplicate is not an error: the reporter's
// intent - "I object to this" - is already on record, so the existing row is
// returned with alreadyExisted=true. Failing instead would teach repeat
// callers that the first report vanished.
func (s *ModerationStore) CreateReport(ctx context.Context, reporterID, entityType, entityID, reason, detail string) (rep models.Report, alreadyExisted bool, err error) {
	id := newID()
	ts := now()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO reports (id, reporter_id, entity_type, entity_id, reason, detail, status, created_at)
		 VALUES (?,?,?,?,?,?,'open',?)`,
		id, reporterID, entityType, entityID, reason, nullIfEmpty(detail), ts)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			rep, selErr := s.openReportFor(ctx, reporterID, entityType, entityID)
			return rep, true, selErr
		}
		return models.Report{}, false, err
	}
	if err := s.ensureOpenCase(ctx, entityType, entityID, reporterID, reason); err != nil {
		return models.Report{}, false, fmt.Errorf("open moderation case: %w", err)
	}
	rep, err = s.reportByID(ctx, id)
	return rep, false, err
}

func (s *ModerationStore) openReportFor(ctx context.Context, reporterID, entityType, entityID string) (models.Report, error) {
	return scanReport(s.db.QueryRowContext(ctx,
		`SELECT `+reportColumns+` FROM reports
		 WHERE reporter_id = ? AND entity_type = ? AND entity_id = ? AND status = 'open'`,
		reporterID, entityType, entityID))
}

func (s *ModerationStore) reportByID(ctx context.Context, id string) (models.Report, error) {
	rep, err := scanReport(s.db.QueryRowContext(ctx,
		`SELECT `+reportColumns+` FROM reports WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return rep, ErrNotFound
	}
	return rep, err
}

// ErrReportNotOpen is returned when a decision lands on a report that is no
// longer open; the API maps it to 409 because the target is real but the move
// is not available from where it stands.
var ErrReportNotOpen = errors.New("report is not open")

// DecideReport closes a report with an outcome and, when it was the last open
// report on the entity, closes the moderation case with the same outcome.
func (s *ModerationStore) DecideReport(ctx context.Context, reportID, decision, actor, note string) (models.Report, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.Report{}, err
	}
	defer tx.Rollback()

	rep, err := scanReport(tx.QueryRowContext(ctx,
		`SELECT `+reportColumns+` FROM reports WHERE id = ? FOR UPDATE`, reportID))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Report{}, ErrNotFound
	}
	if err != nil {
		return models.Report{}, err
	}
	if rep.Status != "open" {
		return models.Report{}, ErrReportNotOpen
	}

	ts := now()
	if _, err := tx.ExecContext(ctx,
		`UPDATE reports SET status = ?, reviewed_by = ?, reviewed_at = ?, resolution_note = ? WHERE id = ?`,
		decision, actor, ts, nullIfEmpty(note), reportID); err != nil {
		return models.Report{}, err
	}

	var remaining int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM reports WHERE entity_type = ? AND entity_id = ? AND status = 'open'`,
		rep.EntityType, rep.EntityID).Scan(&remaining); err != nil {
		return models.Report{}, err
	}
	if remaining == 0 {
		// The case is the queue entry; closing it is what removes the entity
		// from the moderator's list of open work.
		if _, err := tx.ExecContext(ctx,
			`UPDATE moderation_cases
			 SET status = ?, actor = ?, before = 'open', after = ?, detail = ?, updated_at = ?
			 WHERE entity_type = ? AND entity_id = ? AND status IN ('open','in_review')`,
			decision, actor, decision, nullIfEmpty(note), ts, rep.EntityType, rep.EntityID); err != nil {
			return models.Report{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return models.Report{}, err
	}
	return s.reportByID(ctx, reportID)
}

func (s *ModerationStore) ensureOpenCase(ctx context.Context, entityType, entityID, actor, reason string) error {
	ts := now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO moderation_cases (id, entity_type, entity_id, status, reason, actor, created_at, updated_at)
		 VALUES (?,?,?,'open',?,?,?,?)
		 ON CONFLICT (entity_type, entity_id) WHERE status IN ('open','in_review') DO NOTHING`,
		newID(), entityType, entityID, nullIfEmpty(reason), nullIfEmpty(actor), ts, ts)
	return err
}

// ---------- User-confession submission and review ----------

// SubmitUserConfession moves a draft (or a rejected confession the author has
// reworked) into the review queue and opens a moderation case on it.
//
// The read and the write run under FOR UPDATE in one transaction: two
// concurrent submits must not both pass the transition check, and a review
// deciding while a resubmission lands must not interleave into a state the
// graph forbids.
func (s *ModerationStore) SubmitUserConfession(ctx context.Context, userID, id string) (models.UserConfession, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.UserConfession{}, err
	}
	defer tx.Rollback()

	uc, err := scanUserConfession(tx.QueryRowContext(ctx,
		`SELECT `+ucColumns+` FROM user_confessions WHERE id = ? FOR UPDATE`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.UserConfession{}, ErrNotFound
	}
	if err != nil {
		return models.UserConfession{}, err
	}
	if uc.UserID != userID {
		return models.UserConfession{}, ErrForbidden
	}
	if err := moderation.ValidateUGCTransition(moderation.UGCStatus(uc.Status), moderation.UGCSubmitted); err != nil {
		return models.UserConfession{}, err
	}

	// Review columns describe the pending decision, so a resubmission clears
	// the last one. The decision itself is not lost: it was recorded in the
	// moderation case the review closed.
	ts := now()
	if _, err := tx.ExecContext(ctx,
		`UPDATE user_confessions
		 SET status = 'submitted', review_notes = NULL, reviewed_by = NULL, reviewed_at = NULL,
		     rejection_reason = NULL, updated_at = ?
		 WHERE id = ?`, ts, id); err != nil {
		return models.UserConfession{}, err
	}
	if err := txCommitCase(ctx, tx, "user_confession", id, userID, "submitted for review"); err != nil {
		return models.UserConfession{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.UserConfession{}, err
	}
	return s.userConfessionByID(ctx, id)
}

func (s *ModerationStore) userConfessionByID(ctx context.Context, id string) (models.UserConfession, error) {
	uc, err := scanUserConfession(s.db.QueryRowContext(ctx,
		`SELECT `+ucColumns+` FROM user_confessions WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return uc, ErrNotFound
	}
	return uc, err
}

// ReviewUserConfession records a moderator's decision on a queued confession.
//
// Approval publishes to the audience the author asked for: visibility=public
// lands in 'published', anything else lands in 'approved'. Rejection always
// carries a reason - a rejection the author cannot act on is only a closure
// for the moderator.
func (s *ModerationStore) ReviewUserConfession(ctx context.Context, id, decision, actor, note, rejectionReason string) (models.UserConfession, error) {
	if decision != string(moderation.UGCApproved) && decision != string(moderation.UGCRejected) {
		return models.UserConfession{}, fmt.Errorf("decision must be %q or %q", moderation.UGCApproved, moderation.UGCRejected)
	}
	if decision == string(moderation.UGCRejected) && rejectionReason == "" {
		// Defence in depth: the handler rejects this too, but this function is
		// the last line before the write (same contract
		// ContentStore.UpdateConfessionStatus keeps).
		return models.UserConfession{}, fmt.Errorf("a rejection reason is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.UserConfession{}, err
	}
	defer tx.Rollback()

	uc, err := scanUserConfession(tx.QueryRowContext(ctx,
		`SELECT `+ucColumns+` FROM user_confessions WHERE id = ? FOR UPDATE`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.UserConfession{}, ErrNotFound
	}
	if err != nil {
		return models.UserConfession{}, err
	}
	if !moderation.CanReview(moderation.UGCStatus(uc.Status)) {
		return models.UserConfession{}, &moderation.UGCTransitionError{
			From: moderation.UGCStatus(uc.Status), To: moderation.UGCStatus(decision),
		}
	}

	to := decision
	if decision == string(moderation.UGCApproved) && uc.Visibility == moderation.VisibilityPublic {
		to = string(moderation.UGCPublished)
	}

	ts := now()
	publishedAt := nullIfEmpty(uc.PublishedAt)
	if to == string(moderation.UGCPublished) && uc.PublishedAt == "" {
		publishedAt = ts
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE user_confessions
		 SET status = ?, reviewed_by = ?, reviewed_at = ?, review_notes = ?,
		     rejection_reason = ?, published_at = ?, updated_at = ?
		 WHERE id = ?`,
		to, actor, ts, nullIfEmpty(note), nullIfEmpty(rejectionReason), publishedAt, ts, id); err != nil {
		return models.UserConfession{}, err
	}

	detail := note
	if decision == string(moderation.UGCRejected) {
		detail = rejectionReason
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE moderation_cases
		 SET status = 'resolved', actor = ?, before = 'submitted', after = ?, detail = ?, updated_at = ?
		 WHERE entity_type = 'user_confession' AND entity_id = ? AND status IN ('open','in_review')`,
		actor, to, nullIfEmpty(detail), ts, id); err != nil {
		return models.UserConfession{}, err
	}

	if err := tx.Commit(); err != nil {
		return models.UserConfession{}, err
	}
	return s.userConfessionByID(ctx, id)
}

// txCommitCase opens a case inside an existing transaction, the same
// upsert-on-partial-index as ensureOpenCase but able to roll back with the
// state change that caused it.
func txCommitCase(ctx context.Context, tx *db.Tx, entityType, entityID, actor, reason string) error {
	ts := now()
	_, err := tx.ExecContext(ctx,
		`INSERT INTO moderation_cases (id, entity_type, entity_id, status, reason, actor, created_at, updated_at)
		 VALUES (?,?,?,'open',?,?,?,?)
		 ON CONFLICT (entity_type, entity_id) WHERE status IN ('open','in_review') DO NOTHING`,
		newID(), entityType, entityID, nullIfEmpty(reason), nullIfEmpty(actor), ts, ts)
	return err
}

// ---------- The queue ----------

// ModerationQueue is the moderator's work list, oldest first in every
// section: the item that has waited longest gets decided first.
func (s *ModerationStore) ModerationQueue(ctx context.Context) (models.ModerationQueue, error) {
	q := models.ModerationQueue{
		UserConfessions: []models.UserConfession{},
		Reports:         []models.Report{},
		Editorial:       []models.Confession{},
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT `+ucColumns+` FROM user_confessions WHERE status = 'submitted' ORDER BY created_at ASC`)
	if err != nil {
		return q, err
	}
	for rows.Next() {
		uc, err := scanUserConfession(rows)
		if err != nil {
			rows.Close()
			return q, err
		}
		q.UserConfessions = append(q.UserConfessions, uc)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return q, err
	}

	rrows, err := s.db.QueryContext(ctx,
		`SELECT `+reportColumns+` FROM reports WHERE status = 'open' ORDER BY created_at ASC`)
	if err != nil {
		return q, err
	}
	for rrows.Next() {
		rep, err := scanReport(rrows)
		if err != nil {
			rrows.Close()
			return q, err
		}
		q.Reports = append(q.Reports, rep)
	}
	rrows.Close()
	if err := rrows.Err(); err != nil {
		return q, err
	}

	// Editorial pending: canonical confessions parked in a state that needs a
	// human before approval. Draft is authored, not reviewed, and approved or
	// later states are done; neither is queue work.
	erows, err := s.db.QueryContext(ctx,
		`SELECT id, category_id, title, status, updated_at, created_at
		 FROM confessions
		 WHERE status IN ('content_review','theological_review','audio_qa')
		 ORDER BY updated_at ASC`)
	if err != nil {
		return q, err
	}
	for erows.Next() {
		var c models.Confession
		if err := erows.Scan(&c.ID, &c.CategoryID, &c.Title, &c.Status, &c.UpdatedAt, &c.CreatedAt); err != nil {
			erows.Close()
			return q, err
		}
		q.Editorial = append(q.Editorial, c)
	}
	erows.Close()
	if err := erows.Err(); err != nil {
		return q, err
	}

	var openCases int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM moderation_cases WHERE status IN ('open','in_review')`).Scan(&openCases); err != nil {
		return q, err
	}

	q.Counts = map[string]int{
		"user_confessions": len(q.UserConfessions),
		"reports":          len(q.Reports),
		"editorial":        len(q.Editorial),
		"open_cases":       openCases,
	}
	return q, nil
}

// ---------- The §75 audio QA gate ----------

// ErrQAGateState is returned when the QA gate is invoked on a confession that
// is not in audio_qa. The gate is one enforced edge of the editorial
// lifecycle - audio_qa to approved - and firing it from anywhere else would
// let content skip the reviews §22 makes mandatory.
type ErrQAGateState struct {
	Status string
}

func (e *ErrQAGateState) Error() string {
	return fmt.Sprintf("the QA gate runs from audio_qa, not from %q", e.Status)
}

// RunConfessionQA executes the §75 checklist against a confession and
// persists the report either way. On a pass the confession moves to approved
// with qa_passed_at set and a content_moderation_history row recording the
// move; on a fail nothing transitions but the report is still written, so the
// next run and the audio team can see exactly which check kept the gate shut.
func (s *ModerationStore) RunConfessionQA(ctx context.Context, id, actor, note string) (models.QAReport, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.QAReport{}, err
	}
	defer tx.Rollback()

	var status, short, medium, long string
	err = tx.QueryRowContext(ctx,
		`SELECT status, COALESCE(short_text,''), COALESCE(medium_text,''), COALESCE(long_text,'')
		 FROM confessions WHERE id = ? FOR UPDATE`, id).Scan(&status, &short, &medium, &long)
	if errors.Is(err, sql.ErrNoRows) {
		return models.QAReport{}, ErrNotFound
	}
	if err != nil {
		return models.QAReport{}, err
	}
	if status != "audio_qa" {
		return models.QAReport{}, &ErrQAGateState{Status: status}
	}

	facts := moderation.QAFacts{
		HasText: short != "" || medium != "" || long != "",
	}
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audio_assets
		 WHERE content_id = ? AND status IN ('ready','published') AND duration_seconds > 0`,
		id).Scan(&facts.ReadyRenders); err != nil {
		return models.QAReport{}, err
	}
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audio_assets
		 WHERE content_id = ? AND status IN ('uploading','processing')`,
		id).Scan(&facts.RendersInFlight); err != nil {
		return models.QAReport{}, err
	}
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audio_assets a
		 WHERE a.content_id = ? AND a.status IN ('ready','published')
		   AND NOT EXISTS (SELECT 1 FROM voice_rights vr WHERE vr.voice_id = a.voice_id AND vr.status = 'active')`,
		id).Scan(&facts.UnlicensedVoices); err != nil {
		return models.QAReport{}, err
	}

	report := moderation.RunQAChecklist(facts, actor, note, time.Now())
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return models.QAReport{}, err
	}

	ts := now()
	if report.Passed {
		res, err := tx.ExecContext(ctx,
			`UPDATE confessions
			 SET status = 'approved', qa_passed_at = ?, qa_report = ?, updated_at = ?
			 WHERE id = ?`, ts, string(reportJSON), ts, id)
		if err != nil {
			return models.QAReport{}, err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return models.QAReport{}, fmt.Errorf("QA update touched %d rows, expected 1", n)
		}
		if err := recordModerationHistory(ctx, tx, id, status, "approved", actor, note); err != nil {
			return models.QAReport{}, err
		}
	} else {
		if _, err := tx.ExecContext(ctx,
			`UPDATE confessions SET qa_report = ?, updated_at = ? WHERE id = ?`,
			string(reportJSON), ts, id); err != nil {
			return models.QAReport{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return models.QAReport{}, err
	}
	return report, nil
}

// ---------- Editorial history ----------

// recordModerationHistory writes the from/to row for a canonical confession
// status change. The table existed since the baseline with no writer;
// "who moved this confession, from what, to what, and why" was unanswerable.
func recordModerationHistory(ctx context.Context, tx *db.Tx, confessionID, from, to, actor, reason string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO content_moderation_history (id, confession_id, from_status, to_status, actor, reason, created_at)
		 VALUES (?,?,?,?,?,?,?)`,
		newID(), confessionID, from, to, nullIfEmpty(actor), nullIfEmpty(reason), now())
	return err
}

// UpdateConfessionStatusAudited replaces the unaudited PATCH path: it reads
// the current state under lock so ErrNotFound is a 404 rather than a 500, it
// sets published_at only when entering published (the old UPDATE overwrote it
// with NULL on every other move, so re-publishing after a correction rewrote
// the original publish date and unpublishing erased it), and it records the
// transition in content_moderation_history.
func (s *ModerationStore) UpdateConfessionStatusAudited(ctx context.Context, id, status, actor, reason string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var from string
	err = tx.QueryRowContext(ctx,
		`SELECT status FROM confessions WHERE id = ? FOR UPDATE`, id).Scan(&from)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	ts := now()
	if _, err := tx.ExecContext(ctx,
		`UPDATE confessions
		 SET status = ?,
		     published_at = CASE WHEN ? = 'published' THEN ? ELSE published_at END,
		     updated_at = ?
		 WHERE id = ?`, status, status, ts, ts, id); err != nil {
		return err
	}
	if from == status {
		// A no-op PATCH must not fabricate history: nothing moved, so nothing
		// is recorded.
		return tx.Commit()
	}
	if err := recordModerationHistory(ctx, tx, id, from, status, actor, reason); err != nil {
		return err
	}
	return tx.Commit()
}
