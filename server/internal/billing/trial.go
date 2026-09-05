package billing

import "time"

// TrialDay defines one day of the 7-day trial journey.
// Each day maps to categories and a session duration; the engine builds the session.
type TrialDay struct {
	Day         int      `json:"day"` // 1..7
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Categories  []string `json:"categories"` // category slugs
	Duration    int      `json:"duration"`   // seconds
	VoiceID     string   `json:"voice_id,omitempty"`
}

// TrialJourney is the deterministic 7-day progression.
// Day1 Morning … Day7 Weekly Summary — same for every trial user,
// so the experience is testable and never invents theology.
var TrialJourney = []TrialDay{
	{Day: 1, Title: "Morning Healing", Description: "Begin with wholeness", Categories: []string{"healing"}, Duration: 600},
	{Day: 2, Title: "Peace at Noon", Description: "Steady your heart", Categories: []string{"peace"}, Duration: 600},
	{Day: 3, Title: "Faith Foundations", Description: "Anchor in promise", Categories: []string{"faith"}, Duration: 900},
	{Day: 4, Title: "Purpose & Focus", Description: "Clarity for the day", Categories: []string{"purpose"}, Duration: 900},
	{Day: 5, Title: "Gratitude Evening", Description: "Close with thanks", Categories: []string{"gratitude"}, Duration: 600},
	{Day: 6, Title: "Confidence & Wisdom", Description: "Step forward", Categories: []string{"confidence", "wisdom"}, Duration: 1200},
	{Day: 7, Title: "Weekly Summary", Description: "Review and recommit", Categories: []string{"healing", "peace", "faith"}, Duration: 1800},
}

// DayFor returns the trial day for a user who started at start.
// Before start → Day 0 (not started), after Day 7 → 0 (trial complete).
func DayFor(start, now time.Time) int {
	if start.IsZero() || now.Before(start) {
		return 0
	}
	days := int(now.Sub(start).Hours() / 24)
	if days < 0 {
		return 0
	}
	if days >= len(TrialJourney) {
		return 0 // complete
	}
	return days + 1
}

// Day returns the TrialDay for a given day number (1-indexed), or nil.
func Day(n int) *TrialDay {
	if n < 1 || n > len(TrialJourney) {
		return nil
	}
	d := TrialJourney[n-1]
	return &d
}
