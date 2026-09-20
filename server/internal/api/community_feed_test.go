package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

func TestPublicUGCFeed(t *testing.T) {
	conn := dbtest.New(t)
	defer conn.Close()
	h := NewHandler(Config{JWTSecret: "test-secret-value", TokenTTL: "24h"}, conn)
	h.BuildEngine()
	srv := httptest.NewServer(h.Routes())
	defer srv.Close()

	// Seed a user and several user_confessions directly.
	userID := "ugc-feed-user"
	if _, err := conn.ExecContext(context.Background(),
		`INSERT INTO users (id,email,password_hash,status,created_at,updated_at) VALUES (?,'a@example.com','x','active','2026-01-01','2026-01-01')`, userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	// Insert: one that should appear, others that must not.
	inserts := []struct {
		id, vis, st, pub string
	}{
		{"ugc-feed-pub", "public", "published", "2026-09-20T10:00:00Z"},
		{"ugc-feed-priv", "private", "published", "2026-09-20T11:00:00Z"},
		{"ugc-feed-shared", "shared", "published", "2026-09-20T12:00:00Z"},
		{"ugc-feed-draft", "public", "draft", ""},
		{"ugc-feed-sub", "public", "submitted", ""},
		{"ugc-feed-approved", "public", "approved", ""},
	}
	for _, ins := range inserts {
		pub := any(nil)
		if ins.pub != "" {
			pub = ins.pub
		}
		if _, err := conn.ExecContext(context.Background(),
			`INSERT INTO user_confessions (id,user_id,title,text,category_id,is_private,status,visibility,created_at,updated_at,published_at,version)
			 VALUES (?,?,?,?,?,0,?,?,?, ?,?,1)`,
			ins.id, userID, "Title "+ins.id, "Body "+ins.id, nil, ins.st, ins.vis,
			"2026-09-20T09:00:00Z", "2026-09-20T09:00:00Z", pub); err != nil {
			t.Fatalf("insert %s: %v", ins.id, err)
		}
	}

	// Public endpoint must be reachable without auth and return only public+published, anonymous.
	status, body := doRequest(t, srv, http.MethodGet, "/community/confessions", "", "")
	if status != http.StatusOK {
		t.Fatalf("GET /community/confessions: %d %s", status, truncateBody(body))
	}
	var resp struct {
		Confessions []map[string]any `json:"confessions"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode: %v %s", err, truncateBody(body))
	}
	if len(resp.Confessions) != 1 {
		t.Fatalf("feed returned %d confessions, want 1: %+v", len(resp.Confessions), resp.Confessions)
	}
	c := resp.Confessions[0]
	if c["id"] != "ugc-feed-pub" {
		t.Errorf("id = %v, want ugc-feed-pub", c["id"])
	}
	// Anonymity: no user_id, no reviewed_by.
	if _, ok := c["user_id"]; ok {
		t.Error("public UGC feed leaked user_id")
	}
	if _, ok := c["reviewed_by"]; ok {
		t.Error("public UGC feed leaked reviewed_by")
	}
	if c["visibility"] != "public" || c["status"] != "published" {
		t.Errorf("visibility/status = %v/%v, want public/published", c["visibility"], c["status"])
	}

	// v1 alias
	status, body = doRequest(t, srv, http.MethodGet, "/v1/community/confessions", "", "")
	if status != http.StatusOK {
		t.Fatalf("GET /v1/community/confessions: %d %s", status, truncateBody(body))
	}

	// Ordering: newer published_at first.
	if _, err := conn.ExecContext(context.Background(),
		`INSERT INTO user_confessions (id,user_id,title,text,category_id,is_private,status,visibility,created_at,updated_at,published_at,version)
		 VALUES ('ugc-feed-pub2','ugc-feed-user','Second','body second',NULL,0,'published','public','2026-09-20T09:00:00Z','2026-09-20T09:00:00Z','2026-09-20T15:00:00Z',1)`); err != nil {
		t.Fatalf("insert second: %v", err)
	}
	status, body = doRequest(t, srv, http.MethodGet, "/community/confessions", "", "")
	if status != http.StatusOK {
		t.Fatalf("second fetch: %d %s", status, truncateBody(body))
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode second: %v", err)
	}
	if len(resp.Confessions) != 2 || resp.Confessions[0]["id"] != "ugc-feed-pub2" {
		t.Fatalf("ordering wrong: %+v", resp.Confessions)
	}
}
