package search

import (
	"context"
	"database/sql"
	"strings"

	"github.com/Teamthy/i-confess/internal/models"
)

// SearchStore provides full-text and keyword search across content.
type SearchStore struct {
	db *sql.DB
}

func NewSearchStore(db *sql.DB) *SearchStore {
	return &SearchStore{db: db}
}

// SearchRequest represents a search query.
type SearchRequest struct {
	Query        string
	Types        []string // "confession", "category", "collection", "voice"
	PublishedOnly bool
	Limit        int
}

// Result represents a search result.
type Result struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	ImageURL    string `json:"image_url,omitempty"`
	Score       float64 `json:"score,omitempty"`
}

// Search performs a keyword search across content.
// For MVP, this is a simple LIKE-based search.
// V2+ can upgrade to PostgreSQL full-text search or Elasticsearch.
func (s *SearchStore) Search(ctx context.Context, req SearchRequest) ([]Result, error) {
	if req.Query == "" {
		return []Result{}, nil
	}
	if req.Limit == 0 {
		req.Limit = 20
	}
	if len(req.Types) == 0 {
		req.Types = []string{"confession", "category", "collection", "voice"}
	}

	var results []Result
	query := "%" + strings.ToLower(req.Query) + "%"

	// Search confessions
	if contains(req.Types, "confession") {
		confs, err := s.searchConfessions(ctx, query, req.PublishedOnly)
		if err != nil {
			return nil, err
		}
		results = append(results, confs...)
	}

	// Search categories
	if contains(req.Types, "category") {
		cats, err := s.searchCategories(ctx, query, req.PublishedOnly)
		if err != nil {
			return nil, err
		}
		results = append(results, cats...)
	}

	// Search collections
	if contains(req.Types, "collection") {
		colls, err := s.searchCollections(ctx, query, req.PublishedOnly)
		if err != nil {
			return nil, err
		}
		results = append(results, colls...)
	}

	// Search voices
	if contains(req.Types, "voice") {
		voices, err := s.searchVoices(ctx, query)
		if err != nil {
			return nil, err
		}
		results = append(results, voices...)
	}

	// Truncate to limit
	if len(results) > req.Limit {
		results = results[:req.Limit]
	}
	return results, nil
}

func (s *SearchStore) searchConfessions(ctx context.Context, query string, publishedOnly bool) ([]Result, error) {
	q := `SELECT id, title, COALESCE(medium_text, ''), '' FROM confessions WHERE (LOWER(title) LIKE ? OR LOWER(medium_text) LIKE ?)`
	if publishedOnly {
		q += " AND status = 'published'"
	}
	q += " LIMIT 50"

	rows, err := s.db.QueryContext(ctx, q, query, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Result
	for rows.Next() {
		var r Result
		if err := rows.Scan(&r.ID, &r.Title, &r.Description, &r.ImageURL); err != nil {
			continue
		}
		r.Type = "confession"
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *SearchStore) searchCategories(ctx context.Context, query string, publishedOnly bool) ([]Result, error) {
	q := `SELECT id, name, COALESCE(description, ''), COALESCE(icon, '') FROM categories WHERE (LOWER(name) LIKE ? OR LOWER(description) LIKE ?)`
	if publishedOnly {
		q += " AND status = 'published'"
	}
	q += " LIMIT 50"

	rows, err := s.db.QueryContext(ctx, q, query, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Result
	for rows.Next() {
		var r Result
		if err := rows.Scan(&r.ID, &r.Title, &r.Description, &r.ImageURL); err != nil {
			continue
		}
		r.Type = "category"
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *SearchStore) searchCollections(ctx context.Context, query string, publishedOnly bool) ([]Result, error) {
	q := `SELECT id, name, COALESCE(description, ''), '' FROM collections WHERE (LOWER(name) LIKE ? OR LOWER(description) LIKE ?)`
	if publishedOnly {
		q += " AND status = 'published'"
	}
	q += " LIMIT 50"

	rows, err := s.db.QueryContext(ctx, q, query, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Result
	for rows.Next() {
		var r Result
		if err := rows.Scan(&r.ID, &r.Title, &r.Description, &r.ImageURL); err != nil {
			continue
		}
		r.Type = "collection"
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *SearchStore) searchVoices(ctx context.Context, query string) ([]Result, error) {
	q := `SELECT id, name, COALESCE(description, ''), COALESCE(sample_url, '') FROM voices WHERE status = 'active' AND (LOWER(name) LIKE ? OR LOWER(description) LIKE ?) LIMIT 50`

	rows, err := s.db.QueryContext(ctx, q, query, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Result
	for rows.Next() {
		var r Result
		if err := rows.Scan(&r.ID, &r.Title, &r.Description, &r.ImageURL); err != nil {
			continue
		}
		r.Type = "voice"
		results = append(results, r)
	}
	return results, rows.Err()
}

func contains(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}
