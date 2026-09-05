package offline

import "time"

// Licence is the offline download grant. Server issues a licence with expiry
// (TTL = OfflineHoursAllowed from entitlements) and a signed URL (1h).
// Client stores encrypted metadata; server can revoke by deleting the licence.
type Licence struct {
	AssetID   string    `json:"asset_id"`
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"` // licence TTL
	SignedURL string    `json:"signed_url"` // 1h
	IssuedAt  time.Time `json:"issued_at"`
}

// IsExpired is the single check offline playback uses — never trust client clock alone,
// but this is the local guard; server re-validates on renewal.
func (l Licence) IsExpired(now time.Time) bool {
	return now.After(l.ExpiresAt)
}

// NeedsRenewal returns true if the licence expires within 24h.
func (l Licence) NeedsRenewal(now time.Time) bool {
	return l.ExpiresAt.Sub(now) < 24*time.Hour
}
