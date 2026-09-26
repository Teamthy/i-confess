package bible

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Teamthy/i-confess/internal/db"
)

// LocalBibleProvider reads only translations explicitly active for API
// exposure from the local corpus. It is both the low-latency mirror and the
// outage fallback; it never assumes that the presence of text grants a right.
type LocalBibleProvider struct{ DB *db.DB }

const translationColumns = `id,provider,COALESCE(provider_translation_id,id),name,abbrev,language,language_name,
 COALESCE(locale,''),COALESCE(country,''),COALESCE(dialect,''),COALESCE(publisher,''),COALESCE(description,''),
 COALESCE(copyright_text,''),licence,COALESCE(licence_url,''),public_domain,commercial_use,redistribution_allowed,
 modification_allowed,audio_allowed,offline_allowed,copy_allowed,share_allowed,search_index_allowed,api_exposure_allowed,
 attribution_required,COALESCE(attribution_text,''),COALESCE(source_url,''),COALESCE(source_version,''),
 COALESCE(import_version,''),COALESCE(content_hash,''),status,coverage,book_count,chapter_count,verse_count,
 COALESCE((SELECT direction FROM bible_languages WHERE id=bible_versions.language),'ltr')`

func (p *LocalBibleProvider) scanTranslation(row interface{ Scan(...any) error }) (Translation, error) {
	var t Translation
	err := row.Scan(&t.ID, &t.Provider, &t.ProviderTranslationID, &t.Name, &t.Abbreviation, &t.Language, &t.LanguageName,
		&t.Locale, &t.Country, &t.Dialect, &t.Publisher, &t.Description, &t.Copyright, &t.License, &t.LicenseURL,
		&t.PublicDomain, &t.CommercialUse, &t.RedistributionAllowed, &t.ModificationAllowed, &t.AudioAllowed, &t.OfflineAllowed,
		&t.CopyAllowed, &t.ShareAllowed, &t.SearchIndexAllowed, &t.APIExposureAllowed, &t.AttributionRequired, &t.AttributionText,
		&t.SourceURL, &t.SourceVersion, &t.ImportVersion, &t.ContentHash, &t.Status, &t.Coverage, &t.BookCount, &t.ChapterCount, &t.VerseCount, &t.Direction)
	if err != nil {
		return Translation{}, err
	}
	return t, nil
}

func (p *LocalBibleProvider) translation(ctx context.Context, id string) (Translation, error) {
	row := p.DB.QueryRowContext(ctx, `SELECT `+translationColumns+` FROM bible_versions WHERE lower(id)=lower(?) AND deleted_at IS NULL AND status='active' AND api_exposure_allowed=TRUE`, id)
	t, err := p.scanTranslation(row)
	if err == sql.ErrNoRows {
		return Translation{}, ErrNotFound
	}
	if err != nil {
		return Translation{}, err
	}
	return t, nil
}
func (p *LocalBibleProvider) GetTranslation(ctx context.Context, id string) (Translation, error) {
	return p.translation(ctx, id)
}

// GetTranslationRecord is reserved for admin and provider-routing decisions; it
// intentionally returns pending metadata as well as active public translations.
func (p *LocalBibleProvider) GetTranslationRecord(ctx context.Context, id string) (Translation, error) {
	row := p.DB.QueryRowContext(ctx, `SELECT `+translationColumns+` FROM bible_versions WHERE lower(id)=lower(?) AND deleted_at IS NULL`, id)
	t, err := p.scanTranslation(row)
	if err == sql.ErrNoRows {
		return Translation{}, ErrNotFound
	}
	return t, err
}

func (p *LocalBibleProvider) GetApprovedTranslationsByProvider(ctx context.Context, provider string) ([]Translation, error) {
	rows, err := p.DB.QueryContext(ctx, `SELECT `+translationColumns+` FROM bible_versions WHERE deleted_at IS NULL AND status='active' AND api_exposure_allowed=TRUE AND provider=? ORDER BY sort_order,name`, provider)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Translation{}
	for rows.Next() {
		t, e := p.scanTranslation(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func (p *LocalBibleProvider) GetMetadata(ctx context.Context, id string) (Translation, error) {
	return p.GetTranslation(ctx, id)
}

func (p *LocalBibleProvider) GetTranslations(ctx context.Context, language, query string) ([]Translation, error) {
	rows, err := p.DB.QueryContext(ctx, `SELECT `+translationColumns+` FROM bible_versions WHERE deleted_at IS NULL AND status='active' AND api_exposure_allowed=TRUE AND ($1='' OR lower(language)=lower($1) OR lower(language_name)=lower($1)) AND ($2='' OR name ILIKE '%'||$2||'%' OR abbrev ILIKE '%'||$2||'%' OR language_name ILIKE '%'||$2||'%') ORDER BY sort_order,name`, strings.TrimSpace(language), strings.TrimSpace(query))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Translation{}
	for rows.Next() {
		t, e := p.scanTranslation(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func (p *LocalBibleProvider) GetLanguages(ctx context.Context) ([]Language, error) {
	rows, err := p.DB.QueryContext(ctx, `SELECT l.id,COALESCE(l.iso639_1,''),COALESCE(l.iso639_2,''),l.iso639_3,l.bcp47,l.name,l.native_name,COALESCE(l.region,''),l.direction,COALESCE(l.script,''),COALESCE(l.font_family,''),l.status FROM bible_languages l WHERE l.status='active' AND EXISTS (SELECT 1 FROM bible_versions v WHERE v.language=l.id AND v.status='active' AND v.api_exposure_allowed=TRUE AND v.deleted_at IS NULL) ORDER BY l.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Language{}
	for rows.Next() {
		var l Language
		if err := rows.Scan(&l.ID, &l.ISO6391, &l.ISO6392, &l.ISO6393, &l.BCP47, &l.Name, &l.NativeName, &l.Region, &l.Direction, &l.Script, &l.FontFamily, &l.Status); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
func (p *LocalBibleProvider) GetBooks(ctx context.Context, translationID string) ([]BookInfo, error) {
	if _, err := p.translation(ctx, translationID); err != nil {
		return nil, err
	}
	rows, err := p.DB.QueryContext(ctx, `SELECT book_id,display_name,testament,canonical_order,chapter_count FROM bible_translation_books WHERE version_id=? AND has_text=TRUE ORDER BY canonical_order`, translationID)
	if err != nil {
		return nil, err
	}
	out := []BookInfo{}
	for rows.Next() {
		var b BookInfo
		if err := rows.Scan(&b.ID, &b.Name, &b.Testament, &b.CanonicalOrder, &b.ChapterCount); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(out) > 0 {
		return out, nil
	}
	rows, err = p.DB.QueryContext(ctx, `SELECT book_id,book_name,testament,canonical_order,count(DISTINCT chapter) FROM bible_verses WHERE version_id=? AND deleted_at IS NULL GROUP BY book_id,book_name,testament,canonical_order ORDER BY canonical_order`, translationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out = []BookInfo{}
	for rows.Next() {
		var b BookInfo
		if err := rows.Scan(&b.ID, &b.Name, &b.Testament, &b.CanonicalOrder, &b.ChapterCount); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
func (p *LocalBibleProvider) GetBook(ctx context.Context, translationID, bookID string) (BookInfo, error) {
	id, ok := ParseBook(bookID)
	if !ok {
		return BookInfo{}, ErrNotFound
	}
	var b BookInfo
	err := p.DB.QueryRowContext(ctx, `SELECT book_id,book_name,testament,canonical_order,count(DISTINCT chapter) FROM bible_verses WHERE version_id=? AND book_id=? AND deleted_at IS NULL GROUP BY book_id,book_name,testament,canonical_order`, translationID, id).Scan(&b.ID, &b.Name, &b.Testament, &b.CanonicalOrder, &b.ChapterCount)
	if err == sql.ErrNoRows {
		return BookInfo{}, ErrNotFound
	}
	if err != nil {
		return BookInfo{}, err
	}
	if _, err = p.translation(ctx, translationID); err != nil {
		return BookInfo{}, err
	}
	return b, nil
}
func (p *LocalBibleProvider) GetChapter(ctx context.Context, translationID, bookID string, chapterNumber int) (Chapter, error) {
	if chapterNumber < 1 {
		return Chapter{}, ErrNotFound
	}
	translation, err := p.translation(ctx, translationID)
	if err != nil {
		return Chapter{}, err
	}
	book, err := p.GetBook(ctx, translationID, bookID)
	if err != nil {
		return Chapter{}, err
	}
	rows, err := p.DB.QueryContext(ctx, `SELECT verse,text FROM bible_verses WHERE version_id=? AND book_id=? AND chapter=? AND deleted_at IS NULL ORDER BY verse`, translationID, book.ID, chapterNumber)
	if err != nil {
		return Chapter{}, err
	}
	defer rows.Close()
	verses := []Verse{}
	for rows.Next() {
		var v Verse
		if err := rows.Scan(&v.Number, &v.Text); err != nil {
			return Chapter{}, err
		}
		v.ID = CanonicalVerseID(book.ID, chapterNumber, v.Number)
		v.BookID = book.ID
		v.Chapter = chapterNumber
		verses = append(verses, v)
	}
	if err := rows.Err(); err != nil {
		return Chapter{}, err
	}
	if len(verses) == 0 {
		return Chapter{}, ErrNotFound
	}
	return Chapter{Translation: translation, Book: book, Chapter: chapterNumber, Verses: verses}, nil
}
func (p *LocalBibleProvider) GetVerse(ctx context.Context, t, b string, c, v int) (Verse, error) {
	chapter, err := p.GetChapter(ctx, t, b, c)
	if err != nil {
		return Verse{}, err
	}
	for _, verse := range chapter.Verses {
		if verse.Number == v {
			return verse, nil
		}
	}
	return Verse{}, ErrNotFound
}
func (p *LocalBibleProvider) GetPassage(ctx context.Context, t, raw string) (Passage, error) {
	ref, err := ParseReference(raw)
	if err != nil {
		return Passage{}, err
	}
	chapter, err := p.GetChapter(ctx, t, ref.Book, ref.Chapter)
	if err != nil {
		return Passage{}, err
	}
	verses := chapter.Verses
	if ref.StartVerse > 0 {
		verses = []Verse{}
		for _, verse := range chapter.Verses {
			if verse.Number >= ref.StartVerse && verse.Number <= ref.EndVerse {
				verses = append(verses, verse)
			}
		}
	}
	if len(verses) == 0 {
		return Passage{}, ErrNotFound
	}
	return Passage{Reference: strings.TrimSpace(raw), Translation: chapter.Translation, Verses: verses}, nil
}
func (p *LocalBibleProvider) Search(ctx context.Context, query, translationID, bookID string, limit int) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" || len(query) > 200 {
		return nil, fmt.Errorf("invalid search query")
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	bookFilter := ""
	if bookID != "" {
		var ok bool
		bookFilter, ok = ParseBook(bookID)
		if !ok {
			return nil, ErrNotFound
		}
	}
	rows, err := p.DB.QueryContext(ctx, `SELECT v.version_id,v.book_id,v.book_name,v.testament,v.canonical_order,v.chapter,v.verse,v.text
		FROM bible_search_documents d JOIN bible_verses v ON v.id=d.verse_id JOIN bible_versions t ON t.id=d.version_id
		WHERE d.deleted_at IS NULL AND v.deleted_at IS NULL AND t.deleted_at IS NULL AND t.status='active'
		AND t.api_exposure_allowed=TRUE AND t.search_index_allowed=TRUE
		AND (?='' OR v.version_id=?) AND (?='' OR v.book_id=?)
		AND d.document @@ plainto_tsquery('simple',?)
		ORDER BY ts_rank(d.document,plainto_tsquery('simple',?)) DESC,v.canonical_order,v.chapter,v.verse LIMIT ?`,
		translationID, translationID, bookFilter, bookFilter, query, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SearchResult{}
	for rows.Next() {
		var version string
		var b BookInfo
		var v Verse
		var chapter int
		if err := rows.Scan(&version, &b.ID, &b.Name, &b.Testament, &b.CanonicalOrder, &chapter, &v.Number, &v.Text); err != nil {
			return nil, err
		}
		b.ChapterCount = 0
		translation, e := p.translation(ctx, version)
		if e != nil {
			return nil, e
		}
		out = append(out, SearchResult{Translation: translation, Book: b, Chapter: chapter, Verse: v.Number, Text: v.Text})
	}
	return out, rows.Err()
}
func (p *LocalBibleProvider) GetCrossReferences(ctx context.Context, reference string) ([]string, error) {
	ref, err := ParseReference(reference)
	if err != nil {
		return nil, ErrNotFound
	}
	verse := ref.StartVerse
	if verse == 0 {
		verse = 1
	}
	rows, err := p.DB.QueryContext(ctx, `SELECT target_reference FROM bible_cross_references WHERE source_book_id=? AND source_chapter=? AND source_verse_start<=? AND source_verse_end>=? AND reviewed_at IS NOT NULL AND deleted_at IS NULL ORDER BY target_reference`, ref.Book, ref.Chapter, verse, verse)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var target string
		if err := rows.Scan(&target); err != nil {
			return nil, err
		}
		out = append(out, target)
	}
	return out, rows.Err()
}
func (p *LocalBibleProvider) HealthCheck(ctx context.Context) error { return p.DB.PingContext(ctx) }
