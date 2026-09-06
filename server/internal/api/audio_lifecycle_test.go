package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Teamthy/i-confess/internal/audio"
)

// TestPulledAudioStopsServingInExistingSessions covers the PHASE 14 defect.
//
// audio_access.go passed the literal "ready" to the entitlement check instead
// of the asset's status, with a comment arguing the engine only selects ready
// assets so the gate was harmless. The engine does filter at selection — but a
// session queue is a snapshot, so an asset pulled after the session was built
// stays referenced forever. Asserting a constant means the gate cannot fail
// however the catalogue changes.
//
// Proven before the fix: archiving every asset left 2 of 2 items serving
// signed audio.
func TestPulledAudioStopsServingInExistingSessions(t *testing.T) {
	f := newAudioFixture(t)
	ctx := context.Background()

	code, sess := f.createSession(t, f.freeTok, f.voiceStd, 120)
	if code != http.StatusCreated {
		t.Fatalf("create session: %d", code)
	}
	if served := countServed(sess); served == 0 {
		t.Fatal("baseline served no audio at all, so this test proves nothing")
	}

	// Every way an asset gets withdrawn lands in one of these: a human rejects
	// the render, rights are revoked, the file is found to be corrupt, or it is
	// retired. None of them may keep playing into a session built earlier.
	for _, withdrawn := range []audio.AssetStatus{
		audio.StatusArchived, audio.StatusQARejected, audio.StatusFailed, audio.StatusProcessing,
	} {
		t.Run(string(withdrawn), func(t *testing.T) {
			if _, err := f.db.ExecContext(ctx, `UPDATE audio_assets SET status=$1`, string(withdrawn)); err != nil {
				t.Fatal(err)
			}

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/sessions/"+sess.ID, nil)
			req.Header.Set("Authorization", "Bearer "+f.freeTok)
			f.router.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("read session: %d", rec.Code)
			}

			var after sessionResp
			if err := json.Unmarshal(rec.Body.Bytes(), &after); err != nil {
				t.Fatal(err)
			}
			if n := countServed(after); n > 0 {
				t.Errorf("%d of %d items still served signed audio after their assets became %q",
					n, len(after.Items), withdrawn)
			}
			// The listener must be told why, not handed silence.
			for _, it := range after.Items {
				if !it.Locked || it.LockReason == "" {
					t.Errorf("withdrawn item was not marked locked with a reason: %+v", it)
				}
			}

			// Restore so the next subtest starts from a serving catalogue.
			if _, err := f.db.ExecContext(ctx, `UPDATE audio_assets SET status='ready'`); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestPublishedAudioIsServed closes the other half of the disagreement.
//
// The database CHECK allowed 'published', but selection asked for exactly
// 'ready' and the entitlement gate asked for exactly 'ready'. So an asset the
// schema was happy to store could never be selected and could never be played.
func TestPublishedAudioIsServed(t *testing.T) {
	f := newAudioFixture(t)
	ctx := context.Background()

	if _, err := f.db.ExecContext(ctx, `UPDATE audio_assets SET status='published'`); err != nil {
		t.Fatal(err)
	}

	code, sess := f.createSession(t, f.freeTok, f.voiceStd, 120)
	if code != http.StatusCreated {
		t.Fatalf("create session: %d", code)
	}
	if n := countServed(sess); n == 0 {
		t.Error("no audio served for 'published' assets, which the schema permits and QA has passed")
	}
}

func countServed(s sessionResp) int {
	n := 0
	for _, it := range s.Items {
		if it.AudioURL != "" {
			n++
		}
	}
	return n
}
