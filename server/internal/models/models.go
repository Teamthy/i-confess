package models

// Timestamps are stored as RFC3339 strings in SQLite and TIMESTAMPTZ in Postgres.
type Collection struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description,omitempty"`
	Premium     bool   `json:"premium"`
	Status      string `json:"status"`
	SortOrder   int    `json:"sort_order"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type Category struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Premium     bool   `json:"premium"`
	Status      string `json:"status"`
	SortOrder   int    `json:"sort_order"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type Confession struct {
	ID          string              `json:"id"`
	CategoryID  string              `json:"category_id"`
	Title       string              `json:"title"`
	ShortText   string              `json:"short_text,omitempty"`
	MediumText  string              `json:"medium_text,omitempty"`
	LongText    string              `json:"long_text,omitempty"`
	Description string              `json:"description,omitempty"`
	Tags        []string            `json:"tags,omitempty"`
	Intensity   int                 `json:"intensity"`
	Language    string              `json:"language"`
	Status      string              `json:"status"`
	Author      string              `json:"author,omitempty"`
	Version     int                 `json:"version"`
	PublishedAt string              `json:"published_at,omitempty"`
	CreatedAt   string              `json:"created_at"`
	UpdatedAt   string              `json:"updated_at"`
	Variants    []ConfessionVariant `json:"variants,omitempty"`
	Scriptures  []ScriptureRef      `json:"scriptures,omitempty"`
}

type ConfessionVariant struct {
	ID              string `json:"id"`
	ConfessionID    string `json:"confession_id"`
	Label           string `json:"label"`
	DurationSeconds int    `json:"duration_seconds"`
	SortOrder       int    `json:"sort_order"`
}

type ScriptureRef struct {
	ID            string `json:"id"`
	ConfessionID  string `json:"confession_id"`
	Book          string `json:"book"`
	Chapter       int    `json:"chapter,omitempty"`
	Verse         string `json:"verse,omitempty"`
	Translation   string `json:"translation"`
	IsDirectQuote bool   `json:"is_direct_quote"`
	Notes         string `json:"notes,omitempty"`
	SortOrder     int    `json:"sort_order"`
}

type Voice struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"`
	Provider    string `json:"provider,omitempty"`
	Gender      string `json:"gender,omitempty"`
	Language    string `json:"language"`
	Premium     bool   `json:"premium"`
	Status      string `json:"status"`
	SampleURL   string `json:"sample_url,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// AudioAsset is a rendered audio file (PRD S12).
//
// URL holds the object-storage KEY, never a public link, and is json:"-" so it
// cannot leak into a client payload. Callers exchange the key for a
// short-lived signed URL at read time, which is what keeps access tied to
// entitlement rather than to whoever once saw the response.
type AudioAsset struct {
	ID              string `json:"id"`
	ConfessionID    string `json:"confession_id"`
	VariantID       string `json:"variant_id,omitempty"`
	VoiceID         string `json:"voice_id"`
	URL             string `json:"-"`
	DurationSeconds int    `json:"duration_seconds,omitempty"`
	SizeBytes       int64  `json:"size_bytes,omitempty"`
	Checksum        string `json:"checksum,omitempty"`
	Language        string `json:"language,omitempty"`
	Status          string `json:"status"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type User struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	DisplayName   string `json:"display_name,omitempty"`
	Timezone      string `json:"timezone"`
	Status        string `json:"status"`
	EmailVerified bool   `json:"email_verified"`
	MFAEnabled    bool   `json:"mfa_enabled"`
	LastLoginAt   string `json:"last_login_at,omitempty"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at,omitempty"`
}

type Subscription struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Plan      string `json:"plan"`
	Status    string `json:"status"`
	StartedAt string `json:"started_at,omitempty"`
	EndsAt    string `json:"ends_at,omitempty"`
	CreatedAt string `json:"created_at"`
}

type Schedule struct {
	ID              string   `json:"id"`
	UserID          string   `json:"user_id"`
	Label           string   `json:"label"`
	Time            string   `json:"time"`
	DaysOfWeek      []int    `json:"days_of_week"`
	Timezone        string   `json:"timezone"`
	DurationSeconds int      `json:"duration_seconds"`
	VoiceID         string   `json:"voice_id,omitempty"`
	CategoryIDs     []string `json:"category_ids,omitempty"`
	Enabled         bool     `json:"enabled"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

type Session struct {
	ID              string `json:"id"`
	UserID          string `json:"user_id"`
	Type            string `json:"type"`
	DurationSeconds int    `json:"duration_seconds"`
	// TargetDuration is the length the listener asked for. ActualDuration is
	// what the built queue really runs to. They differ whenever complete
	// confessions could not land exactly on the request, which is the normal
	// case — audio is never truncated to close the gap. Surfacing both lets
	// the player say "30 min requested · 29:40 played" instead of letting the
	// progress bar quietly disagree with the label.
	TargetDuration int `json:"target_duration"`
	ActualDuration int `json:"actual_duration"`
	// Strategy records how the target was reconciled with the available
	// content. Persisting it is what makes a session reproducible: rebuilding
	// the same request later must yield the same queue, so the rule that
	// produced it has to travel with it.
	Strategy string `json:"strategy,omitempty"`
	VoiceID  string `json:"voice_id,omitempty"`
	// VoiceDowngraded reports that the requested voice was unavailable on the
	// listener's plan and a permitted voice was substituted. Surfacing this
	// keeps a gated experience from looking like a broken one.
	VoiceDowngraded      bool          `json:"voice_downgraded,omitempty"`
	VoiceDowngradeReason string        `json:"voice_downgrade_reason,omitempty"`
	Status               string        `json:"status"`
	CreatedAt            string        `json:"created_at"`
	StartedAt            string        `json:"started_at,omitempty"`
	CompletedAt          string        `json:"completed_at,omitempty"`
	Items                []SessionItem `json:"items,omitempty"`
}

type SessionItem struct {
	ID              string `json:"id"`
	SessionID       string `json:"session_id"`
	ConfessionID    string `json:"confession_id"`
	VariantID       string `json:"variant_id,omitempty"`
	VoiceID         string `json:"voice_id,omitempty"`
	AudioAssetID    string `json:"audio_asset_id,omitempty"`
	Position        int    `json:"position"`
	DurationSeconds int    `json:"duration_seconds"`
	Status          string `json:"status"`
	// Denormalized for the player payload:
	Title    string `json:"title,omitempty"`
	Category string `json:"category,omitempty"`
	// AudioURL is a short-lived signed URL minted per request, never a raw
	// storage key. Empty when the listener is not entitled to this item.
	AudioURL string `json:"audio_url,omitempty"`
	Text     string `json:"text,omitempty"`
	// Locked marks an item the listener cannot play on their current plan, so
	// the client can show an upgrade affordance instead of a silent gap.
	Locked     bool   `json:"locked,omitempty"`
	LockReason string `json:"lock_reason,omitempty"`
}

type UserConfession struct {
	ID         string `json:"id"`
	UserID     string `json:"user_id"`
	Title      string `json:"title"`
	Text       string `json:"text"`
	CategoryID string `json:"category_id,omitempty"`
	IsPrivate  bool   `json:"is_private"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

type Favorite struct {
	ID         string `json:"id"`
	UserID     string `json:"user_id"`
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	CreatedAt  string `json:"created_at"`
}

type PlaybackRecord struct {
	ID              string `json:"id"`
	UserID          string `json:"user_id"`
	SessionID       string `json:"session_id,omitempty"`
	ConfessionID    string `json:"confession_id,omitempty"`
	DurationSeconds int    `json:"duration_seconds,omitempty"`
	Completed       bool   `json:"completed"`
	Skipped         bool   `json:"skipped"`
	ListenedAt      string `json:"listened_at"`
}

// ============================= AUDIO INFRASTRUCTURE =============================

// ContentVersion separates content from audio production
type ContentVersion struct {
	ID            string `json:"id"`
	ConfessionID  string `json:"confession_id"`
	VersionNumber int    `json:"version_number"`
	Title         string `json:"title"`
	ShortText     string `json:"short_text,omitempty"`
	MediumText    string `json:"medium_text,omitempty"`
	LongText      string `json:"long_text,omitempty"`
	Language      string `json:"language"`
	Status        string `json:"status"` // draft | approved | published | archived
	ApprovedBy    string `json:"approved_by,omitempty"`
	ApprovedAt    string `json:"approved_at,omitempty"`
	PublishedAt   string `json:"published_at,omitempty"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// EnhancedAudioAsset represents complete audio infrastructure with full metadata
type EnhancedAudioAsset struct {
	ID               string `json:"id"`
	ContentID        string `json:"content_id"`
	ContentVersionID string `json:"content_version_id,omitempty"`
	VoiceID          string `json:"voice_id"`

	// Classification
	AssetType   string `json:"asset_type"`   // source | master | stream | preview | download
	QualityTier string `json:"quality_tier"` // standard | high | lossless

	// Storage
	StorageProvider string `json:"storage_provider"`
	StorageKey      string `json:"storage_key"`
	CDNPath         string `json:"cdn_path,omitempty"`

	// Format
	Format    string `json:"format"`    // m4a | mp3 | wav | flac | ogg
	Codec     string `json:"codec"`     // aac | mp3 | pcm | flac | opus
	Container string `json:"container"` // mp4 | mpeg | wav | ogg

	// Audio properties
	DurationSeconds int   `json:"duration_seconds"`
	FileSizeBytes   int64 `json:"file_size_bytes"`
	SampleRate      int   `json:"sample_rate,omitempty"` // Hz
	Bitrate         int   `json:"bitrate,omitempty"`     // kbps
	Channels        int   `json:"channels"`              // 1 | 2

	// Quality metrics
	LoudnessLUFS *float64 `json:"loudness_lufs,omitempty"`
	PeakDB       *float64 `json:"peak_db,omitempty"`

	// Integrity
	ChecksumSHA256 string `json:"checksum_sha256,omitempty"`

	// Lifecycle
	Status      string `json:"status"` // uploading | processing | ready | published | failed | archived
	PublishedAt string `json:"published_at,omitempty"`
	ArchivedAt  string `json:"archived_at,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// AudioGenerationJob tracks async TTS/recording production
type AudioGenerationJob struct {
	ID               string `json:"id"`
	ContentVersionID string `json:"content_version_id"`
	VoiceID          string `json:"voice_id"`

	Provider      string `json:"provider"` // google-tts | azure-tts | openai | human-recording
	ProviderJobID string `json:"provider_job_id,omitempty"`

	QualityTier string `json:"quality_tier"`
	Format      string `json:"format"`

	Status string `json:"status"` // queued | processing | succeeded | failed | cancelled

	AttemptCount int `json:"attempt_count"`
	MaxAttempts  int `json:"max_attempts"`

	RequestedBy string `json:"requested_by,omitempty"`
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`

	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`

	IdempotencyKey string `json:"idempotency_key,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// AudioVariant represents quality-specific encoding
type AudioVariant struct {
	ID           string `json:"id"`
	AudioAssetID string `json:"audio_asset_id"`

	QualityTier   string `json:"quality_tier"` // standard | high | lossless
	Bitrate       int    `json:"bitrate,omitempty"`
	FileSizeBytes int64  `json:"file_size_bytes,omitempty"`

	StorageKey string `json:"storage_key"`
	CDNPath    string `json:"cdn_path,omitempty"`

	ChecksumSHA256 string `json:"checksum_sha256,omitempty"`

	Status string `json:"status"` // processing | ready | failed

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// AudioProcessingLog tracks pipeline steps
type AudioProcessingLog struct {
	ID           string `json:"id"`
	AudioAssetID string `json:"audio_asset_id"`

	Step   string `json:"step"`   // validation | normalization | transcoding | publishing
	Status string `json:"status"` // started | completed | failed

	Details      string `json:"details,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`

	DurationMS int `json:"duration_ms,omitempty"`

	CreatedAt string `json:"created_at"`
}

// AudioChecksum ensures integrity
type AudioChecksum struct {
	ID           string `json:"id"`
	AudioAssetID string `json:"audio_asset_id"`

	SHA256 string `json:"sha256"`
	MD5    string `json:"md5,omitempty"`
	CRC32  string `json:"crc32,omitempty"`

	ValidatedAt string `json:"validated_at"`
	CreatedAt   string `json:"created_at"`
}

// VoiceRights tracks authorization metadata
// (Defined in voice_rights.go)

// SignedURL caches generated secure URLs
type SignedURL struct {
	ID           string `json:"id"`
	AudioAssetID string `json:"audio_asset_id"`
	UserID       string `json:"user_id,omitempty"`

	SignedURL string `json:"signed_url"`

	ExpiresAt   string `json:"expires_at"`
	AccessedAt  string `json:"accessed_at,omitempty"`
	AccessCount int    `json:"access_count"`

	CreatedAt string `json:"created_at"`
}

// AudioPlaybackSession tracks detailed playback
type AudioPlaybackSession struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	AudioAssetID string `json:"audio_asset_id"`
	SessionID    string `json:"session_id,omitempty"`

	StartedAt   string `json:"started_at"`
	PausedAt    string `json:"paused_at,omitempty"`
	ResumedAt   string `json:"resumed_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`

	Status string `json:"status"` // playing | paused | completed | abandoned

	PositionSeconds int `json:"position_seconds"`
	DurationSeconds int `json:"duration_seconds"`

	DeviceID string `json:"device_id,omitempty"`
	Platform string `json:"platform,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// AudioDownload manages licensed downloads
type AudioDownload struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	AudioAssetID string `json:"audio_asset_id"`

	Status string `json:"status"` // queued | downloading | downloaded | failed | removed

	DownloadedAt string `json:"downloaded_at,omitempty"`
	RemovedAt    string `json:"removed_at,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`

	LocalPath     string `json:"local_path,omitempty"`
	FileSizeBytes int64  `json:"file_size_bytes,omitempty"`

	RetryCount int    `json:"retry_count"`
	LastError  string `json:"last_error,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// PlaybackProgress tracks fine-grained position
type PlaybackProgress struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	AudioAssetID string `json:"audio_asset_id"`

	PositionSeconds int `json:"position_seconds"`
	DurationSeconds int `json:"duration_seconds"`

	Completed bool `json:"completed"`

	DeviceID string `json:"device_id,omitempty"`
	Platform string `json:"platform,omitempty"`

	LastUpdatedAt string `json:"last_updated_at"`

	CreatedAt string `json:"created_at"`
}

// AudioEvent represents playback analytics event
type AudioEvent struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id,omitempty"`
	AudioAssetID string `json:"audio_asset_id,omitempty"`

	EventType string `json:"event_type"` // play_request | play_started | play_paused | etc

	PositionSeconds int `json:"position_seconds,omitempty"`
	DurationSeconds int `json:"duration_seconds,omitempty"`

	DeviceID   string `json:"device_id,omitempty"`
	Platform   string `json:"platform,omitempty"`
	AppVersion string `json:"app_version,omitempty"`

	NetworkType   string   `json:"network_type,omitempty"` // wifi | cellular
	BandwidthMbps *float64 `json:"bandwidth_mbps,omitempty"`

	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`

	Metadata string `json:"metadata,omitempty"`

	CreatedAt string `json:"created_at"`
}

// AudioQoEMetrics tracks Quality of Experience
type AudioQoEMetrics struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	AudioAssetID string `json:"audio_asset_id"`

	StartupLatencyMS   int `json:"startup_latency_ms,omitempty"`
	BufferDurationMS   int `json:"buffer_duration_ms,omitempty"`
	RebufferCount      int `json:"rebuffer_count"`
	RebufferDurationMS int `json:"rebuffer_duration_ms,omitempty"`

	CompletionRate *float64 `json:"completion_rate,omitempty"`
	FailureRate    *float64 `json:"failure_rate,omitempty"`

	QualityExperienced string `json:"quality_experienced,omitempty"`

	CreatedAt string `json:"created_at"`
}

// AuthSession is a live login session shown on the security screen (PRD S30, S31).
//
// It deliberately carries no precise location: showing a user a city derived
// from an IP invites false alarms and is not needed to recognise a device.
type AuthSession struct {
	ID         string `json:"id"`
	Platform   string `json:"platform,omitempty"`
	UserAgent  string `json:"user_agent,omitempty"`
	LastUsedAt string `json:"last_used_at,omitempty"`
	CreatedAt  string `json:"created_at"`
	ExpiresAt  string `json:"expires_at"`
	Current    bool   `json:"current,omitempty"`
}

// UserProfile is the presentation identity, kept separate from the security
// record in `users` (PRD §5, §113).
type UserProfile struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Username    string `json:"username,omitempty"`
	Bio         string `json:"bio,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	CoverURL    string `json:"cover_url,omitempty"`
	Timezone    string `json:"timezone"`
	Locale      string `json:"locale"`
	CountryCode string `json:"country_code,omitempty"`
	Language    string `json:"language"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// PublicProfile is the explicit projection shown to other users (PRD §6, §63).
//
// It is a separate type rather than a filtered UserProfile on purpose: adding a
// private field to UserProfile must never silently publish it. Anything public
// has to be added here deliberately.
type PublicProfile struct {
	Username    string `json:"username,omitempty"`
	DisplayName string `json:"display_name"`
	Bio         string `json:"bio,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
}

// Public projects the safe subset. Email, timezone, locale, country and all
// identifiers are deliberately omitted.
func (p *UserProfile) Public() PublicProfile {
	return PublicProfile{
		Username:    p.Username,
		DisplayName: p.DisplayName,
		Bio:         p.Bio,
		AvatarURL:   p.AvatarURL,
	}
}

// UserPreferences holds listening and notification settings (PRD §20).
type UserPreferences struct {
	ID                     string `json:"id"`
	UserID                 string `json:"user_id"`
	DefaultDuration        int    `json:"default_duration"`
	DefaultVoiceID         string `json:"default_voice_id,omitempty"`
	Autoplay               bool   `json:"autoplay"`
	PreferredQuality       string `json:"preferred_quality"`
	DownloadOverWifi       bool   `json:"download_over_wifi"`
	NotificationsEnabled   bool   `json:"notifications_enabled"`
	RecommendationsEnabled bool   `json:"recommendations_enabled"`
	PersonalizationEnabled bool   `json:"personalization_enabled"`
	Language               string `json:"language"`
	Theme                  string `json:"theme"`
	UpdatedAt              string `json:"updated_at,omitempty"`
}

// UserInterest links a user to a category (PRD §14).
//
// Explicit reports whether the user chose this themselves. The flag is
// serialised so clients can render stated preferences differently from
// inferences, rather than presenting a guess as the user's own words (§15).
type UserInterest struct {
	ID         string  `json:"id"`
	UserID     string  `json:"user_id"`
	CategoryID string  `json:"category_id"`
	Weight     float64 `json:"weight"`
	Source     string  `json:"source"`
	Explicit   bool    `json:"explicit"`
	CreatedAt  string  `json:"created_at,omitempty"`
	UpdatedAt  string  `json:"updated_at,omitempty"`
}

// Collection visibility (PRD §37). Private is the default everywhere: a
// personal collection must never become public by omission.
const (
	VisibilityPrivate  = "private"
	VisibilityUnlisted = "unlisted"
	VisibilityPublic   = "public"
)

// UserCollection is a collection a user built (PRD §35).
type UserCollection struct {
	ID          string           `json:"id"`
	UserID      string           `json:"user_id"`
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	CoverURL    string           `json:"cover_url,omitempty"`
	Visibility  string           `json:"visibility"`
	ItemCount   int              `json:"item_count"`
	Items       []CollectionItem `json:"items,omitempty"`
	CreatedAt   string           `json:"created_at"`
	UpdatedAt   string           `json:"updated_at"`
}

// CollectionItem is one entry in a user collection (PRD §36).
type CollectionItem struct {
	ID           string `json:"id"`
	ConfessionID string `json:"confession_id"`
	Title        string `json:"title,omitempty"`
	Position     int    `json:"position"`
}

// UserDevice is a registered device (PRD §45).
//
// Carries no hardware identifiers: enough to recognise an entry, not enough to
// track a person across contexts.
type UserDevice struct {
	DeviceID   string `json:"device_id"`
	Platform   string `json:"platform,omitempty"`
	Name       string `json:"device_name,omitempty"`
	AppVersion string `json:"app_version,omitempty"`
	LastSeenAt string `json:"last_seen_at"`
	CreatedAt  string `json:"created_at"`
}

// NotificationPreferences controls non-essential messaging (PRD §46).
//
// Security notifications are deliberately absent: they follow security policy,
// not user preference, so a compromised account cannot silence its own alerts.
type NotificationPreferences struct {
	UserID            string `json:"user_id"`
	ScheduledSessions bool   `json:"scheduled_sessions"`
	NewContent        bool   `json:"new_content"`
	Recommendations   bool   `json:"recommendations"`
	ProductUpdates    bool   `json:"product_updates"`
	UpdatedAt         string `json:"updated_at,omitempty"`
}

// Download is an offline licence for one audio asset (PRD §28).
//
// StorageKey is the object key, never a URL: the client exchanges it for a
// signed download link, so a licence that has lapsed cannot be redeemed even
// if the record is still on the device.
type Download struct {
	ID              string `json:"id"`
	UserID          string `json:"user_id"`
	AudioAssetID    string `json:"audio_asset_id"`
	ConfessionID    string `json:"confession_id,omitempty"`
	VoiceID         string `json:"voice_id,omitempty"`
	StorageKey      string `json:"-"`
	Title           string `json:"title,omitempty"`
	DurationSeconds int    `json:"duration_seconds"`
	SizeBytes       int64  `json:"size_bytes,omitempty"`
	Checksum        string `json:"checksum,omitempty"`
	Status          string `json:"status"`
	// ExpiresAt is when the offline licence lapses. Surfaced so the client can
	// warn before content stops working rather than failing silently.
	ExpiresAt string `json:"expires_at,omitempty"`
	// Expired is computed at read time so a stale client cannot decide for
	// itself that a licence is still good.
	Expired   bool   `json:"expired"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at,omitempty"`
}
