// Package seed populates a fresh database with a representative MVP content
// library: collections, categories, confessions (with variants + scripture
// references), a default voice, local placeholder audio, and a demo admin.
package seed

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Teamthy/i-confess/internal/db"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/media"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/storage"
	"github.com/Teamthy/i-confess/internal/store"
)

const mediaDir = "data/media"

// Seed runs idempotently: if the database already has categories, it returns early.
// Seed populates a fresh database. The signer receives placeholder audio so the
// dev environment exercises the same keyed, signed delivery path as production.
func Seed(db *db.DB, signer storage.ObjectStorage) error {
	bg := context.Background()
	content := store.NewContentStore(db)
	cats, err := content.ListCategories(bg, true)
	if err != nil {
		return err
	}
	if len(cats) > 0 {
		return nil // already seeded
	}

	audio := store.NewAudioStore(db)
	users := store.NewUserStore(db)

	// ---- Collections ----
	for _, c := range []models.Collection{
		{Name: "Launch", Slug: "launch", Status: "published", SortOrder: 1, Description: "Launch collection — the categories seeded at first run."},
		{Name: "Expanded", Slug: "expanded", Status: "published", SortOrder: 2, Description: "Principal expanded content architecture."},
	} {
		if err := content.CreateCollection(bg, &c); err != nil {
			return err
		}
	}

	// ---- Categories ----
	// Every canonical category is seeded, not a subset. The product decision
	// (D3) is that all 39 exist at launch, so an explore screen never shows a
	// category the backend does not know about. Deriving from
	// CanonicalCategories means the seed cannot drift from it: there is no
	// second list to keep in sync.
	categorySeeds := make([]struct{ name, slug, desc, icon string }, len(CanonicalCategories))
	for i, c := range CanonicalCategories {
		categorySeeds[i] = struct{ name, slug, desc, icon string }{c.Name, c.Slug, c.Description, c.Icon}
	}

	var categoryIDs []string
	for i, cs := range categorySeeds {
		c := &models.Category{
			Name: cs.name, Slug: cs.slug, Description: cs.desc, Icon: cs.icon,
			Status: "published", SortOrder: i + 1,
		}
		if err := content.CreateCategory(bg, c); err != nil {
			return err
		}
		categoryIDs = append(categoryIDs, c.ID)
	}

	// Attach every category to both collections.
	cols, _ := content.ListCollections(bg, true)
	for _, col := range cols {
		for i, cid := range categoryIDs {
			_ = content.AddCategoryToCollection(bg, col.ID, cid, i)
		}
	}

	// ---- Voice ----
	voice := &models.Voice{
		Name: "Grace", Description: "Warm, calm professional narration voice.",
		Type: "professional", Provider: "i-confess", Gender: "female",
		Language: "en", Premium: false, Status: "active",
	}
	if err := audio.CreateVoice(bg, voice); err != nil {
		return err
	}

	// ---- Confessions ----
	// The library comes from the canonical corpus, which is the same source
	// EnsureContent installs in production. There is deliberately no second
	// copy of the text here.

	// Duration variants used across all confessions.
	variantDefs := []struct {
		label   string
		seconds int
	}{{"30s", 30}, {"1m", 60}, {"3m", 180}, {"5m", 300}}

	catByName := map[string]string{}
	for i, cs := range categorySeeds {
		catByName[cs.name] = categoryIDs[i]
	}

	for _, s := range CanonicalConfessions {
		catID := catByName[s.Category]
		var variants []models.ConfessionVariant
		for _, vd := range variantDefs {
			variants = append(variants, models.ConfessionVariant{Label: vd.label, DurationSeconds: vd.seconds})
		}
		c := &models.Confession{
			CategoryID: catID, Title: s.Title, ShortText: s.Short, MediumText: s.Medium, LongText: s.Long,
			Intensity: s.Intensity, Language: "en", Status: "published", Author: "i-confess content team",
			Variants: variants, Scriptures: s.Scriptures,
		}
		if err := content.CreateConfession(bg, c); err != nil {
			return err
		}

		// Generate placeholder audio for each variant and attach it to the voice.
		for _, v := range c.Variants {
			// Store the canonical KEY, never a URL. The API mints a signed,
			// expiring link per request (PRD S11).
			key := storage.AudioKeyFor(c.ID, v.ID, voice.ID, "en", 1)
			if err := signer.Upload(bg, key, media.ToneBytes(v.DurationSeconds), map[string]string{
				"confession_id": c.ID, "voice_id": voice.ID, "language": "en",
			}); err != nil {
				return fmt.Errorf("store placeholder audio: %w", err)
			}
			asset := &models.AudioAsset{
				ConfessionID: c.ID, VariantID: v.ID, VoiceID: voice.ID,
				URL: key, DurationSeconds: v.DurationSeconds, Status: "ready",
			}
			if err := audio.UpsertAsset(bg, asset); err != nil {
				return err
			}
		}
	}

	// ---- Demo admin + demo user ----
	// Demo credentials go through the same password policy as real
	// registrations. Seeding a weak password is how weak passwords survive an
	// audit: the check is on the register handler, and this code path never
	// touches it. Validating here means a future weak demo password fails at
	// seed time instead of quietly creating an account an attacker can guess.
	adminHash, err := hashDemoPassword("Admin!ChangeMe-2026")
	if err != nil {
		return err
	}
	admin, err := users.Create(bg, "admin@iconfess.dev", adminHash, "Admin", "UTC")
	if err != nil {
		return err
	}
	if err := users.SetAdminRole(bg, admin.ID, "super_admin"); err != nil {
		return err
	}

	userHash, err := hashDemoPassword("Demo!ChangeMe-2026")
	if err != nil {
		return err
	}
	if _, err := users.Create(bg, "demo@iconfess.dev", userHash, "Demo User", "Africa/Lagos"); err != nil {
		return err
	}

	log.Printf("seed: created %d categories, %d confessions, 1 voice, demo admin + user",
		len(categorySeeds), len(CanonicalConfessions))
	return nil
}

// EnsureMediaDir creates the media directory if needed (used by tests/tools).
func EnsureMediaDir() error {
	return os.MkdirAll(mediaDir, 0o755)
}

// hashDemoPassword hashes a demo credential after checking it against the
// production password policy. A seed that installs an account no one could
// register is a seed that has drifted from the rules the API enforces.
func hashDemoPassword(pw string) (string, error) {
	if err := auth.ValidatePassword(pw); err != nil {
		return "", fmt.Errorf("seed demo password rejected by policy: %w", err)
	}
	return auth.HashPassword(pw)
}
