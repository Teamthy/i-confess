package store

import (
	"context"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
	"github.com/Teamthy/i-confess/internal/personalization"
)

// seedSignalSession writes a session with its items directly, in whatever
// status spelling the caller wants, so the store's normalisation and filtering
// can be checked against exactly the rows the engine writes.
func seedSignalSession(t *testing.T, d *db.DB, id, userID, status, startedAt string, actual int, deleted bool, items map[string]string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	var deletedAt any
	if deleted {
		deletedAt = now
	}
	if _, err := d.Exec(
		`INSERT INTO sessions (id,user_id,type,duration_seconds,target_duration,actual_duration,status,created_at,started_at,deleted_at)
		 VALUES ($1,$2,'standard',$3,$3,$4,$5,$6,NULLIF($7,''),$8)`,
		id, userID, actual, actual, status, now, startedAt, deletedAt); err != nil {
		t.Fatalf("insert session %s: %v", id, err)
	}
	pos := 0
	for confID, itemStatus := range items {
		pos++
		if _, err := d.Exec(
			`INSERT INTO session_items (id,session_id,confession_id,position,duration_seconds,status)
			 VALUES ($1,$2,$3,$4,60,$5)`,
			id+"-"+confID, id, confID, pos, itemStatus); err != nil {
			t.Fatalf("insert item: %v", err)
		}
	}
}

// TestSignalStoreReadsDecidedItemsOfLiveSessions is the store half of PHASE 43.
// The personalization rules are unit-tested on fixed evidence; this proves the
// evidence the store hands them is the right evidence.
func TestSignalStoreReadsDecidedItemsOfLiveSessions(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	seedConfession(t, d, "cat-1", "conf-1")
	now := time.Now().UTC()
	if _, err := d.Exec(
		`INSERT INTO confessions (id,category_id,title,intensity,language,status,created_at,updated_at)
		 VALUES ('conf-2','cat-1','Second',1,'en','published',$1,$1)`, now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	morning := time.Date(2026, 9, 21, 7, 0, 0, 0, time.UTC).Format(time.RFC3339)

	// Canonical spellings from the engine, one of each kind.
	seedSignalSession(t, d, "s-canon", "user-1", "COMPLETED", morning, 600, false, map[string]string{
		"conf-1": "COMPLETED", "conf-2": "SKIPPED",
	})
	// A second finished session: the same confession completed again is a
	// repeat; a queued item is undecided. (Migration 0006 folded the legacy
	// lowercase spellings onto this vocabulary, so the live CHECK admits
	// only canonical values; the read path still normalises defensively.)
	seedSignalSession(t, d, "s-second", "user-1", "COMPLETED", morning, 900, false, map[string]string{
		"conf-1": "COMPLETED", "conf-2": "QUEUED",
	})
	// Failed is the platform's fault and must not read as a skip; a playing
	// item is undecided. This session is not completed so its length is not
	// a completed duration either.
	seedSignalSession(t, d, "s-open", "user-1", "ACTIVE", morning, 1800, false, map[string]string{
		"conf-1": "FAILED", "conf-2": "PLAYING",
	})
	// A soft-deleted session is gone from history and gone from the signals.
	seedSignalSession(t, d, "s-deleted", "user-1", "COMPLETED", morning, 3600, true, map[string]string{
		"conf-2": "COMPLETED",
	})
	// Another listener's session is not this listener's evidence.
	if _, err := d.Exec(`INSERT INTO users (id,email,password_hash,created_at,updated_at) VALUES ('user-2','two@example.com','x',$1,$1)`,
		now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	seedSignalSession(t, d, "s-other", "user-2", "COMPLETED", morning, 600, false, map[string]string{
		"conf-2": "COMPLETED",
	})
	for _, f := range [][2]string{{"confession", "conf-1"}, {"category", "cat-1"}, {"voice", "voice-x"}} {
		if _, err := d.Exec(`INSERT INTO favorites (id,user_id,entity_type,entity_id,created_at) VALUES ($1,'user-1',$2,$3,$4)`,
			"fav-"+f[1], f[0], f[1], now.Format(time.RFC3339)); err != nil {
			t.Fatal(err)
		}
	}

	sig, err := NewSignalStore(d).Signals(ctx, "user-1", "Africa/Lagos")
	if err != nil {
		t.Fatalf("Signals: %v", err)
	}

	if got := len(sig.Listens); got != 3 {
		t.Fatalf("listens = %d, want 3 (completed, skipped, completed again); got %+v", got, sig.Listens)
	}
	completed, skipped := 0, 0
	for _, l := range sig.Listens {
		if l.CategoryID != "cat-1" {
			t.Errorf("listen %s has category %q, want cat-1 resolved through confessions", l.ConfessionID, l.CategoryID)
		}
		if l.StartedAt.IsZero() {
			t.Errorf("listen %s lost its session start", l.ConfessionID)
		}
		if l.Completed {
			completed++
		} else {
			skipped++
		}
	}
	if completed != 2 || skipped != 1 {
		t.Errorf("completed/skipped = %d/%d, want 2/1", completed, skipped)
	}
	if n := sig.RepeatListens()["conf-1"]; n != 2 {
		t.Errorf("conf-1 completed in two live sessions must be a repeat of 2, got %d", n)
	}
	if got := sig.CompletedDurations; len(got) != 2 || got[0]+got[1] != 1500 {
		t.Errorf("completed durations = %v, want the two finished sessions (600, 900)", got)
	}
	if !sig.Favourites.Confessions["conf-1"] || !sig.Favourites.Categories["cat-1"] || !sig.Favourites.Voices["voice-x"] {
		t.Errorf("favourites not split by kind: %+v", sig.Favourites)
	}
	if sig.Location == nil || sig.Location.String() != "Africa/Lagos" {
		t.Errorf("timezone not honoured: %v", sig.Location)
	}
	if dp, ok := sig.PreferredDaypart(); !ok || dp != personalization.Morning {
		t.Errorf("07:00 UTC is 08:00 in Lagos: preferred daypart = %s (%v), want morning", dp, ok)
	}

	// An unknown timezone degrades to UTC instead of failing the request.
	sig, err = NewSignalStore(d).Signals(ctx, "user-1", "Not/AZone")
	if err != nil {
		t.Fatalf("unknown tz must not fail: %v", err)
	}
	if sig.Location != nil {
		t.Errorf("unknown tz should leave Location nil (UTC), got %v", sig.Location)
	}

	// No evidence is an empty profile, not an error.
	empty, err := NewSignalStore(d).Signals(ctx, "nobody", "")
	if err != nil {
		t.Fatalf("empty: %v", err)
	}
	if len(empty.Present()) != 0 {
		t.Errorf("a listener with no history has no present signals, got %v", empty.Present())
	}
}
