# Audio Infrastructure Platform — Complete Implementation Guide

**Version**: 1.0 (MVP)  
**Based on**: Master Prompt Requirements (162 requirements)  
**Status**: Architecture & Schema Complete  
**Created**: 2026-09-01  

---

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Core Principles](#core-principles)
3. [Database Schema](#database-schema)
4. [Go Service Layers](#go-service-layers)
5. [Flutter Audio Player](#flutter-audio-player)
6. [Admin Dashboard](#admin-dashboard)
7. [Playback System](#playback-system)
8. [Audio Generation Pipeline](#audio-generation-pipeline)
9. [Storage & CDN](#storage--cdn)
10. [Analytics](#analytics)
11. [MVP Implementation Checklist](#mvp-implementation-checklist)

---

## Architecture Overview

The audio infrastructure platform separates concerns into independent, composable layers:

```
                        CONTENT
                           │
                           ↓
                    CONTENT VERSION
                           │
                           ↓
                         VOICE
                           │
                           ↓
                  AUDIO GENERATION
                           │
             ┌─────────────┴─────────────┐
             ↓                           ↓
        TTS Provider              Human Recording
             │                           │
             └─────────────┬─────────────┘
                           ↓
                       RAW AUDIO
                           │
                           ↓
                   MEDIA PROCESSOR
                           │
                           ↓
                  AUDIO VALIDATION
                           │
                           ↓
                  MASTER AUDIO ASSET
                           │
              ┌────────────┼────────────┐
              ↓            ↓            ↓
           STREAM       DOWNLOAD      PREVIEW
              │            │
              └────────────┼────────────┘
                           ↓
                    OBJECT STORAGE
                           │
                           ↓
                          CDN
                           │
                           ↓
                 PLAYBACK RESOLVER
                           │
                    ┌──────┴──────┐
                    ↓             ↓
                 Flutter         Web
                    │
                    ↓
              AUDIO PLAYER
                    │
       ┌────────────┼─────────────┐
       ↓            ↓             ↓
Background       Offline       Lock Screen
Playback         Playback       Controls
       │            │             │
       └────────────┼─────────────┘
                    ↓
             PLAYBACK ANALYTICS
                    │
                    ↓
             RECOMMENDATIONS (V2/V3)
```

### Key Principle: Separation of Concerns

```
Content (confessions, versions)
      ↓
Voice (profiles, authorization)
      ↓
Audio Generation (jobs, workers)
      ↓
Audio Assets (master, variants, streams)
      ↓
Storage & Delivery (object storage, CDN)
      ↓
Playback Authorization (entitlement, signed URLs)
      ↓
Streaming (Flutter, Web players)
      ↓
Analytics (events, QoE metrics)
```

These are **not combined**. Each is independently deployable and testable.

---

## Core Principles

### 1. Audio Binaries ≠ Database

**NEVER** store audio files in PostgreSQL.

```
BAD:  Confession ← Audio Blob ← Database
GOOD: Confession → Content Version → Audio Asset → Storage → CDN
```

### 2. Audio Identity

Every audio asset has complete identity:

```go
type EnhancedAudioAsset struct {
    ContentID           // Which confession
    ContentVersionID    // Which version
    VoiceID             // Which voice
    AssetType           // source | master | stream | preview | download
    QualityTier         // standard | high | lossless
}
```

One confession ≠ one audio file. One confession may have:
- Multiple voices
- Multiple quality tiers  
- Multiple asset types
- Multiple formats

### 3. Async Production

Audio generation is **always async**:

```
API Request
    ↓
Create Job (Status: QUEUED)
    ↓
Return immediately (201 Created)
    ↓
Background Worker processes
    ↓
Audio asset created (Status: READY)
    ↓
Admin notified
```

**Never block on audio generation inside HTTP handlers.**

### 4. Signed URLs for Protected Media

```go
// Never return permanent URLs
// ❌ WRONG
GET /api/audio/123
→ "url": "https://cdn.example.com/audio/123.m4a"

// ✅ CORRECT
GET /api/audio/123/play
→ {
    "url": "https://cdn.example.com/audio/...?signed_token=...&expires_at=2026-09-02T10:00:00Z",
    "expires_at": "2026-09-02T10:00:00Z"
  }
```

### 5. Content Versioning

Content changes create new versions:

```
Confession "Healing"
  └── Version 1 (published)
        └── Audio 1 (Voice A)
        └── Audio 2 (Voice B)
  └── Version 2 (draft)
        └── (No audio yet)
```

**Do not silently overwrite historical audio.**

### 6. Voice Rights are Mandatory

```go
type VoiceRights struct {
    VoiceID                 string // Which voice
    RightsHolder            string // Who authorized it
    AllowedUse              string // tts | recording | streaming | commercial
    Territories             string // GLOBAL | ISO codes
    Status                  string // active | expired | revoked
}
```

Before using a voice, validate:
- IsActive()
- CanGenerateAI()  
- CanUseInTerritory(userCountry)

### 7. Playback Always Works

```
If analytics down       → Playback continues
If generation down      → Existing content plays
If CDN temporarily down → Retry & fallback
```

**Playback is the highest-priority path.**

---

## Database Schema

### Core Audio Tables

#### 1. content_versions

Separates content from production:

```sql
CREATE TABLE content_versions (
    id              TEXT PRIMARY KEY,
    confession_id   TEXT NOT NULL REFERENCES confessions(id),
    version_number  INTEGER NOT NULL,
    title           TEXT NOT NULL,
    status          TEXT NOT NULL,  -- draft | approved | published
    created_at      TEXT NOT NULL
);
```

#### 2. audio_assets (Enhanced)

Complete audio metadata:

```sql
CREATE TABLE audio_assets (
    id                  TEXT PRIMARY KEY,
    content_id          TEXT NOT NULL,
    content_version_id  TEXT,
    voice_id            TEXT NOT NULL,
    
    -- Classification
    asset_type          TEXT NOT NULL,  -- source | master | stream | preview | download
    quality_tier        TEXT NOT NULL,  -- standard | high | lossless
    
    -- Storage location
    storage_provider    TEXT NOT NULL,  -- s3 | gcs | azure | local
    storage_key         TEXT NOT NULL,  -- deterministic path
    cdn_path            TEXT,
    
    -- Technical metadata
    format              TEXT NOT NULL,  -- m4a | mp3 | wav | flac
    codec               TEXT NOT NULL,  -- aac | mp3 | pcm | flac
    duration_seconds    INTEGER NOT NULL,
    file_size_bytes     INTEGER NOT NULL,
    
    -- Quality metrics
    loudness_lufs       REAL,           -- ITU-R BS.1770-4
    peak_db             REAL,
    checksum_sha256     TEXT UNIQUE,
    
    -- Lifecycle
    status              TEXT NOT NULL,  -- uploading | processing | ready | published | failed
    published_at        TEXT,
    
    created_at          TEXT NOT NULL
);
```

#### 3. audio_generation_jobs

Async TTS/recording jobs:

```sql
CREATE TABLE audio_generation_jobs (
    id                  TEXT PRIMARY KEY,
    content_version_id  TEXT NOT NULL,
    voice_id            TEXT NOT NULL,
    
    provider            TEXT NOT NULL,  -- google-tts | azure-tts | openai
    status              TEXT NOT NULL,  -- queued | processing | succeeded | failed
    
    attempt_count       INTEGER NOT NULL DEFAULT 0,
    max_attempts        INTEGER NOT NULL DEFAULT 3,
    
    error_code          TEXT,
    error_message       TEXT,
    
    idempotency_key     TEXT UNIQUE,    -- Prevent duplicate generation
    
    created_at          TEXT NOT NULL
);
```

#### 4. voice_rights

Authorization metadata:

```sql
CREATE TABLE voice_rights (
    id                      TEXT PRIMARY KEY,
    voice_id                TEXT NOT NULL,
    
    rights_holder           TEXT NOT NULL,
    authorization_reference TEXT,
    
    allowed_use             TEXT NOT NULL,  -- tts | recording | streaming | commercial
    territories             TEXT NOT NULL,  -- GLOBAL or comma-separated ISO
    
    start_date              TEXT,
    expiry_date             TEXT,
    status                  TEXT NOT NULL,  -- active | expired | revoked
    
    created_at              TEXT NOT NULL
);
```

#### 5. audio_playback_sessions

Detailed playback tracking:

```sql
CREATE TABLE audio_playback_sessions (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL,
    audio_asset_id  TEXT NOT NULL,
    
    status          TEXT NOT NULL,  -- playing | paused | completed
    position_seconds INTEGER NOT NULL,
    
    device_id       TEXT,
    platform        TEXT,           -- iOS | Android | Web
    
    created_at      TEXT NOT NULL
);
```

#### 6. audio_downloads

Download management:

```sql
CREATE TABLE audio_downloads (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL,
    audio_asset_id  TEXT NOT NULL,
    
    status          TEXT NOT NULL,  -- queued | downloading | downloaded | failed
    downloaded_at   TEXT,
    expires_at      TEXT,           -- License expiry
    
    local_path      TEXT,
    file_size_bytes INTEGER,
    
    created_at      TEXT NOT NULL,
    UNIQUE(user_id, audio_asset_id)
);
```

#### 7. audio_events

Playback analytics:

```sql
CREATE TABLE audio_events (
    id              TEXT PRIMARY KEY,
    user_id         TEXT,
    audio_asset_id  TEXT,
    
    event_type      TEXT NOT NULL,  -- play_started | play_paused | play_completed | download_started
    
    position_seconds INTEGER,
    device_id       TEXT,
    platform        TEXT,
    
    error_code      TEXT,
    
    created_at      TEXT NOT NULL
);
```

#### 8. audio_qoe_metrics

Quality of Experience:

```sql
CREATE TABLE audio_qoe_metrics (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL,
    audio_asset_id  TEXT NOT NULL,
    
    startup_latency_ms  INTEGER,
    rebuffer_count      INTEGER,
    completion_rate     REAL,
    failure_rate        REAL,
    
    created_at      TEXT NOT NULL
);
```

### Index Strategy

Performance-critical indexes:

```sql
-- Audio asset lookups
CREATE INDEX idx_audio_assets_content ON audio_assets(content_id);
CREATE INDEX idx_audio_assets_voice ON audio_assets(voice_id);
CREATE INDEX idx_audio_assets_status ON audio_assets(status);
CREATE INDEX idx_audio_assets_published ON audio_assets(published_at);

-- Generation job tracking
CREATE INDEX idx_audio_generation_jobs_status ON audio_generation_jobs(status);
CREATE INDEX idx_audio_generation_jobs_idempotency ON audio_generation_jobs(idempotency_key);

-- Playback tracking
CREATE INDEX idx_playback_sessions_user ON audio_playback_sessions(user_id);
CREATE INDEX idx_audio_events_user ON audio_events(user_id);
CREATE INDEX idx_audio_events_created ON audio_events(created_at);

-- Downloads
CREATE INDEX idx_downloads_user ON audio_downloads(user_id);
CREATE INDEX idx_downloads_status ON audio_downloads(status);
```

---

## Go Service Layers

### Layer 1: Storage Abstraction

```go
// storage/interface.go
type ObjectStorage interface {
    // Upload a file
    Upload(ctx context.Context, key string, data []byte, metadata map[string]string) error
    
    // Download a file
    Download(ctx context.Context, key string) ([]byte, error)
    
    // Delete a file
    Delete(ctx context.Context, key string) error
    
    // Generate a signed URL for time-limited access
    GenerateSignedURL(ctx context.Context, key string, duration time.Duration) (string, error)
    
    // List objects with prefix
    List(ctx context.Context, prefix string) ([]string, error)
}

// Implementations
// - S3Storage
// - GCSStorage
// - AzureStorage
// - LocalFileStorage
```

### Layer 2: Audio Service

```go
// audio/service.go
type AudioService struct {
    repo    *AudioRepository
    storage ObjectStorage
    jobs    *JobQueue
    provider VoiceProvider
}

// Generate audio from content
func (s *AudioService) GenerateAudio(ctx context.Context, req GenerateRequest) (*AudioGenerationJob, error)

// Get audio asset by ID
func (s *AudioService) GetAsset(ctx context.Context, id string) (*EnhancedAudioAsset, error)

// Find best asset for playback
func (s *AudioService) ResolvePlayable(ctx context.Context, req PlaybackRequest) (*EnhancedAudioAsset, error)

// Publish audio to CDN
func (s *AudioService) PublishAsset(ctx context.Context, id string) error

// Archive old audio
func (s *AudioService) ArchiveAsset(ctx context.Context, id string) error
```

### Layer 3: Playback Resolver

```go
// playback/resolver.go
type PlaybackResolver struct {
    audio       *AudioService
    entitlements *EntitlementService
    cache       *redis.Client
}

// Resolve playable audio with authorization
func (r *PlaybackResolver) Resolve(ctx context.Context, req PlaybackRequest) (*PlaybackResponse, error) {
    // 1. Authenticate user
    user, err := r.auth.GetUser(ctx, req.UserID)
    if err != nil {
        return nil, err
    }
    
    // 2. Check entitlements (free vs premium)
    canAccess, err := r.entitlements.CanAccess(ctx, user, req.ContentID)
    if !canAccess {
        return nil, ErrEntitlementRequired
    }
    
    // 3. Find content and voice
    content, err := r.getContent(ctx, req.ContentID)
    if err != nil {
        return nil, err
    }
    
    // 4. Check voice availability
    voice, err := r.checkVoice(ctx, req.VoiceID, user.Territory)
    if err != nil {
        return nil, err
    }
    
    // 5. Find best audio asset
    asset, err := r.audio.ResolvePlayable(ctx, PlaybackRequest{
        ContentID:  content.ID,
        VoiceID:    voice.ID,
        Quality:    getQuality(user.Subscription),
    })
    if err != nil {
        return nil, err
    }
    
    // 6. Generate signed URL
    signedURL, err := r.generateSignedURL(ctx, asset.StorageKey)
    if err != nil {
        return nil, err
    }
    
    // 7. Return playback payload
    return &PlaybackResponse{
        AudioID:      asset.ID,
        ContentID:    content.ID,
        VoiceID:      voice.ID,
        StreamURL:    signedURL,
        ExpiresAt:    time.Now().Add(4 * time.Hour),
        Duration:     asset.DurationSeconds,
    }, nil
}
```

### Layer 4: Audio Generation Worker

```go
// workers/audio_worker.go
type AudioWorker struct {
    repo     *AudioRepository
    jobs     *JobQueue
    provider VoiceProvider
    storage  ObjectStorage
    processor *AudioProcessor
}

// Process generation jobs
func (w *AudioWorker) Process(ctx context.Context, job *AudioGenerationJob) error {
    // 1. Validate job
    if err := w.validate(job); err != nil {
        return w.failJob(job, "VALIDATION_ERROR", err.Error())
    }
    
    // 2. Get content and voice
    content, err := w.getContent(job.ContentVersionID)
    if err != nil {
        return w.failJob(job, "CONTENT_NOT_FOUND", err.Error())
    }
    voice, err := w.getVoice(job.VoiceID)
    if err != nil {
        return w.failJob(job, "VOICE_NOT_FOUND", err.Error())
    }
    
    // 3. Check voice rights
    if !w.canUseVoice(voice) {
        return w.failJob(job, "VOICE_UNAUTHORIZED", "Voice not authorized for generation")
    }
    
    // 4. Generate audio
    rawAudio, err := w.provider.Generate(ctx, GenerateRequest{
        Text:     content.LongText,
        Voice:    voice,
        Language: content.Language,
    })
    if err != nil {
        return w.retryJob(job, "GENERATION_FAILED", err.Error())
    }
    
    // 5. Process audio (normalize, validate)
    processed, metadata, err := w.processor.Process(rawAudio)
    if err != nil {
        return w.failJob(job, "PROCESSING_FAILED", err.Error())
    }
    
    // 6. Validate output
    if err := w.validateOutput(processed, metadata); err != nil {
        return w.failJob(job, "VALIDATION_FAILED", err.Error())
    }
    
    // 7. Store master asset
    asset, err := w.storeAsset(job, processed, metadata)
    if err != nil {
        return w.failJob(job, "STORAGE_FAILED", err.Error())
    }
    
    // 8. Create variants (streaming, download, preview)
    if err := w.createVariants(asset); err != nil {
        // Log but don't fail - master is more important
    }
    
    // 9. Mark job complete
    job.Status = "SUCCEEDED"
    job.CompletedAt = now()
    return w.repo.UpdateJob(ctx, job)
}
```

### Layer 5: Audio Processor

```go
// audio/processor.go
type AudioProcessor struct {
    config *ProcessingConfig
}

type ProcessingConfig struct {
    TargetLoudness      float64 // -18 LUFS (ITU-R BS.1770-4)
    MaxPeakLevel        float64 // -1 dB
    SampleRate          int     // 48000 Hz
    Channels            int     // 2 (stereo)
    EncodingBitrate     int     // 128 kbps
}

// Full pipeline: decode → normalize → encode → validate
func (p *AudioProcessor) Process(rawAudio []byte) ([]byte, *AudioMetadata, error) {
    // 1. Decode
    decoded, err := p.decode(rawAudio)
    if err != nil {
        return nil, nil, fmt.Errorf("decode failed: %w", err)
    }
    
    // 2. Validate
    if err := p.validate(decoded); err != nil {
        return nil, nil, fmt.Errorf("validation failed: %w", err)
    }
    
    // 3. Normalize loudness (ITU-R BS.1770-4)
    normalized := p.normalizeLoudness(decoded, p.config.TargetLoudness)
    
    // 4. Apply peak limiting
    limited := p.applyPeakLimiting(normalized, p.config.MaxPeakLevel)
    
    // 5. Encode to target format
    encoded, err := p.encode(limited, "m4a", p.config.EncodingBitrate)
    if err != nil {
        return nil, nil, fmt.Errorf("encode failed: %w", err)
    }
    
    // 6. Extract metadata
    metadata := p.extractMetadata(encoded)
    
    // 7. Final validation
    if err := p.validateOutput(encoded, metadata); err != nil {
        return nil, nil, fmt.Errorf("output validation failed: %w", err)
    }
    
    return encoded, metadata, nil
}
```

---

## Flutter Audio Player

### Player Architecture

```dart
class AudioPlayer {
    final _audioPlayer = just_audio.AudioPlayer();
    final PlaybackService _playback;
    final AnalyticsService _analytics;
    
    // State management
    late Stream<PlayerState> _state;
    late Stream<Duration> _duration;
    late Stream<Duration> _position;
    late StreamSubscription _subscription;
}

class PlayerState {
    enum Status { IDLE, LOADING, BUFFERING, PLAYING, PAUSED, COMPLETED, ERROR }
    Status status;
    Duration position;
    Duration duration;
}
```

### Background Playback

```dart
// Configure background audio for both platforms
Future<void> configureBackgroundPlayback() async {
    // iOS
    await AudioSession.instance.configure(
        AudioSessionConfiguration.speech(),
    );
    
    // Android
    await _audioPlayer.setAndroidAudioAttributes(
        const AndroidAudioAttributes(
            contentType: AndroidAudioContentType.speech,
            flags: AndroidAudioFlags.audibilityEnforced,
        ),
    );
}

// Handle interruptions
void _handleInterruptions() {
    audioSession.devicesChangedStream.listen((devices) {
        // Pause on headphone removal
        if (!devices.contains(AudioDevice.headsetEarpiece)) {
            _audioPlayer.pause();
        }
    });
}
```

### Queue Management

```dart
class PlaybackQueue {
    final List<AudioItem> items = [];
    int _currentIndex = 0;
    
    AudioItem get current => items[_currentIndex];
    AudioItem get next => _currentIndex + 1 < items.length ? items[_currentIndex + 1] : null;
    
    void add(AudioItem item) {
        items.add(item);
        _notifyListeners();
    }
    
    Future<void> playNext() async {
        if (next != null) {
            _currentIndex++;
            await _loadAndPlay(current);
        }
    }
    
    Future<void> playPrevious() async {
        if (_currentIndex > 0) {
            _currentIndex--;
            await _loadAndPlay(current);
        }
    }
}
```

### Playback Session Management

```dart
class PlaybackSessionManager {
    Future<void> trackPlayback(AudioItem item) async {
        final session = AudioPlaybackSession(
            audioAssetId: item.id,
            userId: _auth.currentUser.id,
            startedAt: DateTime.now(),
            status: 'playing',
        );
        
        // Save to backend periodically, not every second
        _scheduleProgressSync();
    }
    
    // Batch progress updates
    Future<void> syncProgress() async {
        final progress = PlaybackProgress(
            audioAssetId: _current.id,
            positionSeconds: _audioPlayer.position.inSeconds,
            completed: _audioPlayer.position == _audioPlayer.duration,
        );
        
        try {
            await _api.updateProgress(progress);
        } catch (e) {
            // Don't fail playback if sync fails
            _logger.warn('Progress sync failed: $e');
        }
    }
}
```

### Download Management

```dart
class DownloadManager {
    final DownloadService _downloads;
    
    Stream<DownloadProgress> download(AudioItem item) async* {
        try {
            yield DownloadProgress(status: 'downloading', percent: 0);
            
            // Get signed download URL
            final downloadUrl = await _api.getDownloadUrl(item.id);
            
            // Download with resume capability
            final file = await _downloads.download(
                downloadUrl,
                fileName: '${item.id}.m4a',
                onProgress: (progress) {
                    yield DownloadProgress(
                        status: 'downloading',
                        percent: progress,
                    );
                },
            );
            
            yield DownloadProgress(status: 'downloaded', percent: 100);
        } catch (e) {
            yield DownloadProgress(status: 'failed', error: e.message);
        }
    }
    
    Future<List<DownloadedAudio>> getDownloaded() async {
        // Query local storage for downloaded content
    }
}
```

### Analytics Integration

```dart
class PlaybackAnalytics {
    final AnalyticsService _analytics;
    
    void trackEvent(AudioEvent event) {
        try {
            _analytics.log(
                name: event.type,
                parameters: {
                    'audio_id': event.audioId,
                    'position': event.position,
                    'duration': event.duration,
                    'platform': 'mobile',
                    'device': getDeviceInfo(),
                    'network': await getNetworkInfo(),
                },
            );
        } catch (e) {
            // Never block on analytics
            _logger.warn('Analytics failed: $e');
        }
    }
}
```

---

## Admin Dashboard (Next.js)

### Audio Management Interface

```typescript
// pages/admin/audio.tsx
export const AudioDashboard: React.FC = () => {
    const [assets, setAssets] = useState<AudioAsset[]>([]);
    const [jobs, setJobs] = useState<GenerationJob[]>([]);
    
    useEffect(() => {
        loadAudioAssets();
        loadGenerationJobs();
    }, []);
    
    return (
        <AdminLayout>
            <Tabs defaultValue="assets">
                <TabsContent value="assets">
                    <AudioAssetsTable assets={assets} />
                </TabsContent>
                <TabsContent value="generation">
                    <GenerationQueueMonitor jobs={jobs} />
                </TabsContent>
                <TabsContent value="qc">
                    <AudioQCDashboard />
                </TabsContent>
            </Tabs>
        </AdminLayout>
    );
};
```

### Audio Asset Management

```typescript
interface AudioAssetDetail {
    id: string;
    content: ContentInfo;
    voice: VoiceInfo;
    version: number;
    status: 'uploading' | 'processing' | 'ready' | 'failed';
    duration: number;
    format: string;
    bitrate: number;
    loudness: number;
    published: boolean;
    created: Date;
}

// Actions
export async function publishAudio(id: string): Promise<void>
export async function unpublishAudio(id: string): Promise<void>
export async function replaceAudio(id: string, newFile: File): Promise<void>
export async function archiveAudio(id: string): Promise<void>
export async function previewAudio(id: string): Promise<SignedUrl>
```

### Generation Queue Monitor

```typescript
interface GenerationQueueStats {
    queued: number;
    processing: number;
    completed: number;
    failed: number;
}

interface GenerationJobDetail {
    id: string;
    content: string;
    voice: string;
    provider: string;
    status: JobStatus;
    attempts: number;
    errors: string[];
    startedAt: Date;
    completedAt?: Date;
}

// Real-time updates
export const GenerationQueueMonitor: React.FC<{ jobs: GenerationJob[] }> = ({ jobs }) => {
    // WebSocket subscription for real-time updates
    useSubscription(
        apolloClient,
        gql`subscription {
            generationQueueUpdated {
                stats { queued processing completed failed }
                recentJobs { id status attempts }
            }
        }`
    );
    
    return (
        <div>
            <StatsCards stats={stats} />
            <JobsTable jobs={jobs} />
            <BulkActions />
        </div>
    );
};
```

### Audio QC Checklist

```typescript
interface AudioQCReview {
    audioId: string;
    checks: {
        contentCorrect: boolean;
        voiceCorrect: boolean;
        pronunciationCorrect: boolean;
        durationCorrect: boolean;
        noClipping: boolean;
        noCorruption: boolean;
        loudnessOK: boolean;
        metadataCorrect: boolean;
    };
    notes: string;
    approvedBy: string;
    approvedAt: Date;
}

// A/B preview functionality
export const AudioQCPanel: React.FC<{ current: AudioAsset; candidate: AudioAsset }> = () => {
    return (
        <div className="grid grid-cols-2 gap-4">
            <AudioPreviewCard title="Current" audio={current} />
            <AudioPreviewCard title="Candidate" audio={candidate} />
            <QCChecklistForm />
        </div>
    );
};
```

---

## Playback System

### Play Endpoint

```go
// handlers/playback.go
func (h *Handler) playAudio(w http.ResponseWriter, r *http.Request) {
    contentID := r.PathValue("content_id")
    userID := auth.UserIDFromContext(r)
    
    // Request
    var req struct {
        VoiceID string `json:"voice_id,omitempty"`
        Quality string `json:"quality,omitempty"`
    }
    if err := httpx.DecodeJSON(r, &req); err != nil {
        httpx.WriteError(w, http.StatusBadRequest, "invalid request")
        return
    }
    
    // Resolve playable audio with authorization
    resolver := playback.NewResolver(h.audio, h.entitlements)
    asset, signedURL, err := resolver.Resolve(r.Context(), playback.Request{
        UserID:      userID,
        ContentID:   contentID,
        VoiceID:     req.VoiceID,
        Quality:     req.Quality,
    })
    if err != nil {
        switch err {
        case ErrEntitlementRequired:
            httpx.WriteError(w, http.StatusForbidden, "This content requires Premium")
        case ErrVoiceUnavailable:
            httpx.WriteError(w, http.StatusUnprocessableEntity, "Selected voice not available")
        case ErrAudioNotReady:
            httpx.WriteError(w, http.StatusServiceUnavailable, "Audio not yet ready")
        default:
            httpx.WriteError(w, http.StatusInternalServerError, "Failed to resolve audio")
        }
        return
    }
    
    // Response
    httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{
        "audio_id":   asset.ID,
        "stream_url": signedURL,
        "expires_at": time.Now().Add(4 * time.Hour).Format(time.RFC3339),
        "duration":   asset.DurationSeconds,
        "format":     asset.Format,
    })
}
```

### Download Endpoint

```go
// handlers/download.go
func (h *Handler) downloadAudio(w http.ResponseWriter, r *http.Request) {
    audioID := r.PathValue("audio_id")
    userID := auth.UserIDFromContext(r)
    
    // Check entitlement
    canDownload, err := h.entitlements.CanDownload(r.Context(), userID, audioID)
    if !canDownload {
        httpx.WriteError(w, http.StatusForbidden, "Not authorized to download")
        return
    }
    
    // Get asset
    asset, err := h.audio.GetAsset(r.Context(), audioID)
    if err != nil {
        httpx.WriteError(w, http.StatusNotFound, "Audio not found")
        return
    }
    
    // Generate signed download URL (longer expiry than stream)
    signedURL, err := h.storage.GenerateSignedURL(r.Context(), asset.StorageKey, 24*time.Hour)
    if err != nil {
        httpx.WriteError(w, http.StatusInternalServerError, "Failed to generate download URL")
        return
    }
    
    // Record download start
    h.events.Track(AudioEvent{
        EventType: "download_started",
        AudioID:   audioID,
        UserID:    userID,
    })
    
    httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{
        "download_url": signedURL,
        "expires_at":   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
        "size_bytes":   asset.FileSizeBytes,
    })
}
```

---

## Audio Generation Pipeline

### Generation Request

```go
// API: POST /admin/audio/generate
type GenerateRequest struct {
    ContentVersionID string `json:"content_version_id"`
    VoiceID          string `json:"voice_id"`
    QualityTier      string `json:"quality_tier"` // standard | high
    Format           string `json:"format,omitempty"` // m4a | mp3
    Bulk             bool   `json:"bulk,omitempty"`
}

// Response
type GenerateResponse struct {
    JobID string `json:"job_id"`
    Status string `json:"status"` // queued
}

// Implementation
func (h *Handler) generateAudio(w http.ResponseWriter, r *http.Request) {
    var req GenerateRequest
    if err := httpx.DecodeJSON(r, &req); err != nil {
        httpx.WriteError(w, http.StatusBadRequest, "invalid request")
        return
    }
    
    // Create idempotency key
    key := fmt.Sprintf("%s:%s:%s:%s", req.ContentVersionID, req.VoiceID, req.QualityTier, req.Format)
    
    // Check if already processing
    existing, _ := h.audio.FindJobByIdempotency(r.Context(), key)
    if existing != nil && existing.Status != "failed" {
        httpx.WriteJSON(w, http.StatusAccepted, GenerateResponse{
            JobID:  existing.ID,
            Status: existing.Status,
        })
        return
    }
    
    // Create job
    job := &models.AudioGenerationJob{
        ID:                 newID(),
        ContentVersionID:   req.ContentVersionID,
        VoiceID:            req.VoiceID,
        QualityTier:        req.QualityTier,
        Format:             req.Format,
        Status:             "queued",
        IdempotencyKey:     key,
        RequestedBy:        auth.UserIDFromContext(r),
        CreatedAt:          now(),
    }
    
    if err := h.audio.CreateJob(r.Context(), job); err != nil {
        httpx.WriteError(w, http.StatusInternalServerError, "failed to create job")
        return
    }
    
    // Enqueue for processing
    h.jobs.Enqueue("audio.generate", map[string]interface{}{
        "job_id": job.ID,
    })
    
    httpx.WriteJSON(w, http.StatusAccepted, GenerateResponse{
        JobID:  job.ID,
        Status: "queued",
    })
}
```

### Retry Strategy

```go
const (
    maxRetries = 3
    baseDelay  = 5 * time.Second
)

func exponentialBackoff(attempt int) time.Duration {
    return baseDelay * time.Duration(math.Pow(2, float64(attempt)))
}

func (w *AudioWorker) retryJob(job *AudioGenerationJob, code, msg string) error {
    job.AttemptCount++
    job.ErrorCode = code
    job.ErrorMessage = msg
    
    if job.AttemptCount < job.MaxAttempts {
        // Schedule retry
        delay := exponentialBackoff(job.AttemptCount)
        return w.jobs.RetryAfter(job.ID, delay)
    } else {
        // Give up
        job.Status = "failed"
        return w.repo.UpdateJob(context.Background(), job)
    }
}
```

---

## Storage & CDN

### Storage Abstraction

```go
// storage/interface.go
type ObjectStorage interface {
    Upload(ctx context.Context, key string, data []byte, metadata map[string]string) error
    Download(ctx context.Context, key string) ([]byte, error)
    Delete(ctx context.Context, key string) error
    GenerateSignedURL(ctx context.Context, key string, ttl time.Duration) (string, error)
    List(ctx context.Context, prefix string) ([]string, error)
}

// Implementations
type S3Storage struct {
    client *s3.Client
    bucket string
}

func (s *S3Storage) GenerateSignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
    // Use AWS Signature Version 4 for time-limited access
    presigner := s3.NewPresigner()
    request, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
        Bucket: aws.String(s.bucket),
        Key:    aws.String(key),
    }, func(opts *s3.PresignOptions) {
        opts.Expires = duration.DurationValue(ttl)
    })
    if err != nil {
        return "", err
    }
    return request.URL, nil
}
```

### CDN Integration

```go
// cdn/handler.go
type CDNHandler struct {
    storage ObjectStorage
    cache   *redis.Client
    config  *CDNConfig
}

type CDNConfig struct {
    Provider   string // cloudflare | cloudfront | bunny
    Domain     string
    CacheTTL   time.Duration
    PurgePath  string
}

// Cache invalidation on asset update
func (c *CDNHandler) InvalidateCache(key string) error {
    // Purge old version
    if err := c.purge(key); err != nil {
        // Log but don't fail
        log.Printf("CDN purge failed for %s: %v", key, err)
    }
    return nil
}
```

### Deterministic Storage Keys

```go
// storage/keys.go
func GenerateStorageKey(content *ContentVersion, voice *Voice, assetType, quality string) string {
    return fmt.Sprintf(
        "audio/content/%s/version/%d/voice/%s/%s/%s/audio.m4a",
        content.ID,
        content.VersionNumber,
        voice.ID,
        assetType,  // master, stream, preview, download
        quality,    // standard, high, lossless
    )
}

// Example output:
// audio/content/confession-123/version/1/voice/voice-456/master/standard/audio.m4a
// audio/content/confession-123/version/1/voice/voice-456/stream/standard/audio.m4a
// audio/content/confession-123/version/1/voice/voice-456/preview/standard/audio.m4a
```

---

## Analytics

### Playback Events

```go
type AudioEvent struct {
    ID              string
    UserID          string
    AudioAssetID    string
    EventType       string // play_request | play_started | play_paused | play_completed | download_started
    PositionSeconds int
    DurationSeconds int
    DeviceID        string
    Platform        string // iOS | Android | Web
    NetworkType     string // wifi | cellular
    BandwidthMbps   float64
    ErrorCode       string
    CreatedAt       string
}

// Non-blocking event tracking
func (h *Handler) trackEvent(event *AudioEvent) {
    go func() {
        if err := h.events.Record(context.Background(), event); err != nil {
            log.Printf("Event tracking failed: %v", err)
            // Never fail playback
        }
    }()
}
```

### QoE Metrics

```go
type QoEMetrics struct {
    UserID              string
    AudioAssetID        string
    StartupLatencyMS    int
    BufferDurationMS    int
    RebufferCount       int
    CompletionRate      float64
    FailureRate         float64
}

// Measured after playback completes
func (h *Handler) recordQoE(session *PlaybackSession) error {
    metrics := &QoEMetrics{
        UserID:       session.UserID,
        AudioAssetID: session.AudioAssetID,
        StartupLatencyMS: calculateStartupLatency(session),
        RebufferCount: session.RebufferCount,
        CompletionRate: float64(session.PositionSeconds) / float64(session.DurationSeconds),
    }
    return h.qoe.Record(context.Background(), metrics)
}
```

---

## MVP Implementation Checklist

### Phase 1: Infrastructure (Week 1-2)

- [ ] Database schema deployed ✅
- [ ] Models defined ✅
- [ ] Storage abstraction implemented
- [ ] Base Go service structure
- [ ] Job queue setup

### Phase 2: Audio Generation (Week 3-4)

- [ ] TTS provider integration (Google Cloud TTS)
- [ ] Audio processor (normalization, validation)
- [ ] Generation worker
- [ ] Job retry logic
- [ ] Admin generation endpoints

### Phase 3: Playback (Week 5-6)

- [ ] Playback resolver
- [ ] Signed URL generation
- [ ] Flutter audio player
- [ ] Background playback
- [ ] Play endpoint

### Phase 4: Admin Dashboard (Week 7)

- [ ] Audio assets table
- [ ] Generation queue monitor
- [ ] Basic QC interface
- [ ] Publish/unpublish

### Phase 5: Analytics (Week 8)

- [ ] Event recording
- [ ] QoE metrics
- [ ] Playback dashboard

### Phase 6: Testing & Polish (Week 9-10)

- [ ] End-to-end tests
- [ ] Load testing
- [ ] Security validation
- [ ] Performance optimization

---

## Configuration

### Environment Variables

```bash
# Storage
STORAGE_PROVIDER=s3                    # s3 | gcs | azure | local
S3_BUCKET=iconfess-audio
S3_REGION=us-east-1
S3_ACCESS_KEY=***
S3_SECRET_KEY=***

# CDN
CDN_PROVIDER=cloudflare
CDN_DOMAIN=audio.example.com
CDN_CACHE_TTL=3600

# Audio Processing
AUDIO_TARGET_LOUDNESS=-18              # LUFS
AUDIO_MAX_PEAK=-1                      # dB
AUDIO_SAMPLE_RATE=48000
AUDIO_BITRATE=128                      # kbps

# TTS Provider
TTS_PROVIDER=google
GOOGLE_TTS_PROJECT_ID=***
GOOGLE_TTS_CREDENTIALS_JSON=***

# Processing
WORKER_CONCURRENCY=4
JOB_MAX_RETRIES=3
JOB_TIMEOUT_SECONDS=600
```

---

## Testing Strategy

### Unit Tests

```go
// audio/processor_test.go
func TestAudioNormalization(t *testing.T) {
    // Test loudness normalization to -18 LUFS
}

func TestAudioValidation(t *testing.T) {
    // Test codec, duration, format validation
}
```

### Integration Tests

```go
// audio/worker_test.go
func TestGenerationFlow(t *testing.T) {
    // Test: Content → Voice → Generation → Processing → Asset
}

func TestPlaybackResolution(t *testing.T) {
    // Test: User → Entitlement → Voice → Asset → Signed URL
}
```

### End-to-End Tests

```
User opens app
    ↓
Selects confession
    ↓
API resolves playable audio
    ↓
CDN serves audio
    ↓
Flutter plays
    ↓
Progress tracked
    ↓
Completion recorded
```

---

## Performance Targets

| Metric | Target | Strategy |
|--------|--------|----------|
| Playback startup latency | <2s | CDN, signed URLs, client-side buffering |
| Generation latency | <30s (standard quality) | Async workers, provider optimization |
| Audio asset resolution | <100ms | Caching, optimized queries |
| Storage performance | <500ms upload | Multipart upload, parallel writes |

---

## Security Checklist

- [ ] Signed URLs expire appropriately (4h for streaming, 24h for download)
- [ ] Voice rights validated before generation/playback
- [ ] Admin endpoints require authentication
- [ ] Storage keys don't expose internal structure
- [ ] Analytics doesn't log sensitive data
- [ ] Download URLs require active entitlement
- [ ] Rate limiting on generation/download
- [ ] Audit logging for admin actions

---

## Monitoring & Alerting

### Key Metrics

```
audio_generation_latency_seconds       (histogram)
audio_generation_failures_total        (counter)
playback_resolution_latency_ms         (histogram)
playback_failures_total                (counter)
audio_asset_publication_latency_seconds (histogram)
storage_operation_latency_seconds      (histogram)
cdn_cache_hit_ratio                    (gauge)
```

### Alerts

```
IF audio_generation_failure_rate > 5% THEN alert
IF playback_startup_latency_p95 > 5000ms THEN alert
IF cdn_origin_failures > 0 THEN alert
IF generation_queue_depth > 100 THEN alert
IF storage_operation_error_rate > 1% THEN alert
```

---

## Next Steps (V1 → V2 → V3)

### V1: Polish & Scale
- Multiple audio quality variants
- Advanced audio processing (compression, EQ)
- Adaptive bitrate streaming
- Cross-device progress sync
- Advanced QoE dashboard

### V2: Personalization
- Multi-language support
- Intelligent session assembly
- Voice recommendations
- Regional delivery optimization
- Advanced recommendation engine

### V3: AI Integration
- AI-assisted session generation
- Semantic audio discovery
- Predictive quality optimization
- Dynamic voice selection
- Intelligent content orchestration

---

## References

- Master Prompt: 162 Requirements for Audio Platform (Requirements 1-162)
- Spotify Architecture Principles (Requirements 139-140)
- YouTube Music Architecture Principles (Requirements 140)
- Mobile Audio Best Practices: iOS/Android
- CDN & Object Storage Best Practices
- Audio Quality Standards: ITU-R BS.1770-4 (Loudness)

---

**Document Version**: 1.0  
**Last Updated**: 2026-09-01  
**Status**: ARCHITECTURE COMPLETE, READY FOR IMPLEMENTATION  
