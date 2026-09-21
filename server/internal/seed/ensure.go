package seed

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"

	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/media"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/storage"
	"github.com/Teamthy/i-confess/internal/store"
)

// EnsureContent installs the canonical content library — the 39 categories,
// canonical confessions, canonical voices and collections — into whatever
// database it is pointed at.
//
// It is idempotent and safe to run on every boot, in every environment.
func EnsureContent(ctx context.Context, conn *db.DB) (categories, confessions int, err error) {
	content := store.NewContentStore(conn)
	audio := store.NewAudioStore(conn)

	// Ensure Canonical Collections
	existingCols, err := content.ListCollections(ctx, true)
	if err == nil {
		colSlugs := make(map[string]bool)
		for _, c := range existingCols {
			colSlugs[c.Slug] = true
		}
		for _, c := range []models.Collection{
			{Name: "Launch", Slug: "launch", Status: "published", SortOrder: 1, Description: "Launch collection — 39 canonical areas of life."},
			{Name: "Expanded", Slug: "expanded", Status: "published", SortOrder: 2, Description: "Expanded spiritual and personal confession catalogue."},
		} {
			if !colSlugs[c.Slug] {
				col := c
				_ = content.CreateCollection(ctx, &col)
			}
		}
	}

	// Ensure Canonical Voices
	existingVoices, err := audio.ListVoices(ctx)
	if err == nil {
		voiceNames := make(map[string]bool)
		for _, v := range existingVoices {
			voiceNames[v.Name] = true
		}
		canonicalVoices := []models.Voice{
			{Name: "Grace", Description: "Warm, calm professional narration voice.", Type: "professional", Provider: "i-confess", Gender: "female", Language: "en", Premium: false, Status: "active"},
			{Name: "David", Description: "Clear, grounded and reflective voice.", Type: "professional", Provider: "i-confess", Gender: "male", Language: "en", Premium: false, Status: "active"},
			{Name: "Faith", Description: "Uplifting, resonant expressive voice.", Type: "professional", Provider: "i-confess", Gender: "female", Language: "en", Premium: true, Status: "active"},
		}
		for _, cv := range canonicalVoices {
			if !voiceNames[cv.Name] {
				v := cv
				_ = audio.CreateVoice(ctx, &v)
			}
		}
	}

	existing, err := content.ListCategories(ctx, true)
	if err != nil {
		return 0, 0, fmt.Errorf("list categories: %w", err)
	}
	bySlug := make(map[string]string, len(existing))
	for _, c := range existing {
		bySlug[c.Slug] = c.ID
	}

	for i, canon := range CanonicalCategories {
		if _, ok := bySlug[canon.Slug]; ok {
			continue
		}
		c := &models.Category{
			Name: canon.Name, Slug: canon.Slug, Description: canon.Description,
			Icon: canon.Icon, Status: "published", SortOrder: i + 1,
		}
		if err := content.CreateCategory(ctx, c); err != nil {
			return categories, confessions, fmt.Errorf("create category %q: %w", canon.Slug, err)
		}
		bySlug[canon.Slug] = c.ID
		categories++
	}

	// Confessions are keyed on (category, title) rather than title alone, so
	// the same title under two categories stays distinct.
	have, err := content.ListConfessions(ctx, false)
	if err != nil {
		return categories, 0, fmt.Errorf("list confessions: %w", err)
	}
	present := make(map[string]bool, len(have))
	for _, c := range have {
		present[c.CategoryID+"\x00"+c.Title] = true
	}

	for _, canon := range CanonicalConfessions {
		catID, ok := bySlug[slugFor(canon.Category)]
		if !ok {
			// The corpus test rejects unknown categories, so reaching this
			// means the category insert above failed silently.
			return categories, confessions, fmt.Errorf("no category for confession %q", canon.Title)
		}
		if present[catID+"\x00"+canon.Title] {
			continue
		}

		c := &models.Confession{
			CategoryID: catID, Title: canon.Title,
			ShortText: canon.Short, MediumText: canon.Medium, LongText: canon.Long,
			Intensity: canon.Intensity, Language: "en", Status: "published",
			Author: "i-confess content team", Variants: variantsFor(canon), Scriptures: canon.Scriptures,
		}
		if err := content.CreateConfession(ctx, c); err != nil {
			return categories, confessions, fmt.Errorf("create confession %q: %w", canon.Title, err)
		}
		confessions++
	}

	if categories > 0 || confessions > 0 {
		log.Printf("content: ensured %d categories, %d confessions", categories, confessions)
	}
	return categories, confessions, nil
}

// EnsureCanonicalAudio guarantees one real object-storage asset for every
// canonical confession variant. A fresh production database cannot depend on
// the development-only Seed path: the catalogue would otherwise contain text
// rows with no playable audio, which is G-34. Missing renders are deterministic
// bootstrap fixtures, explicitly labeled as such, and can later be replaced by
// the normal rights-gated generation pipeline without changing the key shape.
//
// The object is uploaded before the database row is upserted. If a deployment
// is interrupted, the next boot retries the upload; a row is never marked ready
// for an object that has not successfully reached storage.
func EnsureCanonicalAudio(ctx context.Context, conn *db.DB, objects storage.ObjectStorage) (int, error) {
	if objects == nil {
		return 0, fmt.Errorf("canonical audio requires object storage")
	}
	content := store.NewContentStore(conn)
	audio := store.NewAudioStore(conn)

	voices, err := audio.ListVoices(ctx)
	if err != nil {
		return 0, fmt.Errorf("list canonical voices: %w", err)
	}
	var voice *models.Voice
	for i := range voices {
		if voices[i].Name == "Grace" && voices[i].Status == "active" {
			voice = &voices[i]
			break
		}
	}
	if voice == nil {
		return 0, fmt.Errorf("canonical voice Grace is missing or inactive")
	}

	categories, err := content.ListCategories(ctx, true)
	if err != nil {
		return 0, fmt.Errorf("list canonical categories: %w", err)
	}
	categoryIDs := make(map[string]string, len(categories))
	for _, category := range categories {
		categoryIDs[category.Name] = category.ID
	}

	storedConfessions, err := content.ListConfessions(ctx, false)
	if err != nil {
		return 0, fmt.Errorf("list canonical confessions: %w", err)
	}
	byKey := make(map[string]models.Confession, len(storedConfessions))
	for _, confession := range storedConfessions {
		byKey[confession.CategoryID+"\x00"+confession.Title] = confession
	}

	created := 0
	for _, canonical := range CanonicalConfessions {
		categoryID := categoryIDs[canonical.Category]
		if categoryID == "" {
			return created, fmt.Errorf("canonical category %q is missing", canonical.Category)
		}
		confession, ok := byKey[categoryID+"\x00"+canonical.Title]
		if !ok {
			return created, fmt.Errorf("canonical confession %q is missing", canonical.Title)
		}
		variants, err := content.Variants(ctx, confession.ID)
		if err != nil {
			return created, fmt.Errorf("list variants for %q: %w", canonical.Title, err)
		}
		if len(variants) == 0 {
			return created, fmt.Errorf("canonical confession %q has no duration variants", canonical.Title)
		}
		version, err := content.EnsureVersion(ctx, confession.ID, confession.Title,
			confession.ShortText, confession.MediumText, confession.LongText, confession.Language,
			"canonical-audio-bootstrap")
		if err != nil {
			return created, fmt.Errorf("snapshot %q: %w", canonical.Title, err)
		}
		for _, variant := range variants {
			key := storage.AudioKeyFor(confession.ID, variant.ID, voice.ID, confession.Language, version.VersionNumber)
			var assetID, storedKey string
			err := conn.QueryRowContext(ctx,
				`SELECT id, storage_key FROM audio_assets
				 WHERE content_id=? AND content_version_id=? AND voice_id=? AND variant_id=?
				   AND asset_type='stream' AND quality_tier='standard' AND deleted_at IS NULL`,
				confession.ID, version.ID, voice.ID, variant.ID).Scan(&assetID, &storedKey)
			if err == nil {
				exists, existsErr := objects.Exists(ctx, storedKey)
				if existsErr != nil {
					return created, fmt.Errorf("check canonical audio %q: %w", canonical.Title, existsErr)
				}
				if exists && storedKey == key {
					continue
				}
			} else if !errors.Is(err, sql.ErrNoRows) {
				return created, fmt.Errorf("find canonical audio %q: %w", canonical.Title, err)
			}

			seconds := variant.DurationSeconds
			if seconds <= 0 {
				seconds = 1
			}
			data := media.ToneBytes(seconds)
			if err := objects.Upload(ctx, key, data, map[string]string{
				"confession_id":      confession.ID,
				"content_version_id": version.ID,
				"voice_id":           voice.ID,
				"audio_source":       "bootstrap_fixture",
			}); err != nil {
				return created, fmt.Errorf("upload canonical audio %q: %w", canonical.Title, err)
			}
			asset := &models.AudioAsset{
				ID: assetID, ConfessionID: confession.ID, VariantID: variant.ID,
				VoiceID: voice.ID, URL: key, DurationSeconds: variant.DurationSeconds,
				SizeBytes: int64(len(data)), Status: "ready", ContentVersionID: version.ID,
				AudioSource: "bootstrap_fixture",
			}
			if err := audio.UpsertAsset(ctx, asset); err != nil {
				return created, fmt.Errorf("record canonical audio %q: %w", canonical.Title, err)
			}
			created++
		}
	}
	return created, nil
}

// slugFor maps a canonical category name to its slug. The corpus stores names
// because they read better in review; the database keys on slugs.
func slugFor(name string) string {
	for _, c := range CanonicalCategories {
		if c.Name == name {
			return c.Slug
		}
	}
	return ""
}

// variantsFor is the duration ladder every confession is published with. The
// session engine picks the rung that best fills a requested duration, so all
// four must exist; a missing rung silently under-fills the queue.
func variantsFor(c CanonicalConfession) []models.ConfessionVariant {
	defs := []struct {
		label   string
		seconds int
	}{{"30s", 30}, {"1m", 60}, {"3m", 180}, {"5m", 300}}

	out := make([]models.ConfessionVariant, 0, len(defs))
	for i, d := range defs {
		out = append(out, models.ConfessionVariant{
			Label: d.label, DurationSeconds: d.seconds, SortOrder: i + 1,
		})
	}
	return out
}
