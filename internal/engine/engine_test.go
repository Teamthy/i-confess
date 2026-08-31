package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/store"
)

// setup builds an in-memory-ish (temp-file) database with a voice, two categories,
// and confessions with audio, then wires the engine.
func setup(t *testing.T) (*Engine, *store.ContentStore, *store.AudioStore, *store.UserStore) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	content := store.NewContentStore(conn)
	audio := store.NewAudioStore(conn)
	users := store.NewUserStore(conn)
	ctx := context.Background()

	// free user
	u, err := users.Create(ctx, "u@test.com", "hash", "U", "UTC")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	_ = u

	voice := &models.Voice{Name: "V", Type: "professional", Language: "en", Status: "active"}
	if err := audio.CreateVoice(ctx, voice); err != nil {
		t.Fatalf("create voice: %v", err)
	}

	for _, cat := range []string{"healing", "faith"} {
		c := &models.Category{Name: cat, Slug: cat, Status: "published"}
		if err := content.CreateCategory(ctx, c); err != nil {
			t.Fatalf("create category: %v", err)
		}
		// two confessions per category, each with a 60s and 300s variant + audio
		for i := 0; i < 2; i++ {
			conf := &models.Confession{
				CategoryID: c.ID, Title: cat + "-conf", MediumText: "text",
				Status: "published", Language: "en",
				Variants: []models.ConfessionVariant{
					{Label: "1m", DurationSeconds: 60},
					{Label: "5m", DurationSeconds: 300},
				},
			}
			if err := content.CreateConfession(ctx, conf); err != nil {
				t.Fatalf("create confession: %v", err)
			}
			for _, v := range conf.Variants {
				a := &models.AudioAsset{
					ConfessionID: conf.ID, VariantID: v.ID, VoiceID: voice.ID,
					URL: "/media/x.wav", DurationSeconds: v.DurationSeconds, Status: "ready",
				}
				if err := audio.UpsertAsset(ctx, a); err != nil {
					t.Fatalf("upsert asset: %v", err)
				}
			}
		}
	}

	return New(content, audio, users), content, audio, users
}

func TestBuildFillsDurationAndCycles(t *testing.T) {
	ctx := context.Background()
	e, content, _, users := setup(t)

	cats, _ := content.ListCategories(ctx, true)
	u, _, _ := users.ByEmail(ctx, "u@test.com")

	sess, err := e.Build(ctx, Request{
		UserID:          u.ID,
		CategoryIDs:     []string{cats[0].ID, cats[1].ID},
		DurationSeconds: 600,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	total := 0
	for _, it := range sess.Items {
		total += it.DurationSeconds
	}
	if total != 600 {
		t.Errorf("expected 600s total, got %d (items=%d)", total, len(sess.Items))
	}
	if sess.VoiceID == "" {
		t.Error("expected a resolved voice id")
	}
}

func TestBuildNoContent(t *testing.T) {
	ctx := context.Background()
	e, content, _, users := setup(t)

	// A category with no confessions → no eligible content.
	c := &models.Category{Name: "empty", Slug: "empty", Status: "published"}
	_ = content.CreateCategory(ctx, c)

	u, _, _ := users.ByEmail(ctx, "u@test.com")
	if _, err := e.Build(ctx, Request{UserID: u.ID, CategoryIDs: []string{c.ID}, DurationSeconds: 300}); err == nil {
		t.Error("expected ErrNoContent for a category with no content")
	}
}
