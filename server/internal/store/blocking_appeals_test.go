package store

import (
	"errors"
	"testing"

	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
	"github.com/Teamthy/i-confess/internal/moderation"
)

// seedModerator creates the account whose id the review and report paths write
// into reviewed_by and moderation_cases.actor. Both columns are foreign keys,
// so a decision recorded against a name that matches no account fails at the
// database rather than at the handler - which is the constraint working.
func seedModerator(t *testing.T, conn *db.DB) {
	t.Helper()
	seedUser(t, conn, "moderator", "moderator@example.com")
}

// rejectedConfessionForAppeal drives a user confession to `rejected`, which is
// the only state that can be appealed. Going through the real review path
// rather than inserting a rejected row matters: the appeal has to work against
// the state a moderator actually produces.
func rejectedConfessionForAppeal(t *testing.T, conn *db.DB, mod *ModerationStore, ucID, userID string) {
	t.Helper()
	seedModerator(t, conn)
	seedUserConfession(t, conn, ucID, userID, string(moderation.UGCSubmitted), "private")
	if _, err := mod.ReviewUserConfession(modCtx(), ucID,
		string(moderation.UGCRejected), "moderator", "", "not suitable"); err != nil {
		t.Fatalf("reject confession: %v", err)
	}
}

// dismissedReportForAppeal files a report against a published confession and
// dismisses it, which is the only report state that can be appealed.
func dismissedReportForAppeal(t *testing.T, conn *db.DB, mod *ModerationStore, confessionID, reporterID string) string {
	t.Helper()
	seedModerator(t, conn)
	seedPublishedConfession(t, conn, confessionID)
	rep, _, err := mod.CreateReport(modCtx(), reporterID, "confession", confessionID, "spam", "")
	if err != nil {
		t.Fatalf("create report: %v", err)
	}
	if _, err := mod.DecideReport(modCtx(), rep.ID, "dismissed", "moderator", "no action"); err != nil {
		t.Fatalf("dismiss report: %v", err)
	}
	return rep.ID
}

// TestAppealLifecycle is the store parity half of the trio: the same edges the
// unit table allows, applied to a real row, with the reopen effects that make
// an overturned appeal mean something.
func TestAppealLifecycle(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	ctx := modCtx()
	mod := NewModerationStore(conn)
	seedUser(t, conn, "appellant", "appellant@example.com")

	rejectedConfessionForAppeal(t, conn, mod, "appeal-uc", "appellant")

	ap, created, err := mod.Appeal(ctx, "appellant", "confession", "appeal-uc",
		"I reworked this and it no longer says what the rejection cited.")
	if err != nil || !created || ap.Status != string(moderation.AppealSubmitted) {
		t.Fatalf("appeal = %+v created=%v err=%v, want a submitted appeal", ap, created, err)
	}

	// One appeal per decision. The retry returns the stored row, so a client
	// that lost the response still sees its own appeal.
	again, created, err := mod.Appeal(ctx, "appellant", "confession", "appeal-uc",
		"A second attempt at the same decision.")
	if err != nil || created {
		t.Fatalf("second appeal = created=%v err=%v, want the existing row", created, err)
	}
	if again.ID != ap.ID {
		t.Errorf("second appeal returned id %q, want the stored %q", again.ID, ap.ID)
	}

	// Overturning sends the confession back to submitted: it re-enters the
	// review queue. It is not published, because an appeal succeeding means the
	// first decision was wrong, not that the opposite one is right.
	decided, err := mod.DecideAppeal(ctx, ap.ID, string(moderation.AppealOverturned), "moderator", "reconsidered")
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if decided.Status != string(moderation.AppealOverturned) {
		t.Fatalf("status = %s, want overturned", decided.Status)
	}
	uc, err := mod.userConfessionByID(ctx, "appeal-uc")
	if err != nil {
		t.Fatalf("read confession: %v", err)
	}
	if uc.Status != string(moderation.UGCSubmitted) {
		t.Errorf("overturned confession is %s, want submitted - an appeal must not publish", uc.Status)
	}
	if openCaseCount(t, conn, "user_confession", "appeal-uc") != 1 {
		t.Error("an overturned confession left no queue entry, so a moderator has nothing to re-review")
	}

	// A decided appeal is final.
	if _, err := mod.DecideAppeal(ctx, ap.ID, string(moderation.AppealUpheld), "moderator", ""); !errors.Is(err, ErrAppealNotDecidable) {
		t.Errorf("re-deciding a closed appeal = %v, want ErrAppealNotDecidable", err)
	}

	// And it has left the queue.
	pending, err := mod.PendingAppeals(ctx)
	if err != nil {
		t.Fatalf("pending: %v", err)
	}
	for _, p := range pending {
		if p.ID == ap.ID {
			t.Error("a decided appeal is still in the pending queue")
		}
	}
}

// TestAppealUpheldChangesNothing is the other half: upholding leaves the
// original decision exactly as it was. A system that quietly softens the
// original decision on appeal has no decisions.
func TestAppealUpheldChangesNothing(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	ctx := modCtx()
	mod := NewModerationStore(conn)
	seedUser(t, conn, "upholder", "upholder@example.com")
	rejectedConfessionForAppeal(t, conn, mod, "upheld-uc", "upholder")

	ap, _, err := mod.Appeal(ctx, "upholder", "confession", "upheld-uc",
		"I believe the rejection was mistaken and here is why.")
	if err != nil {
		t.Fatalf("appeal: %v", err)
	}
	if _, err := mod.DecideAppeal(ctx, ap.ID, string(moderation.AppealUpheld), "moderator", "the reason stands"); err != nil {
		t.Fatalf("decide: %v", err)
	}
	uc, err := mod.userConfessionByID(ctx, "upheld-uc")
	if err != nil {
		t.Fatalf("read confession: %v", err)
	}
	if uc.Status != string(moderation.UGCRejected) {
		t.Errorf("upheld appeal moved the confession to %s, want it to stay rejected", uc.Status)
	}
}

// TestAppealRefusesWhatWasNeverDecided covers the three refusals that stop the
// appeal queue filling with grievances against decisions nobody made.
func TestAppealRefusesWhatWasNeverDecided(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	ctx := modCtx()
	mod := NewModerationStore(conn)
	seedUser(t, conn, "author", "author@example.com")
	seedUser(t, conn, "stranger", "stranger@example.com")

	// A confession still in review has not been rejected.
	seedUserConfession(t, conn, "pending-uc", "author", string(moderation.UGCSubmitted), "private")
	if _, _, err := mod.Appeal(ctx, "author", "confession", "pending-uc",
		"Appealing a review that has not happened yet."); !errors.Is(err, ErrAppealableDecisionNotFound) {
		t.Errorf("appealing an undecided confession = %v, want ErrAppealableDecisionNotFound", err)
	}

	// Somebody else's rejection is not yours to appeal. This is also what stops
	// an account reading a moderator's reasoning about a stranger.
	rejectedConfessionForAppeal(t, conn, mod, "other-uc", "author")
	if _, _, err := mod.Appeal(ctx, "stranger", "confession", "other-uc",
		"Appealing a decision that was not made about me."); !errors.Is(err, ErrAppealNotOwned) {
		t.Errorf("appealing a stranger's rejection = %v, want ErrAppealNotOwned", err)
	}

	// An unknown decision type is not a server failure.
	if _, _, err := mod.Appeal(ctx, "author", "session", "other-uc",
		"Appealing something that is not appealable."); !errors.Is(err, ErrAppealableDecisionNotFound) {
		t.Errorf("appealing an unknown decision type = %v, want ErrAppealableDecisionNotFound", err)
	}

	// A statement too short to read is refused before anything is written.
	if _, _, err := mod.Appeal(ctx, "author", "confession", "other-uc", "no"); err == nil {
		t.Error("a two-character appeal statement was accepted")
	}
}

// TestAppealOfADismissedReportReopensIt proves the report half: an overturned
// appeal puts the report back to open and gives the entity a queue entry again,
// so the complaint is worked rather than merely un-dismissed.
func TestAppealOfADismissedReportReopensIt(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	ctx := modCtx()
	mod := NewModerationStore(conn)
	seedUser(t, conn, "reporter", "reporter@example.com")

	reportID := dismissedReportForAppeal(t, conn, mod, "reported-conf", "reporter")
	ap, _, err := mod.Appeal(ctx, "reporter", "report", reportID,
		"The content is still there and the dismissal did not address it.")
	if err != nil {
		t.Fatalf("appeal: %v", err)
	}
	if _, err := mod.DecideAppeal(ctx, ap.ID, string(moderation.AppealOverturned), "moderator", "you were right"); err != nil {
		t.Fatalf("decide: %v", err)
	}

	rep, err := mod.reportByID(ctx, reportID)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	if rep.Status != "open" {
		t.Errorf("overturned report is %s, want open", rep.Status)
	}
	if openCaseCount(t, conn, "confession", "reported-conf") != 1 {
		t.Error("the reopened report has no queue entry")
	}
}

// TestAppealsAppearInTheModerationQueue is the integration the phase asks for: an
// appeal is queue work, oldest first, and counted alongside the rest.
func TestAppealsAppearInTheModerationQueue(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	ctx := modCtx()
	mod := NewModerationStore(conn)
	seedUser(t, conn, "queued", "queued@example.com")
	rejectedConfessionForAppeal(t, conn, mod, "queued-uc", "queued")

	before, err := mod.ModerationQueue(ctx)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if before.Counts["appeals"] != 0 {
		t.Fatalf("queue starts with %d appeals, want 0", before.Counts["appeals"])
	}

	if _, _, err := mod.Appeal(ctx, "queued", "confession", "queued-uc",
		"This appeal should show up in the moderation queue."); err != nil {
		t.Fatalf("appeal: %v", err)
	}
	after, err := mod.ModerationQueue(ctx)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if after.Counts["appeals"] != 1 || len(after.Appeals) != 1 {
		t.Fatalf("queue reports %d appeals (%d rows), want 1", after.Counts["appeals"], len(after.Appeals))
	}

	// Deciding it drains the queue.
	pending, err := mod.PendingAppeals(ctx)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending = %v err=%v, want one appeal", pending, err)
	}
	if _, err := mod.DecideAppeal(ctx, pending[0].ID, string(moderation.AppealUpheld), "moderator", ""); err != nil {
		t.Fatalf("decide: %v", err)
	}
	drained, err := mod.ModerationQueue(ctx)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if drained.Counts["appeals"] != 0 {
		t.Errorf("a decided appeal is still counted in the queue (%d)", drained.Counts["appeals"])
	}
}

// TestBlockingIsABoundaryAndNotARecord covers the block store: idempotent,
// reversible, scoped to the blocker, and visible in both directions to the
// contact check.
func TestBlockingIsABoundaryAndNotARecord(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	ctx := modCtx()
	blocks := NewBlockStore(conn)
	seedUser(t, conn, "blocker", "blocker@example.com")
	seedUser(t, conn, "blocked", "blocked@example.com")
	seedUser(t, conn, "uninvolved", "uninvolved@example.com")

	rec, created, err := blocks.Block(ctx, "blocker", "blocked", "keeps messaging me")
	if err != nil || !created {
		t.Fatalf("block = created=%v err=%v", created, err)
	}
	if rec.BlockerID != "blocker" || rec.BlockedID != "blocked" {
		t.Errorf("block row = %+v", rec)
	}

	// Idempotent, and the retry reports the row that is actually stored.
	again, created, err := blocks.Block(ctx, "blocker", "blocked", "")
	if err != nil || created {
		t.Fatalf("second block = created=%v err=%v, want the existing row", created, err)
	}
	if again.ID != rec.ID {
		t.Errorf("second block returned id %q, want the stored %q", again.ID, rec.ID)
	}

	// A self-block is refused. Silently filtering a listener's own content out
	// of their own feed would be the worst available answer.
	if _, _, err := blocks.Block(ctx, "blocker", "blocker", ""); err == nil {
		t.Error("a self-block was accepted")
	}
	// And a block against an account that does not exist would drive a filter
	// that silently matches nobody while looking like protection.
	if _, _, err := blocks.Block(ctx, "blocker", "ghost", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("blocking a nonexistent account = %v, want ErrNotFound", err)
	}

	// The contact check sees it in both directions and not beyond the pair.
	for _, pair := range [][2]string{{"blocker", "blocked"}, {"blocked", "blocker"}} {
		got, err := blocks.BlocksBetween(ctx, pair[0], pair[1])
		if err != nil || !got {
			t.Errorf("BlocksBetween(%q,%q) = %v err=%v, want true", pair[0], pair[1], got, err)
		}
	}
	if got, _ := blocks.BlocksBetween(ctx, "blocker", "uninvolved"); got {
		t.Error("a block leaked to an uninvolved account")
	}

	ids, err := blocks.BlockedIDs(ctx, "blocker")
	if err != nil || len(ids) != 1 || ids[0] != "blocked" {
		t.Fatalf("BlockedIDs = %v err=%v, want [blocked]", ids, err)
	}
	// The blocked account's own list is empty: a block is not shown to them.
	if theirs, _ := blocks.BlockedIDs(ctx, "blocked"); len(theirs) != 0 {
		t.Errorf("the blocked account sees blocks: %v", theirs)
	}

	// Reversible, and unblocking something never blocked is not an error.
	if err := blocks.Unblock(ctx, "blocker", "blocked"); err != nil {
		t.Fatalf("unblock: %v", err)
	}
	if err := blocks.Unblock(ctx, "blocker", "blocked"); err != nil {
		t.Fatalf("second unblock should be a no-op: %v", err)
	}
	if got, _ := blocks.BlocksBetween(ctx, "blocker", "blocked"); got {
		t.Error("the block survived an unblock")
	}
	// And the pair can block again: a block is a boundary, not a permanent
	// record of a grievance.
	if _, created, err := blocks.Block(ctx, "blocker", "blocked", ""); err != nil || !created {
		t.Errorf("re-blocking after an unblock = created=%v err=%v, want a fresh block", created, err)
	}
}

// TestBlockedAuthorsAreExcludedInSQL is the point of filtering in the WHERE
// clause rather than in Go: the limit still means `limit` rows the reader may
// see, instead of `limit` rows minus however many were dropped in memory.
func TestBlockedAuthorsAreExcludedInSQL(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	ctx := modCtx()
	eng := NewEngagementStore(conn)
	seedUser(t, conn, "author-a", "a@example.com")
	seedUser(t, conn, "author-b", "b@example.com")
	seedUserConfession(t, conn, "pub-a", "author-a", "published", "public")
	seedUserConfession(t, conn, "pub-b", "author-b", "published", "public")

	all, err := eng.ListPublishedUserConfessions(ctx, 20)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("published = %d, want 2", len(all))
	}

	filtered, err := eng.ListPublishedUserConfessionsExcluding(ctx, 20, []string{"author-a"})
	if err != nil {
		t.Fatalf("filtered list: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != "pub-b" {
		t.Fatalf("filtered = %+v, want only pub-b", filtered)
	}

	// Anonymous readers have no blocks and see everything.
	anon, err := eng.ListPublishedUserConfessionsExcluding(ctx, 20, nil)
	if err != nil || len(anon) != 2 {
		t.Fatalf("anonymous = %d err=%v, want 2", len(anon), err)
	}
}
