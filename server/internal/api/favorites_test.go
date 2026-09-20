package api

import (
	"context"
	"net/http"
	"testing"
)

// The favourites API (§35, PHASE 27).
//
// This surface is what the library's favourites tab reads. Two properties
// matter and neither held before this phase: favouriting twice is one
// favourite, and a favourite comes back with something a human can read.

// seedFavoritable inserts a published confession and its category directly.
// There is no user-facing endpoint that creates editorial content, and going
// through the admin surface would make these tests about authorisation.
func (a *authHarness) seedFavoritable(t *testing.T, confessionID, title, categoryName string) {
	t.Helper()
	ctx := context.Background()
	if _, err := a.db.ExecContext(ctx,
		`INSERT INTO categories (id,name,slug,description,icon,premium,status,sort_order,created_at,updated_at)
		 VALUES (?,?,?,'d','i',0,'published',1,'2026-01-01','2026-01-01')
		 ON CONFLICT (id) DO NOTHING`,
		"cat-"+confessionID, categoryName, "slug-"+confessionID); err != nil {
		t.Fatalf("seed category: %v", err)
	}
	if _, err := a.db.ExecContext(ctx,
		`INSERT INTO confessions (id,category_id,title,short_text,intensity,language,status,author,version,created_at,updated_at)
		 VALUES (?,?,?,'short',1,'en','published','test',1,'2026-01-01','2026-01-01')
		 ON CONFLICT (id) DO NOTHING`,
		confessionID, "cat-"+confessionID, title); err != nil {
		t.Fatalf("seed confession: %v", err)
	}
}

// Tapping the heart repeatedly is one favourite. Before PHASE 27 the insert
// used ON CONFLICT(id) against a freshly generated id, so it could never fire.
func TestFavoriteTwiceIsOneFavorite(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "fav-api-1@test.com")
	a.seedFavoritable(t, "conf-dup", "Held", "Peace")

	body := map[string]any{"entity_type": "confession", "entity_id": "conf-dup"}
	for i := 0; i < 3; i++ {
		if code, _ := a.do("POST", "/me/favorites", token, body); code != http.StatusCreated {
			t.Fatalf("add %d: %d", i, code)
		}
	}

	_, list := a.doList("GET", "/me/favorites", token)
	if len(list) != 1 {
		t.Fatalf("three taps produced %d favourites, want 1", len(list))
	}
}

// A favourite that comes back as a bare id cannot be rendered — which is
// exactly what the library tab displayed before the server learned to resolve
// titles.
func TestFavoritesCarryDisplayableTitles(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "fav-api-2@test.com")
	a.seedFavoritable(t, "conf-title", "I am held by grace", "Peace")

	if code, _ := a.do("POST", "/me/favorites", token,
		map[string]any{"entity_type": "confession", "entity_id": "conf-title"}); code != http.StatusCreated {
		t.Fatalf("add favourite")
	}

	_, list := a.doList("GET", "/me/favorites", token)
	if len(list) != 1 {
		t.Fatalf("got %d favourites", len(list))
	}
	row, ok := list[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected row shape: %T", list[0])
	}
	if row["title"] != "I am held by grace" {
		t.Fatalf("title = %v, want the confession's title", row["title"])
	}
	// The category gives the row context; without it a favourites list is a
	// column of titles with nothing to distinguish them.
	if row["subtitle"] != "Peace" {
		t.Fatalf("subtitle = %v, want the category name", row["subtitle"])
	}
	if row["missing"] == true {
		t.Fatalf("an existing confession was reported missing")
	}
}

// A favourite pointing at content that has gone must still be listed, flagged,
// so the user can clear it.
func TestFavoriteWithMissingTargetIsFlaggedNotDropped(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "fav-api-3@test.com")

	if code, _ := a.do("POST", "/me/favorites", token,
		map[string]any{"entity_type": "confession", "entity_id": "never-existed"}); code != http.StatusCreated {
		t.Fatalf("add favourite")
	}

	_, list := a.doList("GET", "/me/favorites", token)
	if len(list) != 1 {
		t.Fatalf("dangling favourite was dropped: %d rows", len(list))
	}
	row := list[0].(map[string]any)
	if row["missing"] != true {
		t.Fatalf("dangling favourite not flagged: %v", row)
	}
}

// The type filter narrows the list. An unknown type is refused rather than
// silently answered with an empty list, which reads as "you have none".
func TestFavoritesTypeFilter(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "fav-api-4@test.com")
	a.seedFavoritable(t, "conf-filter", "Filtered", "Peace")

	a.do("POST", "/me/favorites", token,
		map[string]any{"entity_type": "confession", "entity_id": "conf-filter"})
	a.do("POST", "/me/favorites", token,
		map[string]any{"entity_type": "category", "entity_id": "cat-conf-filter"})

	_, all := a.doList("GET", "/me/favorites", token)
	if len(all) != 2 {
		t.Fatalf("unfiltered list has %d rows, want 2", len(all))
	}

	_, confessions := a.doList("GET", "/me/favorites?type=confession", token)
	if len(confessions) != 1 {
		t.Fatalf("type=confession returned %d rows, want 1", len(confessions))
	}
	if row := confessions[0].(map[string]any); row["entity_type"] != "confession" {
		t.Fatalf("filter leaked a %v", row["entity_type"])
	}

	code, _ := a.do("GET", "/me/favorites?type=nonsense", token, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("unknown type returned %d, want 400 — an empty list would read as "+
			"'you have no favourites'", code)
	}
}

// Favourites are per account. One listener's saved content must never appear
// in another's library (§71, §72).
func TestFavoritesAreScopedToTheAccount(t *testing.T) {
	a := newAuthHarness(t)
	alice, _ := a.register(t, "fav-alice@test.com")
	bob, _ := a.register(t, "fav-bob@test.com")
	a.seedFavoritable(t, "conf-shared", "Shared", "Peace")

	a.do("POST", "/me/favorites", alice,
		map[string]any{"entity_type": "confession", "entity_id": "conf-shared"})

	_, bobList := a.doList("GET", "/me/favorites", bob)
	if len(bobList) != 0 {
		t.Fatalf("bob sees %d of alice's favourites", len(bobList))
	}

	// Bob favouriting the same confession is his own favourite, not a
	// collision with hers.
	if code, _ := a.do("POST", "/me/favorites", bob,
		map[string]any{"entity_type": "confession", "entity_id": "conf-shared"}); code != http.StatusCreated {
		t.Fatalf("bob could not favourite the same confession")
	}
	_, aliceList := a.doList("GET", "/me/favorites", alice)
	if len(aliceList) != 1 {
		t.Fatalf("alice's list changed when bob favourited: %d rows", len(aliceList))
	}
}

// Unfavouriting removes exactly one, and re-favouriting afterwards works — the
// unique index must not leave anything behind that blocks it.
func TestUnfavoriteThenFavoriteAgain(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "fav-api-5@test.com")
	a.seedFavoritable(t, "conf-cycle", "Cycle", "Peace")

	body := map[string]any{"entity_type": "confession", "entity_id": "conf-cycle"}
	a.do("POST", "/me/favorites", token, body)

	if code, _ := a.do("DELETE", "/me/favorites", token, body); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	_, list := a.doList("GET", "/me/favorites", token)
	if len(list) != 0 {
		t.Fatalf("favourite survived removal: %d rows", len(list))
	}

	if code, _ := a.do("POST", "/me/favorites", token, body); code != http.StatusCreated {
		t.Fatalf("re-favouriting after removal: %d", code)
	}
	_, list = a.doList("GET", "/me/favorites", token)
	if len(list) != 1 {
		t.Fatalf("re-favourite produced %d rows", len(list))
	}
}

// Every library read must require a session. An unauthenticated caller getting
// a 200 here would be an account-data leak.
func TestLibraryReadsRequireAuthentication(t *testing.T) {
	a := newAuthHarness(t)
	for _, path := range []string{
		"/me/favorites",
		"/me/collections",
		"/me/confessions",
	} {
		code, _ := a.do("GET", path, "", nil)
		if code != http.StatusUnauthorized {
			t.Fatalf("GET %s without a token returned %d, want 401", path, code)
		}
	}
}

// The library's third tab reads this. It returns the author's own writing with
// the moderation status attached — a different shape from editorial content,
// and the client decodes it as such.
func TestMyConfessionsReturnsStatusAndText(t *testing.T) {
	a := newAuthHarness(t)
	token, _ := a.register(t, "mine-1@test.com")

	code, created := a.do("POST", "/me/confessions", token, map[string]any{
		"title": "My own words", "text": "I am kept.",
	})
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, created)
	}

	_, list := a.doList("GET", "/me/confessions", token)
	if len(list) != 1 {
		t.Fatalf("got %d personal confessions, want 1", len(list))
	}
	row := list[0].(map[string]any)

	// These four keys are what the client's UserConfession model reads. The
	// editorial Confession model reads short_text/description instead, which
	// is why decoding one as the other produced blank rows.
	for _, key := range []string{"title", "text", "status", "visibility"} {
		if _, ok := row[key]; !ok {
			t.Fatalf("personal confession is missing %q: %v", key, row)
		}
	}
	if row["status"] != "draft" {
		t.Fatalf("a new personal confession has status %v, want draft", row["status"])
	}
	// §22: private is the floor. A new note must not default to any audience.
	if row["visibility"] != "private" {
		t.Fatalf("a new personal confession defaulted to %v, want private", row["visibility"])
	}
	if row["text"] != "I am kept." {
		t.Fatalf("text = %v", row["text"])
	}
}

// One listener's personal writing must never appear in another's library.
func TestMyConfessionsAreScopedToTheAuthor(t *testing.T) {
	a := newAuthHarness(t)
	alice, _ := a.register(t, "mine-alice@test.com")
	bob, _ := a.register(t, "mine-bob@test.com")

	a.do("POST", "/me/confessions", alice, map[string]any{
		"title": "Alice only", "text": "private words",
	})

	_, bobList := a.doList("GET", "/me/confessions", bob)
	if len(bobList) != 0 {
		t.Fatalf("bob can read %d of alice's personal confessions", len(bobList))
	}
}
