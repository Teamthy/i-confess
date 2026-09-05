package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/Teamthy/i-confess/internal/models"
)

// ContentStore manages collections, categories, confessions, variants, and scriptures.
type ContentStore struct{ db *sql.DB }

func NewContentStore(db *sql.DB) *ContentStore { return &ContentStore{db: db} }

// ---------- Collections ----------

func (s *ContentStore) CreateCollection(ctx context.Context, c *models.Collection) error {
	if c.ID == "" {
		c.ID = newID()
	}
	c.CreatedAt, c.UpdatedAt = now(), now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO collections (id,name,slug,description,premium,status,sort_order,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		c.ID, c.Name, c.Slug, c.Description, boolInt(c.Premium), c.Status, c.SortOrder, c.CreatedAt, c.UpdatedAt)
	return err
}

func (s *ContentStore) ListCollections(ctx context.Context, includeUnpublished bool) ([]models.Collection, error) {
	q := `SELECT id,name,slug,COALESCE(description,''),premium,status,sort_order,created_at,updated_at FROM collections`
	if !includeUnpublished {
		q += ` WHERE status = 'published'`
	}
	q += ` ORDER BY sort_order, name`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Collection
	for rows.Next() {
		var c models.Collection
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug, &c.Description, &c.Premium, &c.Status, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---------- Categories ----------

func (s *ContentStore) CreateCategory(ctx context.Context, c *models.Category) error {
	if c.ID == "" {
		c.ID = newID()
	}
	c.CreatedAt, c.UpdatedAt = now(), now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO categories (id,name,slug,description,icon,premium,status,sort_order,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?)`,
		c.ID, c.Name, c.Slug, c.Description, c.Icon, boolInt(c.Premium), c.Status, c.SortOrder, c.CreatedAt, c.UpdatedAt)
	return err
}

func (s *ContentStore) ListCategories(ctx context.Context, includeUnpublished bool) ([]models.Category, error) {
	q := `SELECT id,name,slug,COALESCE(description,''),COALESCE(icon,''),premium,status,sort_order,created_at,updated_at FROM categories`
	if !includeUnpublished {
		q += ` WHERE status = 'published'`
	}
	q += ` ORDER BY sort_order, name`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Category
	for rows.Next() {
		var c models.Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug, &c.Description, &c.Icon, &c.Premium, &c.Status, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *ContentStore) CategoryByID(ctx context.Context, id string) (*models.Category, error) {
	var c models.Category
	err := s.db.QueryRowContext(ctx,
		`SELECT id,name,slug,COALESCE(description,''),COALESCE(icon,''),premium,status,sort_order,created_at,updated_at FROM categories WHERE id = ?`, id).
		Scan(&c.ID, &c.Name, &c.Slug, &c.Description, &c.Icon, &c.Premium, &c.Status, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &c, err
}

func (s *ContentStore) AddCategoryToCollection(ctx context.Context, collectionID, categoryID string, order int) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO collection_categories (collection_id, category_id, sort_order) VALUES (?,?,?)
		 ON CONFLICT(collection_id, category_id) DO UPDATE SET sort_order = excluded.sort_order`,
		collectionID, categoryID, order)
	return err
}

// ---------- Confessions ----------

func (s *ContentStore) CreateConfession(ctx context.Context, c *models.Confession) error {
	if c.ID == "" {
		c.ID = newID()
	}
	if c.Version == 0 {
		c.Version = 1
	}
	c.CreatedAt, c.UpdatedAt = now(), now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO confessions (id,category_id,title,short_text,medium_text,long_text,description,tags,intensity,language,status,author,version,published_at,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.ID, c.CategoryID, c.Title, c.ShortText, c.MediumText, c.LongText, c.Description,
		strings.Join(c.Tags, ","), c.Intensity, c.Language, c.Status, c.Author, c.Version, nullIfEmpty(c.PublishedAt), c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return err
	}
	for _, v := range c.Variants {
		if err := insertVariant(ctx, tx, c.ID, v); err != nil {
			return err
		}
	}
	for _, sc := range c.Scriptures {
		if err := insertScripture(ctx, tx, c.ID, sc); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func insertVariant(ctx context.Context, tx *sql.Tx, confessionID string, v models.ConfessionVariant) error {
	if v.ID == "" {
		v.ID = newID()
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO confession_variants (id,confession_id,label,duration_seconds,sort_order) VALUES (?,?,?,?,?)`,
		v.ID, confessionID, v.Label, v.DurationSeconds, v.SortOrder)
	return err
}

func insertScripture(ctx context.Context, tx *sql.Tx, confessionID string, s models.ScriptureRef) error {
	if s.ID == "" {
		s.ID = newID()
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO scripture_references (id,confession_id,book,chapter,verse,translation,is_direct_quote,notes,sort_order)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		s.ID, confessionID, s.Book, s.Chapter, s.Verse, s.Translation, boolInt(s.IsDirectQuote), s.Notes, s.SortOrder)
	return err
}

func (s *ContentStore) ConfessionByID(ctx context.Context, id string) (*models.Confession, error) {
	var c models.Confession
	var tags, publishedAt sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id,category_id,title,COALESCE(short_text,''),COALESCE(medium_text,''),COALESCE(long_text,''),COALESCE(description,''),COALESCE(tags,''),intensity,language,status,COALESCE(author,''),version,published_at,created_at,updated_at
		 FROM confessions WHERE id = ?`, id).
		Scan(&c.ID, &c.CategoryID, &c.Title, &c.ShortText, &c.MediumText, &c.LongText, &c.Description, &tags, &c.Intensity, &c.Language, &c.Status, &c.Author, &c.Version, &publishedAt, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if tags.Valid && tags.String != "" {
		c.Tags = strings.Split(tags.String, ",")
	}
	if publishedAt.Valid {
		c.PublishedAt = publishedAt.String
	}
	c.Variants, err = s.Variants(ctx, id)
	if err != nil {
		return nil, err
	}
	c.Scriptures, err = s.Scriptures(ctx, id)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *ContentStore) ConfessionsByCategory(ctx context.Context, categoryID string, publishedOnly bool) ([]models.Confession, error) {
	q := `SELECT id,category_id,title,COALESCE(short_text,''),COALESCE(medium_text,''),COALESCE(long_text,''),COALESCE(description,''),COALESCE(tags,''),intensity,language,status,COALESCE(author,''),version,published_at,created_at,updated_at
	      FROM confessions WHERE category_id = ?`
	if publishedOnly {
		q += ` AND status = 'published'`
	}
	// Tie-break on id: created_at can collide when several confessions are
	// written in the same instant, and an unordered tie would make session
	// generation non-reproducible.
	q += ` ORDER BY created_at, id`
	rows, err := s.db.QueryContext(ctx, q, categoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Confession
	for rows.Next() {
		var c models.Confession
		var tags, publishedAt sql.NullString
		if err := rows.Scan(&c.ID, &c.CategoryID, &c.Title, &c.ShortText, &c.MediumText, &c.LongText, &c.Description, &tags, &c.Intensity, &c.Language, &c.Status, &c.Author, &c.Version, &publishedAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		if tags.Valid && tags.String != "" {
			c.Tags = strings.Split(tags.String, ",")
		}
		if publishedAt.Valid {
			c.PublishedAt = publishedAt.String
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *ContentStore) ListConfessions(ctx context.Context, publishedOnly bool) ([]models.Confession, error) {
	q := `SELECT id,category_id,title,COALESCE(short_text,''),COALESCE(medium_text,''),COALESCE(long_text,''),COALESCE(description,''),COALESCE(tags,''),intensity,language,status,COALESCE(author,''),version,published_at,created_at,updated_at
	      FROM confessions`
	if publishedOnly {
		q += ` WHERE status = 'published'`
	}
	q += ` ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Confession
	for rows.Next() {
		var c models.Confession
		var tags, publishedAt sql.NullString
		if err := rows.Scan(&c.ID, &c.CategoryID, &c.Title, &c.ShortText, &c.MediumText, &c.LongText, &c.Description, &tags, &c.Intensity, &c.Language, &c.Status, &c.Author, &c.Version, &publishedAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		if tags.Valid && tags.String != "" {
			c.Tags = strings.Split(tags.String, ",")
		}
		if publishedAt.Valid {
			c.PublishedAt = publishedAt.String
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *ContentStore) UpdateConfessionStatus(ctx context.Context, id, status string) error {
	ts := now()
	publishedAt := nullIfEmpty("")
	if status == "published" {
		publishedAt = ts
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE confessions SET status = ?, published_at = ?, updated_at = ? WHERE id = ?`,
		status, publishedAt, ts, id)
	return err
}

// ---------- Variants & Scriptures ----------

func (s *ContentStore) Variants(ctx context.Context, confessionID string) ([]models.ConfessionVariant, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,confession_id,label,duration_seconds,sort_order FROM confession_variants WHERE confession_id = ? ORDER BY sort_order`, confessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ConfessionVariant
	for rows.Next() {
		var v models.ConfessionVariant
		if err := rows.Scan(&v.ID, &v.ConfessionID, &v.Label, &v.DurationSeconds, &v.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *ContentStore) Scriptures(ctx context.Context, confessionID string) ([]models.ScriptureRef, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,confession_id,book,COALESCE(chapter,0),COALESCE(verse,''),translation,is_direct_quote,COALESCE(notes,''),sort_order
		 FROM scripture_references WHERE confession_id = ? ORDER BY sort_order`, confessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ScriptureRef
	for rows.Next() {
		var sc models.ScriptureRef
		if err := rows.Scan(&sc.ID, &sc.ConfessionID, &sc.Book, &sc.Chapter, &sc.Verse, &sc.Translation, &sc.IsDirectQuote, &sc.Notes, &sc.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
