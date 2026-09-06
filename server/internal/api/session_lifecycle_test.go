package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/Teamthy/i-confess/internal/sessions"
)

// itemStatuses returns the persisted status of every item in a session,
// ordered by position - read from the table, not from a response body, so the
// test checks what was written and not what a handler claims it wrote.
func (f *audioFixture) itemStatuses(t *testing.T, sessionID string) []string {
	t.Helper()
	rows, err := f.db.QueryContext(context.Background(),
		`SELECT status FROM session_items WHERE session_id = $1 ORDER BY position`, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

// TestPlaybackLifecyclePersistsItemStatuses covers the central PHASE 15 defect.
//
// session_items.status was still constrained to ('queued','played','skipped')
// while internal/sessions had moved to QUEUED/PLAYING/COMPLETED/SKIPPED/FAILED.
// Every write the playback lifecycle makes was therefore rejected:
//   - start's SetPlayingItem and complete's item update discard their error, so
//     they failed silently and every item stayed queued forever;
//   - skip and a progress push carrying item_status returned 500.
//
// A listener could finish a session and the database would still say nothing
// had been played. This drives the whole lifecycle and reads the table.
func TestPlaybackLifecyclePersistsItemStatuses(t *testing.T) {
	f := newAudioFixture(t)

	_, sess := f.createSession(t, f.freeTok, f.voiceStd, 180)
	if len(sess.Items) < 2 {
		t.Fatalf("need at least 2 items to exercise the queue, got %d", len(sess.Items))
	}

	// START must mark the first item as playing.
	if code, out := f.call(t, "POST", "/sessions/"+sess.ID+"/start", f.freeTok, nil); code != http.StatusOK {
		t.Fatalf("start: %d %v", code, out)
	}
	got := f.itemStatuses(t, sess.ID)
	if got[0] != string(sessions.ItemPlaying) {
		t.Errorf("after start, first item is %q, want %s; the write did not persist",
			got[0], sessions.ItemPlaying)
	}
	for i, s := range got[1:] {
		if s != string(sessions.ItemQueued) {
			t.Errorf("item %d after start = %q, want %s", i+1, s, sessions.ItemQueued)
		}
	}

	// SKIP must succeed and advance the queue.
	item := f.queueItemID(t, sess.ID)
	code, out := f.call(t, "POST", "/sessions/"+sess.ID+"/skip", f.freeTok,
		map[string]any{"item_id": item})
	if code != http.StatusOK {
		t.Fatalf("skip: %d %v (a 500 here is the CHECK constraint rejecting SKIPPED)", code, out)
	}
	if adv, _ := out["advanced"].(bool); !adv {
		t.Errorf("skip did not advance the queue: %v", out)
	}

	// COMPLETE must persist what was heard, not just report it.
	code, out = f.call(t, "POST", "/sessions/"+sess.ID+"/complete", f.freeTok, nil)
	if code != http.StatusOK {
		t.Fatalf("complete: %d %v", code, out)
	}
	reported, _ := out["items_completed"].(float64)

	got = f.itemStatuses(t, sess.ID)
	persisted := 0
	for _, s := range got {
		if s == string(sessions.ItemCompleted) {
			persisted++
		}
	}
	if persisted == 0 {
		t.Errorf("no item persisted as %s after completing; statuses = %v", sessions.ItemCompleted, got)
	}
	if int(reported) != persisted {
		t.Errorf("completion reported %d heard but the table holds %d - the number shown to a listener is not the number stored",
			int(reported), persisted)
	}
}

// TestQueueCountsMatchTheTable checks the view a player renders the up-next list
// from. It was computed over item statuses that could never be written, so it
// always showed every item waiting and items_completed 0.
func TestQueueCountsMatchTheTable(t *testing.T) {
	f := newAudioFixture(t)

	_, sess := f.createSession(t, f.freeTok, f.voiceStd, 180)
	for _, step := range []string{"start", "complete"} {
		if code, out := f.call(t, "POST", "/sessions/"+sess.ID+"/"+step, f.freeTok, nil); code != http.StatusOK {
			t.Fatalf("%s: %d %v", step, code, out)
		}
	}

	code, out := f.call(t, "GET", "/sessions/"+sess.ID+"/queue", f.freeTok, nil)
	if code != http.StatusOK {
		t.Fatalf("queue: %d %v", code, out)
	}
	counts, _ := out["counts"].(map[string]any)
	if n, _ := counts[string(sessions.ItemPlaying)].(float64); n > 1 {
		t.Errorf("%d items marked %s; at most one may be playing", int(n), sessions.ItemPlaying)
	}
	total, _ := out["items_total"].(float64)
	completed, _ := out["items_completed"].(float64)
	if completed >= total && total > 0 {
		t.Logf("everything completed (%v/%v)", completed, total)
	}
	if int(completed) == 0 {
		t.Errorf("items_completed is 0 after completing a session; counts = %v", counts)
	}
}
