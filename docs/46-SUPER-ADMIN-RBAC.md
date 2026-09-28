# Super Admin & RBAC — Phase 46

## Overview

The super admin console is the ultimate authority surface for iCONFESS. It provides:

- **RBAC**: Role-Based Access Control with granular permissions
- **User management**: Search, suspend, revoke sessions, assign roles
- **System observability**: Health, metrics, queue, cache
- **Security**: Audit logs, security events, token reuse detection
- **Content governance**: Lifecycle, Bible rights, audio QA

All authorization is **server-side**. The client never decides permission; it only renders the server's 403/401.

## Roles

System roles are code-defined in `server/internal/auth/rbac.go` and seeded into `rbac_roles` table.

| Role | Level | Category | Description |
|------|-------|----------|-------------|
| `super_admin` | 100 | administration | Full access, including RBAC management and user erasure. Bypasses all permission checks. |
| `voice_manager` | 75 | audio | Manages voice rights & licensing — highest consequence permission. |
| `content_admin` | 70 | content | Manages categories, confessions, editorial lifecycle. |
| `bible_admin` | 65 | content | Manages Bible translations, rights, plans, audio. |
| `audio_producer` | 60 | audio | Produces, QA, publishes audio. |
| `theological_reviewer` | 60 | content | Theological review stage. |
| `admin` (legacy) | 60 | administration | Broad legacy access. Migrate to specific roles. |
| `support_admin` | 50 | support | User support, moderation decisions. |
| `moderator` | 45 | moderation | Community moderation. |
| `analytics_admin` | 40 | analytics | Read-only metrics, audit, security. |

Level determines assignment privilege: you can only assign roles with **lower level** than your own. Only super_admin can assign any role.

## Permissions

Granular permissions are defined as `resource:action` strings, e.g.:

- `content:read`, `content:write`, `content:publish`, `content:review`, `content:delete`
- `audio:read`, `audio:write`, `audio:qa`, `audio:publish`, `audio:generate`
- `voice:rights:manage` — critical, voice synthesis authorization
- `bible:read`, `bible:write`, `bible:review`, `bible:publish`
- `user:read`, `user:write`, `user:suspend`, `user:delete`, `user:impersonate`
- `role:read`, `role:write`, `role:assign`, `role:delete`
- `moderation:read`, `moderation:decide`
- `system:read`, `system:health`, `system:metrics`, `queue:read`, `queue:write`
- `audit:read`, `audit:export`, `security:read`

`*` grants all permissions (super_admin).

See `PermissionDescriptions` in `rbac.go` for full catalog grouped by category.

## Router enforcement (updated)

`server/internal/api/router.go` now uses RBAC-aware wrappers, not just generic super_admin:

- `admin` — super_admin only (stats, metrics, audit, rbac, system, security, erase, role assignment)
- `contentMgr` — content_admin, bible_admin, theological_reviewer, admin (legacy) — categories, confessions, QA, bible overview/health/catalog
- `moderationMgr` — support_admin, moderator, content_admin, admin — moderation queue, user-confession review, report decisions, appeal decisions
- `audioMgr` — audio_producer, voice_manager — voices list/create, audio attach, generation
- `voiceMgr` — voice_manager only (rights sensitive)
- `supportMgr` — support_admin, admin — user listing, user detail, roles read, sessions list/revoke, admin accounts list, suspend/restore
- `billingMgr` — content_admin, admin — plans read (write remains super_admin)

Super_admin is admitted everywhere because `RequireRoleWithSessions` always allows super_admin bypass (see `rbac.go`).

Both `/admin/*` and `/v1/admin/*` aliases use the same wrappers.

### Impersonation

`POST /admin/users/{id}/impersonate` — super_admin only, audited, 15m TTL, cannot impersonate another super_admin. Issues a regular user session JWT (no admin role) for support debugging. Logged to audit trail as `user_impersonated`.

### Test update

`admin_authorization_test.go` `TestRoleScopingIsEnforced` now uses `/admin/rbac/roles` as the super-only route (previously `/admin/categories` which now admits content_admin via contentMgr) and adds positive cases for content_admin on content routes and super_admin everywhere.

## Database Schema

Migration `0024_rbac.sql` introduces:

- `rbac_roles`: id, name UNIQUE, display_name, description, permissions JSON, is_system, created_by, timestamps. System roles seeded.
- `rbac_permissions`: id, name UNIQUE, display_name, description, category. Full catalog seeded.
- `rbac_user_roles`: id, user_id, role_id, assigned_by, created_at, UNIQUE(user_id, role_id). Supports multiple roles per user.
- Enhanced `audit_logs` with actor_id, actor_role, ip_address, user_agent.

Backward compatibility: `admin_users` table (single role per user) is still read and kept in sync with `rbac_user_roles`. Highest-level role is stored there for legacy middleware.

## Backend Implementation

### `auth/rbac.go`

- Defines `Permission` type and all constants
- `SystemRoles` map with `RoleDefinition` (name, display_name, description, permissions, level, category, is_system)
- `HasPermission(role, perm)`, `HasAnyPermission`, `HasAllPermissions`, `GetPermissionsForRole`, `ListSystemRoles`, `CanAssignRole`
- `PermissionCategories` and `PermissionDescriptions` for UI

### `store/rbac.go`

- `RBACStore` with methods:
  - `ListSystemRoles()`, `ListCustomRoles()`, `ListAllRoles()`
  - `GetRole(name)`, `CreateCustomRole()`, `UpdateCustomRole()`, `DeleteCustomRole()`
  - `ListPermissions()`, `GetUserRoles()`, `AssignRole()`, `RemoveRole()`, `ListUsersWithRoles()`, `GetUserEffectivePermissions()`
- Handles missing tables gracefully for tests that haven't migrated.

### `api/admin_rbac.go`

Super admin endpoints (all behind `admin` middleware which admits super_admin only):

- `GET /admin/rbac/roles` — list all roles
- `GET /admin/rbac/roles/{name}` — get role
- `POST /admin/rbac/roles` — create custom role (super_admin only, validates name pattern `^[a-z0-9_]{3,40}$` and permissions)
- `PUT /admin/rbac/roles/{name}` — update custom role
- `DELETE /admin/rbac/roles/{name}` — delete custom role (fails if assigned)
- `GET /admin/rbac/permissions` — list permissions grouped by category
- `GET /admin/users` — list users with roles, search, pagination
- `GET /admin/users/{id}` — user detail with roles, permissions, subscription, sessions
- `GET /admin/users/{id}/roles` — user roles & effective permissions
- `POST /admin/users/{id}/roles` — assign role (checks `CanAssignRole`)
- `DELETE /admin/users/{id}/roles` — remove role (prevents self-demotion of last super_admin)
- `GET /admin/users/{id}/sessions` — list sessions
- `POST /admin/users/{id}/sessions/revoke` — revoke all sessions
- `GET /admin/system/health` — inventory, subsystems, cache, queue
- `GET /admin/security/overview` — counters, recent security_events
- `GET /admin/audit/export` — export audit as JSON or CSV (super_admin only)

All handlers audit via `recordAudit`.

### `auth/session.go`

Added:

- `RequirePermission(secret, validator, perm)` — checks single permission
- `RequireAnyPermission(secret, validator, perms...)` — checks any permission

Existing `RequireRoleWithSessions` already admits super_admin everywhere.

## Frontend Implementation

### Layout

`web/app/admin/layout.tsx` now has:

- Sidebar with grouped navigation (Platform, Content, Community, People, Business)
- Topbar with super admin badge
- Sticky sidebar, responsive (collapses to horizontal scroll on mobile)
- New CSS in `globals.css`: `.adm-layout`, `.adm-sidebar`, `.adm-super-grid`, `.adm-role-card`, etc.

### Components

`web/components/super-admin.tsx`:

- `SuperAdminOverview`: platform stats, role hierarchy, system & security, recent users. Uses `useAdminResource` for `/admin/stats`, `/admin/rbac/roles`, `/admin/users`, `/admin/system/health`, etc.
- `RBACConsole`: tabs for roles, permissions, matrix. Create custom role form with permission checkboxes, delete custom roles, permission matrix table.
- `SuperAdminUsers`: searchable user list, detail drawer with roles, permissions, sessions, assign/revoke roles, suspend/restore, revoke sessions.
- `SystemHealthConsole`: inventory, subsystems, queue, cache, security counters.
- `SecurityConsole`: security overview, recent security_events, privileged actions.

`web/components/admin.tsx`:

- `AdminGate` enhanced to fetch current user's role and permissions and show badges (super admin green pill, role blue pill, permission count).
- Existing panels (AdminContent, AdminUsers, etc.) remain but now work within new layout.

### Pages

- `/admin` — SuperAdminOverview
- `/admin/super` — SuperAdminOverview (explicit super admin dashboard)
- `/admin/rbac` — RBACConsole
- `/admin/users` — SuperAdminUsers
- `/admin/system` — SystemHealthConsole
- `/admin/security` — SecurityConsole
- `/admin/audit` — Enhanced audit with JSON/CSV export
- `/admin/content`, `/admin/bible`, `/admin/audio`, `/admin/moderation`, `/admin/queue`, `/admin/pricing` — existing, now within new layout

All pages use `AdminGate` which checks token and shows admin role.

## Security Properties

- **Server decides**: Every admin endpoint re-reads role from DB on each request (`SessionValidator`), so demotion/revocation takes effect immediately.
- **Privilege levels**: Role assignment respects hierarchy; cannot assign equal/higher level.
- **Last super admin protection**: Cannot remove own super_admin if last one.
- **Audit**: Every role assignment, revocation, creation, deletion, user status change, session revocation is recorded via `recordAudit` to `audit_logs`.
- **Token binding**: JWT contains session ID; `ValidateSession` checks revocation, expiry, account status, and returns authoritative role.
- **No client-side gate**: UI shows role badges but API enforces; 403 is rendered as message, not hidden.

## Future Work

- Custom roles currently validated only against system permissions; could support permission inheritance.
- UI could add bulk role assignment, user impersonation (permission already defined), and audit log filters.
- Permission middleware could be used on content routes to allow content_admin to publish without super_admin.
- Add `rbac_role_permissions` normalized table if custom roles need many permissions.

## Testing

- `TestEveryAdminRouteRejectsANonAdmin` ensures non-admin never gets 2xx on admin routes.
- `TestRoleScopingIsEnforced` ensures role gates work (e.g., content_admin denied voice_manager route, super_admin admitted everywhere).
- New RBAC endpoints are covered by same sweep because they are registered as `admin`.

Run: `make test` (requires TEST_DATABASE_URL).

## References

- `server/internal/auth/rbac.go`
- `server/internal/store/rbac.go`
- `server/internal/api/admin_rbac.go`
- `server/internal/db/migrations/0024_rbac.sql`
- `web/app/admin/layout.tsx`
- `web/components/super-admin.tsx`
- `docs/09-AUTH.md` (auth & sessions)
- `docs/31-MODERATION.md` (moderation)
