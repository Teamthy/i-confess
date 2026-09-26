package billing

import (
	"testing"
	"time"
)

// TestTrialJourneyMatchesSpec pins the seven-day journey to the seven things it
// is supposed to teach. The copy had drifted to a category sampler — "Morning
// Healing", "Peace at Noon", "Faith Foundations" — which described six
// different categories and none of the product's capabilities, so the trial
// never showed a listener the personalization, the Premium voice or the custom
// builder that the paywall asks them to pay for.
//
// Intent is asserted verbatim rather than by title so a reworded title cannot
// silently drop a day's purpose.
func TestTrialJourneyMatchesSpec(t *testing.T) {
	want := []string{
		IntentFirstConfession,
		IntentMorningNight,
		IntentPersonalization,
		IntentLongerSession,
		IntentPremiumVoice,
		IntentCustomSession,
		IntentWeeklySummary,
	}
	if len(TrialJourney) != len(want) {
		t.Fatalf("journey has %d days, want %d", len(TrialJourney), len(want))
	}
	for i, intent := range want {
		day := TrialJourney[i]
		if day.Day != i+1 {
			t.Errorf("journey[%d].Day = %d, want %d", i, day.Day, i+1)
		}
		if day.Intent != intent {
			t.Errorf("day %d intent = %q, want %q", day.Day, day.Intent, intent)
		}
		if day.Title == "" || day.Description == "" {
			t.Errorf("day %d has empty copy (title=%q description=%q)", day.Day, day.Title, day.Description)
		}
		if day.CTA == "" {
			t.Errorf("day %d has no call to action", day.Day)
		}
		if day.Duration <= 0 {
			t.Errorf("day %d duration = %d, want a positive session length", day.Day, day.Duration)
		}
		if len(day.Categories) == 0 {
			t.Errorf("day %d has no categories to build a session from", day.Day)
		}
	}

	// The flags are what make each day's purpose true rather than merely
	// advertised, so each one is asserted against the day that claims it.
	if TrialJourney[1].SessionCount != 2 {
		t.Errorf("day 2 (morning and night) session_count = %d, want 2", TrialJourney[1].SessionCount)
	}
	if !TrialJourney[2].Personalized {
		t.Error("day 3 is not flagged personalized")
	}
	if TrialJourney[3].Duration <= TrialJourney[0].Duration {
		t.Errorf("day 4 duration %d is not longer than day 1 duration %d", TrialJourney[3].Duration, TrialJourney[0].Duration)
	}
	longest := 0
	for _, d := range TrialJourney {
		if d.Duration > longest {
			longest = d.Duration
		}
	}
	if TrialJourney[3].Duration != longest {
		t.Errorf("day 4 duration %d is not the longest of the week (%d)", TrialJourney[3].Duration, longest)
	}
	if !TrialJourney[4].PremiumVoice {
		t.Error("day 5 is not flagged premium_voice")
	}
	if !TrialJourney[5].Custom {
		t.Error("day 6 is not flagged custom")
	}
	if !TrialJourney[6].Summary {
		t.Error("day 7 is not flagged summary")
	}

	// Every category named must be a real seeded slug, or the engine cannot
	// build the day and the journey advertises a session that does not exist.
	known := map[string]bool{
		"healing": true, "health": true, "finance": true, "wealth": true,
		"breakthrough": true, "marriage": true, "relationships": true, "faith": true,
		"peace": true, "family": true, "purpose": true, "identity": true,
		"wisdom": true, "protection": true, "career": true, "business": true,
		"leadership": true, "favor": true, "provision": true, "confidence": true,
		"discipline": true, "joy": true, "hope": true, "freedom": true,
		"spiritual-growth": true, "prayer": true, "children": true, "parenting": true,
		"direction": true, "creativity": true, "productivity": true,
		"emotional-strength": true, "rest": true, "gratitude": true,
		"forgiveness": true, "love": true, "overcoming-fear": true, "success": true,
		"destiny": true,
	}
	for _, d := range TrialJourney {
		for _, slug := range d.Categories {
			if !known[slug] {
				t.Errorf("day %d names category %q, which is not in the seeded catalogue", d.Day, slug)
			}
		}
	}
}

// TestJourneyIntentsMatchTheJourney keeps the exported intent list and the
// journey table from disagreeing. Two lists of seven that must agree by
// convention is exactly the shape that rots.
func TestJourneyIntentsMatchTheJourney(t *testing.T) {
	if len(JourneyIntents) != len(TrialJourney) {
		t.Fatalf("JourneyIntents has %d entries, journey has %d days", len(JourneyIntents), len(TrialJourney))
	}
	for i, intent := range JourneyIntents {
		if TrialJourney[i].Intent != intent {
			t.Errorf("JourneyIntents[%d] = %q but the journey day is %q", i, intent, TrialJourney[i].Intent)
		}
	}
}

// TestTrialDayForBoundaries keeps the clock mapping honest at its edges: before
// the start there is no day, day seven runs until expiry, and after expiry
// there is no current day to show.
func TestTrialDayForBoundaries(t *testing.T) {
	start := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name string
		now  time.Time
		want int
	}{
		{"before the start", start.Add(-time.Hour), 0},
		{"at the start", start, 1},
		{"end of day one", start.Add(23 * time.Hour), 1},
		{"start of day two", start.Add(24 * time.Hour), 2},
		{"last day", start.Add(6*24*time.Hour + 23*time.Hour), 7},
		{"after the journey", start.Add(7 * 24 * time.Hour), 0},
	} {
		if got := DayFor(start, tc.now); got != tc.want {
			t.Errorf("%s: DayFor = %d, want %d", tc.name, got, tc.want)
		}
	}
	if DayFor(time.Time{}, start) != 0 {
		t.Error("DayFor with a zero start should be 0, not an invented day")
	}
	if Day(0) != nil || Day(8) != nil {
		t.Error("Day outside 1..7 must return nil rather than wrapping")
	}
	if Day(3) == nil || Day(3).Intent != IntentPersonalization {
		t.Error("Day(3) should be the personalization day")
	}
}
