package analytics

// Events — §67–§68 analytics: app_opened → subscription_cancelled.
// No sensitive exfil: never log confession body, email, or Scripture text.
// Primary metrics: recurring sessions, retention, completion, trial→paid, DAU,
// scheduled usage, engagement. All events are actor + entity, not content.

const (
	EventAppOpened         = "app_opened"
	EventSessionCreated    = "session_created"
	EventSessionStarted    = "session_started"
	EventSessionPaused     = "session_paused"
	EventSessionResumed    = "session_resumed"
	EventSessionCompleted  = "session_completed"
	EventSessionSkipped    = "session_skipped"
	EventScheduleCreated   = "schedule_created"
	EventScheduleTriggered = "schedule_triggered"
	EventDownloadCreated   = "download_created"
	EventDownloadRevoked   = "download_revoked"
	EventTrialDayViewed    = "trial_day_viewed"
	// Trial funnel. A journey that can be displayed but not measured cannot
	// answer "did the trial work": these four are the numerator and denominator
	// of trial→paid conversion. Day completion is emitted by the server from
	// the session completion path, never asserted by a client.
	EventTrialStarted           = "trial_started"
	EventTrialDayCompleted      = "trial_day_completed"
	EventTrialConverted         = "trial_converted"
	EventTrialExpired           = "trial_expired"
	EventSubscriptionStarted    = "subscription_started"
	EventSubscriptionVerified   = "subscription_verified"
	EventSubscriptionCancelled  = "subscription_cancelled"
	EventSearchPerformed        = "search_performed"
	EventCategoryViewed         = "category_viewed"
	EventVoicePreviewed         = "voice_previewed"
	EventTemplateCreated        = "template_created"
	EventTemplateShared         = "template_shared"
	EventPlaybackProgressSynced = "playback_progress_synced"
)

// Event is the minimal payload for analytics ingestion (Prod: OTel / Segment / Posthog).
type Event struct {
	Name      string         `json:"name"`
	UserID    string         `json:"user_id"`
	Props     map[string]any `json:"props,omitempty"`
	Timestamp string         `json:"timestamp"`
}

// NoopSink logs to structured log in dev; prod replaces with HTTP sink.
type Sink interface {
	Track(e Event) error
}

type LogSink struct{}

func (LogSink) Track(e Event) error {
	return nil
}

// AllowedEvents returns the complete set of event names that may reach
// analytics, whether emitted by a client or by the server. Anything not in
// this set is rejected: analytics take IDs/counts/enums from the per-event
// allowlist only (§1 master prompt rule).
//
// EventTrialDayCompleted is included because the server emits it from a real
// session completion, but it is NOT in ClientSubmittableEvents — a client
// must never be able to assert its own trial progress.
func AllowedEvents() map[string]bool {
	return map[string]bool{
		EventAppOpened:              true,
		EventSessionCreated:         true,
		EventSessionStarted:         true,
		EventSessionPaused:          true,
		EventSessionResumed:         true,
		EventSessionCompleted:       true,
		EventSessionSkipped:         true,
		EventScheduleCreated:        true,
		EventScheduleTriggered:      true,
		EventDownloadCreated:        true,
		EventDownloadRevoked:        true,
		EventTrialDayViewed:         true,
		EventTrialStarted:           true,
		EventTrialDayCompleted:      true,
		EventTrialConverted:         true,
		EventTrialExpired:           true,
		EventSubscriptionStarted:    true,
		EventSubscriptionVerified:   true,
		EventSubscriptionCancelled:  true,
		EventSearchPerformed:        true,
		EventCategoryViewed:         true,
		EventVoicePreviewed:         true,
		EventTemplateCreated:        true,
		EventTemplateShared:         true,
		EventPlaybackProgressSynced: true,
	}
}

// ClientSubmittableEvents returns the subset of AllowedEvents that an
// authenticated client may POST to /analytics/batch. Server-only events
// (currently only trial_day_completed) are excluded: they are emitted by the
// server from authoritative state, never accepted from a caller.
func ClientSubmittableEvents() map[string]bool {
	allowed := AllowedEvents()
	delete(allowed, EventTrialDayCompleted)
	return allowed
}

// IsAllowed reports whether name is a registered analytics event.
func IsAllowed(name string) bool {
	return AllowedEvents()[name]
}

// IsClientSubmittable reports whether an authenticated client may submit
// an event with the given name.
func IsClientSubmittable(name string) bool {
	return ClientSubmittableEvents()[name]
}

// PIIPropKeys are event-prop keys that are never allowed in analytics,
// stripped at every ingestion point so private spiritual text, emails,
// confessions, notes and highlights never reach analytics or logs.
var PIIPropKeys = []string{"body", "email", "scripture_text", "scripture", "confession", "note", "highlight", "prayer"}

// StripPII removes any known PII prop keys from an event's props, returning
// the (possibly same) map. It never adds keys and preserves the map when
// nothing is stripped.
func StripPII(props map[string]any) map[string]any {
	if len(props) == 0 {
		return props
	}
	stripped := false
	for _, key := range PIIPropKeys {
		if _, ok := props[key]; ok {
			delete(props, key)
			stripped = true
		}
	}
	if !stripped {
		return props
	}
	return props
}
