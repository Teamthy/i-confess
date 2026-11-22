package api

import (
	"net/http"
	"testing"
)

// A writer could create community posts but never read them back: the only
// reads were the anonymous public feeds. GET /me/community/posts returns the
// caller's own posts in every status, and nobody else's.
func TestMyCommunityPostsListsOnlyTheAuthorsOwn(t *testing.T) {
	a := newAuthHarness(t)
	mine, _ := a.register(t, "writer@example.com")
	other, _ := a.register(t, "other@example.com")

	if code, out := a.do("POST", "/community/posts", mine, map[string]string{"body": "I am held.", "visibility": "private"}); code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, out)
	}
	if code, out := a.do("POST", "/community/posts", other, map[string]string{"body": "Not yours.", "visibility": "private"}); code != http.StatusCreated {
		t.Fatalf("create other: %d %v", code, out)
	}
	code, out := a.do("GET", "/v1/me/community/posts", mine, nil)
	if code != http.StatusOK {
		t.Fatalf("list: %d %v", code, out)
	}
	posts, _ := out["posts"].([]any)
	if len(posts) != 1 {
		t.Fatalf("posts = %v, want exactly the author's one", posts)
	}
	p := posts[0].(map[string]any)
	if p["body"] != "I am held." || p["status"] != "submitted" || p["visibility"] != "private" {
		t.Fatalf("post = %v", p)
	}
	if _, leaked := p["author_id"]; leaked {
		t.Fatalf("author id returned: %v", p)
	}
	if code, _ := a.do("GET", "/me/community/posts", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("anonymous list: %d, want 401", code)
	}
}
