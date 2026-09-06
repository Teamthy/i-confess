package community

import (
	"context"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

// This package had no tests at all, and that is precisely why a SQLite-only
// statement (INSERT OR IGNORE) and three missing tables survived the move to
// PostgreSQL: nothing executed this code against a real database. These tests
// exist to make that impossible to repeat.

func seedUser(t *testing.T, d *db.DB) string {
	t.Helper()
	id := "user-" + t.Name()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.Exec(`INSERT INTO users (id, email, password_hash, created_at, updated_at) VALUES ($1,$2,$3,$4,$5)`,
		id, id+"@example.com", "x", now, now)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func TestCreateAndFeedOnlyShowsApprovedSharedPosts(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	author := seedUser(t, d)
	s := NewStore(d)

	p, err := s.Create(ctx, author, "God is faithful.", VisibilityShared)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// A fresh submission is moderated, never auto-published (§22). The feed
	// must not show it while it is still 'submitted'.
	feed, err := s.Feed(ctx, 20)
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}
	if len(feed) != 0 {
		t.Errorf("feed showed %d post(s) before approval; an unmoderated post must not be visible", len(feed))
	}

	if _, err := d.Exec(`UPDATE community_posts SET status=$1 WHERE id=$2`, StatusApproved, p.ID); err != nil {
		t.Fatalf("approve: %v", err)
	}

	feed, err = s.Feed(ctx, 20)
	if err != nil {
		t.Fatalf("Feed after approval: %v", err)
	}
	if len(feed) != 1 {
		t.Fatalf("feed showed %d post(s) after approval, want 1", len(feed))
	}
	if feed[0].ID != p.ID || feed[0].Body != "God is faithful." {
		t.Errorf("feed returned %+v, want the approved post", feed[0])
	}
}

func TestFeedExcludesPrivatePosts(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	author := seedUser(t, d)
	s := NewStore(d)

	p, err := s.Create(ctx, author, "A private prayer.", VisibilityPrivate)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := d.Exec(`UPDATE community_posts SET status=$1 WHERE id=$2`, StatusPublished, p.ID); err != nil {
		t.Fatalf("publish: %v", err)
	}

	feed, err := s.Feed(ctx, 20)
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}
	if len(feed) != 0 {
		t.Errorf("private post appeared in the shared feed (%d post(s))", len(feed))
	}
}

// TestReactIsIdempotent is the regression test for the SQLite-ism. The
// statement used to be INSERT OR IGNORE, which PostgreSQL does not accept; it
// is now ON CONFLICT (post_id, user_id, reaction) DO NOTHING. Running the same
// reaction twice must not error and must not create a second row.
func TestReactIsIdempotent(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	author := seedUser(t, d)
	s := NewStore(d)

	p, err := s.Create(ctx, author, "Amen and amen.", VisibilityShared)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	for i := 1; i <= 2; i++ {
		if err := s.React(ctx, p.ID, author, ReactionAmen); err != nil {
			t.Fatalf("React attempt %d: %v", i, err)
		}
	}

	var count int
	if err := d.QueryRow(`SELECT count(*) FROM community_reactions WHERE post_id=$1 AND user_id=$2`, p.ID, author).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("duplicate reaction stored %d rows, want 1 - the ON CONFLICT clause is not working", count)
	}

	// A different reaction from the same user is a distinct row, not a
	// duplicate: the unique key includes the reaction itself.
	if err := s.React(ctx, p.ID, author, ReactionPray); err != nil {
		t.Fatalf("React (pray): %v", err)
	}
	if err := d.QueryRow(`SELECT count(*) FROM community_reactions WHERE post_id=$1 AND user_id=$2`, p.ID, author).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("got %d reactions after adding a second kind, want 2", count)
	}
}

// TestReactionIsConstrained checks the CHECK constraint that replaced the
// SQLite one, so an unknown reaction cannot be stored.
func TestReactionIsConstrained(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	author := seedUser(t, d)
	s := NewStore(d)

	p, err := s.Create(ctx, author, "Constrained.", VisibilityShared)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.React(ctx, p.ID, author, Reaction("sarcasm")); err == nil {
		t.Error("stored an invalid reaction; community_reactions_reaction_check did not fire")
	}
}
