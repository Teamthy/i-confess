package api

import (
	"github.com/Teamthy/i-confess/internal/voiceengine"
	"github.com/Teamthy/i-confess/internal/voiceeval"
	"net/http"
	"sync"
	"time"

	"github.com/Teamthy/i-confess/internal/db"

	"github.com/Teamthy/i-confess/internal/audio"
	"github.com/Teamthy/i-confess/internal/bible"
	"github.com/Teamthy/i-confess/internal/billing"
	"github.com/Teamthy/i-confess/internal/cache"
	"github.com/Teamthy/i-confess/internal/deletion"
	"github.com/Teamthy/i-confess/internal/email"
	"github.com/Teamthy/i-confess/internal/engine"
	"github.com/Teamthy/i-confess/internal/jobs"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/oauth"
	"github.com/Teamthy/i-confess/internal/ratelimit"
	"github.com/Teamthy/i-confess/internal/scheduler"
	"github.com/Teamthy/i-confess/internal/search"
	"github.com/Teamthy/i-confess/internal/storage"
	"github.com/Teamthy/i-confess/internal/store"
	"github.com/Teamthy/i-confess/internal/voice"
)

// Handler bundles all stores and config needed by the API.
type Handler struct {
	cfg            Config
	bible          bible.BibleProvider
	bibleDiscovery bible.BibleProvider
	users          *store.UserStore
	trials         *store.TrialStore
	cont           *store.ContentStore
	audio          *store.AudioStore
	sess           *store.SessionStore
	sched          *store.ScheduleStore
	eng            *store.EngagementStore
	mod            *store.ModerationStore
	// analytics persists the funnel events. Trial day completion, conversion
	// and cancellation are recorded here rather than in a client batch, so the
	// server is the source of truth for the numbers it reports about itself.
	analytics *store.AnalyticsStore
	// blocks owns listener-set boundaries. Separate from mod because a block is
	// not a moderation action: no moderator takes it and none can see it.
	blocks *store.BlockStore
	// signals reads the seven listener signals personalization ranks on
	// (master-plan 34). Read-only: it derives evidence from tables other
	// features write, so there is no second bookkeeping to drift.
	signals   *store.SignalStore
	engn      *engine.Engine
	search    *search.SearchStore
	templates *store.TemplateStore
	plans     *store.PlanStore
	// queue holds background work. It is the interface rather than the
	// in-memory type so the server can run the durable PostgreSQL queue.
	queue jobs.Queue
	// queueDurable reports which implementation is in use, for the admin view.
	queueDurable bool
	// signer mints short-lived audio URLs. Audio bytes never pass through this
	// API (PRD S11); the client is handed a signed CDN link instead.
	signer storage.ObjectStorage
	// mediaHandler is the development-only signed audio origin.
	mediaHandler http.Handler
	// pipeline renders text to audio. Nil when synthesis is unconfigured, in
	// which case generation endpoints report 503 rather than failing obscurely.
	pipeline *voice.Pipeline
	// playbackResolver resolves playback requests with entitlement checking.
	playbackResolver *audio.PlaybackResolver
	// urlGenerator generates signed URLs for audio streaming and downloads.
	urlGenerator *audio.URLGenerator
	// vrights stores voice authorization metadata.
	vrights *store.VoiceRightsStore
	// vplat stores the licensed minister voice platform (granular rights,
	// models, references, generations).
	vplat *store.VoicePlatformStore
	// vorch is the multi-engine TTS orchestrator. Nil when no voice-engine
	// workers are configured; generation then reports 503.
	vorch *voiceengine.Orchestrator
	// vthresholds are the promotion quality thresholds.
	vthresholds voiceeval.Thresholds
	// vworker runs intake and training; nil disables those endpoints (503).
	vworker *voiceengine.WorkerClient
	vintake VoiceIntakeConfig
	db      *db.DB
	// devTokenSink receives one-time tokens in development and tests. Nil in
	// production, where tokens go only to the email queue.
	devTokenSink func(purpose, email, token string)
	// limiter throttles authentication abuse (S20, S21).
	limiter ratelimit.Enforcer
	// profiles owns profile, preferences and interests (S5, S14, S20).
	profiles *store.ProfileStore
	// mail delivers transactional authentication email asynchronously, so a
	// slow provider cannot fail a registration (S57, S58).
	mail    *email.Queue
	mailCfg email.Config
	// verifiers validate third-party identity tokens (S36, S37).
	verifiers map[string]oauth.Verifier
	// library owns collections, devices and notification preferences.
	library *store.LibraryStore
	// deletion performs account erasure under an explicit retention policy.
	deletion *deletion.Service
	// dispatcher delivers scheduled-session reminders (S19, S47).
	dispatcher *scheduler.Dispatcher
	// downloads manages offline licences (S28).
	downloads *store.DownloadStore
	// idem replays recorded responses so a retried mutation cannot execute
	// twice (S47).
	idem *store.IdempotencyStore
	// Content caches, scoped to this Handler so it only ever serves content
	// from the database it is connected to (S7.1).
	catCache     *cache.Cache[[]models.Category]
	catConfCache *cache.Cache[[]models.Confession]
	voicesCache  *cache.Cache[[]models.Voice]
	cacheMeter   *cache.Meter
	// cacheBus carries invalidations to the other API instances, and
	// cacheOwner identifies this instance so it ignores the echo of its own
	// messages (G-10). A nil bus is a single-instance deployment: writes still
	// invalidate this instance's own caches.
	cacheBus   cache.Bus
	cacheOwner string
	// metrics counts security-relevant events for alerting (S83, S84).
	metrics      *AuthMetrics
	bibleMetrics *bibleOperationMetrics
	// Store notification collaborators (IC-003, PR B). Nil means "resolve from
	// the environment", which is what production does; tests supply them
	// directly, and a deployment that reads credentials from a secret manager
	// can too.
	appleNotify  appleNotificationVerifier
	googleNotify googleNotificationResolver
	playAck      billing.PlayAcknowledger
	// cacheStats reports cache hit-rate for /metrics (§7.1)
	cacheStats func() CacheStats
	// routes records every registered endpoint, so the API spec is generated
	// from the same calls that serve traffic and cannot drift.
	routes   *routeRecorder
	authMW   map[string]func(http.Handler) http.Handler
	authMWmu sync.Mutex
	isProd   bool
	// rbac owns custom roles and user-role assignments for the super admin console.
	rbac *store.RBACStore
}

// SetLimiter installs a rate limiter. Production passes a Redis-backed
// enforcer so limits hold across replicas; without it each instance would allow
// the full burst independently.
func (h *Handler) SetLimiter(e ratelimit.Enforcer) {
	if e != nil {
		h.limiter = e
	}
}

// SetMailer installs the transactional email queue and templates.
func (h *Handler) SetMailer(q *email.Queue, cfg email.Config) {
	h.mail, h.mailCfg = q, cfg
}

// SetDevTokenSink installs a development/test hook for one-time tokens.
func (h *Handler) SetDevTokenSink(f func(purpose, email, token string)) { h.devTokenSink = f }

// SetProduction configures production mode for security headers and hardening.
func (h *Handler) SetProduction(prod bool) { h.isProd = prod }

// SetPlaybackResolver installs the audio playback resolver.
func (h *Handler) SetPlaybackResolver(r *audio.PlaybackResolver) { h.playbackResolver = r }

// SetURLGenerator installs the signed URL generator.
func (h *Handler) SetURLGenerator(g *audio.URLGenerator) { h.urlGenerator = g }

type Config struct {
	JWTSecret string
	TokenTTL  string
}

func NewHandler(cfg Config, db *db.DB) *Handler {
	return &Handler{
		cfg:            cfg,
		bible:          &bible.LocalBibleProvider{DB: db},
		bibleDiscovery: &bible.LocalBibleProvider{DB: db},
		users:          store.NewUserStore(db),
		analytics:      store.NewAnalyticsStore(db),
		blocks:         store.NewBlockStore(db),
		signals:        store.NewSignalStore(db),
		// The trial store writes its own funnel events: expiry is a clock fact
		// that no single handler reliably observes, so the transition records it.
		trials:       store.NewTrialStore(db).WithAnalytics(store.NewAnalyticsStore(db)),
		cont:         store.NewContentStore(db),
		audio:        store.NewAudioStore(db),
		sess:         store.NewSessionStore(db),
		sched:        store.NewScheduleStore(db),
		eng:          store.NewEngagementStore(db),
		mod:          store.NewModerationStore(db),
		search:       search.NewSearchStore(db),
		templates:    store.NewTemplateStore(db),
		plans:        store.NewPlanStore(db),
		queue:        jobs.NewMemoryQueue(),
		vrights:      store.NewVoiceRightsStore(db),
		vplat:        store.NewVoicePlatformStore(db),
		db:           db,
		limiter:      ratelimit.New(),
		profiles:     store.NewProfileStore(db),
		verifiers:    map[string]oauth.Verifier{},
		library:      store.NewLibraryStore(db),
		deletion:     deletion.NewService(db),
		downloads:    store.NewDownloadStore(db),
		idem:         store.NewIdempotencyStore(db),
		catCache:     cache.New[[]models.Category](5*time.Minute, 10*time.Minute),
		catConfCache: cache.New[[]models.Confession](2*time.Minute, 5*time.Minute),
		voicesCache:  cache.New[[]models.Voice](5*time.Minute, 10*time.Minute),
		cacheMeter:   &cache.Meter{},
		metrics:      NewAuthMetrics(),
		bibleMetrics: newBibleOperationMetrics(),
		routes:       &routeRecorder{},
		authMW:       make(map[string]func(http.Handler) http.Handler),
		rbac:         store.NewRBACStore(db),
	}
}

// auditRights records a change to a voice licence. Failures are logged and
// swallowed: the rights change itself already succeeded, and losing an audit
// line must not roll it back.
func (h *Handler) auditRights(r *http.Request, voiceID string, aiGranted bool, attestation string) {
	action := "voice_rights_updated"
	if aiGranted {
		action = "voice_rights_ai_generation_granted"
	}
	if attestation == "" {
		attestation = "no attestation supplied"
	}
	// Persisted, not merely logged: a rights dispute needs a queryable record,
	// and log retention is measured in days while a licence decision matters
	// for years (S51).
	h.recordAudit(r, action, "voice", voiceID, attestation, "success")
}

// usersDB exposes the underlying database handle for tests that need to
// manipulate subscription state directly.

// BuildEngine wires the session engine after handler construction.
func (h *Handler) BuildEngine() {
	h.cacheStats = h.cacheStatsSnapshot
	h.engn = engine.New(h.cont, h.audio, h.users)
}

// GetQueue returns the job queue for external wiring (e.g., workers).
func (h *Handler) GetQueue() jobs.Queue {
	return h.queue
}

// SetQueue replaces the in-process queue with a durable one.
//
// The default is jobs.MemoryQueue, which is right for tests and wrong for a
// server: everything still queued when the process stops is gone, and a stop is
// a deploy rather than an accident. The server calls this at startup.
func (h *Handler) SetQueue(q jobs.Queue) {
	h.queue = q
	h.queueDurable = true
}
