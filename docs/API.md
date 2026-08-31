# i-confess â€” API Contract (MVP)

Base URL: `http://<host>:8080`

- All bodies are JSON.
- Auth: `Authorization: Bearer <token>` (JWT).
- Errors: `{"error": "<message>"}` with an appropriate status.

## Authentication

| Method | Path              | Auth | Body                                  | Returns            |
|--------|-------------------|------|---------------------------------------|--------------------|
| POST   | `/auth/register`  | â€”    | `{email, password, display_name?, timezone?}` | `{token, user}` |
| POST   | `/auth/login`     | â€”    | `{email, password}`                   | `{token, user}`    |

`password` must be â‰¥ 8 chars. `timezone` defaults to `UTC`.

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
| GET    | `/me`   | âœ“    | `{user, plan}`               |

## Sessions

| Method | Path               | Auth | Body                                                  | Returns    |
|--------|--------------------|------|-------------------------------------------------------|------------|
| POST   | `/sessions`        | âœ“    | `{category_ids[], duration_seconds, voice_id?}`       | `Session`  |
| GET    | `/sessions/{id}`   | âœ“    | â€”                                                     | `Session`  |
| PATCH  | `/sessions/{id}`   | âœ“    | `{status}` (`playing|completed|abandoned`)            | `{status}` |
| GET    | `/sessions`        | âœ“    | â€”                                                     | `Session[]`|

`duration_seconds` âˆˆ [60, 10800]. The engine returns ordered `items`, each with
`audio_url`, `title`, `category`, `text`, and `duration_seconds`.

## Schedules

| Method | Path                | Auth | Body                                                                  | Returns     |
|--------|---------------------|------|-----------------------------------------------------------------------|-------------|
| GET    | `/schedules`        | âœ“    | â€”                                                                     | `Schedule[]`|
| POST   | `/schedules`        | âœ“    | `{label, time "HH:MM", days_of_week[], timezone, duration_seconds, voice_id?, category_ids[]?, enabled?}` | `Schedule` |
| PATCH  | `/schedules/{id}`   | âœ“    | partial update of the above                                           | `Schedule`  |
| DELETE | `/schedules/{id}`   | âœ“    | â€”                                                                     | `204`       |

`days_of_week`: `1=Mon â€¦ 7=Sun`. Scheduling is **timezone-aware**; the mobile client
is responsible for local alarm/notification delivery (PRD Â§27).

## Favorites

| Method | Path             | Auth | Body                                    | Returns     |
|--------|------------------|------|-----------------------------------------|-------------|
| POST   | `/me/favorites`  | âœ“    | `{entity_type, entity_id}`              | `Favorite`  |
| DELETE | `/me/favorites`  | âœ“    | `{entity_type, entity_id}`              | `204`       |
| GET    | `/me/favorites`  | âœ“    | `?type=` optional filter                | `Favorite[]`|

`entity_type` âˆˆ `confession | category | session | voice`.

## History

| Method | Path          | Auth | Body                                                       | Returns            |
|--------|---------------|------|------------------------------------------------------------|--------------------|
| GET    | `/me/history` | âœ“    | â€”                                                          | `PlaybackRecord[]` |
| POST   | `/me/history` | âœ“    | `{session_id?, confession_id?, duration_seconds, completed, skipped}` | `PlaybackRecord` |

## Personal confessions (private by default)

| Method | Path               | Auth | Body                          | Returns            |
|--------|--------------------|------|-------------------------------|--------------------|
| POST   | `/me/confessions`  | âœ“    | `{title, text, category_id?}` | `UserConfession`   |
| GET    | `/me/confessions`  | âœ“    | â€”                             | `UserConfession[]` |

## Admin (requires an admin role token)

| Method | Path                            | Body / Notes                                   |
|--------|---------------------------------|------------------------------------------------|
| GET    | `/admin/stats`                  | dashboard summary                              |
| POST   | `/admin/categories`             | `{name, slug, description?, icon?, premium?, status?, sort_order?}` |
| GET    | `/admin/categories`             | all categories (incl. unpublished)             |
| POST   | `/admin/confessions`            | full confession incl. `variants[]` + `scriptures[]` |
| GET    | `/admin/confessions`            | all confessions (incl. draft)                  |
| GET    | `/admin/confessions/{id}`       | â€”                                              |
| PATCH  | `/admin/confessions/{id}`       | `{status}` (lifecycle transition)              |
| POST   | `/admin/voices`                 | `{name, description?, type?, provider?, gender?, language?, premium?, status?, sample_url?}` |
| GET    | `/admin/voices`                 | â€”                                              |
| POST   | `/admin/audio`                  | `{confession_id, variant_id?, voice_id, url, duration_seconds?, size_bytes?, status?}` |
| POST   | `/admin/users/role`             | `{user_id, role}`                              |
| POST   | `/admin/users/subscription`     | `{user_id, plan "free|premium", status?}`      |

### Admin roles (PRD Â§75)

`super_admin` Â· `content_admin` Â· `audio_producer` Â· `theological_reviewer` Â·
`support_admin` Â· `analytics_admin`

### Confession lifecycle (PRD Â§12)

`draft â†’ content_review â†’ theological_review â†’ audio_production â†’ audio_qa â†’
approved â†’ published â†’ archived`

## Data model (summary)

See `migrations/postgres/0001_schema.sql` for the canonical schema. Core entities:
`collections`, `categories`, `collection_categories`, `confessions`,
`confession_variants`, `scripture_references`, `voices`, `voice_licenses`,
`audio_assets`, `users`, `subscriptions`, `session_preferences`, `schedules`,
`sessions`, `session_items`, `favorites`, `playback_history`, `user_confessions`,
`user_confession_audio`, `admin_users`, `audit_logs`.
