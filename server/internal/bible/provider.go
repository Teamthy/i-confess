package bible

import (
	"context"
	"errors"
	"time"
)

var (
	ErrUnavailable = errors.New("Bible content unavailable")
	ErrNotFound    = errors.New("Bible content not found")
	ErrRestricted  = errors.New("translation is not available for this use")
)

// Translation is the provider-neutral application contract. Rights are
// independent switches; unknown or unreviewed rights are always false.
type Translation struct {
	ID                    string    `json:"id"`
	Provider              string    `json:"provider"`
	ProviderTranslationID string    `json:"provider_translation_id"`
	Name                  string    `json:"name"`
	Abbreviation          string    `json:"abbreviation"`
	Language              string    `json:"language_code"`
	LanguageName          string    `json:"language_name"`
	Locale                string    `json:"locale,omitempty"`
	Country               string    `json:"country,omitempty"`
	Dialect               string    `json:"dialect,omitempty"`
	Direction             string    `json:"direction"`
	Publisher             string    `json:"publisher,omitempty"`
	Description           string    `json:"description,omitempty"`
	Copyright             string    `json:"copyright,omitempty"`
	License               string    `json:"license,omitempty"`
	LicenseURL            string    `json:"license_url,omitempty"`
	PublicDomain          bool      `json:"public_domain"`
	CommercialUse         bool      `json:"commercial_use"`
	RedistributionAllowed bool      `json:"redistribution_allowed"`
	ModificationAllowed   bool      `json:"modification_allowed"`
	AudioAllowed          bool      `json:"audio_allowed"`
	OfflineAllowed        bool      `json:"offline_allowed"`
	CopyAllowed           bool      `json:"copy_allowed"`
	ShareAllowed          bool      `json:"share_allowed"`
	SearchIndexAllowed    bool      `json:"search_index_allowed"`
	APIExposureAllowed    bool      `json:"api_exposure_allowed"`
	AttributionRequired   bool      `json:"attribution_required"`
	AttributionText       string    `json:"attribution_text,omitempty"`
	SourceURL             string    `json:"source_url,omitempty"`
	SourceVersion         string    `json:"source_version,omitempty"`
	ImportVersion         string    `json:"import_version,omitempty"`
	ContentHash           string    `json:"content_hash,omitempty"`
	Status                string    `json:"status"`
	Coverage              string    `json:"coverage,omitempty"`
	BookCount             int       `json:"book_count,omitempty"`
	ChapterCount          int       `json:"chapter_count,omitempty"`
	VerseCount            int       `json:"verse_count,omitempty"`
	ImportedAt            time.Time `json:"imported_at,omitempty"`
}

type Language struct {
	ID         string `json:"id"`
	ISO6391    string `json:"iso639_1,omitempty"`
	ISO6392    string `json:"iso639_2,omitempty"`
	ISO6393    string `json:"iso639_3"`
	BCP47      string `json:"bcp47"`
	Name       string `json:"name"`
	NativeName string `json:"native_name"`
	Region     string `json:"region,omitempty"`
	Direction  string `json:"direction"`
	Script     string `json:"script,omitempty"`
	FontFamily string `json:"font_family,omitempty"`
	Status     string `json:"status"`
}

type BookInfo struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Testament      string `json:"testament"`
	CanonicalOrder int    `json:"canonical_order"`
	ChapterCount   int    `json:"chapter_count"`
}

type Verse struct {
	ID      string `json:"id"`
	Number  int    `json:"number"`
	Text    string `json:"text"`
	BookID  string `json:"book_id"`
	Chapter int    `json:"chapter"`
}

type Chapter struct {
	Translation Translation `json:"translation"`
	Book        BookInfo    `json:"book"`
	Chapter     int         `json:"chapter"`
	Verses      []Verse     `json:"verses"`
}

type Passage struct {
	Reference   string      `json:"reference"`
	Translation Translation `json:"translation"`
	Verses      []Verse     `json:"verses"`
}

type SearchResult struct {
	Translation Translation `json:"translation"`
	Book        BookInfo    `json:"book"`
	Chapter     int         `json:"chapter"`
	Verse       int         `json:"verse"`
	Text        string      `json:"text"`
}

// BibleProvider is the only interface used by Bible services and HTTP
// handlers. Implementations normalize upstream-specific wire formats here.
type BibleProvider interface {
	GetLanguages(context.Context) ([]Language, error)
	GetTranslations(context.Context, string, string) ([]Translation, error)
	GetTranslation(context.Context, string) (Translation, error)
	GetBooks(context.Context, string) ([]BookInfo, error)
	GetBook(context.Context, string, string) (BookInfo, error)
	GetChapter(context.Context, string, string, int) (Chapter, error)
	GetVerse(context.Context, string, string, int, int) (Verse, error)
	GetPassage(context.Context, string, string) (Passage, error)
	Search(context.Context, string, string, string, int) ([]SearchResult, error)
	GetCrossReferences(context.Context, string) ([]string, error)
	GetMetadata(context.Context, string) (Translation, error)
	HealthCheck(context.Context) error
}
