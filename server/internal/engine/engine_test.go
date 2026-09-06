package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/Teamthy/i-confess/internal/db/dbtest"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/store"
)

// setup builds an in-memory-ish (temp-file) database with a voice, two categories,
// and confessions with audio, then wires the engine.
func setup(t *testing.T) (*Engine, *store.ContentStore, *store.AudioStore, *store.UserStore) {
	t.Helper()
	conn := dbtest.New(t)

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

// The fixture gives every confession a 60s and a 300s variant. Asking for 100
// seconds is the interesting case: only the 60s variant fits, so the deepest
// under-fill is a single 60s item and the only overshoot candidate is 300s.
// The strategies therefore disagree loudly, which is what makes this a useful
// probe that the strategy actually reaches the packer.
func TestBuildHonoursTheRequestedStrategy(t *testing.T) {
	ctx := context.Background()
	e, content, _, users := setup(t)
	cats, _ := content.ListCategories(ctx, true)
	u, _, _ := users.ByEmail(ctx, "u@test.com")
	catIDs := []string{cats[0].ID, cats[1].ID}

	build := func(s Strategy) (*models.Session, error) {
		return e.Build(ctx, Request{
			UserID: u.ID, CategoryIDs: catIDs, DurationSeconds: 100, Strategy: s,
		})
	}

	t.Run("under never exceeds the request", func(t *testing.T) {
		sess, err := build(StrategyUnder)
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if sess.ActualDuration != 60 {
			t.Errorf("actual = %d, want 60", sess.ActualDuration)
		}
		if sess.ActualDuration > sess.TargetDuration {
			t.Errorf("UNDER ran long: %d > %d", sess.ActualDuration, sess.TargetDuration)
		}
	})

	t.Run("over covers the request", func(t *testing.T) {
		sess, err := build(StrategyOver)
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		if sess.ActualDuration != 360 {
			t.Errorf("actual = %d, want 360", sess.ActualDuration)
		}
		if sess.ActualDuration < sess.TargetDuration {
			t.Errorf("OVER ran short: %d < %d", sess.ActualDuration, sess.TargetDuration)
		}
	})

	t.Run("exact fails rather than approximating", func(t *testing.T) {
		if _, err := build(StrategyExact); !errors.Is(err, ErrNoExactFit) {
			t.Errorf("err = %v, want ErrNoExactFit", err)
		}
	})
}

func TestBuildRecordsTargetActualAndStrategy(t *testing.T) {
	ctx := context.Background()
	e, content, _, users := setup(t)
	cats, _ := content.ListCategories(ctx, true)
	u, _, _ := users.ByEmail(ctx, "u@test.com")

	sess, err := e.Build(ctx, Request{
		UserID: u.ID, CategoryIDs: []string{cats[0].ID, cats[1].ID},
		DurationSeconds: 600, Strategy: StrategyUnder,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if sess.TargetDuration != 600 {
		t.Errorf("TargetDuration = %d, want 600", sess.TargetDuration)
	}
	if sess.ActualDuration != sess.DurationSeconds {
		t.Errorf("ActualDuration %d disagrees with DurationSeconds %d", sess.ActualDuration, sess.DurationSeconds)
	}
	if sess.Strategy != string(StrategyUnder) {
		t.Errorf("Strategy = %q, want %q", sess.Strategy, StrategyUnder)
	}
	// Every queue position must be contiguous, or the player skips a slot.
	for i, it := range sess.Items {
		if it.Position != i {
			t.Fatalf("item %d has Position %d", i, it.Position)
		}
	}
}

// An unspecified strategy is a normal request and must fall back to the
// documented default rather than to whichever case a switch happens to hit.
func TestBuildDefaultsToBalanced(t *testing.T) {
	ctx := context.Background()
	e, content, _, users := setup(t)
	cats, _ := content.ListCategories(ctx, true)
	u, _, _ := users.ByEmail(ctx, "u@test.com")

	sess, err := e.Build(ctx, Request{
		UserID: u.ID, CategoryIDs: []string{cats[0].ID, cats[1].ID}, DurationSeconds: 600,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if sess.Strategy != string(DefaultStrategy) {
		t.Errorf("Strategy = %q, want default %q", sess.Strategy, DefaultStrategy)
	}
}

// TestBuildIsDeterministic guards reproducibility, which the product depends
// on: a session created today must be the same session tomorrow, and an admin
// previewing a build must see what the user will actually get.
//
// This is a regression test. Category order used to come from ranging a Go
// map, which randomises per call, so two identical requests could produce
// different queues.
func TestBuildIsDeterministic(t *testing.T) {
	ctx := context.Background()
	e, content, _, users := setup(t)
	cats, _ := content.ListCategories(ctx, true)
	u, _, _ := users.ByEmail(ctx, "u@test.com")
	req := Request{
		UserID: u.ID, CategoryIDs: []string{cats[0].ID, cats[1].ID},
		DurationSeconds: 1370, Strategy: StrategyBalanced,
	}

	first, err := e.Build(ctx, req)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(first.Items) == 0 {
		t.Fatal("empty session")
	}
	// Repeated builds, not just two: map ordering varies run to run, so a
	// single comparison would pass by luck about half the time.
	for i := 0; i < 25; i++ {
		got, err := e.Build(ctx, req)
		if err != nil {
			t.Fatalf("build %d: %v", i, err)
		}
		if got.ActualDuration != first.ActualDuration {
			t.Fatalf("build %d: duration %d, want %d", i, got.ActualDuration, first.ActualDuration)
		}
		if len(got.Items) != len(first.Items) {
			t.Fatalf("build %d: %d items, want %d", i, len(got.Items), len(first.Items))
		}
		for j := range first.Items {
			if got.Items[j].ConfessionID != first.Items[j].ConfessionID ||
				got.Items[j].VariantID != first.Items[j].VariantID {
				t.Fatalf("build %d item %d: %s/%s, want %s/%s",
					i, j, got.Items[j].ConfessionID, got.Items[j].VariantID,
					first.Items[j].ConfessionID, first.Items[j].VariantID)
			}
		}
	}
}

// Category order in the request is meaningful: the listener asked for healing
// first, so healing should lead the queue.
func TestBuildPreservesRequestedCategoryOrder(t *testing.T) {
	ctx := context.Background()
	e, content, _, users := setup(t)
	cats, _ := content.ListCategories(ctx, true)
	u, _, _ := users.ByEmail(ctx, "u@test.com")

	forward, err := e.Build(ctx, Request{
		UserID: u.ID, CategoryIDs: []string{cats[0].ID, cats[1].ID}, DurationSeconds: 600,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	reverse, err := e.Build(ctx, Request{
		UserID: u.ID, CategoryIDs: []string{cats[1].ID, cats[0].ID}, DurationSeconds: 600,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if forward.Items[0].Category != cats[0].ID {
		t.Errorf("first item category = %s, want %s", forward.Items[0].Category, cats[0].ID)
	}
	if reverse.Items[0].Category != cats[1].ID {
		t.Errorf("first item category = %s, want %s", reverse.Items[0].Category, cats[1].ID)
	}
}
