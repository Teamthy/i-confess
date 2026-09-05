package community

import "time"

// Policy §70 — Community feed, shared confessions, prayer requests, reactions, moderation.
// Never auto-publish user content as system content (§10). Every post is
// DRAFT → SUBMITTED → UNDER_REVIEW → APPROVED|REJECTED → PUBLISHED|ARCHIVED.
// Admin queue is in server/internal/api/moderation (already exists).

const (
	VisibilityPrivate = "private"
	VisibilityShared  = "shared"
)

const (
	StatusDraft       = "draft"
	StatusSubmitted   = "submitted"
	StatusUnderReview = "under_review"
	StatusApproved    = "approved"
	StatusRejected    = "rejected"
	StatusPublished   = "published"
	StatusArchived    = "archived"
)

type Post struct {
	ID         string    `json:"id"`
	AuthorID   string    `json:"author_id"`
	Body       string    `json:"body"` // user confession text, never becomes system confession without review
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

// Feed is ordered by recency, approved only.
func FilterFeed(posts []Post) []Post {
	var out []Post
	for _, p := range posts {
		if CanPublish(p.Status) && p.Visibility == VisibilityShared {
			out = append(out, p)
		}
	}
	return out
}
