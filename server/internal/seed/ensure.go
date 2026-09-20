package seed

import (
	"context"
	"fmt"
	"log"

	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/models"
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
