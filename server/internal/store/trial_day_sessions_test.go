package store

import (
	"context"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

// A claim left unattached by a request that died must not block the day
// forever: once it is older than the build window, the next request owns it.
func TestStaleTrialDayClaimCanBeTakenOver(t *testing.T) {
	db := dbtest.New(t)
	defer db.Close()
	ctx := context.Background()
	users := NewUserStore(db)
	trials := NewTrialStore(db)
	userID := trialUser(t, users, "stale-claim@example.com")

	tr, err := trials.Start(ctx, userID, time.Now().UTC().Add(-time.Hour))
	if err != nil {
		t.Fatalf("start trial: %v", err)
	}
	claimedAt := time.Now().UTC().Add(-10 * time.Minute)
	if _, err := db.ExecContext(ctx,
		`INSERT INTO trial_day_sessions (id,trial_id,user_id,day,slot,created_at,updated_at) VALUES (?,?,?,?,?,?,?)`,
		newID(), tr.ID, userID, 2, 1, claimedAt.Format(time.RFC3339), claimedAt.Format(time.RFC3339)); err != nil {
		t.Fatalf("seed claim: %v", err)
	}

	now := time.Now().UTC()
	// Still inside the window: a build may be running, so hands off.
	if took, err := trials.TakeOverStaleDaySlot(ctx, userID, 2, 1, claimedAt.Add(-time.Minute), now); err != nil || took {
		t.Fatalf("fresh claim taken over: took=%v err=%v", took, err)
	}
	// Past the window: abandoned, so this caller takes it.
	if took, err := trials.TakeOverStaleDaySlot(ctx, userID, 2, 1, now.Add(-2*time.Minute), now); err != nil || !took {
		t.Fatalf("stale claim not taken over: took=%v err=%v", took, err)
	}
	// And a second caller in the same instant does not also get it.
	if took, _ := trials.TakeOverStaleDaySlot(ctx, userID, 2, 1, now.Add(-2*time.Minute), now); took {
		t.Fatal("two callers took over the same slot")
	}
}
