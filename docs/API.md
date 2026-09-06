# i-confess — API Contract (MVP)

Base URL: `http://<host>:8080`

- All bodies are JSON.
- Auth: `Authorization: Bearer <token>` (JWT).
- Errors: `{"error": "<message>"}` with an appropriate status.

## Authentication

| Method | Path              | Auth | Body                                  | Returns            |
|--------|-------------------|------|---------------------------------------|--------------------|
| POST   | `/auth/register`  | —    | `{email, password, display_name?, timezone?}` | `{token, user}` |
| POST   | `/auth/login`     | —    | `{email, password}`                   | `{token, user}`    |

`password` must be ≥ 8 chars. `timezone` defaults to `UTC`.

## Content (public, published only)

| Method | Path                            | Returns                                        |
|--------|---------------------------------|------------------------------------------------|
| GET    | `/collections`                  | `Collection[]`                                 |
| GET    | `/categories`                   | `Category[]`                                   |
| GET    | `/categories/{id}/confessions`  | `Confession[]` (published)                     |
| GET    | `/confessions/{id}`             | `Confession` (with variants + scriptures)      |
| GET    | `/voices`                       | `Voice[]`                                      |

## Profile & preferences

| Method | Path    | Auth | Returns                     |
|--------|---------|------|------------------------------|
| GET    | `/me`   | ✓    | `{user, plan}`               |

## Sessions

| Method | Path               | Auth | Body                                                  | Returns    |
|--------|--------------------|------|-------------------------------------------------------|------------|
| POST   | `/sessions`        | ✓    | `{category_ids[], duration_seconds, voice_id?}`       | `Session`  |
| GET    | `/sessions/{id}`   | ✓    | —                                                     | `Session`  |
| PATCH  | `/sessions/{id}`   | ✓    | `{status}` (`playing|completed|abandoned`)            | `{status}` |
| GET    | `/sessions`        | ✓    | —                                                     | `Session[]`|

`duration_seconds` ∈ [60, 10800]. The engine returns ordered `items`, each with
`audio_url`, `title`, `category`, `text`, and `duration_seconds`.

## Schedules

| Method | Path                | Auth | Body                                                                  | Returns     |
|--------|---------------------|------|-----------------------------------------------------------------------|-------------|
| GET    | `/schedules`        | ✓    | —                                                                     | `Schedule[]`|
| POST   | `/schedules`        | ✓    | `{label, time "HH:MM", days_of_week[], timezone, duration_seconds, voice_id?, category_ids[]?, enabled?}` | `Schedule` |
| PATCH  | `/schedules/{id}`   | ✓    | partial update of the above                                           | `Schedule`  |
| DELETE | `/schedules/{id}`   | ✓    | —                                                                     | `204`       |

`days_of_week`: `1=Mon … 7=Sun`. Scheduling is **timezone-aware**; the mobile client
is responsible for local alarm/notification delivery (PRD §27).

## Favorites

| Method | Path             | Auth | Body                                    | Returns     |
|--------|------------------|------|-----------------------------------------|-------------|
| POST   | `/me/favorites`  | ✓    | `{entity_type, entity_id}`              | `Favorite`  |
| DELETE | `/me/favorites`  | ✓    | `{entity_type, entity_id}`              | `204`       |
| GET    | `/me/favorites`  | ✓    | `?type=` optional filter                | `Favorite[]`|

`entity_type` ∈ `confession | category | session | voice`.

## History

| Method | Path          | Auth | Body                                                       | Returns            |
|--------|---------------|------|------------------------------------------------------------|--------------------|
| GET    | `/me/history` | ✓    | —                                                          | `PlaybackRecord[]` |
| POST   | `/me/history` | ✓    | `{session_id?, confession_id?, duration_seconds, completed, skipped}` | `PlaybackRecord` |

## Personal confessions (private by default)

| Method | Path               | Auth | Body                          | Returns            |
|--------|--------------------|------|-------------------------------|--------------------|
| POST   | `/me/confessions`  | ✓    | `{title, text, category_id?}` | `UserConfession`   |
| GET    | `/me/confessions`  | ✓    | —                             | `UserConfession[]` |

## Admin (requires an admin role token)

| Method | Path                            | Body / Notes                                   |
|--------|---------------------------------|------------------------------------------------|
| GET    | `/admin/stats`                  | dashboard summary                              |
| POST   | `/admin/categories`             | `{name, slug, description?, icon?, premium?, status?, sort_order?}` |
| GET    | `/admin/categories`             | all categories (incl. unpublished)             |
| POST   | `/admin/confessions`            | full confession incl. `variants[]` + `scriptures[]` |
| GET    | `/admin/confessions`            | all confessions (incl. draft)                  |
| GET    | `/admin/confessions/{id}`       | —                                              |
| PATCH  | `/admin/confessions/{id}`       | `{status}` (lifecycle transition)              |
| POST   | `/admin/voices`                 | `{name, description?, type?, provider?, gender?, language?, premium?, status?, sample_url?}` |
| GET    | `/admin/voices`                 | —                                              |
| POST   | `/admin/audio`                  | `{confession_id, variant_id?, voice_id, url, duration_seconds?, size_bytes?, status?}` |
| POST   | `/admin/users/role`             | `{user_id, role}`                              |
| POST   | `/admin/users/subscription`     | `{user_id, plan "free|premium", status?}`      |

### Admin roles (PRD §75)

`super_admin` · `content_admin` · `audio_producer` · `theological_reviewer` ·
`support_admin` · `analytics_admin`

### Confession lifecycle (PRD §12)

`draft → content_review → theological_review → audio_production → audio_qa →
approved → published → archived`

## Data model (summary)

See `migrations/postgres/0001_schema.sql` for the canonical schema. Core entities:
`collections`, `categories`, `collection_categories`, `confessions`,
`confession_variants`, `scripture_references`, `voices`, `voice_licenses`,
`audio_assets`, `users`, `subscriptions`, `session_preferences`, `schedules`,
`sessions`, `session_items`, `favorites`, `playback_history`, `user_confessions`,
`user_confession_audio`, `admin_users`, `audit_logs`.
