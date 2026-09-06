package store

import (
	"context"
	"testing"

	"github.com/Teamthy/i-confess/internal/content"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
	"github.com/Teamthy/i-confess/internal/models"
)

// TestOnlyServedStatusesReachSessionBuilding covers gap G-38.
//
// The store used to filter session-eligible confessions with a literal
// `status = 'published'` while the rule lived in internal/content. The
// behaviour was right by accident: nothing read the authority, so a change to
// what should be served would not have reached session building.
func TestOnlyServedStatusesReachSessionBuilding(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	ctx := context.Background()

	s := NewContentStore(conn)
	cat := &models.Category{Name: "Governance", Slug: "governance", Status: "published", SortOrder: 1}
	if err := s.CreateCategory(ctx, cat); err != nil {
		t.Fatalf("create category: %v", err)
	}

	// One confession per lifecycle status.
	for _, st := range content.All() {
		c := &models.Confession{
			CategoryID: cat.ID, Title: "In " + string(st), ShortText: "a",
			MediumText: "ab", LongText: "abc", Status: string(st), Language: "en", Intensity: 1,
		}
		if err := s.CreateConfession(ctx, c); err != nil {
			t.Fatalf("create confession in %s: %v", st, err)
		}
	}

	got, err := s.ConfessionsByCategory(ctx, cat.ID, true)
	if err != nil {
		t.Fatalf("ConfessionsByCategory: %v", err)
	}

	served := map[string]bool{}
	for _, st := range content.ServedStatuses() {
		served[st] = true
	}

	if len(got) != len(served) {
		t.Errorf("query returned %d confessions, the authority serves %d", len(got), len(served))
	}
	for _, c := range got {
		if !served[c.Status] {
			t.Errorf("confession in status %q reached session building but the authority does not serve it", c.Status)
		}
		if !content.IsServedToNewSessions(c.Status) {
			t.Errorf("confession in status %q contradicts IsServedToNewSessions", c.Status)
		}
	}

	// The specific states that must not be offered to a new session.
	for _, st := range []content.Status{content.StatusDeprecated, content.StatusArchived, content.StatusDraft} {
		for _, c := range got {
			if c.Status == string(st) {
				t.Errorf("status %q was served to a new session; it must not be", st)
			}
		}
	}

	// And unfiltered reads still see everything, so admin surfaces are not
	// accidentally narrowed by the same change.
	all, err := s.ConfessionsByCategory(ctx, cat.ID, false)
	if err != nil {
		t.Fatalf("unfiltered read: %v", err)
	}
	if len(all) != len(content.All()) {
		t.Errorf("unfiltered read returned %d confessions, want %d", len(all), len(content.All()))
	}
}
