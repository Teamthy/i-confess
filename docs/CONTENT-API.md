# I-Confess Content Domain API

## Overview

The Content Domain provides a complete system for managing confessions (structured content), organizing them into categories and collections, managing voice assets, and creating personalized listening sessions.

## Core Concepts

### Collections
- Top-level groupings of content (e.g., "The 39 Anglian Articles", "Personal Devotions")
- Can contain multiple categories
- Have a publish status (draft | published | archived)
- Support premium-only content

### Categories
- Groupings within collections or standalone (e.g., "Pride", "Lust", "Prayer")
- Confessions belong to a single category
- Support sort order for UI display
- Can be marked as premium

### Confessions
- The core content unit representing a structured confession/meditation
- Text in multiple lengths (short_text, medium_text, long_text)
- Support tags for search and filtering
- Have an intensity level (1-5) to indicate emotional weight
- Follow a status lifecycle for content workflow

### Confession Variants
- Different length versions of a confession for flexible session creation
- Common variants: 30s, 1m, 3m, 5m, 10m
- Each variant can be read by different voices and stored separately

### Scripture References
- Biblical references attached to confessions
- Support both direct quotes and paraphrases
- Can include notes and cite specific translations

### Voices
- Text-to-speech voice profiles for audio delivery
- Attributes: gender, language, provider (Google TTS, Azure TTS, etc.)
- Premium voices available to premium subscribers
- Can be marked active/inactive

### Audio Assets
- Pre-generated or on-demand audio files for confession variants
- Linked to a confession, variant, and voice
- Status: queued | processing | ready | failed
- Stored on CDN or media server

### Sessions
- Personalized listening sessions assembled from confessions
- Built by the session engine based on user preferences
- Contains ordered items (SessionItem records)
- Types: short | medium | long (based on duration)

### Session Items
- Individual confessions within a session
- Denormalized data for quick playback (title, category, audio_url, text)
- Status tracking: queued | playing | completed | skipped

## API Endpoints

### Public Content Access (No Authentication)

#### List Collections
```
GET /collections
Response: []Collection
```

#### List Categories
```
GET /categories
Response: []Category
```

#### Get Confessions by Category
```
GET /categories/{id}/confessions
Response: []Confession
```

#### Get Single Confession
```
GET /confessions/{id}
Response: Confession (with variants and scriptures)
```

#### List Voices
```
GET /voices
Response: []Voice
```

### Authenticated User Endpoints

#### Create Session
```
POST /sessions
Request: {
  "category_ids": ["cat1", "cat2"],
  "duration_seconds": 1800,
  "voice_id": "optional-voice-id"
}
Response: Session (with items)
```

The session engine will:
1. Validate requested voice (premium check)
2. Find published confessions in requested categories
3. Filter to only confessions with audio for the chosen voice
4. Cycle through categories round-robin
5. Pack in longest-fitting variants to meet duration budget

#### Get Session
```
GET /sessions/{id}
Response: Session
```

#### Update Session Status
```
PATCH /sessions/{id}
Request: { "status": "playing|completed|abandoned" }
Response: { "status": "..." }
```

#### List My Sessions
```
GET /sessions
Response: []Session (limited to 50 most recent)
```

#### Create Schedule
```
POST /schedules
Request: {
  "label": "Morning Prayer",
  "time": "06:00",
  "days_of_week": [0, 1, 2, 3, 4],
  "timezone": "America/New_York",
  "duration_seconds": 1800,
  "voice_id": "optional",
  "category_ids": ["prayer"],
  "enabled": true
}
Response: Schedule
```

Schedules enable automated session creation at specified times.

#### Record Playback
```
POST /me/history
Request: {
  "session_id": "optional",
  "confession_id": "required",
  "duration_seconds": 30,
  "completed": true,
  "skipped": false
}
Response: PlaybackRecord
```

#### Get Playback History
```
GET /me/history
Response: []PlaybackRecord (limited to 50 most recent)
```

#### Add to Favorites
```
POST /me/favorites
Request: {
  "entity_type": "confession|category|voice",
  "entity_id": "id"
}
Response: Favorite
```

#### Remove from Favorites
```
DELETE /me/favorites
Request: {
  "entity_type": "confession|category|voice",
  "entity_id": "id"
}
Response: 204 No Content
```

#### List Favorites
```
GET /me/favorites?type=confession
Response: []Favorite
```

#### Create User Confession
```
POST /me/confessions
Request: {
  "title": "My Confession",
  "text": "Full text...",
  "category_id": "optional",
  "is_private": false
}
Response: UserConfession
```

Users can create and share their own confessions.

#### List My Confessions
```
GET /me/confessions
Response: []UserConfession
```

### Admin Endpoints (Requires admin role)

#### Category Management

**Create Category**
```
POST /admin/categories
Request: {
  "name": "Repentance",
  "slug": "repentance",
  "description": "...",
  "icon": "🙏",
  "premium": false,
  "status": "draft|published|archived",
  "sort_order": 1
}
Response: Category
```

**List All Categories**
```
GET /admin/categories
Response: []Category (includes draft/archived)
```

#### Confession Management

**Create Confession**
```
POST /admin/confessions
Request: {
  "category_id": "required",
  "title": "Confession Title",
  "short_text": "Short version",
  "medium_text": "Medium version",
  "long_text": "Full text",
  "language": "en",
  "status": "draft",
  "author": "Anonymous",
  "intensity": 2,
  "tags": ["sin", "repentance"],
  "variants": [
    {
      "label": "30s",
      "duration_seconds": 30,
      "sort_order": 1
    },
    ...
  ],
  "scriptures": [
    {
      "book": "Romans",
      "chapter": 6,
      "verse": "23",
      "translation": "KJV",
      "is_direct_quote": false,
      "notes": "...",
      "sort_order": 1
    },
    ...
  ]
}
Response: Confession
```

**List All Confessions**
```
GET /admin/confessions
Response: []Confession (includes drafts)
```

**Get Single Confession**
```
GET /admin/confessions/{id}
Response: Confession
```

**Update Confession Status**
```
PATCH /admin/confessions/{id}
Request: { "status": "draft|content_review|theological_review|audio_production|audio_qa|approved|published|archived" }
Response: { "status": "..." }
```

Status workflow:
- draft → content_review (content admins review for doctrinal alignment)
- content_review → theological_review (theological reviewers approve)
- theological_review → audio_production (audio producer generates variants)
- audio_production → audio_qa (QA checks audio quality)
- audio_qa → approved or back to audio_production
- approved → published (content goes live)
- published → archived (retire old content)

#### Voice Management

**Create Voice**
```
POST /admin/voices
Request: {
  "name": "Sarah",
  "description": "Professional female voice",
  "type": "professional|minister|generic",
  "provider": "google-tts|azure-tts|openai",
  "gender": "male|female|neutral",
  "language": "en|es|fr|...",
  "premium": false,
  "status": "active|inactive",
  "sample_url": "https://cdn.example.com/sample.mp3"
}
Response: Voice
```

**List All Voices**
```
GET /admin/voices
Response: []Voice
```

#### Audio Asset Management

**Create/Update Audio Asset**
```
POST /admin/audio
Request: {
  "confession_id": "required",
  "variant_id": "required",
  "voice_id": "required",
  "url": "https://cdn.example.com/audio.mp3",
  "duration_seconds": 30,
  "size_bytes": 524288,
  "status": "ready|processing|failed"
}
Response: AudioAsset
```

#### User Management

**Set User Role**
```
POST /admin/users/role
Request: {
  "user_id": "...",
  "role": "super_admin|content_admin|audio_producer|theological_reviewer|support_admin|analytics_admin"
}
Response: { "user_id": "...", "role": "..." }
```

**Remove User Role**
```
DELETE /admin/users/role
Request: { "user_id": "..." }
Response: 204 No Content
```

**Set User Subscription**
```
POST /admin/users/subscription
Request: {
  "user_id": "...",
  "plan": "free|premium",
  "status": "active|cancelled|expired"
}
Response: { "user_id": "...", "plan": "...", "status": "..." }
```

**List Admin Users**
```
GET /admin/users/admins
Response: []AdminUser
```

**Set User Status**
```
POST /admin/users/status
Request: {
  "user_id": "...",
  "status": "active|suspended|deleted"
}
Response: { "user_id": "...", "status": "..." }
```

#### Dashboard

**Admin Stats**
```
GET /admin/stats
Response: {
  "categories": 15,
  "confessions": 450,
  "published": 320,
  "voices": 8,
  "confessions_by_status": {
    "draft": 50,
    "content_review": 20,
    "theological_review": 30,
    "audio_production": 40,
    "audio_qa": 25,
    "approved": 15,
    "published": 320,
    "archived": 100
  }
}
```

## Status Codes

- **200 OK**: Successful read
- **201 Created**: Successful write
- **204 No Content**: Successful delete
- **400 Bad Request**: Invalid input
- **401 Unauthorized**: Missing/invalid token
- **403 Forbidden**: Insufficient permissions
- **404 Not Found**: Resource not found
- **409 Conflict**: Duplicate (e.g., slug already exists)
- **422 Unprocessable Entity**: Valid input but cannot process (e.g., no audio for voice)
- **500 Internal Server Error**: Server error

## Session Building Algorithm

The session engine builds deterministic sessions using the following algorithm:

1. **Voice Resolution**
   - If user requests a premium voice and doesn't have premium, fallback to first free voice
   - Validate voice exists and is active

2. **Content Eligibility**
   - Collect all published confessions in requested categories
   - Filter to only those with ready audio for the chosen voice
   - Group by category

3. **Round-Robin Packing**
   - Cycle through categories to ensure representation
   - For each category, select the longest variant that fits remaining duration
   - Minimize waste by preferring longer variants for greedy packing
   - Stop when remaining duration is less than minimum variant

4. **Session Assembly**
   - Order items as selected
   - Denormalize title, category, audio URL for playback payload
   - Create session with "created" status

## Audio Generation Pipeline

The audio production workflow:

1. **Content Admin**: Creates confession with variants in draft
2. **Content Review**: Admin reviews text for doctrinal alignment → content_review
3. **Theological Review**: Theological reviewer approves → theological_review
4. **Audio Production**: Audio producer:
   - Selects voices to generate audio for
   - Submits job queue entry for each (confession_id, variant_id, voice_id)
   - Status → audio_production
5. **Job Worker**: Process audio generation jobs asynchronously
   - Call TTS provider (Google, Azure, etc.)
   - Store audio file to CDN
   - Create AudioAsset record with status=ready
6. **Audio QA**: QA engineer reviews audio quality
   - Listen to samples
   - Approve (approved) or reject (audio_production)
7. **Publish**: Approved confessions → published
   - Content now available to all users

## Caching and Performance

- **Published categories/confessions**: Cacheable (change rarely)
- **Voices**: Cacheable (change rarely)
- **Audio assets**: Served from CDN
- **Sessions**: Generated on-demand, cached for user (session stored in DB)
- **Playback records**: Written immediately, read for history (DB)

## Search and Discovery

Future enhancements (v2):

- **Full-text search**: Search confessions by title, text, tags, scripture refs
- **Category browse**: Filter by intensity, language, denomination
- **Recommendations**: Based on playback history, favorites, time-of-day
- **Trending**: Popular confessions in community
- **Collections**: Curated topic bundles (Lent, Advent, Daily Devotion, etc.)
