package community

import "time"

// Policy §70 — Community feed, shared confessions, prayer requests, reactions, moderation.
// Never auto-publish user content as system content (§10). Every post is
// DRAFT → SUBMITTED → UNDER_REVIEW → APPROVED|REJECTED → PUBLISHED|ARCHIVED.

const (
	// VisibilityPrivate is visible to the author only.
	VisibilityPrivate = "private"

	// VisibilityShared is visible to the author's circle.
	VisibilityShared = "shared"

	// VisibilityPublic is visible to everyone, and is the destination of the
	// moderation pipeline. Reaching it requires an admin approval.
	VisibilityPublic = "public"
)

var visibilities = []string{VisibilityPrivate, VisibilityShared, VisibilityPublic}

// IsValidVisibility reports whether s is one of the three levels.
func IsValidVisibility(s string) bool {
	for _, v := range visibilities {
		if v == s {
			return true
		}
	}
	return false
}

const (
	StatusDraft       = "draft"
	StatusSubmitted   = "submitted"
	StatusUnderReview = "under_review"
	StatusApproved    = "approved"
	StatusRejected    = "rejected"
	StatusPublished   = "published"
	StatusArchived    = "archived"
)

// Post is the internal domain representation with author identity.
type Post struct {
	ID         string    `json:"id"`
	AuthorID   string    `json:"author_id,omitempty"`
	Body       string    `json:"body"`
	Visibility string    `json:"visibility"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// FeedPost is the public feed representation: AuthorID is completely omitted for anonymity (IC-006).
type FeedPost struct {
	ID         string    `json:"id"`
	Body       string    `json:"body"`
	Visibility string    `json:"visibility"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

type Reaction string

const (
	ReactionAmen  Reaction = "amen"
	ReactionHeart Reaction = "heart"
	ReactionPray  Reaction = "pray"
)

// CanPublish returns true only for admin-approved posts.
func CanPublish(status string) bool { return status == StatusApproved || status == StatusPublished }

// FilterFeed returns approved posts the community feed may show: shared and
// public. Private posts are excluded regardless of status.
func FilterFeed(posts []Post) []FeedPost {
	var out []FeedPost
	for _, p := range posts {
		if CanPublish(p.Status) && (p.Visibility == VisibilityShared || p.Visibility == VisibilityPublic) {
			out = append(out, FeedPost{
				ID:         p.ID,
				Body:       p.Body,
				Visibility: p.Visibility,
				Status:     p.Status,
				CreatedAt:  p.CreatedAt,
			})
		}
	}
	return out
}

// FilterPublic returns only posts published to everyone.
func FilterPublic(posts []Post) []FeedPost {
	var out []FeedPost
	for _, p := range posts {
		if CanPublish(p.Status) && p.Visibility == VisibilityPublic {
			out = append(out, FeedPost{
				ID:         p.ID,
				Body:       p.Body,
				Visibility: p.Visibility,
				Status:     p.Status,
				CreatedAt:  p.CreatedAt,
			})
		}
	}
	return out
}
