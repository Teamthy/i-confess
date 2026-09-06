package community

import "time"

// Policy §70 — Community feed, shared confessions, prayer requests, reactions, moderation.
// Never auto-publish user content as system content (§10). Every post is
// DRAFT → SUBMITTED → UNDER_REVIEW → APPROVED|REJECTED → PUBLISHED|ARCHIVED.
// Admin queue is in server/internal/api/moderation (already exists).

// The three visibility levels from directive sections 22 and 70.
//
// VisibilityPublic closes gap G-4. It was missing entirely, which meant the
// public moderation pipeline in section 22 had nothing to publish to: a post
// an administrator approved could only ever reach the author's own circle,
// because "shared" was the widest value the column could hold.
const (
	// VisibilityPrivate is visible to the author only.
	VisibilityPrivate = "private"

	// VisibilityShared is visible to the author's circle.
	VisibilityShared = "shared"

	// VisibilityPublic is visible to everyone, and is the destination of the
	// moderation pipeline. Reaching it requires an admin approval.
	VisibilityPublic = "public"
)

// visibilities is the authority behind IsValidVisibility. The CHECK constraint
// in migrations/0003 carries the same three values.
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

// FilterFeed returns approved posts the community feed may show: shared and
// public. Private posts are excluded regardless of status.
func FilterFeed(posts []Post) []Post {
	var out []Post
	for _, p := range posts {
		if CanPublish(p.Status) && (p.Visibility == VisibilityShared || p.Visibility == VisibilityPublic) {
			out = append(out, p)
		}
	}
	return out
}

// FilterPublic returns only posts the moderation pipeline has published to
// everyone. This is the feed section 22 describes, and before VisibilityPublic
// existed it could not have returned anything.
func FilterPublic(posts []Post) []Post {
	var out []Post
	for _, p := range posts {
		if CanPublish(p.Status) && p.Visibility == VisibilityPublic {
			out = append(out, p)
		}
	}
	return out
}
