package bible

import (
	"context"
	"strings"
)

// ReviewedProvider composes the audited PostgreSQL corpus with HelloAO only
// for translations present in the reviewed registry. Provider discovery never
// grants rights. If HelloAO is down, already-approved local editions continue
// to serve normally; a remote source outage never causes an unreviewed fallback.
type ReviewedProvider struct {
	Local  *LocalBibleProvider
	Remote BibleProvider
}

func (p *ReviewedProvider) record(ctx context.Context, id string) (Translation, error) {
	record, err := p.Local.GetTranslationRecord(ctx, id)
	if err != nil {
		return Translation{}, err
	}
	if !strings.EqualFold(record.Status, "active") || !record.APIExposureAllowed {
		return Translation{}, ErrNotFound
	}
	return record, nil
}

func (p *ReviewedProvider) remoteRecord(ctx context.Context, record Translation) (Translation, error) {
	if p.Remote == nil || record.ContentHash == "" {
		return Translation{}, ErrUnavailable
	}
	remote, err := p.Remote.GetTranslation(ctx, record.ProviderTranslationID)
	if err != nil {
		return Translation{}, err
	}
	// An upstream revision cannot silently replace the exact edition that was
	// reviewed. The provider advertises the digest of its complete translation;
	// any drift requires a new pinned edition and another rights decision.
	if !strings.EqualFold(remote.ContentHash, record.ContentHash) {
		return Translation{}, ErrUnavailable
	}
	remote.ID = record.ID
	remote.Provider = record.Provider
	remote.ProviderTranslationID = record.ProviderTranslationID
	remote.Name = record.Name
	remote.Abbreviation = record.Abbreviation
	remote.Language = record.Language
	remote.LanguageName = record.LanguageName
	remote.Locale = record.Locale
	remote.Country = record.Country
	remote.Dialect = record.Dialect
	remote.Publisher = record.Publisher
	remote.Description = record.Description
	remote.Copyright = record.Copyright
	remote.License = record.License
	remote.LicenseURL = record.LicenseURL
	remote.PublicDomain = record.PublicDomain
	remote.CommercialUse = record.CommercialUse
	remote.RedistributionAllowed = record.RedistributionAllowed
	remote.ModificationAllowed = record.ModificationAllowed
	remote.AudioAllowed = record.AudioAllowed
	remote.OfflineAllowed = record.OfflineAllowed
	remote.CopyAllowed = record.CopyAllowed
	remote.ShareAllowed = record.ShareAllowed
	remote.SearchIndexAllowed = record.SearchIndexAllowed
	remote.APIExposureAllowed = record.APIExposureAllowed
	remote.AttributionRequired = record.AttributionRequired
	remote.AttributionText = record.AttributionText
	remote.SourceURL = record.SourceURL
	remote.SourceVersion = record.SourceVersion
	remote.ImportVersion = record.ImportVersion
	remote.ContentHash = record.ContentHash
	remote.Status = record.Status
	remote.Direction = record.Direction
	remote.Coverage = record.Coverage
	return remote, nil
}

func (p *ReviewedProvider) GetLanguages(ctx context.Context) ([]Language, error) {
	local, err := p.Local.GetTranslations(ctx, "", "")
	if err != nil {
		return nil, err
	}
	out := make([]Language, 0)
	seen := map[string]bool{}
	add := func(t Translation) {
		if seen[t.Language] {
			return
		}
		seen[t.Language] = true
		out = append(out, Language{ID: t.Language, ISO6391: t.Language, BCP47: t.Locale, ISO6393: t.Language, Name: t.LanguageName, NativeName: t.LanguageName, Direction: t.Direction, Status: "active"})
	}
	for _, t := range local {
		if t.Provider != "helloao" {
			add(t)
		}
	}
	if p.Remote != nil {
		remoteRows, err := p.Local.GetApprovedTranslationsByProvider(ctx, "helloao")
		if err == nil {
			for _, row := range remoteRows {
				if _, err := p.remoteRecord(ctx, row); err == nil {
					add(row)
				}
			}
		}
	}
	return out, nil
}

func (p *ReviewedProvider) GetTranslations(ctx context.Context, language, query string) ([]Translation, error) {
	local, err := p.Local.GetTranslations(ctx, language, query)
	if err != nil {
		return nil, err
	}
	out := make([]Translation, 0, len(local))
	for _, item := range local {
		if item.Provider != "helloao" { // never list a remote edition from stale local metadata
			out = append(out, item)
		}
	}
	if p.Remote == nil {
		return out, nil
	}
	rows, err := p.Local.GetApprovedTranslationsByProvider(ctx, "helloao")
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if language != "" && !strings.EqualFold(row.Language, language) && !strings.EqualFold(row.LanguageName, language) {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(row.Name+" "+row.Abbreviation+" "+row.LanguageName), strings.ToLower(query)) {
			continue
		}
		if metadata, remoteErr := p.remoteRecord(ctx, row); remoteErr == nil {
			out = append(out, metadata)
		}
	}
	return out, nil
}

func (p *ReviewedProvider) GetTranslation(ctx context.Context, id string) (Translation, error) {
	row, err := p.record(ctx, id)
	if err != nil {
		return Translation{}, err
	}
	if row.Provider == "helloao" {
		return p.remoteRecord(ctx, row)
	}
	return row, nil
}

func (p *ReviewedProvider) GetMetadata(ctx context.Context, id string) (Translation, error) {
	return p.GetTranslation(ctx, id)
}
func (p *ReviewedProvider) GetBooks(ctx context.Context, id string) ([]BookInfo, error) {
	row, err := p.record(ctx, id)
	if err != nil {
		return nil, err
	}
	if row.Provider != "helloao" {
		return p.Local.GetBooks(ctx, id)
	}
	if _, err := p.remoteRecord(ctx, row); err != nil {
		return nil, err
	}
	return p.Remote.GetBooks(ctx, row.ProviderTranslationID)
}
func (p *ReviewedProvider) GetBook(ctx context.Context, id, book string) (BookInfo, error) {
	row, err := p.record(ctx, id)
	if err != nil {
		return BookInfo{}, err
	}
	if row.Provider != "helloao" {
		return p.Local.GetBook(ctx, id, book)
	}
	if _, err := p.remoteRecord(ctx, row); err != nil {
		return BookInfo{}, err
	}
	return p.Remote.GetBook(ctx, row.ProviderTranslationID, book)
}
func (p *ReviewedProvider) GetChapter(ctx context.Context, id, book string, chapter int) (Chapter, error) {
	row, err := p.record(ctx, id)
	if err != nil {
		return Chapter{}, err
	}
	if row.Provider != "helloao" {
		return p.Local.GetChapter(ctx, id, book, chapter)
	}
	t, err := p.remoteRecord(ctx, row)
	if err != nil {
		return Chapter{}, err
	}
	out, err := p.Remote.GetChapter(ctx, row.ProviderTranslationID, book, chapter)
	if err != nil {
		return Chapter{}, err
	}
	if !strings.EqualFold(out.Translation.ContentHash, row.ContentHash) {
		return Chapter{}, ErrUnavailable
	}
	out.Translation = t
	return out, nil
}
func (p *ReviewedProvider) GetVerse(ctx context.Context, id, book string, chapter, verse int) (Verse, error) {
	ch, err := p.GetChapter(ctx, id, book, chapter)
	if err != nil {
		return Verse{}, err
	}
	for _, v := range ch.Verses {
		if v.Number == verse {
			return v, nil
		}
	}
	return Verse{}, ErrNotFound
}
func (p *ReviewedProvider) GetPassage(ctx context.Context, id, ref string) (Passage, error) {
	row, err := p.record(ctx, id)
	if err != nil {
		return Passage{}, err
	}
	if row.Provider != "helloao" {
		return p.Local.GetPassage(ctx, id, ref)
	}
	t, err := p.remoteRecord(ctx, row)
	if err != nil {
		return Passage{}, err
	}
	out, err := p.Remote.GetPassage(ctx, row.ProviderTranslationID, ref)
	if err != nil {
		return Passage{}, err
	}
	if !strings.EqualFold(out.Translation.ContentHash, row.ContentHash) {
		return Passage{}, ErrUnavailable
	}
	out.Translation = t
	return out, nil
}
func (p *ReviewedProvider) Search(ctx context.Context, q, id, book string, limit int) ([]SearchResult, error) {
	if id != "" {
		row, err := p.record(ctx, id)
		if err != nil {
			return nil, err
		}
		if row.Provider == "helloao" {
			return nil, ErrUnavailable
		}
		return p.Local.Search(ctx, q, id, book, limit)
	}
	return p.Local.Search(ctx, q, "", book, limit)
}
func (p *ReviewedProvider) GetCrossReferences(ctx context.Context, ref string) ([]string, error) {
	return p.Local.GetCrossReferences(ctx, ref)
}
func (p *ReviewedProvider) HealthCheck(ctx context.Context) error {
	if p.Remote != nil {
		if checker, ok := p.Remote.(interface{ HealthCheck(context.Context) error }); ok {
			return checker.HealthCheck(ctx)
		}
	}
	return p.Local.HealthCheck(ctx)
}
