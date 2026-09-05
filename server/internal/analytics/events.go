package analytics

// Events — §67–§68 analytics: app_opened → subscription_cancelled.
// No sensitive exfil: never log confession body, email, or Scripture text.
// Primary metrics: recurring sessions, retention, completion, trial→paid, DAU,
// scheduled usage, engagement. All events are actor + entity, not content.

const (
	EventAppOpened              = "app_opened"
	EventSessionCreated         = "session_created"
	EventSessionStarted         = "session_started"
	EventSessionPaused          = "session_paused"
	EventSessionResumed         = "session_resumed"
	EventSessionCompleted       = "session_completed"
	EventSessionSkipped         = "session_skipped"
	EventScheduleCreated        = "schedule_created"
	EventScheduleTriggered      = "schedule_triggered"
	EventDownloadCreated        = "download_created"
	EventDownloadRevoked        = "download_revoked"
	EventTrialDayViewed         = "trial_day_viewed"
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
