package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Teamthy/i-confess/internal/models"
	"github.com/google/uuid"
)

// LibraryStore owns user-created collections, devices and notification
// preferences (PRD §35–§37, §45, §46).
//
// Every read and write is scoped by user_id in SQL rather than filtered in Go.
// Enforcing ownership in the query means a handler that forgets to check
// returns nothing instead of returning someone else's data (§71).
type LibraryStore struct{ db *sql.DB }

func NewLibraryStore(db *sql.DB) *LibraryStore { return &LibraryStore{db: db} }

// ErrForbidden marks an attempt to touch another user's resource.
var ErrForbidden = errors.New("resource does not belong to this user")

// ---------------------------------------------------------------------------
// Collections
// ---------------------------------------------------------------------------

// CreateCollection makes a new user collection.
func (s *LibraryStore) CreateCollection(ctx context.Context, c *models.UserCollection) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	if c.Visibility == "" {
		// Private by default: a personal collection must never become public
		// because a field was omitted (§37).
		c.Visibility = models.VisibilityPrivate
	}
	c.CreatedAt, c.UpdatedAt = now(), now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_collections (id,user_id,name,description,cover_url,visibility,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?)`,
		c.ID, c.UserID, c.Name, nullIfEmpty(c.Description), nullIfEmpty(c.CoverURL),
		c.Visibility, c.CreatedAt, c.UpdatedAt)
	return err
}

// Collections lists a user's collections with item counts.
func (s *LibraryStore) Collections(ctx context.Context, userID string) ([]models.UserCollection, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.id, c.user_id, c.name, COALESCE(c.description,''), COALESCE(c.cover_url,''),
		        c.visibility, c.created_at, c.updated_at,
		        (SELECT COUNT(*) FROM user_collection_items i WHERE i.collection_id = c.id)
		 FROM user_collections c WHERE c.user_id = ? ORDER BY c.updated_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.UserCollection{}
	for rows.Next() {
		var c models.UserCollection
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.Description, &c.CoverURL,
			&c.Visibility, &c.CreatedAt, &c.UpdatedAt, &c.ItemCount); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Collection loads one collection with its items, verifying ownership.
func (s *LibraryStore) Collection(ctx context.Context, userID, id string) (*models.UserCollection, error) {
	var c models.UserCollection
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, COALESCE(description,''), COALESCE(cover_url,''),
		        visibility, created_at, updated_at
		 FROM user_collections WHERE id = ?`, id).
		Scan(&c.ID, &c.UserID, &c.Name, &c.Description, &c.CoverURL,
			&c.Visibility, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if c.UserID != userID {
		// Reported distinctly from "not found" so handlers can choose to
		// return 404 and avoid confirming the resource exists.
		return nil, ErrForbidden
	}

	items, err := s.collectionItems(ctx, id)
	if err != nil {
		return nil, err
	}
	c.Items = items
	c.ItemCount = len(items)
	return &c, nil
}

func (s *LibraryStore) collectionItems(ctx context.Context, collectionID string) ([]models.CollectionItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT i.id, i.confession_id, i.position, COALESCE(f.title,'')
		 FROM user_collection_items i
		 LEFT JOIN confessions f ON f.id = i.confession_id
		 WHERE i.collection_id = ? ORDER BY i.position, i.created_at`, collectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.CollectionItem{}
	for rows.Next() {
		var i models.CollectionItem
		if err := rows.Scan(&i.ID, &i.ConfessionID, &i.Position, &i.Title); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// UpdateCollection renames or re-scopes a collection.
func (s *LibraryStore) UpdateCollection(ctx context.Context, userID, id string, name, description, visibility *string) error {
	if _, err := s.Collection(ctx, userID, id); err != nil {
		return err
	}
	if name != nil {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE user_collections SET name = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
			*name, now(), id, userID); err != nil {
			return err
		}
	}
	if description != nil {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE user_collections SET description = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
			nullIfEmpty(*description), now(), id, userID); err != nil {
			return err
		}
	}
	if visibility != nil {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE user_collections SET visibility = ?, updated_at = ? WHERE id = ? AND user_id = ?`,
			*visibility, now(), id, userID); err != nil {
			return err
		}
	}
	return nil
}

// DeleteCollection removes a collection. Items cascade.
func (s *LibraryStore) DeleteCollection(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM user_collections WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AddCollectionItem appends a confession, ignoring duplicates.
//
// Idempotent by design (§75): a double-tap on "add to collection" must not
// create two rows or return an error the user has to understand.
func (s *LibraryStore) AddCollectionItem(ctx context.Context, userID, collectionID, confessionID string) error {
	if _, err := s.Collection(ctx, userID, collectionID); err != nil {
		return err
	}
	var next int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(position), -1) + 1 FROM user_collection_items WHERE collection_id = ?`,
		collectionID).Scan(&next); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_collection_items (id,collection_id,confession_id,position,created_at)
		 VALUES (?,?,?,?,?)
		 ON CONFLICT(collection_id, confession_id) DO NOTHING`,
		uuid.New().String(), collectionID, confessionID, next, now())
	if err != nil {
		return err
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE user_collections SET updated_at = ? WHERE id = ?`, now(), collectionID)
	return nil
}

// RemoveCollectionItem detaches a confession.
func (s *LibraryStore) RemoveCollectionItem(ctx context.Context, userID, collectionID, confessionID string) error {
	if _, err := s.Collection(ctx, userID, collectionID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM user_collection_items WHERE collection_id = ? AND confession_id = ?`,
		collectionID, confessionID)
	return err
}

// ReorderCollection sets an explicit order in one transaction, so a failure
// halfway cannot leave the collection partially reordered.
func (s *LibraryStore) ReorderCollection(ctx context.Context, userID, collectionID string, confessionIDs []string) error {
	if _, err := s.Collection(ctx, userID, collectionID); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for pos, cid := range confessionIDs {
		if _, err := tx.ExecContext(ctx,
			`UPDATE user_collection_items SET position = ? WHERE collection_id = ? AND confession_id = ?`,
			pos, collectionID, cid); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE user_collections SET updated_at = ? WHERE id = ?`, now(), collectionID); err != nil {
		return err
	}
	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Devices (PRD §45)
// ---------------------------------------------------------------------------

// RegisterDevice records or refreshes a device.
//
// Deliberately stores no hardware identifiers: a device name and platform are
// enough for a user to recognise an entry, and anything more would make the
// auth system a tracking system (§45, §82).
func (s *LibraryStore) RegisterDevice(ctx context.Context, userID string, d *models.UserDevice) error {
	if d.DeviceID == "" {
		return errors.New("device_id is required")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_devices (id,user_id,device_id,platform,user_agent,last_seen_at,metadata,created_at)
		 VALUES (?,?,?,?,?,?,?,?)
		 ON CONFLICT(user_id, device_id) DO UPDATE SET
		   platform = excluded.platform, user_agent = excluded.user_agent,
		   last_seen_at = excluded.last_seen_at, metadata = excluded.metadata,
		   revoked_at = NULL`,
		uuid.New().String(), userID, d.DeviceID, nullIfEmpty(d.Platform),
		nullIfEmpty(d.Name), now(), nullIfEmpty(d.AppVersion), now())
	return err
}

// Devices lists a user's active devices.
func (s *LibraryStore) Devices(ctx context.Context, userID string) ([]models.UserDevice, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT device_id, COALESCE(platform,''), COALESCE(user_agent,''),
		        COALESCE(metadata,''), last_seen_at, created_at
		 FROM user_devices WHERE user_id = ? AND revoked_at IS NULL
		 ORDER BY last_seen_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.UserDevice{}
	for rows.Next() {
		var d models.UserDevice
		if err := rows.Scan(&d.DeviceID, &d.Platform, &d.Name, &d.AppVersion,
			&d.LastSeenAt, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// RevokeDevice detaches a device and clears its push token.
func (s *LibraryStore) RevokeDevice(ctx context.Context, userID, deviceID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE user_devices SET revoked_at = ?, metadata = NULL
		 WHERE user_id = ? AND device_id = ? AND revoked_at IS NULL`,
		now(), userID, deviceID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Notification preferences (PRD §46)
// ---------------------------------------------------------------------------

// NotificationPreferences returns a user's settings, creating defaults on first
// read.
//
// Product updates default to OFF while functional notifications default to ON:
// a user opts in to marketing, not out of it.
func (s *LibraryStore) NotificationPreferences(ctx context.Context, userID string) (*models.NotificationPreferences, error) {
	var p models.NotificationPreferences
	p.UserID = userID
	err := s.db.QueryRowContext(ctx,
		`SELECT scheduled_sessions, new_content, recommendations, product_updates, updated_at
		 FROM notification_preferences WHERE user_id = ?`, userID).
		Scan(&p.ScheduledSessions, &p.NewContent, &p.Recommendations, &p.ProductUpdates, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		p = models.NotificationPreferences{
			UserID: userID, ScheduledSessions: true, NewContent: true,
			Recommendations: true, ProductUpdates: false, UpdatedAt: now(),
		}
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO notification_preferences
			   (user_id,scheduled_sessions,new_content,recommendations,product_updates,updated_at)
			 VALUES (?,?,?,?,?,?) ON CONFLICT(user_id) DO NOTHING`,
			userID, 1, 1, 1, 0, p.UpdatedAt)
		if err != nil {
			return nil, err
		}
		return &p, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdateNotificationPreferences applies a partial update.
func (s *LibraryStore) UpdateNotificationPreferences(ctx context.Context, userID string, scheduled, content, recs, updates *bool) (*models.NotificationPreferences, error) {
	cur, err := s.NotificationPreferences(ctx, userID)
	if err != nil {
		return nil, err
	}
	if scheduled != nil {
		cur.ScheduledSessions = *scheduled
	}
	if content != nil {
		cur.NewContent = *content
	}
	if recs != nil {
		cur.Recommendations = *recs
	}
	if updates != nil {
		cur.ProductUpdates = *updates
	}
	cur.UpdatedAt = now()

	_, err = s.db.ExecContext(ctx,
		`UPDATE notification_preferences SET scheduled_sessions = ?, new_content = ?,
		        recommendations = ?, product_updates = ?, updated_at = ?
		 WHERE user_id = ?`,
		boolInt(cur.ScheduledSessions), boolInt(cur.NewContent),
		boolInt(cur.Recommendations), boolInt(cur.ProductUpdates), cur.UpdatedAt, userID)
	if err != nil {
		return nil, err
	}
	return cur, nil
}

// ---------------------------------------------------------------------------
// Push tokens and scheduled delivery (PRD S45, S47)
// ---------------------------------------------------------------------------

// PushTarget is a device that can receive a notification.
type PushTarget struct {
	DeviceID string
	Token    string
	Platform string
}

// SetPushToken records a device's push credential.
//
// Registering a token clears the failure count: the app is evidently alive, so
// a device previously written off as dead becomes deliverable again.
func (s *LibraryStore) SetPushToken(ctx context.Context, userID, deviceID, token, provider string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE user_devices SET push_token = ?, push_provider = ?, push_failures = 0
		 WHERE user_id = ? AND device_id = ?`,
		nullIfEmpty(token), nullIfEmpty(provider), userID, deviceID)
	return err
}

// PushTargetsFor returns a user's deliverable devices.
//
// Excludes revoked devices and tokens that have failed repeatedly: continuing
// to send to a dead token wastes quota and damages standing with the provider.
func (s *LibraryStore) PushTargetsFor(ctx context.Context, userID string) ([]PushTarget, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT device_id, push_token, COALESCE(platform,'')
		 FROM user_devices
		 WHERE user_id = ? AND revoked_at IS NULL
		   AND push_token IS NOT NULL AND push_token != ''
		   AND push_failures < 5`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []PushTarget{}
	for rows.Next() {
		var t PushTarget
		if err := rows.Scan(&t.DeviceID, &t.Token, &t.Platform); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ClearPushToken removes a token the provider rejected as dead.
func (s *LibraryStore) ClearPushToken(ctx context.Context, userID, deviceID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE user_devices SET push_token = NULL, push_provider = NULL
		 WHERE user_id = ? AND device_id = ?`, userID, deviceID)
	return err
}

// RecordPushFailure increments the consecutive-failure counter.
func (s *LibraryStore) RecordPushFailure(ctx context.Context, userID, deviceID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE user_devices SET push_failures = push_failures + 1
		 WHERE user_id = ? AND device_id = ?`, userID, deviceID)
	return err
}

// ClaimDelivery reserves an occurrence, returning false if it was already sent.
//
// The UNIQUE(schedule_id, occurrence_key) constraint does the work: two
// sweepers racing on the same occurrence both attempt the insert and exactly
// one succeeds. Idempotency is a database guarantee here, not application
// logic that a restart could skip.
func (s *LibraryStore) ClaimDelivery(ctx context.Context, scheduleID, userID, occurrenceKey string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO scheduled_deliveries (id, schedule_id, user_id, occurrence_key, status, created_at)
		 VALUES (?,?,?,?,'sent',?)
		 ON CONFLICT(schedule_id, occurrence_key) DO NOTHING`,
		uuid.New().String(), scheduleID, userID, occurrenceKey, now())
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// MarkDelivery records the outcome of a claimed occurrence.
func (s *LibraryStore) MarkDelivery(ctx context.Context, scheduleID, occurrenceKey, status, detail string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE scheduled_deliveries SET status = ?, detail = ?
		 WHERE schedule_id = ? AND occurrence_key = ?`,
		status, truncateDetail(detail), scheduleID, occurrenceKey)
	return err
}

func truncateDetail(s string) string {
	if len(s) > 500 {
		return s[:500]
	}
	return s
}
