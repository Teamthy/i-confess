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
