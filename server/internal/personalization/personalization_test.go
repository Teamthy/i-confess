package personalization

import (
	"reflect"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/models"
)

// A small catalogue: three categories, two confessions each, in catalogue
// order. Nothing about it favours any category, so whatever the ranking does
// is the signals' doing.
func catalogue() ([]models.Category, map[string][]models.Confession) {
	cats := []models.Category{
		{ID: "cat-healing", Name: "Healing", SortOrder: 1},
		{ID: "cat-peace", Name: "Peace", SortOrder: 2},
		{ID: "cat-purpose", Name: "Purpose", SortOrder: 3},
	}
	confs := map[string][]models.Confession{}
	for _, c := range cats {
		confs[c.ID] = []models.Confession{
			{ID: c.ID + "-1", CategoryID: c.ID, Title: c.Name + " one"},
			{ID: c.ID + "-2", CategoryID: c.ID, Title: c.Name + " two"},
		}
	}
	return cats, confs
}

func at(hour int) time.Time {
	return time.Date(2026, 9, 21, hour, 0, 0, 0, time.UTC)
}

func rankWith(sig Signals, now time.Time) Recommendation {
	cats, confs := catalogue()
	return Rank(Input{Categories: cats, Confessions: confs, Signals: sig, Now: now})
}

func topCategory(r Recommendation) string { return r.Categories[0].Category.ID }

// TestSevenListenerSignalsAreTheProductContract is the phase's central claim.
//
// Master-plan item 34 names seven signals. The earlier draft of this test
// asserted seven names while the implementation carried repeat listening as a
// yes/no flag, which made the count true and the contract false. This version
// checks the thing that matters: each signal is a real quantity derived from
// evidence, and each one alone changes the ranking against the same
// catalogue. A signal that cannot move the result is decoration.
func TestSevenListenerSignalsAreTheProductContract(t *testing.T) {
	want := []Signal{
		"categories_listened_to", "completion_rate", "time_of_day",
		"session_duration", "favourites", "skips", "repeat_listening",
	}
	if got := All(); !reflect.DeepEqual(got, want) {
		t.Fatalf("All() = %v, want the seven master-plan signals %v", got, want)
	}

	noon := at(12)
	baseline := rankWith(Signals{}, noon)
	if topCategory(baseline) != "cat-healing" {
		t.Fatalf("with no evidence the catalogue order must win; got %s", topCategory(baseline))
	}
	if baseline.SuggestedDurationSeconds != 0 {
		t.Fatalf("no completed session must leave the suggestion at 0, got %d", baseline.SuggestedDurationSeconds)
	}
	if len(baseline.Present) != 0 {
		t.Fatalf("no evidence must report no present signals, got %v", baseline.Present)
	}

	// 1. Categories listened to: three completed listens in Purpose lift it
	// above the catalogue's first category.
	listened := Signals{Listens: []Listen{
		{SessionID: "s1", ConfessionID: "cat-purpose-1", CategoryID: "cat-purpose", Completed: true},
		{SessionID: "s2", ConfessionID: "cat-purpose-2", CategoryID: "cat-purpose", Completed: true},
		{SessionID: "s3", ConfessionID: "cat-purpose-1", CategoryID: "cat-purpose", Completed: true},
	}}
	if got := rankWith(listened, noon); topCategory(got) != "cat-purpose" {
		t.Errorf("signal 1 (categories listened to) did not move the ranking: top=%s", topCategory(got))
	}
	if n := listened.CategoriesListenedTo()["cat-purpose"]; n != 3 {
		t.Errorf("signal 1 must be a count, got %d", n)
	}

	// 2. Completion rate: with the same typical duration, a listener who
	// abandons most items is offered a shorter session than one who finishes.
	finisher := Signals{CompletedDurations: []int{30 * 60}, Listens: []Listen{
		{SessionID: "s1", ConfessionID: "a", CategoryID: "cat-peace", Completed: true},
		{SessionID: "s1", ConfessionID: "b", CategoryID: "cat-peace", Completed: true},
		{SessionID: "s1", ConfessionID: "c", CategoryID: "cat-peace", Completed: true},
		{SessionID: "s1", ConfessionID: "d", CategoryID: "cat-peace", Completed: true},
	}}
	abandoner := Signals{CompletedDurations: []int{30 * 60}, Listens: []Listen{
		{SessionID: "s1", ConfessionID: "a", CategoryID: "cat-peace", Completed: true},
		{SessionID: "s1", ConfessionID: "b", CategoryID: "cat-peace", Completed: false},
		{SessionID: "s1", ConfessionID: "c", CategoryID: "cat-peace", Completed: false},
		{SessionID: "s1", ConfessionID: "d", CategoryID: "cat-peace", Completed: false},
	}}
	fin, ab := rankWith(finisher, noon), rankWith(abandoner, noon)
	if ab.SuggestedDurationSeconds >= 30*60 || fin.SuggestedDurationSeconds <= 30*60 {
		t.Errorf("signal 2 (completion rate) did not move the suggested duration: abandoner=%d finisher=%d",
			ab.SuggestedDurationSeconds, fin.SuggestedDurationSeconds)
	}
	if rate, n := abandoner.CompletionRate(); rate != 0.25 || n != 4 {
		t.Errorf("signal 2 must be a rate over a sample, got %v/%d", rate, n)
	}

	// 3. Time of day: Peace was heard in the mornings. At 07:00 it leads; at
	// 21:00, with identical evidence, it does not outrank a category with the
	// same number of listens heard at night.
	timed := Signals{Listens: []Listen{
		{SessionID: "s1", ConfessionID: "cat-peace-1", CategoryID: "cat-peace", Completed: true, StartedAt: at(7)},
		{SessionID: "s2", ConfessionID: "cat-purpose-1", CategoryID: "cat-purpose", Completed: true, StartedAt: at(22)},
	}}
	if got := rankWith(timed, at(7)); topCategory(got) != "cat-peace" {
		t.Errorf("signal 3 (time of day) morning request should lead with the morning category, got %s", topCategory(got))
	}
	if got := rankWith(timed, at(22)); topCategory(got) != "cat-purpose" {
		t.Errorf("signal 3 (time of day) night request should lead with the night category, got %s", topCategory(got))
	}

	// 4. Session duration: the typical finished length sets the suggestion.
	long := Signals{CompletedDurations: []int{55 * 60, 60 * 60, 65 * 60}}
	if got := rankWith(long, noon).SuggestedDurationSeconds; got != 60*60 {
		t.Errorf("signal 4 (session duration) median 60m should suggest 60m, got %d", got)
	}
	if long.TypicalDuration() != 60*60 {
		t.Errorf("signal 4 must be a duration, got %d", long.TypicalDuration())
	}

	// 5. Favourites: a favourite category leads, and a favourite confession
	// leads inside its category.
	fav := Signals{Favourites: Favourites{
		Categories:  map[string]bool{"cat-purpose": true},
		Confessions: map[string]bool{"cat-purpose-2": true},
	}}
	got := rankWith(fav, noon)
	if topCategory(got) != "cat-purpose" {
		t.Errorf("signal 5 (favourites) category did not lead: %s", topCategory(got))
	}
	if got.Confessions[0].Confession.ID != "cat-purpose-2" {
		t.Errorf("signal 5 (favourites) confession did not lead its category: %s", got.Confessions[0].Confession.ID)
	}

	// 6. Skips: a skipped category sinks below the catalogue order, and a
	// skipped confession yields to its sibling.
	skipped := Signals{Listens: []Listen{
		{SessionID: "s1", ConfessionID: "cat-healing-1", CategoryID: "cat-healing", Completed: false},
	}}
	got = rankWith(skipped, noon)
	if topCategory(got) == "cat-healing" {
		t.Errorf("signal 6 (skips) did not sink the skipped category")
	}
	for _, rc := range got.Confessions {
		if rc.Confession.CategoryID == "cat-healing" {
			if rc.Confession.ID != "cat-healing-2" {
				t.Errorf("signal 6 (skips) the skipped confession should yield to its sibling, got %s first", rc.Confession.ID)
			}
			break
		}
	}
	if n := skipped.SkipsByConfession()["cat-healing-1"]; n != 1 {
		t.Errorf("signal 6 must be a count, got %d", n)
	}

	// 7. Repeat listening: the same confession completed in three separate
	// sessions is a count of three, not a flag; it lifts its category, leads
	// its category, and heads the listen-again rail. One listen is not a
	// repeat, and the same confession twice in one session is still one.
	repeat := Signals{Listens: []Listen{
		{SessionID: "s1", ConfessionID: "cat-peace-2", CategoryID: "cat-peace", Completed: true},
		{SessionID: "s2", ConfessionID: "cat-peace-2", CategoryID: "cat-peace", Completed: true},
		{SessionID: "s3", ConfessionID: "cat-peace-2", CategoryID: "cat-peace", Completed: true},
		{SessionID: "s3", ConfessionID: "cat-peace-2", CategoryID: "cat-peace", Completed: true},
		{SessionID: "s4", ConfessionID: "cat-purpose-1", CategoryID: "cat-purpose", Completed: true},
	}}
	if n := repeat.RepeatListens()["cat-peace-2"]; n != 3 {
		t.Errorf("signal 7 must count distinct sessions: got %d, want 3", n)
	}
	if _, ok := repeat.RepeatListens()["cat-purpose-1"]; ok {
		t.Errorf("signal 7: a single listen is not a repeat")
	}
	got = rankWith(repeat, noon)
	if len(got.ListenAgain) != 1 || got.ListenAgain[0].Confession.ID != "cat-peace-2" || got.ListenAgain[0].Score != 3 {
		t.Errorf("signal 7 listen-again rail = %+v, want cat-peace-2 with score 3", got.ListenAgain)
	}
	// Against a category with an equal number of completed listens, the
	// repeated one wins — the repeat is evidence beyond the listens.
	tie := Signals{Listens: []Listen{
		{SessionID: "s1", ConfessionID: "cat-peace-2", CategoryID: "cat-peace", Completed: true},
		{SessionID: "s2", ConfessionID: "cat-peace-2", CategoryID: "cat-peace", Completed: true},
		{SessionID: "s3", ConfessionID: "cat-healing-1", CategoryID: "cat-healing", Completed: true},
		{SessionID: "s4", ConfessionID: "cat-healing-2", CategoryID: "cat-healing", Completed: true},
	}}
	if got := rankWith(tie, noon); topCategory(got) != "cat-peace" {
		t.Errorf("signal 7 (repeat listening) did not break the tie in favour of the repeated category: %s", topCategory(got))
	}

	// Every signal reports itself present when it has evidence.
	all := Signals{
		Listens: append(append([]Listen(nil), repeat.Listens...), Listen{
			SessionID: "s9", ConfessionID: "cat-healing-1", CategoryID: "cat-healing", Completed: false, StartedAt: at(8),
		}, Listen{SessionID: "s10", ConfessionID: "cat-healing-2", CategoryID: "cat-healing", Completed: true, StartedAt: at(8)}),
		CompletedDurations: []int{600},
		Favourites:         Favourites{Confessions: map[string]bool{"x": true}},
	}
	if got := all.Present(); !reflect.DeepEqual(got, want) {
		t.Errorf("Present() with evidence for every signal = %v, want all seven %v", got, want)
	}
}

func TestRankingIsDeterministic(t *testing.T) {
	sig := Signals{Listens: []Listen{
		{SessionID: "s1", ConfessionID: "cat-peace-1", CategoryID: "cat-peace", Completed: true, StartedAt: at(7)},
		{SessionID: "s2", ConfessionID: "cat-purpose-1", CategoryID: "cat-purpose", Completed: false, StartedAt: at(7)},
	}, CompletedDurations: []int{900}, Favourites: Favourites{Categories: map[string]bool{"cat-healing": true}}}
	first := rankWith(sig, at(7))
	for i := 0; i < 20; i++ {
		if again := rankWith(sig, at(7)); !reflect.DeepEqual(first, again) {
			t.Fatalf("run %d differed from the first run", i)
		}
	}
}

func TestSuggestedDurationLadder(t *testing.T) {
	cases := []struct {
		typical int
		rate    float64
		sample  int
		want    int
	}{
		{0, 1, 10, 0},           // nothing finished: no suggestion
		{7 * 60, 1, 0, 10 * 60}, // snaps up to the first rung
		{29 * 60, 0, 0, 30 * 60},
		{29 * 60, 0.2, 2, 30 * 60},  // low rate but sample too small to act on
		{29 * 60, 0.2, 3, 15 * 60},  // low rate: one rung shorter
		{29 * 60, 0.95, 3, 45 * 60}, // high rate: one rung longer
		{10 * 60, 0.1, 9, 10 * 60},  // cannot go below the ladder
		{180 * 60, 1, 9, 180 * 60},  // cannot go above it
	}
	for _, c := range cases {
		if got := SuggestDuration(c.typical, c.rate, c.sample); got != c.want {
			t.Errorf("SuggestDuration(%d, %v, %d) = %d, want %d", c.typical, c.rate, c.sample, got, c.want)
		}
	}
	if !reflect.DeepEqual(Ladder(), []int{600, 900, 1800, 2700, 3600, 5400, 7200, 10800}) {
		t.Errorf("ladder drifted: %v", Ladder())
	}
}

func TestDaypartsFollowTheListenersTimezone(t *testing.T) {
	lagos, err := time.LoadLocation("Africa/Lagos") // UTC+1, no DST
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	// 04:30 UTC is night in UTC and 05:30 — morning — in Lagos.
	l := Listen{SessionID: "s", ConfessionID: "c", CategoryID: "k", Completed: true,
		StartedAt: time.Date(2026, 9, 21, 4, 30, 0, 0, time.UTC)}
	utc := Signals{Listens: []Listen{l}}
	local := Signals{Listens: []Listen{l}, Location: lagos}
	if dp, _ := utc.PreferredDaypart(); dp != Night {
		t.Errorf("UTC daypart = %s, want night", dp)
	}
	if dp, _ := local.PreferredDaypart(); dp != Morning {
		t.Errorf("Lagos daypart = %s, want morning", dp)
	}
}

func TestDaypartBoundaries(t *testing.T) {
	for h, want := range map[int]Daypart{0: Night, 4: Night, 5: Morning, 11: Morning, 12: Afternoon, 16: Afternoon, 17: Evening, 20: Evening, 21: Night, 23: Night} {
		if got := DaypartOf(at(h)); got != want {
			t.Errorf("hour %02d = %s, want %s", h, got, want)
		}
	}
}
