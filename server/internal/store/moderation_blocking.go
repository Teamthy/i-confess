package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/moderation"
)

var (
	// ErrAppealNotDecidable is returned when an appeal is already closed. A
	// decided appeal is final, so a second decision is a caller error rather
	// than an idempotent retry.
	ErrAppealNotDecidable = errors.New("appeal has already been decided")
	// ErrAppealNotOwned is returned when an account tries to appeal a decision
	// that was not made about them.
	ErrAppealNotOwned = errors.New("the decision being appealed does not belong to this account")
	// ErrAppealableDecisionNotFound is returned when the decision being
	// appealed does not exist, or exists but is not in a state that can be
	// appealed.
	ErrAppealableDecisionNotFound = errors.New("no appealable decision with that id")
)

// UserBlock is one account's boundary against another.
type UserBlock struct {
	ID        string `json:"id"`
	BlockerID string `json:"blocker_id"`
	BlockedID string `json:"blocked_id"`
	Reason    string `json:"reason,omitempty"`
	CreatedAt string `json:"created_at"`
}

// BlockStore owns user_blocks.
//
// Blocking is deliberately not part of ModerationStore. It is a listener's own
// action on their own view, taken without a moderator and reversible without
// one; putting it beside the moderator's decision writers would make it look
// like part of the enforcement surface, which is the confusion the domain
// package warns about.
type BlockStore struct{ db *db.DB }

func NewBlockStore(database *db.DB) *BlockStore { return &BlockStore{db: database} }

// Block records a boundary, idempotently. Blocking someone already blocked
// returns the existing row rather than an error: the listener's intent is
// unambiguous, and a retry after a lost response must not read as a failure.
func (s *BlockStore) Block(ctx context.Context, blockerID, blockedID, reason string) (*UserBlock, bool, error) {
	if err := moderation.ValidateBlock(blockerID, blockedID); err != nil {
		return nil, false, err
	}
	if err := moderation.ValidateBlockReason(reason); err != nil {
		return nil, false, err
	}
	// Refusing an unknown account matters. Without it a block row can name an
	// id that matches nobody, and the filter it drives silently does nothing
	// while looking like protection.
	if ok, err := s.userExists(ctx, blockedID); err != nil {
		return nil, false, err
	} else if !ok {
		return nil, false, ErrNotFound
	}

	ts := now()
	rec := &UserBlock{ID: newID(), BlockerID: blockerID, BlockedID: blockedID, Reason: reason, CreatedAt: ts}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO user_blocks (id,blocker_id,blocked_id,reason,created_at,updated_at)
		 VALUES (?,?,?,?,?,?)
		 ON CONFLICT (blocker_id,blocked_id) DO NOTHING`,
		rec.ID, rec.BlockerID, rec.BlockedID, nullIfEmpty(rec.Reason), ts, ts)
	if err != nil {
		return nil, false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		existing, err := s.block(ctx, blockerID, blockedID)
		if err != nil {
			return nil, false, err
		}
		return existing, false, nil
	}
	return rec, true, nil
}

// Unblock removes the boundary. Unblocking something that was never blocked is
// not an error: the listener asked for the boundary to be gone and it is.
func (s *BlockStore) Unblock(ctx context.Context, blockerID, blockedID string) error {
	if err := moderation.ValidateBlock(blockerID, blockedID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM user_blocks WHERE blocker_id=? AND blocked_id=?`, blockerID, blockedID)
	return err
}

// Blocks returns the accounts this listener has blocked, newest first.
func (s *BlockStore) Blocks(ctx context.Context, blockerID string) ([]UserBlock, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,blocker_id,blocked_id,COALESCE(reason,''),created_at
		 FROM user_blocks WHERE blocker_id=? ORDER BY created_at DESC`, blockerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserBlock{}
	for rows.Next() {
		var b UserBlock
		if err := rows.Scan(&b.ID, &b.BlockerID, &b.BlockedID, &b.Reason, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// BlockedIDs returns the ids this listener has blocked, for filtering a feed.
func (s *BlockStore) BlockedIDs(ctx context.Context, blockerID string) ([]string, error) {
	if blockerID == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT blocked_id FROM user_blocks WHERE blocker_id=?`, blockerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// BlocksBetween reports whether a block exists in either direction between two
// accounts. One query rather than two, because the caller only ever needs the
// answer and a reaction check runs on a hot path.
func (s *BlockStore) BlocksBetween(ctx context.Context, a, b string) (bool, error) {
	if a == "" || b == "" || a == b {
		return false, nil
	}
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM user_blocks
		 WHERE (blocker_id=? AND blocked_id=?) OR (blocker_id=? AND blocked_id=?)`,
		a, b, b, a).Scan(&n)
	return n > 0, err
}

func (s *BlockStore) block(ctx context.Context, blockerID, blockedID string) (*UserBlock, error) {
	var b UserBlock
	err := s.db.QueryRowContext(ctx,
		`SELECT id,blocker_id,blocked_id,COALESCE(reason,''),created_at
		 FROM user_blocks WHERE blocker_id=? AND blocked_id=?`, blockerID, blockedID).
		Scan(&b.ID, &b.BlockerID, &b.BlockedID, &b.Reason, &b.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// PostAuthor returns the author of a community post.
//
// It lives here rather than in the community store because the block check is
// the only caller that needs it, and putting the lookup next to the rule it
// serves keeps the two from drifting. An unknown post is ErrNotFound: reacting
// to a post that does not exist is a 404 whether or not a block is involved.
func (s *BlockStore) PostAuthor(ctx context.Context, postID string) (string, error) {
	if postID == "" {
		return "", ErrNotFound
	}
	var author string
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id FROM community_posts WHERE id=?`, postID).Scan(&author)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return author, nil
}

func (s *BlockStore) userExists(ctx context.Context, id string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE id=?`, id).Scan(&n)
	return n > 0, err
}

// ---------------------------------------------------------------------------
// Appeals
// ---------------------------------------------------------------------------

// ModerationAppeal is a listener's answer to a decision made about them.
type ModerationAppeal struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	DecisionType string `json:"decision_type"`
	DecisionID   string `json:"decision_id"`
	Statement    string `json:"statement"`
	Status       string `json:"status"`
	ReviewedBy   string `json:"reviewed_by,omitempty"`
	ReviewedAt   string `json:"reviewed_at,omitempty"`
	DecisionNote string `json:"decision_note,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// Appeal creates an appeal against a decision, idempotently per decision.
//
// Three things are checked before a row is written, because an appeal that
// passes without them is worse than no appeal:
//
//   - the decision must exist and belong to this account, so nobody can appeal
//     a stranger's rejection and read the moderator's reasoning;
//   - the decision must be in an appealable state — a report that is still open
//     has not been decided yet, and a confession still in review has not been
//     rejected;
//   - the statement is bounded, because a moderator reads it.
func (s *ModerationStore) Appeal(ctx context.Context, userID, decisionType, decisionID, statement string) (*ModerationAppeal, bool, error) {
	if userID == "" {
		return nil, false, ErrNotFound
	}
	if !moderation.ValidAppealDecisionType(decisionType) {
		return nil, false, fmt.Errorf("%w: %q is not an appealable decision", ErrAppealableDecisionNotFound, decisionType)
	}
	if err := moderation.ValidateAppealStatement(statement); err != nil {
		return nil, false, err
	}

	switch decisionType {
	case "report":
		rep, err := s.reportByID(ctx, decisionID)
		if err != nil {
			return nil, false, err
		}
		if rep.ReporterID != userID {
			return nil, false, ErrAppealNotOwned
		}
		// Only a dismissal is appealable. An open report is still being worked
		// and a resolved one went the reporter's way, so neither is a grievance.
		if rep.Status != "dismissed" {
			return nil, false, fmt.Errorf("%w: the report is %s, not dismissed", ErrAppealableDecisionNotFound, rep.Status)
		}
	case "confession":
		uc, err := s.userConfessionByID(ctx, decisionID)
		if err != nil {
			return nil, false, err
		}
		if uc.UserID != userID {
			return nil, false, ErrAppealNotOwned
		}
		if uc.Status != string(moderation.UGCRejected) {
			return nil, false, fmt.Errorf("%w: the confession is %s, not rejected", ErrAppealableDecisionNotFound, uc.Status)
		}
	}

	ts := now()
	rec := &ModerationAppeal{
		ID: newID(), UserID: userID, DecisionType: decisionType, DecisionID: decisionID,
		Statement: statement, Status: string(moderation.AppealSubmitted),
		CreatedAt: ts, UpdatedAt: ts,
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO moderation_appeals
		 (id,user_id,decision_type,decision_id,statement,status,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?)
		 ON CONFLICT (decision_type,decision_id) DO NOTHING`,
		rec.ID, rec.UserID, rec.DecisionType, rec.DecisionID, rec.Statement, rec.Status, ts, ts)
	if err != nil {
		return nil, false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		existing, err := s.appealFor(ctx, decisionType, decisionID)
		if err != nil {
			return nil, false, err
		}
		return existing, false, nil
	}
	return rec, true, nil
}

// AppealsFor returns one account's appeals, newest first.
func (s *ModerationStore) AppealsFor(ctx context.Context, userID string) ([]ModerationAppeal, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+appealColumns+` FROM moderation_appeals
		 WHERE user_id=? AND deleted_at IS NULL ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ModerationAppeal{}
	for rows.Next() {
		a, err := scanAppeal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// PendingAppeals returns undecided appeals oldest first, which is the order a
// queue drains in. It is the appeals half of the moderation queue.
func (s *ModerationStore) PendingAppeals(ctx context.Context) ([]ModerationAppeal, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+appealColumns+` FROM moderation_appeals
		 WHERE status IN ('submitted','under_review') AND deleted_at IS NULL
		 ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ModerationAppeal{}
	for rows.Next() {
		a, err := scanAppeal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DecideAppeal closes an appeal and, when it is overturned, reopens the work
// the original decision closed.
//
// Overturning is deliberately not a grant. An overturned report goes back to
// open and its entity gets a queue entry again; an overturned confession goes
// back to submitted, which re-enters the review queue. Neither is published and
// neither is resolved, because an appeal succeeding means the first decision was
// wrong — it does not mean the opposite decision is right. That call still
// belongs to the moderator, with the content in front of them.
func (s *ModerationStore) DecideAppeal(ctx context.Context, appealID, decision, actor, note string) (*ModerationAppeal, error) {
	if !moderation.ValidAppealDecision(decision) {
		return nil, fmt.Errorf("unknown appeal decision %q", decision)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	ap, err := scanAppeal(tx.QueryRowContext(ctx,
		`SELECT `+appealColumns+` FROM moderation_appeals WHERE id=? FOR UPDATE`, appealID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := moderation.ValidateAppealTransition(
		moderation.AppealStatus(ap.Status), moderation.AppealStatus(decision)); err != nil {
		var terr *moderation.AppealTransitionError
		if errors.As(err, &terr) && moderation.IsAppealTerminal(moderation.AppealStatus(ap.Status)) {
			return nil, ErrAppealNotDecidable
		}
		return nil, err
	}

	ts := now()
	if _, err := tx.ExecContext(ctx,
		`UPDATE moderation_appeals
		 SET status=?, reviewed_by=?, reviewed_at=?, decision_note=?, updated_at=?
		 WHERE id=?`,
		decision, actor, ts, nullIfEmpty(note), ts, appealID); err != nil {
		return nil, err
	}

	if decision == string(moderation.AppealOverturned) {
		if err := s.reopenDecision(ctx, tx, ap, actor, ts); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	out, err := s.appealByID(ctx, appealID)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// reopenDecision undoes the effect of the decision an overturned appeal was
// filed against, and nothing else.
func (s *ModerationStore) reopenDecision(ctx context.Context, tx *db.Tx, ap ModerationAppeal, actor, ts string) error {
	switch ap.DecisionType {
	case "report":
		// reports carries no updated_at column - it predates the uniform row
		// metadata - so the reopen clears the decision fields instead of
		// stamping a time. Clearing is also the honest result: an overturned
		// dismissal means the report is undecided again, and the moderator's
		// original reasoning survives on the appeal row rather than being
		// overwritten here.
		res, err := tx.ExecContext(ctx,
			`UPDATE reports SET status='open', reviewed_by=NULL, reviewed_at=NULL, resolution_note=NULL
			 WHERE id=? AND status='dismissed'`, ap.DecisionID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			// The report moved on without us. Leaving the appeal overturned is
			// still true — the dismissal was wrong — but there is nothing to
			// reopen, and inventing a state change would be a lie in the audit
			// trail.
			return nil
		}
		var entityType, entityID string
		if err := tx.QueryRowContext(ctx,
			`SELECT entity_type, entity_id FROM reports WHERE id=?`, ap.DecisionID).
			Scan(&entityType, &entityID); err != nil {
			return err
		}
		// The case is the queue entry, so reopening the report without one
		// would put the entity back in the list of complaints and leave the
		// moderator nothing to work on.
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO moderation_cases (id, entity_type, entity_id, status, reason, actor, created_at, updated_at)
			 VALUES (?,?,?,'open',?,?,?,?)
			 ON CONFLICT (entity_type, entity_id) WHERE status IN ('open','in_review') DO NOTHING`,
			newID(), entityType, entityID,
			nullIfEmpty("reopened by an overturned appeal"), nullIfEmpty(actor), ts, ts); err != nil {
			return err
		}
	case "confession":
		_, err := tx.ExecContext(ctx,
			`UPDATE user_confessions SET status=?, updated_at=? WHERE id=? AND status=?`,
			string(moderation.UGCSubmitted), ts, ap.DecisionID, string(moderation.UGCRejected))
		if err != nil {
			return err
		}
		var entityType, entityID = "user_confession", ap.DecisionID
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO moderation_cases (id, entity_type, entity_id, status, reason, actor, created_at, updated_at)
			 VALUES (?,?,?,'open',?,?,?,?)
			 ON CONFLICT (entity_type, entity_id) WHERE status IN ('open','in_review') DO NOTHING`,
			newID(), entityType, entityID,
			nullIfEmpty("requeued by an overturned appeal"), nullIfEmpty(actor), ts, ts); err != nil {
			return err
		}
	}
	return nil
}

func (s *ModerationStore) appealFor(ctx context.Context, decisionType, decisionID string) (*ModerationAppeal, error) {
	ap, err := scanAppeal(s.db.QueryRowContext(ctx,
		`SELECT `+appealColumns+` FROM moderation_appeals WHERE decision_type=? AND decision_id=?`,
		decisionType, decisionID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &ap, nil
}

func (s *ModerationStore) appealByID(ctx context.Context, id string) (*ModerationAppeal, error) {
	ap, err := scanAppeal(s.db.QueryRowContext(ctx,
		`SELECT `+appealColumns+` FROM moderation_appeals WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &ap, nil
}

const appealColumns = `id,user_id,decision_type,decision_id,statement,status,
	COALESCE(reviewed_by,''),COALESCE(reviewed_at,''),COALESCE(decision_note,''),
	created_at,updated_at`

type appealScanner interface {
	Scan(dest ...any) error
}

func scanAppeal(row appealScanner) (ModerationAppeal, error) {
	var a ModerationAppeal
	err := row.Scan(&a.ID, &a.UserID, &a.DecisionType, &a.DecisionID, &a.Statement,
		&a.Status, &a.ReviewedBy, &a.ReviewedAt, &a.DecisionNote, &a.CreatedAt, &a.UpdatedAt)
	return a, err
}

// BlockedAuthorFilter returns the SQL fragment and arguments that exclude a set
// of authors from a published-UGC read. Returning the fragment rather than
// filtering in Go matters: a limit applied after an in-memory filter returns
// fewer rows than the caller asked for, which reads as a shorter feed rather
// than as the filter working.
func BlockedAuthorFilter(blocked []string, placeholder func(int) string) (string, []any) {
	if len(blocked) == 0 {
		return "", nil
	}
	args := make([]any, len(blocked))
	holders := make([]string, len(blocked))
	for i, id := range blocked {
		args[i] = id
		holders[i] = placeholder(i + 1)
	}
	return " AND user_id NOT IN (" + strings.Join(holders, ",") + ")", args
}
