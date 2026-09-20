package store

import (
	"context"
	"testing"

	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

// Favourites (§35, PHASE 27).
//
// The table is polymorphic, which makes two things easy to get wrong and both
// were wrong before this phase: the same entity could be favourited twice, and
// a favourite carried no name so nothing could render it.

func engCtx() context.Context { return context.Background() }

func seedEngUser(t *testing.T, conn *db.DB, id string) {
	t.Helper()
	if _, err := conn.ExecContext(engCtx(),
		`INSERT INTO users (id,email,password_hash,status,created_at,updated_at)
		 VALUES (?,?,'x','active','2026-01-01','2026-01-01')`, id, id+"@example.com"); err != nil {
		t.Fatalf("insert user %s: %v", id, err)
	}
}

func seedEngContent(t *testing.T, conn *db.DB) {
	t.Helper()
	if _, err := conn.ExecContext(engCtx(),
		`INSERT INTO categories (id,name,slug,description,icon,premium,status,sort_order,created_at,updated_at)
		 VALUES ('engcat','Peace','peace','Quiet','i',0,'published',1,'2026-01-01','2026-01-01')
		 ON CONFLICT (id) DO NOTHING`); err != nil {
		t.Fatalf("insert category: %v", err)
	}
	if _, err := conn.ExecContext(engCtx(),
		`INSERT INTO confessions (id,category_id,title,short_text,intensity,language,status,author,version,created_at,updated_at)
		 VALUES ('engconf','engcat','I am held','held',1,'en','published','test',1,'2026-01-01','2026-01-01')
		 ON CONFLICT (id) DO NOTHING`); err != nil {
		t.Fatalf("insert confession: %v", err)
	}
}

// Favouriting is idempotent. Before PHASE 27 `AddFavorite` used
// `ON CONFLICT(id)` against a freshly generated id, so the conflict could never
// fire and three taps of the heart wrote three rows.
func TestAddFavoriteIsIdempotent(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	s := NewEngagementStore(conn)
	seedEngUser(t, conn, "fav-u1")

	first, err := s.AddFavorite(engCtx(), "fav-u1", "confession", "c1")
	if err != nil {
		t.Fatalf("first add: %v", err)
	}
	for i := 0; i < 3; i++ {
		again, err := s.AddFavorite(engCtx(), "fav-u1", "confession", "c1")
		if err != nil {
			t.Fatalf("repeat add %d: %v", i, err)
		}
		// The id returned must be the stored one, not the one that was built
		// and discarded — a client handed a fabricated id holds a handle that
		// matches no row.
		if again.ID != first.ID {
			t.Fatalf("repeat add returned id %q, want the stored %q", again.ID, first.ID)
		}
		if again.CreatedAt != first.CreatedAt {
			t.Fatalf("re-favouriting moved created_at from %q to %q", first.CreatedAt, again.CreatedAt)
		}
	}

	list, err := s.ListFavorites(engCtx(), "fav-u1", "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("four adds of the same entity produced %d rows, want 1", len(list))
	}
}

// Two users favouriting the same confession are two favourites, and the
// uniqueness constraint must not collapse them.
func TestFavoriteUniquenessIsPerUser(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	s := NewEngagementStore(conn)
	seedEngUser(t, conn, "fav-u2")
	seedEngUser(t, conn, "fav-u3")

	if _, err := s.AddFavorite(engCtx(), "fav-u2", "confession", "shared"); err != nil {
		t.Fatalf("u2: %v", err)
	}
	if _, err := s.AddFavorite(engCtx(), "fav-u3", "confession", "shared"); err != nil {
		t.Fatalf("u3: %v", err)
	}
	for _, u := range []string{"fav-u2", "fav-u3"} {
		list, err := s.ListFavorites(engCtx(), u, "")
		if err != nil {
			t.Fatalf("list %s: %v", u, err)
		}
		if len(list) != 1 {
			t.Fatalf("%s has %d favourites, want 1", u, len(list))
		}
	}
}

// The same entity id under two different types is two distinct favourites:
// the natural key is (user, type, id), not (user, id).
func TestFavoriteUniquenessIsPerEntityType(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	s := NewEngagementStore(conn)
	seedEngUser(t, conn, "fav-u4")

	if _, err := s.AddFavorite(engCtx(), "fav-u4", "confession", "same-id"); err != nil {
		t.Fatalf("confession: %v", err)
	}
	if _, err := s.AddFavorite(engCtx(), "fav-u4", "category", "same-id"); err != nil {
		t.Fatalf("category: %v", err)
	}
	list, err := s.ListFavorites(engCtx(), "fav-u4", "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d favourites, want 2 (one per entity type)", len(list))
	}
}

// A favourite with no resolved title is unrenderable, which is what the
// library's favourites tab displayed before this phase.
func TestListFavoritesDetailedResolvesTitles(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	s := NewEngagementStore(conn)
	seedEngUser(t, conn, "fav-u5")
	seedEngContent(t, conn)

	if _, err := s.AddFavorite(engCtx(), "fav-u5", "confession", "engconf"); err != nil {
		t.Fatalf("add confession: %v", err)
	}
	if _, err := s.AddFavorite(engCtx(), "fav-u5", "category", "engcat"); err != nil {
		t.Fatalf("add category: %v", err)
	}

	list, err := s.ListFavoritesDetailed(engCtx(), "fav-u5", "")
	if err != nil {
		t.Fatalf("detailed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d favourites, want 2", len(list))
	}

	byID := map[string]string{}
	subtitles := map[string]string{}
	for _, f := range list {
		if f.Missing {
			t.Fatalf("favourite %s/%s reported missing though it exists", f.EntityType, f.EntityID)
		}
		byID[f.EntityID] = f.Title
		subtitles[f.EntityID] = f.Subtitle
	}
	if byID["engconf"] != "I am held" {
		t.Fatalf("confession title = %q, want %q", byID["engconf"], "I am held")
	}
	// The category a confession sits in is what makes a favourites list
	// readable rather than a column of bare titles.
	if subtitles["engconf"] != "Peace" {
		t.Fatalf("confession subtitle = %q, want the category name %q", subtitles["engconf"], "Peace")
	}
	if byID["engcat"] != "Peace" {
		t.Fatalf("category title = %q, want %q", byID["engcat"], "Peace")
	}
}

// A favourite pointing at content that no longer exists must still be listed,
// flagged. Dropping it silently would leave an entry the user can see in their
// export and never clear.
func TestListFavoritesDetailedFlagsMissingTargets(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	s := NewEngagementStore(conn)
	seedEngUser(t, conn, "fav-u6")

	if _, err := s.AddFavorite(engCtx(), "fav-u6", "confession", "deleted-long-ago"); err != nil {
		t.Fatalf("add: %v", err)
	}
	list, err := s.ListFavoritesDetailed(engCtx(), "fav-u6", "")
	if err != nil {
		t.Fatalf("detailed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("a dangling favourite was dropped: got %d rows", len(list))
	}
	if !list[0].Missing {
		t.Fatalf("dangling favourite not flagged: %+v", list[0])
	}
}

// The type filter narrows the list; it must also survive hydration.
func TestListFavoritesDetailedFiltersByType(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	s := NewEngagementStore(conn)
	seedEngUser(t, conn, "fav-u7")
	seedEngContent(t, conn)

	if _, err := s.AddFavorite(engCtx(), "fav-u7", "confession", "engconf"); err != nil {
		t.Fatalf("add confession: %v", err)
	}
	if _, err := s.AddFavorite(engCtx(), "fav-u7", "category", "engcat"); err != nil {
		t.Fatalf("add category: %v", err)
	}

	list, err := s.ListFavoritesDetailed(engCtx(), "fav-u7", "confession")
	if err != nil {
		t.Fatalf("detailed: %v", err)
	}
	if len(list) != 1 || list[0].EntityType != "confession" {
		t.Fatalf("type filter returned %d rows: %+v", len(list), list)
	}
	if list[0].Title == "" {
		t.Fatalf("filtered favourite lost its resolved title")
	}
}

// Published UGC reader (G-40): only public+published user confessions are
// visible, ordered newest first, and private/shared or non-published rows are
// excluded.
func TestListPublishedUserConfessions(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	s := NewEngagementStore(conn)
	seedEngUser(t, conn, "ugc-u1")
	ctx := engCtx()

	// Create four user confessions: one that should appear, three that must not.
	// The store's CreateUserConfession sets status/visibility from the model,
	// but we insert directly to control published_at ordering.
	cases := []struct {
		id         string
		title      string
		visibility string
		status     string
		published  string
		shouldShow bool
	}{
		{"ugc-pub-1", "Public testimony", "public", "published", "2026-09-20T10:00:00Z", true},
		{"ugc-priv-1", "Private diary", "private", "published", "2026-09-20T11:00:00Z", false},
		{"ugc-shared-1", "Circle only", "shared", "published", "2026-09-20T12:00:00Z", false},
		{"ugc-draft-1", "Draft public intent", "public", "draft", "", false},
		{"ugc-sub-1", "Submitted public", "public", "submitted", "", false},
		{"ugc-approved-1", "Approved but not yet published", "public", "approved", "", false},
	}
	for _, c := range cases {
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO user_confessions (id,user_id,title,text,category_id,is_private,status,visibility,created_at,updated_at,published_at,version)
			 VALUES (?,?,?,?,?,0,?,?,?, ?,?,1)`,
			c.id, "ugc-u1", c.title, "body "+c.title, nil, c.status, c.visibility,
			"2026-09-20T09:00:00Z", "2026-09-20T09:00:00Z",
			nullIfEmpty(c.published)); err != nil {
			t.Fatalf("insert %s: %v", c.id, err)
		}
	}

	list, err := s.ListPublishedUserConfessions(ctx, 20)
	if err != nil {
		t.Fatalf("ListPublished: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("published feed returned %d rows, want 1: %+v", len(list), list)
	}
	if list[0].ID != "ugc-pub-1" {
		t.Fatalf("feed returned %q, want %q", list[0].ID, "ugc-pub-1")
	}
	if list[0].Visibility != "public" || list[0].Status != "published" {
		t.Fatalf("feed row has visibility=%q status=%q, want public/published", list[0].Visibility, list[0].Status)
	}

	// Ordering: newest published_at first.
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO user_confessions (id,user_id,title,text,category_id,is_private,status,visibility,created_at,updated_at,published_at,version)
		 VALUES ('ugc-pub-2','ugc-u1','Second testimony','body second',NULL,0,'published','public','2026-09-20T09:00:00Z','2026-09-20T09:00:00Z','2026-09-20T15:00:00Z',1)`); err != nil {
		t.Fatalf("insert second: %v", err)
	}
	list, err = s.ListPublishedUserConfessions(ctx, 20)
	if err != nil {
		t.Fatalf("ListPublished second: %v", err)
	}
	if len(list) != 2 || list[0].ID != "ugc-pub-2" {
		t.Fatalf("ordering wrong, got %+v", list)
	}
}

// Removing a favourite must remove exactly the one named, leaving the
// listener's other favourites alone.
func TestRemoveFavoriteIsScoped(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	s := NewEngagementStore(conn)
	seedEngUser(t, conn, "fav-u8")

	for _, id := range []string{"a", "b"} {
		if _, err := s.AddFavorite(engCtx(), "fav-u8", "confession", id); err != nil {
			t.Fatalf("add %s: %v", id, err)
		}
	}
	if err := s.RemoveFavorite(engCtx(), "fav-u8", "confession", "a"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	list, err := s.ListFavorites(engCtx(), "fav-u8", "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].EntityID != "b" {
		t.Fatalf("after removing 'a' the list is %+v", list)
	}

	// And re-favouriting after a removal works: the unique index must not
	// leave a tombstone that blocks it.
	if _, err := s.AddFavorite(engCtx(), "fav-u8", "confession", "a"); err != nil {
		t.Fatalf("re-add after remove: %v", err)
	}
	list, _ = s.ListFavorites(engCtx(), "fav-u8", "")
	if len(list) != 2 {
		t.Fatalf("re-add produced %d rows, want 2", len(list))
	}
}
