package db_test

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
	"testing"

	"github.com/Teamthy/i-confess/internal/audio"
	"github.com/Teamthy/i-confess/internal/community"
	"github.com/Teamthy/i-confess/internal/content"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
	"github.com/Teamthy/i-confess/internal/jobs"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/moderation"
	"github.com/Teamthy/i-confess/internal/sessions"
	trialdomain "github.com/Teamthy/i-confess/internal/trial"
)

// TestEveryStatusColumnIsConstrained closes G-2.
//
// Twenty-three tables carry a status column. For most of them the permitted
// values existed only in a trailing SQL comment, which PostgreSQL does not
// enforce: a typo wrote an invalid state and nothing noticed until a query
// filtered on it and quietly returned no rows.
func TestEveryStatusColumnIsConstrained(t *testing.T) {
	raw := dbtest.Raw(t)
	ctx := context.Background()

	// Every column literally named "status".
	rows, err := raw.QueryContext(ctx, `
		SELECT c.table_name
		FROM information_schema.columns c
		JOIN information_schema.tables t
		  ON t.table_name = c.table_name AND t.table_schema = c.table_schema
		WHERE c.table_schema = 'public'
		  AND t.table_type = 'BASE TABLE'
		  AND c.column_name = 'status'
		ORDER BY c.table_name`)
	if err != nil {
		t.Fatalf("list status columns: %v", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate: %v", err)
	}
	if len(tables) == 0 {
		t.Fatal("no status columns found - this test would pass against an empty schema")
	}

	// Tables with a CHECK constraint that mentions "status".
	constrained := map[string]bool{}
	crows, err := raw.QueryContext(ctx, `
		SELECT con.conrelid::regclass::text
		FROM pg_constraint con
		JOIN pg_attribute att
		  ON att.attrelid = con.conrelid AND att.attnum = ANY (con.conkey)
		WHERE con.contype = 'c' AND att.attname = 'status'`)
	if err != nil {
		t.Fatalf("list check constraints: %v", err)
	}
	defer crows.Close()
	for crows.Next() {
		var name string
		if err := crows.Scan(&name); err != nil {
			t.Fatalf("scan constraint: %v", err)
		}
		constrained[strings.TrimSuffix(name, "_check")] = true
		constrained[name] = true
	}
	if err := crows.Err(); err != nil {
		t.Fatalf("iterate constraints: %v", err)
	}

	for _, tbl := range tables {
		if !constrained[tbl] && !constrained[tbl+"_status_check"] {
			t.Errorf("table %q has a status column with no CHECK constraint", tbl)
		}
	}
	t.Logf("%d status columns, all constrained", len(tables))
}

// TestDatabaseVocabularyMatchesGoConstants is the check that would have caught
// the error this migration was written with.
//
// The first draft of 0002 derived each vocabulary from string literals appearing
// in SQL. That misses every value held in a Go constant and passed as $1, which
// is how the store layer is written. The constraint for community_posts allowed
// four of the seven statuses the package declares, and two tests failed on
// insert. Comparing the constraint against the constants makes that a permanent
// failure rather than a lucky catch.
func TestDatabaseVocabularyMatchesGoConstants(t *testing.T) {
	raw := dbtest.Raw(t)
	ctx := context.Background()

	cases := []struct {
		table    string
		expected []string
	}{
		{"collections", []string{"draft", "published", "archived"}},
		{"categories", []string{"draft", "published", "archived", "pending_deletion", "deleted"}},
		{"confessions", contentStatusStrings()},
		{"voices", []string{"active", "inactive", "archived", "pending_deletion", "deleted"}},
		{"content_versions", []string{"draft", "approved", "published", "archived"}},
		{"audio_assets", assetStatusStrings()},
		{"audio_generation_jobs", jobStatusStrings()},
		{"audio_variants", []string{"processing", "ready", "failed"}},
		{"audio_processing_logs", []string{"started", "completed", "failed"}},
		{"voice_rights", []string{"pending", "active", "expired", "revoked"}},
		{"audio_playback_sessions", []string{"playing", "paused", "completed", "abandoned"}},
		{"audio_downloads", []string{"queued", "downloading", "downloaded", "failed", "removed"}},
		{"users", []string{"active", "suspended", "pending_deletion", "deleted"}},
		{"subscriptions", []string{
			models.SubscriptionActive, models.SubscriptionTrial, models.SubscriptionGrace,
			models.SubscriptionCancelled, models.SubscriptionExpired, models.SubscriptionRefunded,
			models.SubscriptionSuspended,
		}},
		{"sessions", []string{
			string(sessions.Draft), string(sessions.Ready), string(sessions.Scheduled),
			string(sessions.Starting), string(sessions.Active), string(sessions.Paused),
			string(sessions.Interrupted), string(sessions.Completed), string(sessions.Cancelled),
			string(sessions.Expired), string(sessions.Failed),
		}},
		{"session_items", []string{"QUEUED", "PLAYING", "COMPLETED", "SKIPPED", "FAILED"}},
		{"user_confessions", moderation.UGCStatuses()},
		{"user_confession_audio", []string{"queued", "processing", "ready", "failed"}},
		{"jobs", []string{
			jobs.StatusQueued, jobs.StatusRunning, jobs.StatusCompleted, jobs.StatusFailed,
			jobs.StatusDeadLetter,
		}},
		{"scheduled_deliveries", []string{"sent", "queued", "failed", "skipped"}},
		{"moderation_cases", moderation.CaseStatuses()},
		{"reports", moderation.ReportStatuses()},
		{"community_posts", []string{
			community.StatusDraft, community.StatusSubmitted, community.StatusUnderReview,
			community.StatusApproved, community.StatusRejected, community.StatusPublished,
			community.StatusArchived,
		}},
	}

	for _, tc := range cases {
		def, err := checkDefinition(ctx, raw, tc.table, "status")
		if err != nil {
			t.Fatalf("%s: %v", tc.table, err)
		}
		if def == "" {
			t.Errorf("%s: no CHECK constraint on status", tc.table)
			continue
		}

		// pg_get_constraintdef renders each value as 'draft'::text, so a naive
		// split on the quote character yields "::text," as a value.
		got := map[string]bool{}
		for _, m := range valueRe.FindAllStringSubmatch(def, -1) {
			got[m[1]] = true
		}
		if len(got) == 0 {
			t.Errorf("%s: parsed no values out of constraint %q", tc.table, def)
			continue
		}

		want := map[string]bool{}
		for _, v := range tc.expected {
			want[v] = true
			if !got[v] {
				t.Errorf("%s: Go declares status %q but the database constraint rejects it", tc.table, v)
			}
		}
		for v := range got {
			if !want[v] {
				t.Errorf("%s: database allows status %q which no Go constant declares", tc.table, v)
			}
		}
	}
}

var valueRe = regexp.MustCompile(`'([^']*)'(?:::\w+)?`)

// TestTheologicalReviewVocabularyParityAgainstConstraint protects the
// canonical-review gate introduced in PHASE 40. Like the lifecycle tests, it
// reads the installed PostgreSQL CHECK rather than migration text.
func TestTheologicalReviewVocabularyParityAgainstConstraint(t *testing.T) {
	raw := dbtest.Raw(t)
	def, err := checkDefinition(context.Background(), raw, "confessions", "theological_review_status")
	if err != nil {
		t.Fatalf("confessions.theological_review_status: %v", err)
	}
	want := map[string]bool{"unreviewed": true, "reviewed": true, "needs_revision": true}
	got := map[string]bool{}
	for _, match := range valueRe.FindAllStringSubmatch(def, -1) {
		got[match[1]] = true
	}
	for value := range want {
		if !got[value] {
			t.Errorf("theology code accepts %q but live CHECK rejects it", value)
		}
	}
	for value := range got {
		if !want[value] {
			t.Errorf("live theology CHECK accepts %q but code does not declare it", value)
		}
	}
}

// TestTrialVocabularyParityAgainstConstraint is the Phase 36 pattern used by
// content in TestContentVocabularyParityAgainstConstraint: compare the Go
// vocabulary with the CHECK installed in the live PostgreSQL test database.
func TestTrialVocabularyParityAgainstConstraint(t *testing.T) {
	raw := dbtest.Raw(t)
	def, err := checkDefinition(context.Background(), raw, "trials", "state")
	if err != nil {
		t.Fatalf("trials.state: %v", err)
	}
	got := map[string]bool{}
	for _, match := range valueRe.FindAllStringSubmatch(def, -1) {
		got[match[1]] = true
	}
	want := map[string]bool{}
	for _, state := range trialdomain.All() {
		want[string(state)] = true
	}
	for value := range want {
		if !got[value] {
			t.Errorf("trial code accepts %q but the live CHECK rejects it", value)
		}
	}
	for value := range got {
		if !want[value] {
			t.Errorf("trials.state CHECK accepts %q but trial code does not declare it", value)
		}
	}
}

func contentStatusStrings() []string {
	out := make([]string, 0, len(content.All()))
	for _, status := range content.All() {
		out = append(out, string(status))
	}
	return out
}

func assetStatusStrings() []string {
	out := make([]string, 0, len(audio.All()))
	for _, status := range audio.All() {
		out = append(out, string(status))
	}
	return out
}

func jobStatusStrings() []string {
	out := make([]string, 0, len(audio.AllJobStatuses()))
	for _, status := range audio.AllJobStatuses() {
		out = append(out, string(status))
	}
	return out
}

func checkDefinition(ctx context.Context, raw *sql.DB, table, column string) (string, error) {
	var def sql.NullString
	err := raw.QueryRowContext(ctx, `
		SELECT pg_get_constraintdef(con.oid)
		FROM pg_constraint con
		JOIN pg_attribute att
		  ON att.attrelid = con.conrelid AND att.attnum = ANY (con.conkey)
		WHERE con.conrelid = $1::regclass AND con.contype = 'c' AND att.attname = $2
		LIMIT 1`, table, column).Scan(&def)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return def.String, nil
}
