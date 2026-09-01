# Authentication & Authorization Guide

This document describes the complete authentication and authorization system for the i-confess backend.

## Table of Contents

- [Overview](#overview)
- [Authentication Methods](#authentication-methods)
- [JWT Claims](#jwt-claims)
- [Password Management](#password-management)
- [Admin Roles](#admin-roles)
- [API Endpoints](#api-endpoints)
- [Middleware](#middleware)
- [Security Considerations](#security-considerations)

---

## Overview

The i-confess backend uses **JWT (JSON Web Tokens)** for stateless authentication, combined with **bcrypt** for secure password hashing. All authentication credentials should be transmitted over HTTPS/TLS in production.

### Key Features

✅ **JWT-based authentication** (HS256 signing)
✅ **Bcrypt password hashing** (cost factor: 12)
✅ **Role-based access control (RBAC)** for admin operations
✅ **Token expiration** and refresh capability
✅ **Account status management** (active, suspended, deleted)
✅ **Audit logging** for sensitive operations

---

## Authentication Methods

### 1. User Registration

**Endpoint**: `POST /auth/register`

Create a new user account.

**Request**:
```json
{
  "email": "user@example.com",
  "password": "MySecurePass123!",
  "display_name": "John Doe",
  "timezone": "America/New_York"
}
```

**Validation**:
- Email must be unique and valid
- Password minimum 8 characters
- Timezone defaults to "UTC" if not provided

**Response** (201 Created):
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "user@example.com",
    "display_name": "John Doe",
    "timezone": "America/New_York",
    "status": "active",
    "created_at": "2026-09-01T10:00:00Z"
  }
}
```

**Auto-provisioning**:
- User automatically gets a "free" subscription
- Default session preferences created (1800 seconds = 30 minutes)

### 2. User Login

**Endpoint**: `POST /auth/login`

Authenticate an existing user.

**Request**:
```json
{
  "email": "user@example.com",
  "password": "MySecurePass123!"
}
```

**Response** (200 OK):
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": { ... }
}
```

**Error Cases**:
- 401 Unauthorized: Invalid email/password combination
- 403 Forbidden: Account is suspended or deleted

### 3. Token Refresh

**Endpoint**: `POST /auth/refresh` (Authenticated)

Get a new access token without re-entering credentials.

**Request**:
```bash
curl -X POST http://localhost:8080/auth/refresh \
  -H "Authorization: Bearer <current-token>"
```

**Response** (200 OK):
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

**Use Case**: Client detects token expiration (via `exp` claim) and calls this endpoint to get a fresh token.

### 4. Get Current User Profile

**Endpoint**: `GET /me` (Authenticated)

Retrieve authenticated user's profile and subscription info.

**Request**:
```bash
curl http://localhost:8080/me \
  -H "Authorization: Bearer <token>"
```

**Response** (200 OK):
```json
{
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "user@example.com",
    "display_name": "John Doe",
    "timezone": "America/New_York",
    "status": "active",
    "created_at": "2026-09-01T10:00:00Z"
  },
  "plan": "free"
}
```

---

## JWT Claims

### Standard Claims

```json
{
  "sub": "550e8400-e29b-41d4-a716-446655440000",
  "email": "user@example.com",
  "role": "content_admin",
  "iss": "i-confess",
  "iat": 1693574400,
  "exp": 1693660800
}
```

| Claim | Type | Description |
|-------|------|-------------|
| `sub` (Subject) | UUID | User ID |
| `email` | string | User email |
| `role` | string | Admin role (empty for regular users) |
| `iss` (Issuer) | string | Always "i-confess" |
| `iat` (Issued At) | timestamp | Token creation time (Unix seconds) |
| `exp` (Expiration) | timestamp | Token expiration time |

### Token Lifetimes

| Environment | TTL | Use Case |
|-------------|-----|----------|
| Development | 720 hours (30 days) | Long-lived for testing |
| Staging | 24 hours | Realistic expiration |
| Production | 4 hours | Short-lived for security |

**To change token TTL**:
```bash
export TOKEN_TTL=8h  # Any valid Go duration string
```

---

## Password Management

### Change Password

**Endpoint**: `POST /auth/change-password` (Authenticated)

Update the user's password.

**Request**:
```json
{
  "current_password": "MySecurePass123!",
  "new_password": "NewSecurePass456!"
}
```

**Validation**:
- Current password must match
- New password minimum 8 characters
- New password must differ from current

**Response** (200 OK):
```json
{
  "message": "password updated successfully"
}
```

**Error Cases**:
- 401 Unauthorized: Current password incorrect
- 400 Bad Request: New password too short

### Password Reset (Future Enhancement)

Future versions will support:
- Email-based password reset tokens (temporary, 1-hour expiry)
- Reset link generation and validation
- Email delivery via SendGrid/Twilio

---

## Admin Roles

### Role Hierarchy

| Role | Purpose | Permissions |
|------|---------|-------------|
| `super_admin` | Platform operator | All operations, user/admin management |
| `content_admin` | Content editor | Create/edit confessions, categories, collections |
| `audio_producer` | Audio specialist | Manage voices, audio variants, TTS jobs |
| `theological_reviewer` | Spiritual review | Approve content for publication |
| `support_admin` | Customer support | Manage user status, subscriptions, issues |
| `analytics_admin` | Data analyst | Access metrics, dashboards, reports |

### Set Admin Role

**Endpoint**: `POST /admin/users/role` (Admin Only)

Promote a user to an admin role.

**Request**:
```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "role": "content_admin"
}
```

**Valid Roles**: `super_admin`, `content_admin`, `audio_producer`, `theological_reviewer`, `support_admin`, `analytics_admin`

**Response** (200 OK):
```json
{
  "message": "admin role set successfully"
}
```

### Remove Admin Role

**Endpoint**: `DELETE /admin/users/role` (Admin Only)

Demote an admin back to a regular user.

**Request**:
```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

**Response** (200 OK):
```json
{
  "message": "admin role removed successfully"
}
```

### List All Admins

**Endpoint**: `GET /admin/users/admins` (Admin Only)

Retrieve all users with admin roles.

**Response** (200 OK):
```json
[
  {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "admin@example.com",
    "display_name": "Admin User",
    "timezone": "UTC",
    "status": "active",
    "created_at": "2026-09-01T10:00:00Z"
  }
]
```

### Set User Status

**Endpoint**: `POST /admin/users/status` (Admin Only)

Update a user's account status (e.g., suspend, activate).

**Request**:
```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "suspended"
}
```

**Valid Statuses**: `active`, `suspended`, `deleted`

**Response** (200 OK):
```json
{
  "message": "user status updated successfully"
}
```

### Set User Subscription

**Endpoint**: `POST /admin/users/subscription` (Admin Only)

Update a user's subscription plan and status.

**Request**:
```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "plan": "premium",
  "status": "active"
}
```

**Valid Plans**: `free`, `premium`
**Valid Statuses**: `active`, `cancelled`, `expired`

**Response** (200 OK):
```json
{
  "message": "subscription updated successfully"
}
```

---

## API Endpoints

### Public Endpoints (No Auth Required)

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/auth/register` | Create new account |
| POST | `/auth/login` | Authenticate user |
| GET | `/collections` | List public content |
| GET | `/categories` | List categories |
| GET | `/confessions/{id}` | Get confession detail |
| GET | `/voices` | List voices |

### Authenticated User Endpoints (JWT Required)

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/me` | Get profile + subscription |
| POST | `/auth/change-password` | Update password |
| POST | `/auth/refresh` | Get new token |
| POST | `/sessions` | Create session |
| GET | `/sessions/{id}` | Get session |
| GET | `/schedules` | List recurring schedules |
| POST | `/me/favorites` | Add favorite |
| GET | `/me/history` | View listening history |

### Admin Endpoints (JWT + Admin Role Required)

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/admin/stats` | Dashboard metrics |
| POST | `/admin/categories` | Create category |
| POST | `/admin/confessions` | Create confession |
| POST | `/admin/voices` | Manage voices |
| POST | `/admin/users/role` | Set admin role |
| DELETE | `/admin/users/role` | Remove admin role |
| GET | `/admin/users/admins` | List admins |
| POST | `/admin/users/status` | Update user status |
| POST | `/admin/users/subscription` | Update subscription |

---

## Middleware

### Authentication Middleware

All authenticated endpoints use:
```go
authed := auth.Middleware(h.cfg.JWTSecret)
mux.Handle("GET /me", authed(http.HandlerFunc(h.me)))
```

**Behavior**:
1. Extracts token from `Authorization: Bearer <token>` header
2. Validates JWT signature and expiration
3. Attaches claims to request context
4. Returns 401 if token missing or invalid

### Admin Middleware

All admin endpoints use:
```go
admin := auth.AdminMiddleware(h.cfg.JWTSecret)
mux.Handle("GET /admin/stats", admin(http.HandlerFunc(h.adminStats)))
```

**Behavior**:
1. Runs all auth middleware checks
2. Verifies user has a non-empty `role` claim
3. Returns 403 if user is not an admin

### Role-Based Access Control (Future)

For fine-grained role checking:
```go
contentAdmins := auth.RoleBasedMiddleware(secret, auth.RoleContentAdmin, auth.RoleSuperAdmin)
mux.Handle("POST /admin/categories", contentAdmins(handler))
```

---

## Security Considerations

### Best Practices

✅ **Use HTTPS/TLS** in production (enforce via Nginx/ALB)
✅ **Secure JWT Secret** with strong entropy (32+ bytes minimum)
✅ **Rotate secrets regularly** (every 30 days recommended)
✅ **Set appropriate token TTL** (shorter = more secure, longer = better UX)
✅ **Implement rate limiting** on auth endpoints (prevent brute-force)
✅ **Log authentication events** for audit trail
✅ **Never log passwords or tokens**

### Common Attacks & Mitigations

| Attack | Mitigation |
|--------|-----------|
| **Brute-force login** | Rate limiting (e.g., 5 attempts per 15 min per IP) |
| **Token theft** | HTTPS-only, short TTL, secure HttpOnly cookies (client-side) |
| **Token forgery** | Validate signature with HMAC-SHA256 |
| **Expired token use** | Check `exp` claim during parsing |
| **Cross-site requests** | Validate `Origin` header, use CSRF tokens for sensitive ops |
| **Privilege escalation** | Validate admin role server-side, never trust client claims |

### Environment Variables

```bash
# Authentication
JWT_SECRET="<32+ random bytes, base64 or hex encoded>"
TOKEN_TTL="4h"              # Production (dev: 720h, staging: 24h)

# Security
SECURE_COOKIES=true         # HttpOnly, Secure, SameSite=Strict
RATE_LIMIT_AUTH="5/15m"     # 5 attempts per 15 minutes
REQUIRE_TLS=true            # Redirect HTTP → HTTPS
```

### Secrets Management

**DO NOT**:
- ❌ Commit `.env` files to git
- ❌ Store secrets in environment files checked into version control
- ❌ Use weak JWT secrets
- ❌ Log authentication credentials

**DO**:
- ✅ Use AWS Secrets Manager / HashiCorp Vault
- ✅ Rotate secrets every 30 days
- ✅ Use strong random secrets (generate with: `openssl rand -base64 32`)
- ✅ Audit access to secrets

### Audit Logging

Sensitive operations log events for compliance:

```
2026-09-01T10:05:30Z [INFO] admin=content_admin action=set_user_role user=550e8400-... target_user=abc12345-... role=audio_producer
2026-09-01T10:06:15Z [INFO] admin=super_admin action=set_user_status user=550e8400-... target_user=xyz67890-... status=suspended
2026-09-01T10:07:00Z [INFO] user=abc12345-... action=change_password timestamp=2026-09-01T10:07:00Z
```

---

## Testing

Run the full authentication test suite:

```bash
go test -v ./internal/auth
go test -v ./internal/api -run Auth
```

Example test output:
```
=== RUN   TestHashPassword
--- PASS: TestHashPassword (0.001s)
=== RUN   TestCheckPassword
--- PASS: TestCheckPassword (0.001s)
=== RUN   TestSignToken
--- PASS: TestSignToken (0.001s)
=== RUN   TestParseToken
--- PASS: TestParseToken (0.001s)
=== RUN   TestMiddleware
--- PASS: TestMiddleware (0.002s)
=== RUN   TestAdminMiddleware
--- PASS: TestAdminMiddleware (0.001s)
=== RUN   TestLogin
--- PASS: TestLogin (0.001s)
=== RUN   TestRegister
--- PASS: TestRegister (0.001s)
=== RUN   TestChangePassword
--- PASS: TestChangePassword (0.001s)
```

---

## Example Workflow

### 1. User Signup & Login

```bash
# Register
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "john@example.com",
    "password": "SecurePass123!",
    "display_name": "John Doe",
    "timezone": "America/New_York"
  }'

# Response:
# {
#   "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
#   "user": { ... }
# }

# Save token
export TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
```

### 2. Access Protected Resource

```bash
# Get user profile
curl http://localhost:8080/me \
  -H "Authorization: Bearer $TOKEN"

# Create a session
curl -X POST http://localhost:8080/sessions \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "category_ids": ["healing", "faith"],
    "duration_seconds": 1800,
    "voice_id": "grace"
  }'
```

### 3. Token Refresh (Before Expiration)

```bash
# Get new token
curl -X POST http://localhost:8080/auth/refresh \
  -H "Authorization: Bearer $TOKEN"

# Response:
# {
#   "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
# }

# Update token
export TOKEN="new_token_here"
```

### 4. Admin Operations

```bash
# List all admins (requires admin role)
curl http://localhost:8080/admin/users/admins \
  -H "Authorization: Bearer $ADMIN_TOKEN"

# Promote user to admin
curl -X POST http://localhost:8080/admin/users/role \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "role": "content_admin"
  }'

# Suspend a user
curl -X POST http://localhost:8080/admin/users/status \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "status": "suspended"
  }'
```

---

## Troubleshooting

### "unauthorized" Error

**Cause**: Missing or invalid token

**Solution**:
1. Check `Authorization` header is present
2. Token format must be: `Authorization: Bearer <token>`
3. Verify token hasn't expired (check `exp` claim)
4. Verify JWT secret matches server configuration

### "admin access required" Error

**Cause**: User doesn't have an admin role

**Solution**:
1. Use a super_admin account to promote the user
2. Call `POST /admin/users/role` with appropriate role
3. Verify promotion was successful with `GET /admin/users/admins`

### "current password is incorrect" Error

**Cause**: Old password verification failed during change

**Solution**:
1. Double-check the current password
2. Password is case-sensitive
3. Ensure no extra whitespace in password

---

## References

- [JWT.io](https://jwt.io/) — JWT debugging tool
- [bcrypt](https://en.wikipedia.org/wiki/Bcrypt) — Password hashing algorithm
- [OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)
- [OWASP Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html)
