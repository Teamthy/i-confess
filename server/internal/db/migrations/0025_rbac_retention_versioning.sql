-- RBAC tables were added by migration 0024 after the original retention and
-- versioning sweep in 0015. Keep role metadata, the permission catalogue, and
-- assignments on the same row-lifecycle contract as every other application
-- table.
ALTER TABLE rbac_permissions ADD COLUMN IF NOT EXISTS deleted_at TEXT;
ALTER TABLE rbac_permissions ADD COLUMN IF NOT EXISTS row_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE rbac_roles ADD COLUMN IF NOT EXISTS deleted_at TEXT;
ALTER TABLE rbac_roles ADD COLUMN IF NOT EXISTS row_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE rbac_user_roles ADD COLUMN IF NOT EXISTS deleted_at TEXT;
ALTER TABLE rbac_user_roles ADD COLUMN IF NOT EXISTS row_version INTEGER NOT NULL DEFAULT 1;
