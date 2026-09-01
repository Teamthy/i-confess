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

type AudioAsset struct {
	ID              string `json:"id"`
	ConfessionID    string `json:"confession_id"`
	VariantID       string `json:"variant_id,omitempty"`
	VoiceID         string `json:"voice_id"`
	URL             string `json:"url"`
	DurationSeconds int    `json:"duration_seconds,omitempty"`
	SizeBytes       int64  `json:"size_bytes,omitempty"`
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
	ID              string        `json:"id"`
	UserID          string        `json:"user_id"`
	Type            string        `json:"type"`
	DurationSeconds int           `json:"duration_seconds"`
	VoiceID         string        `json:"voice_id,omitempty"`
	Status          string        `json:"status"`
	CreatedAt       string        `json:"created_at"`
	StartedAt       string        `json:"started_at,omitempty"`
	CompletedAt     string        `json:"completed_at,omitempty"`
	Items           []SessionItem `json:"items,omitempty"`
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
	AudioURL string `json:"audio_url,omitempty"`
	Text     string `json:"text,omitempty"`
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
	ID         string `json:"id"`
	ConfessionID string `json:"confession_id"`
	VersionNumber int  `json:"version_number"`
	Title      string `json:"title"`
	ShortText  string `json:"short_text,omitempty"`
	MediumText string `json:"medium_text,omitempty"`
	LongText   string `json:"long_text,omitempty"`
	Language   string `json:"language"`
	Status     string `json:"status"` // draft | approved | published | archived
	ApprovedBy string `json:"approved_by,omitempty"`
	ApprovedAt string `json:"approved_at,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// EnhancedAudioAsset represents complete audio infrastructure with full metadata
type EnhancedAudioAsset struct {
	ID                 string `json:"id"`
	ContentID          string `json:"content_id"`
	ContentVersionID   string `json:"content_version_id,omitempty"`
	VoiceID            string `json:"voice_id"`
	
	// Classification
	AssetType          string `json:"asset_type"` // source | master | stream | preview | download
	QualityTier        string `json:"quality_tier"` // standard | high | lossless
	
	// Storage
	StorageProvider    string `json:"storage_provider"`
	StorageKey         string `json:"storage_key"`
	CDNPath            string `json:"cdn_path,omitempty"`
	
	// Format
	Format             string `json:"format"` // m4a | mp3 | wav | flac | ogg
	Codec              string `json:"codec"` // aac | mp3 | pcm | flac | opus
	Container          string `json:"container"` // mp4 | mpeg | wav | ogg
	
	// Audio properties
	DurationSeconds    int `json:"duration_seconds"`
	FileSizeBytes      int64 `json:"file_size_bytes"`
	SampleRate         int `json:"sample_rate,omitempty"` // Hz
	Bitrate            int `json:"bitrate,omitempty"` // kbps
	Channels           int `json:"channels"` // 1 | 2
	
	// Quality metrics
	LoudnessLUFS       *float64 `json:"loudness_lufs,omitempty"`
	PeakDB             *float64 `json:"peak_db,omitempty"`
	
	// Integrity
	ChecksumSHA256     string `json:"checksum_sha256,omitempty"`
	
	// Lifecycle
	Status             string `json:"status"` // uploading | processing | ready | published | failed | archived
	PublishedAt        string `json:"published_at,omitempty"`
	ArchivedAt         string `json:"archived_at,omitempty"`
	
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

// AudioGenerationJob tracks async TTS/recording production
type AudioGenerationJob struct {
	ID                 string `json:"id"`
	ContentVersionID   string `json:"content_version_id"`
	VoiceID            string `json:"voice_id"`
	
	Provider           string `json:"provider"` // google-tts | azure-tts | openai | human-recording
	ProviderJobID      string `json:"provider_job_id,omitempty"`
	
	QualityTier        string `json:"quality_tier"`
	Format             string `json:"format"`
	
	Status             string `json:"status"` // queued | processing | succeeded | failed | cancelled
	
	AttemptCount       int `json:"attempt_count"`
	MaxAttempts        int `json:"max_attempts"`
	
	RequestedBy        string `json:"requested_by,omitempty"`
	StartedAt          string `json:"started_at,omitempty"`
	CompletedAt        string `json:"completed_at,omitempty"`
	
	ErrorCode          string `json:"error_code,omitempty"`
	ErrorMessage       string `json:"error_message,omitempty"`
	
	IdempotencyKey     string `json:"idempotency_key,omitempty"`
	
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

// AudioVariant represents quality-specific encoding
type AudioVariant struct {
	ID            string `json:"id"`
	AudioAssetID  string `json:"audio_asset_id"`
	
	QualityTier   string `json:"quality_tier"` // standard | high | lossless
	Bitrate       int `json:"bitrate,omitempty"`
	FileSizeBytes int64 `json:"file_size_bytes,omitempty"`
	
	StorageKey    string `json:"storage_key"`
	CDNPath       string `json:"cdn_path,omitempty"`
	
	ChecksumSHA256 string `json:"checksum_sha256,omitempty"`
	
	Status        string `json:"status"` // processing | ready | failed
	
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// AudioProcessingLog tracks pipeline steps
type AudioProcessingLog struct {
	ID            string `json:"id"`
	AudioAssetID  string `json:"audio_asset_id"`
	
	Step          string `json:"step"` // validation | normalization | transcoding | publishing
	Status        string `json:"status"` // started | completed | failed
	
	Details       string `json:"details,omitempty"`
	ErrorMessage  string `json:"error_message,omitempty"`
	
	DurationMS    int `json:"duration_ms,omitempty"`
	
	CreatedAt     string `json:"created_at"`
}

// AudioChecksum ensures integrity
type AudioChecksum struct {
	ID           string `json:"id"`
	AudioAssetID string `json:"audio_asset_id"`
	
	SHA256       string `json:"sha256"`
	MD5          string `json:"md5,omitempty"`
	CRC32        string `json:"crc32,omitempty"`
	
	ValidatedAt  string `json:"validated_at"`
	CreatedAt    string `json:"created_at"`
}

// VoiceRights tracks authorization metadata
type VoiceRights struct {
	ID                      string `json:"id"`
	VoiceID                 string `json:"voice_id"`
	
	RightsHolder            string `json:"rights_holder"`
	AuthorizationReference  string `json:"authorization_reference,omitempty"`
	
	AllowedUse              string `json:"allowed_use"` // tts | recording | streaming | commercial
	Territories             string `json:"territories"` // GLOBAL or comma-separated ISO codes
	
	StartDate               string `json:"start_date,omitempty"`
	ExpiryDate              string `json:"expiry_date,omitempty"`
	Status                  string `json:"status"` // pending | active | expired | revoked
	
	Metadata                string `json:"metadata,omitempty"`
	
	CreatedAt               string `json:"created_at"`
	UpdatedAt               string `json:"updated_at"`
}

// SignedURL caches generated secure URLs
type SignedURL struct {
	ID           string `json:"id"`
	AudioAssetID string `json:"audio_asset_id"`
	UserID       string `json:"user_id,omitempty"`
	
	SignedURL    string `json:"signed_url"`
	
	ExpiresAt    string `json:"expires_at"`
	AccessedAt   string `json:"accessed_at,omitempty"`
	AccessCount  int `json:"access_count"`
	
	CreatedAt    string `json:"created_at"`
}

// AudioPlaybackSession tracks detailed playback
type AudioPlaybackSession struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	AudioAssetID string `json:"audio_asset_id"`
	SessionID    string `json:"session_id,omitempty"`
	
	StartedAt    string `json:"started_at"`
	PausedAt     string `json:"paused_at,omitempty"`
	ResumedAt    string `json:"resumed_at,omitempty"`
	CompletedAt  string `json:"completed_at,omitempty"`
	
	Status       string `json:"status"` // playing | paused | completed | abandoned
	
	PositionSeconds int `json:"position_seconds"`
	DurationSeconds int `json:"duration_seconds"`
	
	DeviceID     string `json:"device_id,omitempty"`
	Platform     string `json:"platform,omitempty"`
	
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// AudioDownload manages licensed downloads
type AudioDownload struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	AudioAssetID string `json:"audio_asset_id"`
	
	Status       string `json:"status"` // queued | downloading | downloaded | failed | removed
	
	DownloadedAt string `json:"downloaded_at,omitempty"`
	RemovedAt    string `json:"removed_at,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
	
	LocalPath    string `json:"local_path,omitempty"`
	FileSizeBytes int64 `json:"file_size_bytes,omitempty"`
	
	RetryCount   int `json:"retry_count"`
	LastError    string `json:"last_error,omitempty"`
	
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// PlaybackProgress tracks fine-grained position
type PlaybackProgress struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	AudioAssetID string `json:"audio_asset_id"`
	
	PositionSeconds int `json:"position_seconds"`
	DurationSeconds int `json:"duration_seconds"`
	
	Completed    bool `json:"completed"`
	
	DeviceID     string `json:"device_id,omitempty"`
	Platform     string `json:"platform,omitempty"`
	
	LastUpdatedAt string `json:"last_updated_at"`
	
	CreatedAt    string `json:"created_at"`
}

// AudioEvent represents playback analytics event
type AudioEvent struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id,omitempty"`
	AudioAssetID string `json:"audio_asset_id,omitempty"`
	
	EventType    string `json:"event_type"` // play_request | play_started | play_paused | etc
	
	PositionSeconds int `json:"position_seconds,omitempty"`
	DurationSeconds int `json:"duration_seconds,omitempty"`
	
	DeviceID     string `json:"device_id,omitempty"`
	Platform     string `json:"platform,omitempty"`
	AppVersion   string `json:"app_version,omitempty"`
	
	NetworkType  string `json:"network_type,omitempty"` // wifi | cellular
	BandwidthMbps *float64 `json:"bandwidth_mbps,omitempty"`
	
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	
	Metadata     string `json:"metadata,omitempty"`
	
	CreatedAt    string `json:"created_at"`
}

// AudioQoEMetrics tracks Quality of Experience
type AudioQoEMetrics struct {
	ID                  string `json:"id"`
	UserID              string `json:"user_id"`
	AudioAssetID        string `json:"audio_asset_id"`
	
	StartupLatencyMS    int `json:"startup_latency_ms,omitempty"`
	BufferDurationMS    int `json:"buffer_duration_ms,omitempty"`
	RebufferCount       int `json:"rebuffer_count"`
	RebufferDurationMS  int `json:"rebuffer_duration_ms,omitempty"`
	
	CompletionRate      *float64 `json:"completion_rate,omitempty"`
	FailureRate         *float64 `json:"failure_rate,omitempty"`
	
	QualityExperienced  string `json:"quality_experienced,omitempty"`
	
	CreatedAt           string `json:"created_at"`
}
