-- community — §70 feed, moderated never auto-publish
CREATE TABLE IF NOT EXISTS community_posts (
    id TEXT PRIMARY KEY,
    author_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body TEXT NOT NULL,
    visibility TEXT NOT NULL DEFAULT 'private' CHECK (visibility IN ('private','shared')),
    status TEXT NOT NULL DEFAULT 'submitted' CHECK (status IN ('draft','submitted','under_review','approved','rejected','published','archived')),
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_community_feed ON community_posts(visibility, status, created_at) WHERE visibility='shared';
CREATE TABLE IF NOT EXISTS community_reactions (
    id TEXT PRIMARY KEY,
    post_id TEXT NOT NULL REFERENCES community_posts(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reaction TEXT NOT NULL CHECK (reaction IN ('amen','heart','pray')),
    created_at TEXT NOT NULL,
    UNIQUE(post_id, user_id, reaction)
);
