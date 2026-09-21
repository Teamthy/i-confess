package store

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/billing"
	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

// The store half of §36 (G-3): CAS writes, a clock that the read path itself
// catches up, and a one-per-account guarantee the database enforces. These
// run against PostgreSQL for the same reason every store test here does:
// the constraint, the race and the uniqueness are the product.

var trialBase = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

func newTrialsStore(t *testing.T) (*TrialsStore, *db.DB) {
	t.Helper()
	conn := dbtest.New(t)
	t.Cleanup(func() { conn.Close() })
	seedUser(t, conn, "tr-u1", "tr1@test.dev")
	return NewTrialsStore(conn), conn
}

func mustTrial(t *testing.T, s *TrialsStore, userID string, now time.Time) billing.TrialStatus {
	t.Helper()
	tr, err := s.Current(context.Background(), userID, now)
	if err != nil {
		t.Fatalf("Current(%s): %v", userID, err)
	}
	return billing.TrialStatus(tr.Status)
}

func TestTrialFullWalk(t *testing.T) {
	s, _ := newTrialsStore(t)
	ctx := context.Background()

	// First read creates the eligible row: the offer stands, no clock.
	tr, err := s.Current(ctx, "tr-u1", trialBase)
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	if tr.Status != string(billing.TrialEligible) || tr.Day != 0 || tr.StartedAt != "" {
		t.Fatalf("first read = %+v, want a clock-free eligible row", tr)
	}

	// The claim sets the seven-day clock in the same write as the move.
	tr, err = s.Start(ctx, "tr-u1", trialBase)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if tr.StartedAt != "2026-10-01T00:00:00Z" {
		t.Errorf("started_at = %q", tr.StartedAt)
	}
	end := trialBase.Add(billing.TrialDuration)
	if want := end.UTC().Format(time.RFC3339); tr.EndsAt != want {
		t.Errorf("ends_at = %q, want %q", tr.EndsAt, want)
	}

	// Any read promotes a running clock: started→active is not an event, it
	// is what "the clock started" means.
	tr, err = s.Current(ctx, "tr-u1", trialBase.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if tr.Status != string(billing.TrialActive) || tr.Day != 1 {
		t.Errorf("one hour in: %+v, want active day 1", tr)
	}

	// Day math follows the record, not the wall: four days in is day 4.
	if tr, err = s.Current(ctx, "tr-u1", trialBase.Add(97*time.Hour)); err != nil {
		t.Fatal(err)
	} else if tr.Day != 5 {
		t.Errorf("day 97h in = %d, want 5", tr.Day)
	}

	// Inside the last day, ACTIVE becomes EXPIRING on its own.
	tr, err = s.Current(ctx, "tr-u1", end.Add(-12*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if tr.Status != string(billing.TrialExpiring) || tr.Day != 7 {
		t.Errorf("12h before the end: %+v, want expiring day 7", tr)
	}

	// Past the wire: EXPIRED, day 0 — and it stays 0.
	if tr, err = s.Current(ctx, "tr-u1", end.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if tr.Status != string(billing.TrialExpired) || tr.Day != 0 {
		t.Errorf("after the end: %+v, want expired day 0", tr)
	}

	// A late purchase still converts. The clock cannot be re-armed, but the
	// record may close as converted.
	tr, err = s.Convert(ctx, "tr-u1", end.Add(time.Hour))
	if err != nil {
		t.Fatalf("late convert: %v", err)
	}
	if tr.Status != string(billing.TrialConverted) || tr.ConvertedAt == "" {
		t.Errorf("converted: %+v", tr)
	}

	// Converted is terminal: even the sweep does not touch it.
	if moved, err := s.Sweep(ctx, end.Add(72*time.Hour)); err != nil || moved != 0 {
		t.Errorf("sweep after conversion moved %d rows, err %v", moved, err)
	}
	if got := mustTrial(t, s, "tr-u1", end.Add(96*time.Hour)); got != billing.TrialConverted {
		t.Errorf("settled state = %s", got)
	}
}

func TestTrialClaimIsOncePerAccount(t *testing.T) {
	s, _ := newTrialsStore(t)
	ctx := context.Background()

	if _, err := s.Start(ctx, "tr-u1", trialBase); err != nil {
		t.Fatalf("start: %v", err)
	}
	// A second claim — same minute, or after the whole trial ran out — is a
	// refusal naming the state found and the moves allowed from it. This is
	// the sentence G-3 existed to make possible.
	_, err := s.Start(ctx, "tr-u1", trialBase.Add(time.Second))
	var trans *billing.TrialTransitionError
	if !errors.As(err, &trans) {
		t.Fatalf("second start: err = %v, want a transition refusal", err)
	}
	if trans.From != billing.TrialStarted && trans.From != billing.TrialActive {
		t.Errorf("refusal from %q; the race window may have moved it, but not past started→active", trans.From)
	}

	// Expired does not re-arm, ever.
	if _, err := s.Expire(ctx, "tr-u1", trialBase); err != nil {
		// Expire from started is on the graph; from active it is too.
		t.Fatalf("expire: %v", err)
	}
	if _, err := s.Start(ctx, "tr-u1", trialBase.Add(time.Hour)); err == nil {
		t.Fatal("a trial re-started after expiring; §36 grants one offer per account")
	}
}

func TestConvertNeverFromEligible(t *testing.T) {
	s, _ := newTrialsStore(t)
	ctx := context.Background()
	if _, err := s.Current(ctx, "tr-u1", trialBase); err != nil {
		t.Fatal(err)
	}
	// Buying without ever claiming does not consume the offer — but it must
	// not mark the trial converted either: nothing converted.
	_, err := s.Convert(ctx, "tr-u1", trialBase)
	var trans *billing.TrialTransitionError
	if !errors.As(err, &trans) || trans.From != billing.TrialEligible {
		t.Fatalf("convert from eligible: %v, want a refusal naming eligible", err)
	}
	if got := mustTrial(t, s, "tr-u1", trialBase); got != billing.TrialEligible {
		t.Errorf("state after refused convert = %s", got)
	}
	// Re-converting an already converted trial is idempotent, not an error:
	// receipt verification runs repeatedly (restore, renewal webhooks).
	if _, err := s.Start(ctx, "tr-u1", trialBase); err != nil {
		t.Fatal(err)
	}
	tr, err := s.Convert(ctx, "tr-u1", trialBase)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Convert(ctx, "tr-u1", trialBase.Add(time.Minute)); err != nil {
		t.Errorf("second convert: %v, want idempotent", err)
	}
	if tr.ConvertedAt == "" {
		t.Error("converted_at not stamped")
	}
}

func TestTrialSweepMovesWholeRows(t *testing.T) {
	s, conn := newTrialsStore(t)
	ctx := context.Background()
	now := trialBase

	// Three accounts at three clock positions, no reads — the sweep must
	// settle what the laziness of Current would otherwise defer.
	rows := []struct {
		user, status, started, ends string
	}{
		{"tr-s1", "started", now.Add(-time.Hour).UTC().Format(time.RFC3339), now.Add(48 * time.Hour).UTC().Format(time.RFC3339)},    // -> active
		{"tr-s2", "active", now.Add(-72 * time.Hour).UTC().Format(time.RFC3339), now.Add(6 * time.Hour).UTC().Format(time.RFC3339)}, // -> expiring
		{"tr-s3", "active", now.Add(-80 * time.Hour).UTC().Format(time.RFC3339), now.Add(-time.Minute).UTC().Format(time.RFC3339)},  // -> expired
	}
	for _, u := range []struct{ id, email string }{{"tr-s1", "s1@t.dev"}, {"tr-s2", "s2@t.dev"}, {"tr-s3", "s3@t.dev"}} {
		seedUser(t, conn, u.id, u.email)
	}
	for i, r := range rows {
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO trials (id,user_id,status,started_at,ends_at,created_at,updated_at)
			 VALUES (?,?,?,?,?,?,?)`,
			"tr-row-"+string(rune('a'+i)), r.user, r.status, r.started, r.ends,
			now.UTC().Format(time.RFC3339), now.UTC().Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}

	moved, err := s.Sweep(ctx, now)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if moved != 3 {
		t.Errorf("sweep moved %d rows, want 3", moved)
	}
	for user, want := range map[string]billing.TrialStatus{
		"tr-s1": billing.TrialActive,
		"tr-s2": billing.TrialExpiring,
		"tr-s3": billing.TrialExpired,
	} {
		if got := mustTrial(t, s, user, now); got != want {
			t.Errorf("swept %s = %s, want %s", user, got, want)
		}
	}
	// And again: the sweep is a fixpoint, not an event generator.
	if moved, err := s.Sweep(ctx, now); err != nil || moved != 0 {
		t.Errorf("second sweep moved %d, want 0 (err %v)", moved, err)
	}
}

func TestTrialSweepExpiresStartedClocksThatAlreadyRan(t *testing.T) {
	s, conn := newTrialsStore(t)
	ctx := context.Background()
	now := trialBase
	seedUser(t, conn, "tr-s4", "s4@t.dev")
	// Claimed, but nothing read it before the end: it must NOT get a free
	// day of ACTIVE. Expire-first ordering in Sweep is what this pins.
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO trials (id,user_id,status,started_at,ends_at,created_at,updated_at)
		 VALUES ('tr-row-d','tr-s4','started',?,?,?,?)`,
		now.Add(-80*time.Hour).UTC().Format(time.RFC3339),
		now.Add(-time.Hour).UTC().Format(time.RFC3339),
		now.UTC().Format(time.RFC3339), now.UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if moved, err := s.Sweep(ctx, now); err != nil || moved != 1 {
		t.Fatalf("sweep: moved %d err %v", moved, err)
	}
	if got := mustTrial(t, s, "tr-s4", now); got != billing.TrialExpired {
		t.Errorf("state = %s, want expired (never an unearned active day)", got)
	}
}

func TestSetStatusCannotJumpTheGraph(t *testing.T) {
	s, _ := newTrialsStore(t)
	ctx := context.Background()
	if _, err := s.Start(ctx, "tr-u1", trialBase); err != nil {
		t.Fatal(err)
	}
	// eligible is behind it now; the PATCH path (same store method) refuses.
	if _, err := s.SetStatus(ctx, "tr-u1", billing.TrialEligible, trialBase); err == nil {
		t.Fatal("backwards SetStatus accepted")
	}
	// Forward is fine — this is the hook for a worker or a test that refuses
	// to sleep for seven days.
	tr, err := s.SetStatus(ctx, "tr-u1", billing.TrialExpiring, trialBase)
	if err != nil {
		t.Fatalf("forward SetStatus: %v", err)
	}
	if tr.Status != string(billing.TrialExpiring) {
		t.Errorf("status = %s", tr.Status)
	}
}

func TestTrialsUniquePerUser(t *testing.T) {
	s, conn := newTrialsStore(t)
	ctx := context.Background()
	if _, err := s.Current(ctx, "tr-u1", trialBase); err != nil {
		t.Fatal(err)
	}
	_, err := conn.ExecContext(ctx,
		`INSERT INTO trials (id,user_id,status,created_at,updated_at)
		 VALUES ('second-row','tr-u1','eligible',?,?)`,
		trialBase.Format(time.RFC3339), trialBase.Format(time.RFC3339))
	if err == nil {
		t.Fatal("a second trial row for one user was accepted; one account, one clock")
	}
}

// TestTrialVocabularyParityAgainstConstraint: the same contract internal/
// content and internal/moderation keep. The CHECK in 0014 and the constants
// in billing must be one vocabulary, verified against the live database.
func TestTrialVocabularyParityAgainstConstraint(t *testing.T) {
	_, conn := newTrialsStore(t)
	var def string
	if err := conn.QueryRowContext(context.Background(),
		`SELECT pg_get_constraintdef(oid) FROM pg_constraint
		 WHERE conname = 'trials_status_check'`).Scan(&def); err != nil {
		t.Fatalf("read the trials status CHECK: %v", err)
	}
	re := regexp.MustCompile(`'([a-z]+)'`)
	var dbStates []string
	for _, m := range re.FindAllStringSubmatch(def, -1) {
		dbStates = append(dbStates, m[1])
	}
	want := make([]string, 0, 6)
	for _, s := range billing.TrialStatuses() {
		want = append(want, string(s))
	}
	sort.Strings(dbStates)
	sort.Strings(want)
	if len(dbStates) != len(want) {
		t.Fatalf("constraint allows %v, Go vocabulary is %v", dbStates, want)
	}
	for i := range dbStates {
		if dbStates[i] != want[i] {
			t.Fatalf("constraint allows %v, Go vocabulary is %v", dbStates, want)
		}
	}
}

// clockStep is pure; its table test lives here because the promotion rules
// are what the read path and Sweep share.
func TestClockStepTable(t *testing.T) {
	start := trialBase
	end := start.Add(billing.TrialDuration)
	cases := []struct {
		status billing.TrialStatus
		now    time.Time
		want   billing.TrialStatus
		move   bool
		why    string
	}{
		{billing.TrialEligible, end, "", false, "eligible has no clock to chase"},
		{billing.TrialStarted, start.Add(time.Minute), billing.TrialActive, true, "a running clock is active"},
		{billing.TrialStarted, end.Add(time.Second), billing.TrialExpired, true, "run out before first read: no unearned day"},
		{billing.TrialActive, start.Add(time.Hour), "", false, "mid-journey"},
		{billing.TrialActive, end.Add(-billing.TrialExpiringWindow), billing.TrialExpiring, true, "the window includes its first instant"},
		{billing.TrialActive, end.Add(-time.Second), billing.TrialExpiring, true, "one second out"},
		{billing.TrialActive, end, billing.TrialExpired, true, "at the wire"},
		{billing.TrialExpiring, end.Add(-time.Hour), "", false, "still inside the window"},
		{billing.TrialExpiring, end, billing.TrialExpired, true, "the warning becomes the end"},
		{billing.TrialExpired, end.Add(time.Hour), "", false, "expired owes the clock nothing"},
		{billing.TrialConverted, end.Add(time.Hour), "", false, "terminal"},
	}
	for _, c := range cases {
		got, move := clockStep(c.status, start, end, c.now)
		if got != c.want || move != c.move {
			t.Errorf("clockStep(%s, now=%v) = (%q,%v), want (%q,%v) — %s",
				c.status, c.now, got, move, c.want, c.move, c.why)
		}
	}
}
