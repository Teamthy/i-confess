package deletion

import (
	"context"
	"testing"
	"time"
)

// community_posts.author_id references users(id) directly. The generic erasure
// query scopes non-user_id columns through an intermediate parent table with
// "WHERE col IN (SELECT id FROM parent WHERE user_id = ?)", which asks
// community_posts for a user_id column it does not have. Adding the policy was
// therefore not enough: parentTableFor had to report that the parent is users,
// and applyPolicy had to recognise that case and delete directly.
//
// This test exists because the schema-coverage test caught the missing policy
// but nothing exercised the query the policy generates. A policy that is
// present and wrong is worse than one that is absent, because it looks handled.
func TestErasureRemovesCommunityContent(t *testing.T) {
	conn := newDB(t)
	ctx := context.Background()
	seedUser(t, conn, "comm-user", "comm@example.com")

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := conn.Exec(`INSERT INTO community_posts (id, author_id, body, visibility, status, created_at) VALUES ($1,$2,$3,$4,$5,$6)`,
		"post-1", "comm-user", "A shared confession.", "shared", "published", now); err != nil {
		t.Fatalf("insert post: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO community_reactions (id, post_id, user_id, reaction, created_at) VALUES ($1,$2,$3,$4,$5)`,
		"react-1", "post-1", "comm-user", "amen", now); err != nil {
		t.Fatalf("insert reaction: %v", err)
	}

	if _, err := newService(t, conn).Erase(ctx, "comm-user"); err != nil {
		t.Fatalf("Erase: %v", err)
	}

	var posts, reactions int
	if err := conn.QueryRow(`SELECT count(*) FROM community_posts`).Scan(&posts); err != nil {
		t.Fatalf("count posts: %v", err)
	}
	if err := conn.QueryRow(`SELECT count(*) FROM community_reactions`).Scan(&reactions); err != nil {
		t.Fatalf("count reactions: %v", err)
	}
	if posts != 0 {
		t.Errorf("%d community post(s) survived erasure", posts)
	}
	if reactions != 0 {
		t.Errorf("%d reaction(s) survived erasure", reactions)
	}
}

// TestEveryPolicyGeneratesRunnableSQL guards the general case. It calls the
// real applyPolicy rather than reconstructing the query, because a copy of the
// logic tests the copy: an earlier draft of this test rebuilt the DELETE by
// hand and so reported failures for tables whose policy is Anonymise, which
// never runs a DELETE at all.
//
// It runs inside a rolled-back transaction so the statements are executed
// against the live schema without erasing anything.
func TestEveryPolicyGeneratesRunnableSQL(t *testing.T) {
	conn := newDB(t)
	ctx := context.Background()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback()

	for _, p := range Policies {
		if _, err := applyPolicy(ctx, tx, p, "no-such-user"); err != nil {
			t.Errorf("policy for %q (column %q, action %s) fails: %v",
				p.Table, p.column(), p.Action, err)
		}
	}
}
