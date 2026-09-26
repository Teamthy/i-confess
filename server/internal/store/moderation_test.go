package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
	"github.com/Teamthy/i-confess/internal/moderation"
)

func modCtx() context.Context { return context.Background() }

// Every helper takes the test's connection. dbtest.New mints a fresh database
// per call, so a helper that called dbtest.New or dbtest.Raw on its own would
// seed a database the store under test never sees.

func seedUser(t *testing.T, conn *db.DB, id, email string) {
	t.Helper()
	if _, err := conn.ExecContext(modCtx(),
		`INSERT INTO users (id,email,password_hash,status,created_at,updated_at)
		 VALUES (?,?,'x','active','2026-01-01','2026-01-01')`, id, email); err != nil {
		t.Fatalf("insert user %s: %v", id, err)
	}
}

func seedModCat(t *testing.T, conn *db.DB) {
	t.Helper()
	if _, err := conn.ExecContext(modCtx(),
		`INSERT INTO categories (id,name,slug,description,icon,premium,status,sort_order,created_at,updated_at)
		 VALUES ('modcat','Mod','mod','d','i',0,'published',1,'2026-01-01','2026-01-01')
		 ON CONFLICT (id) DO NOTHING`); err != nil {
		t.Fatalf("insert category: %v", err)
	}
}

func seedPublishedConfession(t *testing.T, conn *db.DB, id string) {
	t.Helper()
	seedModCat(t, conn)
	if _, err := conn.ExecContext(modCtx(),
		`INSERT INTO confessions (id,category_id,title,short_text,intensity,language,status,author,version,published_at,created_at,updated_at)
		 VALUES (?,'modcat','t','text',1,'en','published','test',1,'2026-02-01','2026-01-01','2026-01-01')`, id); err != nil {
		t.Fatalf("insert confession %s: %v", id, err)
	}
}

func seedUserConfession(t *testing.T, conn *db.DB, id, userID, status, visibility string) {
	t.Helper()
	if _, err := conn.ExecContext(modCtx(),
		`INSERT INTO user_confessions (id,user_id,title,text,is_private,status,visibility,created_at,updated_at)
		 VALUES (?,?,'mine','words',1,?,?,'2026-01-01','2026-01-01')`,
		id, userID, status, visibility); err != nil {
		t.Fatalf("insert user confession %s: %v", id, err)
	}
}

func openCaseCount(t *testing.T, conn *db.DB, entityType, entityID string) int {
	t.Helper()
	var n int
	if err := conn.QueryRowContext(modCtx(),
		`SELECT COUNT(*) FROM moderation_cases WHERE entity_type=? AND entity_id=? AND status IN ('open','in_review')`,
		entityType, entityID).Scan(&n); err != nil {
		t.Fatalf("count cases: %v", err)
	}
	return n
}

func TestReportDeduplicatesPerReporterPerEntity(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	s := NewModerationStore(conn)
	ctx := modCtx()

	seedUser(t, conn, "rep-u1", "rep-u1@example.com")
	seedPublishedConfession(t, conn, "rep-c1")

	rep, already, err := s.CreateReport(ctx, "rep-u1", "confession", "rep-c1", "this is wrong", "")
	if err != nil {
		t.Fatalf("create report: %v", err)
	}
	if already {
		t.Fatal("first report claimed to be a duplicate")
	}
	if rep.Status != "open" {
		t.Errorf("report status = %q, want open", rep.Status)
	}

	rep2, already, err := s.CreateReport(ctx, "rep-u1", "confession", "rep-c1", "still wrong", "more detail")
	if err != nil {
		t.Fatalf("duplicate report should not error: %v", err)
	}
	if !already {
		t.Error("second identical report should report alreadyExisted")
	}
	if rep2.ID != rep.ID {
		t.Errorf("duplicate returned a different row: %s vs %s", rep2.ID, rep.ID)
	}

	var n int
	if err := conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM reports WHERE entity_id='rep-c1'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("one open report per reporter per entity: found %d rows", n)
	}
	if got := openCaseCount(t, conn, "confession", "rep-c1"); got != 1 {
		t.Errorf("one case should have opened, found %d", got)
	}

	// A second reporter is a second report on the same case.
	seedUser(t, conn, "rep-u2", "rep-u2@example.com")
	if _, _, err := s.CreateReport(ctx, "rep-u2", "confession", "rep-c1", "me too", ""); err != nil {
		t.Fatalf("second reporter: %v", err)
	}
	if got := openCaseCount(t, conn, "confession", "rep-c1"); got != 1 {
		t.Errorf("two reports are still one case, found %d", got)
	}
}

func TestReportRejectsUnseeableEntities(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	s := NewModerationStore(conn)
	ctx := modCtx()

	seedModCat(t, conn)
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO confessions (id,category_id,title,intensity,language,status,author,version,created_at,updated_at)
		 VALUES ('rep-draft','modcat','t',1,'en','draft','test',1,'2026-01-01','2026-01-01')`); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name       string
		entityType string
		entityID   string
		want       bool
	}{
		{"missing confession", "confession", "nope", false},
		{"unpublished confession", "confession", "rep-draft", false},
		{"missing post", "community_post", "nope", false},
	} {
		got, err := s.ReportableEntityExists(ctx, tc.entityType, tc.entityID)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("%s: ReportableEntityExists = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestDecideReportClosesCaseWithLastOpenReport(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	s := NewModerationStore(conn)
	ctx := modCtx()

	seedUser(t, conn, "dec-a", "dec-a@example.com")
	seedUser(t, conn, "dec-b", "dec-b@example.com")
	seedUser(t, conn, "dec-admin", "dec-admin@example.com")
	seedPublishedConfession(t, conn, "dec-c1")

	r1, _, err := s.CreateReport(ctx, "dec-a", "confession", "dec-c1", "first", "")
	if err != nil {
		t.Fatal(err)
	}
	r2, _, err := s.CreateReport(ctx, "dec-b", "confession", "dec-c1", "second", "")
	if err != nil {
		t.Fatal(err)
	}

	d1, err := s.DecideReport(ctx, r1.ID, "dismissed", "dec-admin", "not actionable")
	if err != nil {
		t.Fatalf("decide first: %v", err)
	}
	if d1.Status != "dismissed" || d1.ReviewedBy != "dec-admin" || d1.ResolutionNote != "not actionable" {
		t.Errorf("decision not recorded: %+v", d1)
	}
	if got := openCaseCount(t, conn, "confession", "dec-c1"); got != 1 {
		t.Errorf("one report still open: case should be open, found %d", got)
	}

	if _, err := s.DecideReport(ctx, r2.ID, "resolved", "dec-admin", "pulled"); err != nil {
		t.Fatalf("decide second: %v", err)
	}
	if got := openCaseCount(t, conn, "confession", "dec-c1"); got != 0 {
		t.Errorf("last open report decided: case should be closed, found %d", got)
	}

	var status, after string
	if err := conn.QueryRowContext(ctx,
		`SELECT status, COALESCE(after,'') FROM moderation_cases WHERE entity_type='confession' AND entity_id='dec-c1'`).Scan(&status, &after); err != nil {
		t.Fatal(err)
	}
	if status != "resolved" || after != "resolved" {
		t.Errorf("case = (%s, after %s), want (resolved, resolved)", status, after)
	}

	// The closed report cannot be decided again.
	if _, err := s.DecideReport(ctx, r2.ID, "dismissed", "dec-admin", ""); !errors.Is(err, ErrReportNotOpen) {
		t.Errorf("second decision on closed report: got %v, want ErrReportNotOpen", err)
	}
	// A resolved report does not block a fresh complaint.
	if _, already, err := s.CreateReport(ctx, "dec-a", "confession", "dec-c1", "it is back", ""); err != nil || already {
		t.Errorf("fresh report after resolution: already=%v err=%v", already, err)
	}
	if got := openCaseCount(t, conn, "confession", "dec-c1"); got != 1 {
		t.Errorf("the fresh complaint should reopen a case, found %d", got)
	}
}

func TestSubmitUserConfessionFlow(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	s := NewModerationStore(conn)
	ctx := modCtx()

	seedUser(t, conn, "sub-u1", "sub-u1@example.com")
	seedUser(t, conn, "sub-u2", "sub-u2@example.com")
	seedUserConfession(t, conn, "sub-c1", "sub-u1", "draft", "private")

	uc, err := s.SubmitUserConfession(ctx, "sub-u1", "sub-c1")
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if uc.Status != "submitted" {
		t.Errorf("status = %q, want submitted", uc.Status)
	}
	if got := openCaseCount(t, conn, "user_confession", "sub-c1"); got != 1 {
		t.Errorf("submission should open a case, found %d", got)
	}

	var trans *moderation.UGCTransitionError
	if _, err := s.SubmitUserConfession(ctx, "sub-u1", "sub-c1"); !errors.As(err, &trans) {
		t.Errorf("double submit: got %v, want UGCTransitionError", err)
	}
	if _, err := s.SubmitUserConfession(ctx, "sub-u2", "sub-c1"); !errors.Is(err, ErrForbidden) {
		t.Errorf("submitting someone else's confession: got %v, want ErrForbidden", err)
	}
	if _, err := s.SubmitUserConfession(ctx, "sub-u1", "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("submitting a missing id: got %v, want ErrNotFound", err)
	}
	// An author cannot decide their own work by submitting: the decision
	// transitions stay refused from draft. (Enforced by the transition table;
	// restated here so a future edit cannot quietly add draft->approved.)
	if err := moderation.ValidateUGCTransition(moderation.UGCDraft, moderation.UGCApproved); err == nil {
		t.Error("draft -> approved must stay illegal or submission is a publish button")
	}
}

func TestReviewPublishesOnlyWhatTheAuthorOfferedPublicly(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	s := NewModerationStore(conn)
	ctx := modCtx()

	seedUser(t, conn, "rev-u1", "rev-u1@example.com")
	seedUser(t, conn, "rev-admin", "rev-admin@example.com")
	seedUserConfession(t, conn, "rev-c1", "rev-u1", "draft", "public")
	if _, err := s.SubmitUserConfession(ctx, "rev-u1", "rev-c1"); err != nil {
		t.Fatal(err)
	}

	uc, err := s.ReviewUserConfession(ctx, "rev-c1", "approved", "rev-admin", "solid", "")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if uc.Status != "published" {
		t.Errorf("public confession after approval = %q, want published", uc.Status)
	}
	if uc.ReviewedBy != "rev-admin" || uc.ReviewedAt == "" || uc.ReviewNotes != "solid" {
		t.Errorf("review fields not recorded: %+v", uc)
	}
	if uc.PublishedAt == "" {
		t.Error("published confession must carry published_at")
	}
	if got := openCaseCount(t, conn, "user_confession", "rev-c1"); got != 0 {
		t.Errorf("review should resolve its case, found %d open", got)
	}

	// Private intent approves without publishing.
	seedUserConfession(t, conn, "rev-c2", "rev-u1", "draft", "shared")
	if _, err := s.SubmitUserConfession(ctx, "rev-u1", "rev-c2"); err != nil {
		t.Fatal(err)
	}
	uc2, err := s.ReviewUserConfession(ctx, "rev-c2", "approved", "rev-admin", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if uc2.Status != "approved" {
		t.Errorf("shared confession after approval = %q, want approved", uc2.Status)
	}
	if uc2.PublishedAt != "" {
		t.Error("approved-not-published must not carry published_at")
	}
}

func TestReviewRequiresAReasonToReject(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	s := NewModerationStore(conn)
	ctx := modCtx()

	seedUser(t, conn, "rej-u1", "rej-u1@example.com")
	seedUser(t, conn, "rej-admin", "rej-admin@example.com")
	seedUserConfession(t, conn, "rej-c1", "rej-u1", "draft", "private")
	if _, err := s.SubmitUserConfession(ctx, "rej-u1", "rej-c1"); err != nil {
		t.Fatal(err)
	}

	if _, err := s.ReviewUserConfession(ctx, "rej-c1", "rejected", "rej-admin", "", ""); err == nil ||
		!strings.Contains(err.Error(), "reason") {
		t.Errorf("silent rejection: got %v, want a reason-required error", err)
	}
	// The failed call must not have decided anything.
	uc, _ := s.userConfessionByID(ctx, "rej-c1")
	if uc.Status != "submitted" {
		t.Errorf("failed rejection changed status to %q", uc.Status)
	}

	uc, err := s.ReviewUserConfession(ctx, "rej-c1", "rejected", "rej-admin", "", "scripture misquoted")
	if err != nil {
		t.Fatalf("rejection with reason: %v", err)
	}
	if uc.Status != "rejected" || uc.RejectionReason != "scripture misquoted" {
		t.Errorf("rejection not recorded: %+v", uc)
	}

	// A reworked confession re-enters the queue; the old rejection fields clear.
	if _, err := s.SubmitUserConfession(ctx, "rej-u1", "rej-c1"); err != nil {
		t.Fatalf("resubmit after rejection: %v", err)
	}
	uc, _ = s.userConfessionByID(ctx, "rej-c1")
	if uc.Status != "submitted" || uc.RejectionReason != "" || uc.ReviewedBy != "" {
		t.Errorf("resubmission should clear the last decision, got %+v", uc)
	}
}

func TestReviewRefusesADraft(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	s := NewModerationStore(conn)
	ctx := modCtx()

	seedUser(t, conn, "dr-u1", "dr-u1@example.com")
	seedUser(t, conn, "dr-admin", "dr-admin@example.com")
	seedUserConfession(t, conn, "dr-c1", "dr-u1", "draft", "private")

	var trans *moderation.UGCTransitionError
	if _, err := s.ReviewUserConfession(ctx, "dr-c1", "approved", "dr-admin", "", ""); !errors.As(err, &trans) {
		t.Errorf("reviewing a draft: got %v, want UGCTransitionError", err)
	}
}

func TestModerationQueueDrainsAsWorkIsDone(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	s := NewModerationStore(conn)
	ctx := modCtx()

	seedUser(t, conn, "q-u1", "q-u1@example.com")
	seedUser(t, conn, "q-admin", "q-admin@example.com")
	seedPublishedConfession(t, conn, "q-c1")
	seedUserConfession(t, conn, "q-uc1", "q-u1", "draft", "private")

	// Editorial pending: one confession in a human review state.
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO confessions (id,category_id,title,intensity,language,status,author,version,created_at,updated_at)
		 VALUES ('q-ed1','modcat','t',1,'en','theological_review','test',1,'2026-01-01','2026-01-01')`); err != nil {
		t.Fatal(err)
	}

	q, err := s.ModerationQueue(ctx)
	if err != nil {
		t.Fatalf("queue: %v", err)
	}
	if q.Counts["user_confessions"] != 0 || q.Counts["reports"] != 0 {
		t.Errorf("queue should start with only editorial: %+v", q.Counts)
	}
	if q.Counts["editorial"] != 1 || q.Editorial[0].ID != "q-ed1" {
		t.Errorf("editorial section wrong: %+v", q.Editorial)
	}

	if _, err := s.SubmitUserConfession(ctx, "q-u1", "q-uc1"); err != nil {
		t.Fatal(err)
	}
	rep, _, err := s.CreateReport(ctx, "q-u1", "confession", "q-c1", "offensive", "")
	if err != nil {
		t.Fatal(err)
	}

	q, _ = s.ModerationQueue(ctx)
	if q.Counts["user_confessions"] != 1 || q.Counts["reports"] != 1 || q.Counts["open_cases"] != 2 {
		t.Errorf("queue after seeding: %+v", q.Counts)
	}

	if _, err := s.ReviewUserConfession(ctx, "q-uc1", "approved", "q-admin", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DecideReport(ctx, rep.ID, "resolved", "q-admin", "handled"); err != nil {
		t.Fatal(err)
	}

	q, _ = s.ModerationQueue(ctx)
	if q.Counts["user_confessions"] != 0 || q.Counts["reports"] != 0 || q.Counts["open_cases"] != 0 {
		t.Errorf("queue after drain: %+v", q.Counts)
	}
	if q.Counts["editorial"] != 1 {
		t.Errorf("editorial work is unaffected by UGC decisions: %+v", q.Counts)
	}
	// Empty sections serialise as [], never null.
	if q.UserConfessions == nil || q.Reports == nil {
		t.Error("empty queue sections must be empty slices, not nil")
	}
}

// ---------- QA gate seeding ----------

func seedQAFixture(t *testing.T, conn *db.DB, confID, confStatus, assetStatus string, licensed bool) {
	t.Helper()
	ctx := modCtx()
	seedModCat(t, conn)

	if _, err := conn.ExecContext(ctx,
		`INSERT INTO confessions (id,category_id,title,short_text,intensity,language,status,author,version,created_at,updated_at)
		 VALUES (?,'modcat','t','text',1,'en',?,'test',1,'2026-01-01','2026-01-01')`, confID, confStatus); err != nil {
		t.Fatalf("insert confession: %v", err)
	}
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO voices (id,name,type,language,premium,status,created_at,updated_at)
		 VALUES ('qa-voice','V','professional','en',0,'active','2026-01-01','2026-01-01')
		 ON CONFLICT (id) DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	if licensed {
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO voice_rights (id, voice_id, rights_holder, allowed_use, territories, status, created_at, updated_at)
			 VALUES (?, 'qa-voice', 'holder', 'tts', 'GLOBAL', 'active', '2026-01-01', '2026-01-01')`,
			"qa-rights-"+confID); err != nil {
			t.Fatalf("insert voice rights: %v", err)
		}
	}
	if assetStatus != "" {
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO audio_assets (id, content_id, voice_id, storage_key, duration_seconds, file_size_bytes, status, created_at, updated_at)
			 VALUES (?, ?, 'qa-voice', 'k/x', 120, 4096, ?, '2026-01-01', '2026-01-01')`,
			"qa-asset-"+confID, confID, assetStatus); err != nil {
			t.Fatalf("insert audio asset: %v", err)
		}
	}
}

func TestRunConfessionQAPassesAndRecords(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	s := NewModerationStore(conn)
	ctx := modCtx()

	seedUser(t, conn, "qa-admin", "qa-admin@example.com")
	seedQAFixture(t, conn, "qa-c1", "audio_qa", "ready", true)

	rep, err := s.RunConfessionQA(ctx, "qa-c1", "qa-admin", "checked")
	if err != nil {
		t.Fatalf("QA run: %v", err)
	}
	if !rep.Passed {
		t.Fatalf("gate should pass: %+v", rep.Checks)
	}

	var status, qaPassedAt, qaReport string
	if err := conn.QueryRowContext(ctx,
		`SELECT status, COALESCE(qa_passed_at,''), COALESCE(qa_report,'') FROM confessions WHERE id='qa-c1'`).Scan(&status, &qaPassedAt, &qaReport); err != nil {
		t.Fatal(err)
	}
	if status != "approved" {
		t.Errorf("status = %q, want approved", status)
	}
	if qaPassedAt == "" {
		t.Error("qa_passed_at not set on a pass")
	}
	if !strings.Contains(qaReport, `"passed":true`) {
		t.Errorf("persisted report does not record the pass: %s", qaReport)
	}

	var hist int
	if err := conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM content_moderation_history
		 WHERE confession_id='qa-c1' AND from_status='audio_qa' AND to_status='approved' AND actor='qa-admin'`).Scan(&hist); err != nil {
		t.Fatal(err)
	}
	if hist != 1 {
		t.Errorf("expected one history row for the gate, found %d", hist)
	}
}

func TestRunConfessionQAFailsWithoutReadyAudio(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	s := NewModerationStore(conn)
	ctx := modCtx()

	seedUser(t, conn, "qa-admin2", "qa-admin2@example.com")
	seedQAFixture(t, conn, "qa-c2", "audio_qa", "processing", true)

	rep, err := s.RunConfessionQA(ctx, "qa-c2", "qa-admin2", "")
	if err != nil {
		t.Fatalf("QA run should not error on a failed checklist: %v", err)
	}
	if rep.Passed {
		t.Fatal("a confession with its render still processing must not pass the gate")
	}
	failed := map[string]bool{}
	for _, c := range rep.Checks {
		if !c.Passed {
			failed[c.Name] = true
		}
	}
	if !failed["audio_ready"] || !failed["no_render_in_flight"] {
		t.Errorf("expected audio_ready and no_render_in_flight to fail, got %v", failed)
	}

	var status, qaPassedAt, qaReport string
	if err := conn.QueryRowContext(ctx,
		`SELECT status, COALESCE(qa_passed_at,''), COALESCE(qa_report,'') FROM confessions WHERE id='qa-c2'`).Scan(&status, &qaPassedAt, &qaReport); err != nil {
		t.Fatal(err)
	}
	if status != "audio_qa" {
		t.Errorf("a failed gate must not transition, status = %q", status)
	}
	if qaPassedAt != "" {
		t.Error("qa_passed_at set on a failed gate")
	}
	// The failed report is persisted: a failed gate leaves evidence.
	if !strings.Contains(qaReport, `"passed":false`) {
		t.Errorf("failed run not persisted: %s", qaReport)
	}
}

func TestRunConfessionQACatchesAnUnlicensedVoice(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	s := NewModerationStore(conn)
	ctx := modCtx()

	seedUser(t, conn, "qa-admin3", "qa-admin3@example.com")
	seedQAFixture(t, conn, "qa-c3", "audio_qa", "ready", false)

	rep, err := s.RunConfessionQA(ctx, "qa-c3", "qa-admin3", "")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Passed {
		t.Fatal("a ready render on an unlicensed voice must not pass the gate")
	}
	found := false
	for _, c := range rep.Checks {
		if c.Name == "voices_licensed" && !c.Passed {
			found = true
		}
	}
	if !found {
		t.Errorf("voices_licensed should be the failing check: %+v", rep.Checks)
	}
}

func TestRunConfessionQAGateState(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	s := NewModerationStore(conn)
	ctx := modCtx()

	seedUser(t, conn, "qa-admin4", "qa-admin4@example.com")
	seedQAFixture(t, conn, "qa-c4", "draft", "ready", true)

	var gateErr *ErrQAGateState
	if _, err := s.RunConfessionQA(ctx, "qa-c4", "qa-admin4", ""); !errors.As(err, &gateErr) {
		t.Errorf("QA on a draft: got %v, want ErrQAGateState", err)
	}
	if _, err := s.RunConfessionQA(ctx, "missing", "qa-admin4", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("QA on a missing id: got %v, want ErrNotFound", err)
	}
}

func TestUpdateConfessionStatusAudited(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	s := NewModerationStore(conn)
	ctx := modCtx()

	seedUser(t, conn, "au-admin", "au-admin@example.com")
	seedModCat(t, conn)
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO confessions (id,category_id,title,intensity,language,status,author,version,created_at,updated_at)
		 VALUES ('au-c1','modcat','t',1,'en','approved','test',1,'2026-01-01','2026-01-01')`); err != nil {
		t.Fatal(err)
	}

	if _, err := s.UpdateConfessionStatusAudited(ctx, "au-c1", "published", "au-admin", "ship"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	var publishedAt string
	if err := conn.QueryRowContext(ctx,
		`SELECT COALESCE(published_at,'') FROM confessions WHERE id='au-c1'`).Scan(&publishedAt); err != nil {
		t.Fatal(err)
	}
	if publishedAt == "" {
		t.Error("publishing must set published_at")
	}
	firstPublish := publishedAt

	// Moving off published must not erase when it first went live.
	if _, err := s.UpdateConfessionStatusAudited(ctx, "au-c1", "archived", "au-admin", "superseded"); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if err := conn.QueryRowContext(ctx,
		`SELECT COALESCE(published_at,'') FROM confessions WHERE id='au-c1'`).Scan(&publishedAt); err != nil {
		t.Fatal(err)
	}
	if publishedAt != firstPublish {
		t.Errorf("archiving rewrote published_at: %q -> %q", firstPublish, publishedAt)
	}

	var histCount int
	if err := conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM content_moderation_history WHERE confession_id='au-c1'`).Scan(&histCount); err != nil {
		t.Fatal(err)
	}
	if histCount != 2 {
		t.Errorf("two real moves should write two history rows, found %d", histCount)
	}

	// A no-op PATCH records nothing.
	if _, err := s.UpdateConfessionStatusAudited(ctx, "au-c1", "archived", "au-admin", "again"); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM content_moderation_history WHERE confession_id='au-c1'`).Scan(&histCount); err != nil {
		t.Fatal(err)
	}
	if histCount != 2 {
		t.Errorf("a no-op PATCH must not fabricate history, found %d rows", histCount)
	}

	if _, err := s.UpdateConfessionStatusAudited(ctx, "missing", "published", "au-admin", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing id: got %v, want ErrNotFound (the old path returned a 500)", err)
	}
}
