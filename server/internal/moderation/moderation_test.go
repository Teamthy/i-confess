package moderation

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

// constraintValues reads a CHECK constraint's literal list back out of the
// live database. Asserting against the migration file would only prove what
// was written; reading pg_get_constraintdef proves what the database enforces.
func constraintValues(t *testing.T, table, column string) map[string]bool {
	t.Helper()
	raw := dbtest.Raw(t)

	var def string
	err := raw.QueryRowContext(context.Background(),
		`SELECT pg_get_constraintdef(oid) FROM pg_constraint
		  WHERE conrelid = $1::regclass AND conname = $2`,
		table, table+"_"+column+"_check").Scan(&def)
	if err != nil {
		t.Fatalf("no CHECK constraint named %s_%s_check on %s: %v", table, column, table, err)
	}

	out := map[string]bool{}
	chunks := strings.Split(def, "'")
	for i := 1; i < len(chunks); i += 2 {
		out[chunks[i]] = true
	}
	if len(out) == 0 {
		t.Fatalf("parsed no values from %q - the parser is broken, not the schema", def)
	}
	return out
}

func assertParity(t *testing.T, what string, dbValues map[string]bool, codeValues []string) {
	t.Helper()
	code := map[string]bool{}
	for _, v := range codeValues {
		code[v] = true
	}

	var onlyInCode, onlyInDB []string
	for v := range code {
		if !dbValues[v] {
			onlyInCode = append(onlyInCode, v)
		}
	}
	for v := range dbValues {
		if !code[v] {
			onlyInDB = append(onlyInDB, v)
		}
	}
	sort.Strings(onlyInCode)
	sort.Strings(onlyInDB)

	if len(onlyInCode) > 0 {
		t.Errorf("%s the code accepts but the database rejects (these fail at write time as a 500): %s",
			what, strings.Join(onlyInCode, ", "))
	}
	if len(onlyInDB) > 0 {
		t.Errorf("%s the database accepts but the code does not know (unhandled branches): %s",
			what, strings.Join(onlyInDB, ", "))
	}
	if len(onlyInCode) == 0 && len(onlyInDB) == 0 {
		t.Logf("%s agrees in both directions: %d values", what, len(code))
	}
}

// TestUGCStatusMatchesTheDatabase is the same contract
// content.TestLifecycleMatchesTheDatabase keeps for the editorial lifecycle:
// the Go authority and the CHECK constraint cannot drift apart silently.
func TestUGCStatusMatchesTheDatabase(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()

	assertParity(t, "user-confession statuses", constraintValues(t, "user_confessions", "status"), UGCStatuses())
}

// TestUGCVisibilityMatchesTheDatabase covers the constraint migration 0009
// added: visibility had been an unconstrained column since the baseline.
func TestUGCVisibilityMatchesTheDatabase(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()

	assertParity(t, "user-confession visibilities", constraintValues(t, "user_confessions", "visibility"), UGCVisibilities())
}

func TestReportStatusesMatchTheDatabase(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()

	assertParity(t, "report statuses", constraintValues(t, "reports", "status"), ReportStatuses())
}

func TestCaseStatusesMatchTheDatabase(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()

	assertParity(t, "case statuses", constraintValues(t, "moderation_cases", "status"), CaseStatuses())
}

// TestEveryUGCStatusIsWritable is the end-to-end half of parity: a status the
// constraint accepts but the write path cannot persist is a 500 in waiting.
func TestEveryUGCStatusIsWritable(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	ctx := context.Background()
	raw := dbtest.Raw(t)

	if _, err := raw.ExecContext(ctx,
		`INSERT INTO users (id,email,password_hash,status,created_at,updated_at)
		 VALUES ('uc-write-user','uc-write@example.com','x','active','2026-01-01','2026-01-01')`); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	for _, s := range UGCStatuses() {
		t.Run(s, func(t *testing.T) {
			id := "uc-write-" + s
			if _, err := raw.ExecContext(ctx,
				`INSERT INTO user_confessions (id,user_id,title,text,is_private,status,visibility,created_at,updated_at)
				 VALUES ($1,'uc-write-user','t','x',1,$2,'private','2026-01-01','2026-01-01')`, id, s); err != nil {
				t.Errorf("cannot persist status %q: %v", s, err)
			}
		})
	}
}

func TestEveryUGCVisibilityIsWritable(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	ctx := context.Background()
	raw := dbtest.Raw(t)

	if _, err := raw.ExecContext(ctx,
		`INSERT INTO users (id,email,password_hash,status,created_at,updated_at)
		 VALUES ('uc-vis-user','uc-vis@example.com','x','active','2026-01-01','2026-01-01')`); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	for _, v := range UGCVisibilities() {
		t.Run(v, func(t *testing.T) {
			id := "uc-vis-" + v
			if _, err := raw.ExecContext(ctx,
				`INSERT INTO user_confessions (id,user_id,title,text,is_private,status,visibility,created_at,updated_at)
				 VALUES ($1,'uc-vis-user','t','x',1,'draft',$2,'2026-01-01','2026-01-01')`, id, v); err != nil {
				t.Errorf("cannot persist visibility %q: %v", v, err)
			}
		})
	}
}

func TestUGCTransitionTable(t *testing.T) {
	legal := map[string][]string{
		"draft":     {"submitted", "archived"},
		"submitted": {"approved", "rejected", "published", "archived"},
		"rejected":  {"submitted", "archived"},
		"approved":  {"archived"},
		"published": {"archived"},
		"archived":  {},
	}

	for from, tos := range legal {
		for _, to := range tos {
			if err := ValidateUGCTransition(UGCStatus(from), UGCStatus(to)); err != nil {
				t.Errorf("%s -> %s should be legal, got %v", from, to, err)
			}
		}
	}

	illegal := [][2]string{
		{"draft", "approved"},  // skipping the queue
		{"draft", "published"}, // skipping the queue
		{"approved", "draft"},  // un-deciding a decision
		{"approved", "rejected"},
		{"rejected", "approved"}, // a rejection stands until resubmitted
		{"published", "draft"},
		{"archived", "submitted"}, // the archive is final
	}
	for _, move := range illegal {
		if err := ValidateUGCTransition(UGCStatus(move[0]), UGCStatus(move[1])); err == nil {
			t.Errorf("%s -> %s should be refused", move[0], move[1])
		}
	}

	if CanReview(UGCDraft) {
		t.Error("a draft must not be reviewable - the author never offered it")
	}
	if !CanReview(UGCSubmitted) {
		t.Error("a queued confession must be reviewable - that is what the queue is")
	}
	if CanReview(UGCApproved) {
		t.Error("re-deciding a decided confession rewrites history")
	}
}

func TestRejectionIsNotInTheAllowedWithoutQueue(t *testing.T) {
	// The review endpoint maps rejected confessions that are re-submitted;
	// approved ones are done. If approved gained a path back to submitted,
	// a moderator's decision would silently re-enter the queue.
	for _, to := range UGCAllowedFrom(UGCApproved) {
		if to == UGCSubmitted {
			t.Error("approved confessions must not re-enter the queue without a new author action")
		}
	}
}

func TestRunQAChecklistLogic(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)

	passFacts := QAFacts{HasText: true, ReadyRenders: 2, RendersInFlight: 0, UnlicensedVoices: 0}
	rep := RunQAChecklist(passFacts, "admin-1", "ship it", now)
	if !rep.Passed {
		t.Errorf("all facts healthy should pass, got fails: %+v", rep.Checks)
	}
	if rep.Actor != "admin-1" || rep.RanAt != "2026-09-19T12:00:00Z" {
		t.Errorf("report identity wrong: %+v", rep)
	}
	if len(rep.Checks) != 4 {
		t.Fatalf("checklist should have 4 checks, got %d", len(rep.Checks))
	}

	cases := []struct {
		name       string
		mutate     func(*QAFacts)
		failedName string
	}{
		{"no text", func(f *QAFacts) { f.HasText = false }, "text_present"},
		{"no ready render", func(f *QAFacts) { f.ReadyRenders = 0 }, "audio_ready"},
		{"render in flight", func(f *QAFacts) { f.RendersInFlight = 1 }, "no_render_in_flight"},
		{"unlicensed voice", func(f *QAFacts) { f.UnlicensedVoices = 2 }, "voices_licensed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			facts := passFacts
			tc.mutate(&facts)
			rep := RunQAChecklist(facts, "admin-1", "", now)
			if rep.Passed {
				t.Fatalf("broken fact should fail the gate")
			}
			found := false
			for _, c := range rep.Checks {
				if c.Name == tc.failedName {
					found = true
					if c.Passed {
						t.Errorf("check %s should be the one that failed", c.Name)
					}
					if c.Detail == "" {
						t.Errorf("a failed check without a reason is not actionable")
					}
				} else if !c.Passed {
					t.Errorf("check %s failed but its fact was healthy", c.Name)
				}
			}
			if !found {
				t.Errorf("expected a check named %s", tc.failedName)
			}
		})
	}
}
