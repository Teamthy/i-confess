-- RBAC hardening: custom roles, multiple roles per user, permission tracking.

CREATE TABLE IF NOT EXISTS rbac_roles (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description  TEXT,
    permissions  TEXT NOT NULL DEFAULT '[]', -- JSON array of permission strings
    is_system    INTEGER NOT NULL DEFAULT 0,
    created_by   TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS rbac_permissions (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description  TEXT,
    category     TEXT NOT NULL DEFAULT 'other',
    created_at   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS rbac_user_roles (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL,
    role_id     TEXT NOT NULL,
    assigned_by TEXT,
    created_at  TEXT NOT NULL,
    UNIQUE(user_id, role_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (role_id) REFERENCES rbac_roles(id) ON DELETE CASCADE
);

-- Seed system roles into rbac_roles for queryability (idempotent)
-- These are marked is_system=1 and their permissions are the source of truth from code,
-- but storing them allows JOINs and audit.
INSERT INTO rbac_roles (id, name, display_name, description, permissions, is_system, created_at, updated_at)
VALUES
('system:super_admin', 'super_admin', 'Super Admin', 'Full platform access', '["*"]', 1, now(), now()),
('system:content_admin', 'content_admin', 'Content Admin', 'Manages categories, confessions, editorial lifecycle', '["content:read","content:write","content:publish","content:review","content:delete","category:manage","bible:read","bible:write","audio:read","moderation:read","system:read"]', 1, now(), now()),
('system:audio_producer', 'audio_producer', 'Audio Producer', 'Produces, QA, publishes audio', '["audio:read","audio:write","audio:qa","audio:publish","audio:archive","audio:generate","voice:read","voice:write","content:read","system:read","queue:read"]', 1, now(), now()),
('system:voice_manager', 'voice_manager', 'Voice Manager', 'Manages voice rights and licensing', '["voice:read","voice:write","voice:rights:read","voice:rights:manage","audio:read","audio:write","audio:qa","audio:generate","system:read"]', 1, now(), now()),
('system:theological_reviewer', 'theological_reviewer', 'Theological Reviewer', 'Reviews content for theological accuracy', '["content:read","content:review","bible:read","bible:review","moderation:read"]', 1, now(), now()),
('system:support_admin', 'support_admin', 'Support Admin', 'Supports users, moderation', '["user:read","user:write","moderation:read","moderation:write","moderation:decide","content:read","bible:read","system:read","audit:read"]', 1, now(), now()),
('system:analytics_admin', 'analytics_admin', 'Analytics Admin', 'Views analytics and audit', '["system:read","system:metrics","system:health","queue:read","audit:read","audit:export","security:read","content:read","user:read","billing:read","bible:read"]', 1, now(), now()),
('system:moderator', 'moderator', 'Moderator', 'Moderates community content', '["moderation:read","moderation:write","moderation:decide","content:read","user:read","audit:read"]', 1, now(), now()),
('system:bible_admin', 'bible_admin', 'Bible Admin', 'Manages Bible translations and rights', '["bible:read","bible:write","bible:review","bible:publish","bible:audio:manage","content:read","system:read"]', 1, now(), now()),
('system:admin', 'admin', 'Admin (Legacy)', 'Legacy broad access', '["content:read","content:write","content:publish","audio:read","audio:write","voice:read","user:read","user:write","moderation:read","moderation:write","bible:read","bible:write","system:read","audit:read"]', 1, now(), now())
ON CONFLICT(name) DO NOTHING;

-- Seed permissions catalog
INSERT INTO rbac_permissions (id, name, display_name, description, category, created_at)
VALUES
('perm:content:read', 'content:read', 'View Content', 'View categories and confessions', 'content', now()),
('perm:content:write', 'content:write', 'Edit Content', 'Create and edit content', 'content', now()),
('perm:content:publish', 'content:publish', 'Publish Content', 'Publish and archive content', 'content', now()),
('perm:content:review', 'content:review', 'Review Content', 'Review editorial stages', 'content', now()),
('perm:content:delete', 'content:delete', 'Delete Content', 'Delete content', 'content', now()),
('perm:category:manage', 'category:manage', 'Manage Categories', 'Manage categories', 'content', now()),
('perm:audio:read', 'audio:read', 'View Audio', 'View audio assets', 'audio', now()),
('perm:audio:write', 'audio:write', 'Manage Audio', 'Manage audio assets', 'audio', now()),
('perm:audio:qa', 'audio:qa', 'Audio QA', 'Approve or reject audio', 'audio', now()),
('perm:audio:publish', 'audio:publish', 'Publish Audio', 'Publish audio', 'audio', now()),
('perm:audio:archive', 'audio:archive', 'Archive Audio', 'Archive audio', 'audio', now()),
('perm:audio:generate', 'audio:generate', 'Generate Audio', 'Trigger audio generation', 'audio', now()),
('perm:voice:read', 'voice:read', 'View Voices', 'View voices', 'audio', now()),
('perm:voice:write', 'voice:write', 'Manage Voices', 'Manage voices', 'audio', now()),
('perm:voice:rights:read', 'voice:rights:read', 'View Voice Rights', 'View voice rights', 'audio', now()),
('perm:voice:rights:manage', 'voice:rights:manage', 'Manage Voice Rights', 'Manage voice rights - high consequence', 'audio', now()),
('perm:bible:read', 'bible:read', 'View Bible', 'View Bible catalog', 'bible', now()),
('perm:bible:write', 'bible:write', 'Manage Bible', 'Manage Bible translations', 'bible', now()),
('perm:bible:review', 'bible:review', 'Review Bible Rights', 'Review Bible rights', 'bible', now()),
('perm:bible:publish', 'bible:publish', 'Publish Bible', 'Publish Bible content', 'bible', now()),
('perm:bible:audio:manage', 'bible:audio:manage', 'Manage Bible Audio', 'Manage Bible audio', 'bible', now()),
('perm:user:read', 'user:read', 'View Users', 'View user accounts', 'users', now()),
('perm:user:write', 'user:write', 'Edit Users', 'Edit users', 'users', now()),
('perm:user:suspend', 'user:suspend', 'Suspend Users', 'Suspend users', 'users', now()),
('perm:user:delete', 'user:delete', 'Delete Users', 'Erase users', 'users', now()),
('perm:user:impersonate', 'user:impersonate', 'Impersonate Users', 'Impersonate for support', 'users', now()),
('perm:role:read', 'role:read', 'View Roles', 'View roles', 'rbac', now()),
('perm:role:write', 'role:write', 'Manage Roles', 'Create/edit roles', 'rbac', now()),
('perm:role:assign', 'role:assign', 'Assign Roles', 'Assign roles', 'rbac', now()),
('perm:role:delete', 'role:delete', 'Delete Roles', 'Delete roles', 'rbac', now()),
('perm:moderation:read', 'moderation:read', 'View Moderation', 'View moderation', 'moderation', now()),
('perm:moderation:write', 'moderation:write', 'Manage Moderation', 'Manage moderation', 'moderation', now()),
('perm:moderation:decide', 'moderation:decide', 'Decide Moderation', 'Decide moderation', 'moderation', now()),
('perm:billing:read', 'billing:read', 'View Billing', 'View billing', 'billing', now()),
('perm:billing:write', 'billing:write', 'Manage Billing', 'Manage billing', 'billing', now()),
('perm:pricing:manage', 'pricing:manage', 'Manage Pricing', 'Manage pricing', 'billing', now()),
('perm:system:read', 'system:read', 'View System', 'View system health', 'system', now()),
('perm:system:write', 'system:write', 'Manage System', 'Manage system', 'system', now()),
('perm:system:metrics', 'system:metrics', 'View Metrics', 'View metrics', 'system', now()),
('perm:system:health', 'system:health', 'View Health', 'View health checks', 'system', now()),
('perm:queue:read', 'queue:read', 'View Queue', 'View job queue', 'system', now()),
('perm:queue:write', 'queue:write', 'Manage Queue', 'Manage job queue', 'system', now()),
('perm:cache:manage', 'cache:manage', 'Manage Cache', 'Manage caches', 'system', now()),
('perm:audit:read', 'audit:read', 'View Audit', 'View audit logs', 'audit', now()),
('perm:audit:export', 'audit:export', 'Export Audit', 'Export audit logs', 'audit', now()),
('perm:security:read', 'security:read', 'View Security', 'View security events', 'audit', now()),
('perm:security:write', 'security:write', 'Manage Security', 'Manage security', 'audit', now())
ON CONFLICT(name) DO NOTHING;

-- Enhanced audit log with RBAC context
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS actor_id TEXT;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS actor_role TEXT;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS ip_address TEXT;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS user_agent TEXT;

CREATE INDEX IF NOT EXISTS idx_audit_actor ON audit_logs(actor_id);
CREATE INDEX IF NOT EXISTS idx_audit_actor_role ON audit_logs(actor_role);

-- Add index for user search
CREATE INDEX IF NOT EXISTS idx_users_email_search ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);
