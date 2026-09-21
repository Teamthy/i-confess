package billing

import "time"

// TrialDay defines one day of the 7-day trial journey.
//
// A day is not only copy. Each day teaches one specific thing about the
// product, and the flags below are what make that true rather than merely
// claimed: Day 2 really asks for two sessions, Day 3 really substitutes the
// listener's own interests, Day 5 really selects a Premium voice. Intent is
// the machine-checkable statement of the day's purpose; the test suite asserts
// the seven intents exactly, so the copy cannot drift away from the journey the
// product promises without a test failing.
type TrialDay struct {
	Day int `json:"day"` // 1..7
	// Intent is the single thing this day is for. It is part of the wire
	// contract so a client can render the right affordance (a "build your own"
	// button on Day 6, a voice picker on Day 5) instead of guessing from prose.
	Intent string `json:"intent"`
	Title  string `json:"title"`
	// Description is one line shown under the title.
	Description string `json:"description"`
	// CTA is the action label for the day. It was declared by the typed client
	// and never sent, so every journey row rendered without a call to action.
	CTA        string   `json:"cta"`
	Categories []string `json:"categories"` // category slugs
	Duration   int      `json:"duration"`   // seconds per session
	VoiceID    string   `json:"voice_id,omitempty"`

	// SessionCount is how many sessions the day asks for. Two is Day 2's whole
	// point: a morning and a night, which is the habit the trial is building.
	SessionCount int `json:"session_count,omitempty"`
	// Personalized means the categories above are the fallback only; the
	// journey resolves the listener's own interests instead.
	Personalized bool `json:"personalized,omitempty"`
	// PremiumVoice means the day deliberately uses a Premium voice, so a
	// listener who has not converted sees the paywall's actual difference.
	PremiumVoice bool `json:"premium_voice,omitempty"`
	// Custom means the day hands the listener the builder rather than a
	// pre-built session.
	Custom bool `json:"custom,omitempty"`
	// Summary means the day reviews the week instead of introducing something.
	Summary bool `json:"summary,omitempty"`
}

// Journey intents, in day order. These are the seven things a trial teaches and
// they are asserted verbatim by TestTrialJourneyMatchesSpec: D1 first
// confession, D2 morning+night, D3 personalization, D4 longer session,
// D5 premium voice, D6 custom session, D7 weekly summary.
const (
	IntentFirstConfession = "first_confession"
	IntentMorningNight    = "morning_and_night"
	IntentPersonalization = "personalization"
	IntentLongerSession   = "longer_session"
	IntentPremiumVoice    = "premium_voice"
	IntentCustomSession   = "custom_session"
	IntentWeeklySummary   = "weekly_summary"
)

// JourneyIntents is the canonical day-order list of intents. DayFor/Day index
// into TrialJourney; tests index into this.
var JourneyIntents = []string{
	IntentFirstConfession,
	IntentMorningNight,
	IntentPersonalization,
	IntentLongerSession,
	IntentPremiumVoice,
	IntentCustomSession,
	IntentWeeklySummary,
}

// TrialJourney is the deterministic 7-day progression. The shape of each day is
// the same for every trial user, so the experience is testable and never
// invents theology; what varies is Day 3's category set, which is resolved from
// the listener's own interests at request time.
//
// Durations are deliberate: Day 1 is short enough to finish in one sitting,
// Day 2 splits into two short bookends, Day 4 is the longest single session of
// the week, and Day 7 is a review rather than a new length to sustain.
var TrialJourney = []TrialDay{
	{
		Day: 1, Intent: IntentFirstConfession,
		Title:       "Your first confession",
		Description: "Speak one out loud and hear it answered",
		CTA:         "Begin your first confession",
		Categories:  []string{"healing"},
		Duration:    600,
	},
	{
		Day: 2, Intent: IntentMorningNight,
		Title:        "Morning and night",
		Description:  "Two short sessions, one at each end of your day",
		CTA:          "Plan your morning and night",
		Categories:   []string{"peace", "gratitude"},
		Duration:     480,
		SessionCount: 2,
	},
	{
		Day: 3, Intent: IntentPersonalization,
		Title:        "Made for you",
		Description:  "A session built from what you told us you carry",
		CTA:          "Open today's session",
		Categories:   []string{"faith"},
		Duration:     600,
		Personalized: true,
	},
	{
		Day: 4, Intent: IntentLongerSession,
		Title:       "Go deeper",
		Description: "Your longest session of the week — stay with it",
		CTA:         "Start the long session",
		Categories:  []string{"spiritual-growth"},
		Duration:    1800,
	},
	{
		Day: 5, Intent: IntentPremiumVoice,
		Title:        "A voice you know",
		Description:  "Hear today's confession read by a Premium voice",
		CTA:          "Listen with a Premium voice",
		Categories:   []string{"faith"},
		Duration:     900,
		PremiumVoice: true,
	},
	{
		Day: 6, Intent: IntentCustomSession,
		Title:       "Make it yours",
		Description: "Build a session from the categories you choose",
		CTA:         "Build your own session",
		Categories:  []string{"purpose", "hope"},
		Duration:    900,
		Custom:      true,
	},
	{
		Day: 7, Intent: IntentWeeklySummary,
		Title:       "Your week in summary",
		Description: "Review the week and decide what comes next",
		CTA:         "Review your week",
		Categories:  []string{"gratitude", "hope"},
		Duration:    900,
		Summary:     true,
	},
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
