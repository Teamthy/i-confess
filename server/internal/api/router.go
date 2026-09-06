package api

import (
	"net/http"

	"github.com/Teamthy/i-confess/internal/adminui"
	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/ratelimit"
	"github.com/Teamthy/i-confess/internal/tracing"
	"github.com/Teamthy/i-confess/internal/webapp"
)

// Routes builds the full HTTP handler with all routes registered.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()

	// Authentication endpoints are the highest-value attack surface, so they
	// are throttled by client address before any database work happens (S20).
	perIP := func(rule ratelimit.Rule, name string) func(http.Handler) http.Handler {
		return ratelimit.MiddlewareFor(h.limiter, rule, name, clientIP)
	}
	loginLimit := perIP(ratelimit.LoginPerIP, "login")
	registerLimit := perIP(ratelimit.Registration, "register")
	emailLimit := perIP(ratelimit.EmailSend, "email")

	// Health probes (PRD S85). Liveness and readiness are separate on purpose:
	// a liveness probe that fails on a database blip makes the orchestrator
	// restart a healthy process and turns an outage into a crash loop.
	h.route(mux, "GET /healthz", "public", "ops", "Liveness", nil, h.livez)
	h.route(mux, "GET /health/live", "public", "ops", "Liveness", nil, h.livez)
	h.route(mux, "GET /health/ready", "public", "ops", "Readiness with subsystem detail", nil, h.readyz)

	// Machine-readable API description, generated from the route table above.
	mux.HandleFunc("GET /metrics", h.promMetrics)
	mux.HandleFunc("GET /openapi.json", h.serveOpenAPI)

	// Listener web application (SPA), served at the site root.
	mux.Handle("GET /", webapp.Handler())

	// Admin console (SPA)
	mux.Handle("GET /admin", http.RedirectHandler("/admin/", http.StatusMovedPermanently))
	mux.Handle("GET /admin/", http.StripPrefix("/admin/", adminui.Handler()))

	// Placeholder/dev audio assets (generated locally; replaced by CDN in production).
	// Development audio origin. Previously this served the media directory
	// with an unguarded http.FileServer, which handed out every audio file to
	// anyone who could guess a path. It now enforces the same signature and
	// expiry rules as the production CDN (PRD S11).
	if h.mediaHandler != nil {
		mux.Handle("GET /media/", h.mediaHandler)
	}

	// Public auth
	h.route(mux, "POST /auth/register", "public", "auth", "Create an account and send a verification email", registerLimit, h.register)
	h.route(mux, "POST /auth/login", "public", "auth", "Sign in; returns mfa_required when a second factor is enrolled", loginLimit, h.login)
	h.route(mux, "POST /auth/verify-email", "public", "auth", "Verify an email address with a one-time token", nil, h.verifyEmail)
	h.route(mux, "POST /auth/resend-verification", "public", "auth", "Resend the verification email", emailLimit, h.resendVerification)
	h.route(mux, "POST /auth/request-password-reset", "public", "auth", "Request a password reset link", emailLimit, h.requestPasswordReset)
	h.route(mux, "POST /auth/reset-password", "public", "auth", "Set a new password using a reset token", nil, h.resetPassword)

	// Public content (read-only, published only)
	h.route(mux, "GET /collections", "public", "content", "Published collections", nil, h.listCollections)
	h.route(mux, "GET /categories", "public", "content", "Published categories (cached SWR 5m)", nil, h.cachedListCategories)
	h.route(mux, "GET /categories/{id}/confessions", "public", "content", "Confessions in a category (cached SWR 2m)", nil, h.cachedCategoryConfessions)
	h.route(mux, "GET /confessions/{id}", "public", "content", "One confession", nil, h.getConfession)
	h.route(mux, "GET /voices", "public", "content", "Available voices (cached SWR 5m)", nil, h.cachedListVoices)

	// Authenticated user routes
	// Every authenticated route validates the session server-side, so logout,
	// suspension and password changes take effect immediately rather than when
	// the token happens to expire (PRD S3, S54).
	sv := sessionValidator{h: h}

	authed := auth.MiddlewareWithSessions(h.cfg.JWTSecret, sv)
	h.route(mux, "GET /me", "user", "profile", "Account summary", authed, h.me)
	h.route(mux, "GET /home", "user", "home", "Home sections", authed, h.home)

	// Profile domain (PRD S51, S56). Separate resources rather than one giant
	// document, plus a bootstrap aggregate for cold start.
	h.route(mux, "GET /me/bootstrap", "user", "profile", "Everything needed to start the app", authed, h.bootstrap)
	h.route(mux, "GET /me/profile", "user", "profile", "Read profile", authed, h.getProfile)
	h.route(mux, "PATCH /me/profile", "user", "profile", "Update profile", authed, h.updateProfile)
	// Avatar upload is throttled: image processing is the most CPU-expensive
	// thing an authenticated user can trigger.
	h.route(mux, "POST /me/avatar", "user", "profile", "Upload an avatar; EXIF is stripped", func(n http.Handler) http.Handler { return registerLimit(authed(n)) }, h.uploadAvatar)
	h.route(mux, "DELETE /me/avatar", "user", "profile", "Remove the avatar", authed, h.deleteAvatar)
	h.route(mux, "GET /me/preferences", "user", "profile", "Read preferences", authed, h.getPreferences)
	h.route(mux, "PATCH /me/preferences", "user", "profile", "Update preferences", authed, h.updatePreferences)
	// User collections (PRD S53).
	h.route(mux, "GET /me/collections", "user", "library", "List collections", authed, h.listCollections2)
	h.route(mux, "POST /me/collections", "user", "library", "Create a collection (private by default)", authed, h.createCollection)
	h.route(mux, "GET /me/collections/{id}", "user", "library", "Read a collection", authed, h.getCollection)
	h.route(mux, "PATCH /me/collections/{id}", "user", "library", "Update a collection", authed, h.updateCollection)
	h.route(mux, "DELETE /me/collections/{id}", "user", "library", "Delete a collection", authed, h.deleteCollection)
	h.route(mux, "POST /me/collections/{id}/items", "user", "library", "Add a confession to a collection", authed, h.addCollectionItem)
	h.route(mux, "DELETE /me/collections/{id}/items/{confessionId}", "user", "library", "Remove an item", authed, h.removeCollectionItem)
	h.route(mux, "PATCH /me/collections/{id}/reorder", "user", "library", "Reorder items", authed, h.reorderCollection)

	// Devices and notifications (PRD S45, S46).
	h.route(mux, "POST /me/devices", "user", "devices", "Register a device and its push token", authed, h.registerDevice)
	h.route(mux, "GET /me/devices", "user", "devices", "List devices", authed, h.listDevices)
	h.route(mux, "DELETE /me/devices/{id}", "user", "devices", "Remove a device", authed, h.revokeDevice)
	h.route(mux, "GET /me/notifications", "user", "devices", "Notification preferences", authed, h.getNotificationPreferences)
	h.route(mux, "PATCH /me/notifications", "user", "devices", "Update notification preferences", authed, h.updateNotificationPreferences)

	// Account deletion (PRD S40, S50). Requesting is throttled and requires
	// re-authentication; cancelling deliberately is not, so a user racing to
	// save their account from an attacker is not slowed down.
	h.route(mux, "GET /me/deletion", "user", "account", "Preview what deletion removes and retains", authed, h.deletionPreview)
	h.route(mux, "POST /me/deletion", "user", "account", "Schedule account deletion", func(n http.Handler) http.Handler { return registerLimit(authed(n)) }, h.requestDeletion)
	h.route(mux, "DELETE /me/deletion", "user", "account", "Cancel a scheduled deletion", authed, h.cancelDeletion)

	// Offline downloads (PRD S28).
	h.route(mux, "GET /me/downloads", "user", "downloads", "List offline licences", authed, h.listDownloads)
	h.route(mux, "POST /me/downloads", "user", "downloads", "Take a confession offline (premium)", authed, h.createDownload)
	h.route(mux, "POST /me/downloads/{id}/refresh", "user", "downloads", "Renew an offline licence", authed, h.refreshDownload)
	h.route(mux, "DELETE /me/downloads/{id}", "user", "downloads", "Release an offline licence", authed, h.deleteDownload)

	// Data export (PRD S49).
	h.route(mux, "GET /me/export", "user", "account", "Download all personal data", authed, h.exportData)

	h.route(mux, "GET /me/interests", "user", "profile", "Interests, split into explicit and inferred", authed, h.getInterests)
	h.route(mux, "PUT /me/interests", "user", "profile", "Replace explicitly chosen interests", authed, h.putInterests)
	h.route(mux, "POST /auth/change-password", "user", "auth", "Change password and revoke other sessions", authed, h.changePassword)
	h.route(mux, "POST /auth/refresh", "user", "auth", "Rotate the session and issue a new access token", authed, h.refreshToken)
	h.route(mux, "POST /auth/logout", "user", "auth", "End this session", authed, h.logout)
	// Social sign-in (PRD S36, S37). Throttled like password login: an identity
	// token is a credential and the endpoint is equally attackable.
	h.route(mux, "POST /auth/social/{provider}", "public", "auth", "Exchange a provider identity token for a session", loginLimit, h.socialSignIn)

	// Linking requires an authenticated session, which is what proves the user
	// owns the account a new credential is being attached to (S38).
	h.route(mux, "GET /auth/identities", "user", "auth", "List linked sign-in providers", authed, h.listIdentities)
	h.route(mux, "POST /auth/identities/{provider}", "user", "auth", "Link a provider to this account", authed, h.linkIdentity)
	h.route(mux, "DELETE /auth/identities/{provider}", "user", "auth", "Unlink a provider", authed, h.unlinkIdentity)

	// Two-factor authentication (PRD S41, S81).
	h.route(mux, "GET /auth/mfa", "user", "mfa", "Two-factor status", authed, h.mfaStatus)
	h.route(mux, "POST /auth/mfa/begin", "user", "mfa", "Start enrolment; returns a server-generated secret", authed, h.beginMFAEnrolment)
	h.route(mux, "POST /auth/mfa/confirm", "user", "mfa", "Confirm enrolment and receive recovery codes", authed, h.confirmMFAEnrolment)
	h.route(mux, "POST /auth/mfa/disable", "user", "mfa", "Disable two-factor authentication", authed, h.disableMFA)

	h.route(mux, "GET /auth/sessions", "user", "auth", "List live sign-ins", authed, h.listAuthSessions)
	h.route(mux, "DELETE /auth/sessions/{id}", "user", "auth", "Sign out one device", authed, h.revokeAuthSession)
	h.route(mux, "POST /auth/logout-all", "user", "auth", "End every session for this account", authed, h.logoutAll)
	h.route(mux, "POST /auth/consent", "user", "auth", "Record a consent decision", authed, h.recordConsent)
	h.route(mux, "POST /auth/security-events", "user", "auth", "Record a client-side security event", authed, h.securityEvent)

	h.route(mux, "POST /sessions", "user", "sessions", "Compose a session; audio URLs are signed", authed, h.createSession)
	h.route(mux, "GET /sessions/{id}", "user", "sessions", "Read a session with freshly signed audio", authed, h.getSession)
	h.route(mux, "PATCH /sessions/{id}", "user", "sessions", "Update session status", authed, h.updateSessionStatus)
	idempotent := h.idempotencyMiddleware

	h.route(mux, "GET /sessions", "user", "sessions", "List sessions", authed, h.listMySessions)

	h.route(mux, "GET /schedules", "user", "schedules", "List schedules", authed, h.listSchedules)
	h.route(mux, "POST /schedules", "user", "schedules", "Create a schedule", authed, h.createSchedule)
	h.route(mux, "PATCH /schedules/{id}", "user", "schedules", "Update a schedule", authed, h.updateSchedule)
	h.route(mux, "DELETE /schedules/{id}", "user", "schedules", "Delete a schedule", authed, h.deleteSchedule)
	h.route(mux, "POST /schedules/{id}/start", "user", "schedules", "Trigger a schedule — builds a session", authed, func(w http.ResponseWriter, r *http.Request) {
		idempotent(http.HandlerFunc(h.triggerSchedule)).ServeHTTP(w, r)
	})
	h.route(mux, "POST /sessions/preview", "user", "sessions", "Preview a session (dry-run)", authed, h.previewSession)
	h.route(mux, "POST /templates", "user", "templates", "Create a template", authed, h.createTemplate)
	h.route(mux, "GET /templates", "user", "templates", "List templates", authed, h.listTemplates)
	h.route(mux, "GET /templates/{id}", "user", "templates", "Read a template", authed, h.getTemplate)
	h.route(mux, "GET /t/{token}", "public", "templates", "Resolve shareable template", nil, h.getTemplateByShareToken)

	h.route(mux, "POST /me/favorites", "user", "library", "Add a favourite", authed, h.addFavorite)
	h.route(mux, "DELETE /me/favorites", "user", "library", "Remove a favourite", authed, h.removeFavorite)
	h.route(mux, "GET /me/favorites", "user", "library", "List favourites", authed, h.listFavorites)

	h.route(mux, "GET /me/history", "user", "library", "Listening history", authed, h.history)
	h.route(mux, "POST /me/history", "user", "library", "Record playback", authed, h.recordPlayback)

	h.route(mux, "POST /me/confessions", "user", "library", "Create a personal confession", authed, h.createUserConfession)
	h.route(mux, "GET /recommendations", "user", "home", "Personalized recommendations", authed, h.recommendations)
	h.route(mux, "GET /me/confessions", "user", "library", "List personal confessions", authed, h.listUserConfessions)

	// Admin routes
	admin := auth.RequireRoleWithSessions(h.cfg.JWTSecret, sv)
	h.route(mux, "GET /admin/stats", "admin", "admin-ops", "Platform totals", admin, h.adminStats)

	h.route(mux, "POST /admin/categories", "admin", "admin-content", "Create a category", admin, h.adminCreateCategory)
	h.route(mux, "GET /admin/categories", "admin", "admin-content", "List all categories including drafts", admin, h.adminListCategories)

	h.route(mux, "POST /admin/confessions", "admin", "admin-content", "Create a confession", admin, h.adminCreateConfession)
	h.route(mux, "GET /admin/confessions", "admin", "admin-content", "List all confessions including drafts", admin, h.adminListConfessions)
	h.route(mux, "GET /admin/confessions/{id}", "admin", "admin-content", "Read a confession with variants", admin, h.adminGetConfession)
	h.route(mux, "PATCH /admin/confessions/{id}", "admin", "admin-content", "Update or publish a confession", admin, h.adminUpdateConfessionStatus)
	h.route(mux, "POST /admin/confessions/{id}/qa", "admin", "admin-content", "Run Audio QA checklist (§75) before APPROVED", admin, h.adminQAConfession)

	h.route(mux, "GET /admin/moderation/queue", "admin", "admin-content", "Moderation queue (UGC + editorial pending)", admin, h.adminListModerationQueue)
	h.route(mux, "POST /admin/moderation/user-confessions/{id}/review", "admin", "admin-content", "Review a user confession (approved|rejected)", admin, h.adminReviewUserConfession)

	h.route(mux, "POST /admin/voices", "admin", "admin-voice", "Create a voice", admin, h.adminCreateVoice)
	h.route(mux, "GET /admin/voices", "admin", "admin-voice", "List voices", admin, h.adminListVoices)

	h.route(mux, "POST /admin/audio", "admin", "admin-audio", "Attach an audio asset to a confession", admin, h.adminUpsertAudio)

	h.route(mux, "POST /admin/users/role", "admin", "admin-users", "Grant an admin role", admin, h.adminSetUserRole)
	h.route(mux, "DELETE /admin/users/role", "admin", "admin-users", "Revoke an admin role", admin, h.adminRemoveUserRole)
	h.route(mux, "GET /admin/users/admins", "admin", "admin-users", "List admin accounts", admin, h.adminListAdmins)
	h.route(mux, "POST /admin/users/status", "admin", "admin-users", "Suspend or restore an account", admin, h.adminSetUserStatus)
	h.route(mux, "POST /admin/users/subscription", "admin", "admin-users", "Set a subscription plan", admin, h.adminSetSubscription)

	// Operational surfaces. Admin-only: failure counts reveal whether an attack
	// is landing, which is what an attacker most wants to know (S83).
	h.route(mux, "GET /admin/metrics", "admin", "admin-ops", "Security event counters", admin, h.metricsHandler)
	h.route(mux, "GET /admin/audit", "admin", "admin-ops", "Recent privileged actions", admin, h.adminAuditTrail)

	// Voice rights and synthesis (PRD S13, S14, S61).
	//
	// Restricted to the voice manager rather than any admin: a mistake here can
	// mean synthesizing a person's voice without permission. SUPER_ADMIN is
	// admitted by RoleBasedMiddleware.
	voiceMgr := auth.RequireRoleWithSessions(h.cfg.JWTSecret, sv, auth.RoleVoiceManager)
	h.route(mux, "GET /admin/voices/{id}/rights", "voice_manager", "admin-voice", "Rights record with a live evaluation", voiceMgr, h.adminGetVoiceRights)
	h.route(mux, "PUT /admin/voices/{id}/rights", "voice_manager", "admin-voice", "Set rights; AI grants require an attestation", voiceMgr, h.adminUpsertVoiceRights)
	h.route(mux, "POST /admin/voices/{id}/rights/revoke", "voice_manager", "admin-voice", "Revoke every use of a voice", voiceMgr, h.adminRevokeVoiceRights)

	// Generation is an audio-team operation but consumes voice rights, so both
	// roles may run it.
	// Immediate erasure is SUPER_ADMIN only: RequireRoleWithSessions admits
	// super admins everywhere, and naming no other role keeps it to them.
	mux.Handle("POST /admin/users/{id}/erase", auth.RequireRoleWithSessions(h.cfg.JWTSecret, sv)(http.HandlerFunc(h.adminEraseUser)))

	audioMgr := auth.RequireRoleWithSessions(h.cfg.JWTSecret, sv, auth.RoleAudioProducer, auth.RoleVoiceManager)
	h.route(mux, "POST /admin/audio/generate", "audio_producer,voice_manager", "admin-audio", "Generate audio; refused 451 when voice rights disallow it", audioMgr, h.adminGenerateAudio)

	// Generation requests and their outcomes. These are what make a generation
	// observable: until now a render was a synchronous call with no record, so
	// there was nothing to poll and nothing to retry.
	h.route(mux, "GET /admin/queue", "admin", "admin-queue", "Background queue: counts by status and the job types this server runs", admin, h.adminQueueStats)
	h.route(mux, "POST /admin/queue/requeue", "admin", "admin-queue", "Release dead-lettered jobs back into the queue", admin, h.adminQueueRequeue)
	h.route(mux, "GET /admin/audio/jobs", "audio_producer,voice_manager", "admin-audio", "List generation requests, newest first", audioMgr, h.adminListAudioJobs)
	h.route(mux, "GET /admin/audio/jobs/{id}", "audio_producer,voice_manager", "admin-audio", "One generation request with its outcome", audioMgr, h.adminGetAudioJob)

	// Asset QA. Generated audio enters 'processing'; nothing could previously
	// move it out, because the codebase had no UPDATE audio_assets at all.
	// Approving is what makes a render servable, and rejecting records who
	// decided and why.
	h.route(mux, "POST /admin/audio/{id}/qa/approve", "audio_producer,voice_manager", "admin-audio", "Approve a render for listeners", audioMgr, h.adminApproveAudio)
	h.route(mux, "POST /admin/audio/{id}/qa/reject", "audio_producer,voice_manager", "admin-audio", "Reject a render; a note is required", audioMgr, h.adminRejectAudio)
	h.route(mux, "POST /admin/audio/{id}/publish", "audio_producer,voice_manager", "admin-audio", "Surface an approved render in discovery", audioMgr, h.adminPublishAudio)
	h.route(mux, "POST /admin/audio/{id}/archive", "audio_producer,voice_manager", "admin-audio", "Withdraw a render, including from existing sessions", audioMgr, h.adminArchiveAudio)

	// ─────────────────────────────────────────────────────────────────
	// Routes that existed only under /v1/. The comment below promises every
	// API route is served under both prefixes; these had drifted, so a client
	// using the documented unprefixed paths got a 404 on the entire playback
	// surface. TestRouteParityBetweenPrefixes keeps the two sets aligned.
	h.route(mux, "POST /sessions/{id}/start", "user", "sessions", "Start a session (DRAFT→ACTIVE)", authed, func(w http.ResponseWriter, r *http.Request) {
		idempotent(http.HandlerFunc(h.startSession)).ServeHTTP(w, r)
	})
	h.route(mux, "POST /sessions/{id}/pause", "user", "sessions", "Pause a session", authed, h.pauseSession)
	h.route(mux, "POST /sessions/{id}/resume", "user", "sessions", "Resume a paused session", authed, h.resumeSession)
	h.route(mux, "POST /sessions/{id}/complete", "user", "sessions", "Complete a session", authed, func(w http.ResponseWriter, r *http.Request) {
		idempotent(http.HandlerFunc(h.completeSession)).ServeHTTP(w, r)
	})
	h.route(mux, "GET /sessions/{id}/queue", "user", "sessions", "Queue for a session", authed, h.getSessionQueue)
	h.route(mux, "POST /sessions/{id}/progress", "user", "sessions", "Sync playback progress", authed, func(w http.ResponseWriter, r *http.Request) {
		idempotent(http.HandlerFunc(h.syncProgress)).ServeHTTP(w, r)
	})
	h.route(mux, "POST /sessions/{id}/skip", "user", "sessions", "Skip queue item", authed, h.skipSessionItem)
	h.route(mux, "PATCH /templates/{id}", "user", "templates", "Update a template", authed, h.updateTemplate)
	h.route(mux, "DELETE /templates/{id}", "user", "templates", "Delete a template", authed, h.deleteTemplate)
	h.route(mux, "POST /templates/{id}/start", "user", "templates", "Start a template — builds a session", authed, func(w http.ResponseWriter, r *http.Request) {
		idempotent(http.HandlerFunc(h.startTemplate)).ServeHTTP(w, r)
	})
	h.route(mux, "GET /subscription", "user", "subscription", "Current plan and entitlements", authed, h.getSubscription)
	h.route(mux, "GET /entitlements", "user", "subscription", "Entitlement flags", authed, h.getEntitlements)
	h.route(mux, "GET /subscriptions/trial", "user", "subscription", "Trial journey (Day1..Day7)", authed, h.getTrial)
	h.route(mux, "POST /subscriptions/verify", "user", "subscription", "Verify store receipt (server-side, billing.Verifier)", authed, func(w http.ResponseWriter, r *http.Request) {
		idempotent(http.HandlerFunc(h.verifySubscriptionV2)).ServeHTTP(w, r)
	})
	h.route(mux, "POST /community/posts", "user", "community", "Create community post (moderated, never auto-publish)", nil, h.createCommunityPost)
	h.route(mux, "GET /community/feed", "public", "community", "Approved community feed", nil, h.feedCommunity)
	h.route(mux, "POST /community/posts/{id}/react", "user", "community", "React amen/heart/pray", nil, h.reactCommunity)
	h.route(mux, "POST /ai/parse", "user", "ai", "AI NLU → categories/duration (never invents theology)", nil, h.aiParse)
	h.route(mux, "POST /analytics/batch", "user", "analytics", "Batch analytics events (no PII)", nil, h.analyticsBatch)
	h.route(mux, "GET /search", "public", "content", "Search confessions, categories, voices, Scripture", nil, h.searchAll)
	h.route(mux, "GET /admin/plans", "admin", "admin-content", "List pricing plans (admin-editable)", admin, h.adminListPlans)
	h.route(mux, "PUT /admin/plans", "admin", "admin-content", "Create or update a pricing plan", admin, h.adminUpsertPlan)
	// Versioned aliases — §46, §110-§112.
	// Every API route is also served under /v1/* for versioned clients.
	// Legacy without prefix is kept for backward compat until minVersion forces upgrade.
	// Idempotency (§47) is enforced on mutating session/progress/subscription/download endpoints.
	// Public auth (v1)
	h.route(mux, "POST /v1/auth/register", "public", "auth", "Create an account and send a verification email", registerLimit, h.register)
	h.route(mux, "POST /v1/auth/login", "public", "auth", "Sign in; returns mfa_required when a second factor is enrolled", loginLimit, h.login)
	h.route(mux, "POST /v1/auth/verify-email", "public", "auth", "Verify an email address with a one-time token", nil, h.verifyEmail)
	h.route(mux, "POST /v1/auth/resend-verification", "public", "auth", "Resend the verification email", emailLimit, h.resendVerification)
	h.route(mux, "POST /v1/auth/request-password-reset", "public", "auth", "Request a password reset link", emailLimit, h.requestPasswordReset)
	h.route(mux, "POST /v1/auth/reset-password", "public", "auth", "Set a new password using a reset token", nil, h.resetPassword)
	h.route(mux, "POST /v1/auth/social/{provider}", "public", "auth", "Exchange a provider identity token for a session", loginLimit, h.socialSignIn)
	// Public content (v1) — 39 categories, backend-controlled taxonomy
	h.route(mux, "GET /v1/collections", "public", "content", "Published collections", nil, h.listCollections)
	h.route(mux, "GET /v1/categories", "public", "content", "Published categories (cached SWR 5m)", nil, h.cachedListCategories)
	h.route(mux, "GET /v1/categories/{id}/confessions", "public", "content", "Confessions in a category (cached SWR 2m)", nil, h.cachedCategoryConfessions)
	h.route(mux, "GET /v1/confessions/{id}", "public", "content", "One confession", nil, h.getConfession)
	h.route(mux, "GET /v1/voices", "public", "content", "Available voices (cached SWR 5m)", nil, h.cachedListVoices)
	// Health versioned
	h.route(mux, "GET /v1/healthz", "public", "ops", "Liveness", nil, h.livez)
	h.route(mux, "GET /v1/health/live", "public", "ops", "Liveness", nil, h.livez)
	h.route(mux, "GET /v1/health/ready", "public", "ops", "Readiness with subsystem detail", nil, h.readyz)
	mux.HandleFunc("GET /v1/openapi.json", h.serveOpenAPI)
	// Authenticated user (v1) — all with server-side session validation
	h.route(mux, "GET /v1/me", "user", "profile", "Account summary", authed, h.me)
	h.route(mux, "GET /v1/me/bootstrap", "user", "profile", "Everything needed to start the app", authed, h.bootstrap)
	h.route(mux, "GET /v1/home", "user", "home", "Home sections (feature-flagged)", authed, h.home)
	h.route(mux, "GET /v1/me/profile", "user", "profile", "Read profile", authed, h.getProfile)
	h.route(mux, "PATCH /v1/me/profile", "user", "profile", "Update profile", authed, h.updateProfile)
	h.route(mux, "POST /v1/me/avatar", "user", "profile", "Upload an avatar; EXIF is stripped", func(n http.Handler) http.Handler { return registerLimit(authed(n)) }, h.uploadAvatar)
	h.route(mux, "DELETE /v1/me/avatar", "user", "profile", "Remove the avatar", authed, h.deleteAvatar)
	h.route(mux, "GET /v1/me/preferences", "user", "profile", "Read preferences", authed, h.getPreferences)
	h.route(mux, "PATCH /v1/me/preferences", "user", "profile", "Update preferences", authed, h.updatePreferences)
	h.route(mux, "GET /v1/me/collections", "user", "library", "List collections", authed, h.listCollections2)
	h.route(mux, "POST /v1/me/collections", "user", "library", "Create a collection (private by default)", authed, h.createCollection)
	h.route(mux, "GET /v1/me/collections/{id}", "user", "library", "Read a collection", authed, h.getCollection)
	h.route(mux, "PATCH /v1/me/collections/{id}", "user", "library", "Update a collection", authed, h.updateCollection)
	h.route(mux, "DELETE /v1/me/collections/{id}", "user", "library", "Delete a collection", authed, h.deleteCollection)
	h.route(mux, "POST /v1/me/collections/{id}/items", "user", "library", "Add a confession to a collection", authed, h.addCollectionItem)
	h.route(mux, "DELETE /v1/me/collections/{id}/items/{confessionId}", "user", "library", "Remove an item", authed, h.removeCollectionItem)
	h.route(mux, "PATCH /v1/me/collections/{id}/reorder", "user", "library", "Reorder items", authed, h.reorderCollection)
	h.route(mux, "POST /v1/me/devices", "user", "devices", "Register a device and its push token", authed, h.registerDevice)
	h.route(mux, "GET /v1/me/devices", "user", "devices", "List devices", authed, h.listDevices)
	h.route(mux, "DELETE /v1/me/devices/{id}", "user", "devices", "Remove a device", authed, h.revokeDevice)
	h.route(mux, "GET /v1/me/notifications", "user", "devices", "Notification preferences", authed, h.getNotificationPreferences)
	h.route(mux, "PATCH /v1/me/notifications", "user", "devices", "Update notification preferences", authed, h.updateNotificationPreferences)
	h.route(mux, "GET /v1/me/deletion", "user", "account", "Preview what deletion removes and retains", authed, h.deletionPreview)
	h.route(mux, "POST /v1/me/deletion", "user", "account", "Schedule account deletion", func(n http.Handler) http.Handler { return registerLimit(authed(n)) }, h.requestDeletion)
	h.route(mux, "DELETE /v1/me/deletion", "user", "account", "Cancel a scheduled deletion", authed, h.cancelDeletion)
	h.route(mux, "GET /v1/me/downloads", "user", "downloads", "List offline licences", authed, h.listDownloads)
	h.route(mux, "POST /v1/me/downloads", "user", "downloads", "Take a confession offline (premium)", func(n http.Handler) http.Handler { return authed(idempotent(n)) }, h.createDownload)
	h.route(mux, "POST /v1/me/downloads/{id}/refresh", "user", "downloads", "Renew an offline licence", authed, h.refreshDownload)
	h.route(mux, "DELETE /v1/me/downloads/{id}", "user", "downloads", "Release an offline licence", authed, h.deleteDownload)
	h.route(mux, "GET /v1/me/export", "user", "account", "Download all personal data", authed, h.exportData)
	h.route(mux, "GET /v1/me/interests", "user", "profile", "Interests, split into explicit and inferred", authed, h.getInterests)
	h.route(mux, "PUT /v1/me/interests", "user", "profile", "Replace explicitly chosen interests", authed, h.putInterests)
	h.route(mux, "POST /v1/auth/change-password", "user", "auth", "Change password and revoke other sessions", authed, h.changePassword)
	h.route(mux, "POST /v1/auth/refresh", "user", "auth", "Rotate the session and issue a new access token", authed, h.refreshToken)
	h.route(mux, "POST /v1/auth/logout", "user", "auth", "End this session", authed, h.logout)
	h.route(mux, "GET /v1/auth/identities", "user", "auth", "List linked sign-in providers", authed, h.listIdentities)
	h.route(mux, "POST /v1/auth/identities/{provider}", "user", "auth", "Link a provider to this account", authed, h.linkIdentity)
	h.route(mux, "DELETE /v1/auth/identities/{provider}", "user", "auth", "Unlink a provider", authed, h.unlinkIdentity)
	h.route(mux, "GET /v1/auth/mfa", "user", "mfa", "Two-factor status", authed, h.mfaStatus)
	h.route(mux, "POST /v1/auth/mfa/begin", "user", "mfa", "Start enrolment; returns a server-generated secret", authed, h.beginMFAEnrolment)
	h.route(mux, "POST /v1/auth/mfa/confirm", "user", "mfa", "Confirm enrolment and receive recovery codes", authed, h.confirmMFAEnrolment)
	h.route(mux, "POST /v1/auth/mfa/disable", "user", "mfa", "Disable two-factor authentication", authed, h.disableMFA)
	h.route(mux, "GET /v1/auth/sessions", "user", "auth", "List live sign-ins", authed, h.listAuthSessions)
	h.route(mux, "DELETE /v1/auth/sessions/{id}", "user", "auth", "Sign out one device", authed, h.revokeAuthSession)
	h.route(mux, "POST /v1/auth/logout-all", "user", "auth", "End every session for this account", authed, h.logoutAll)
	h.route(mux, "POST /v1/auth/consent", "user", "auth", "Record a consent decision", authed, h.recordConsent)
	h.route(mux, "POST /v1/auth/security-events", "user", "auth", "Record a client-side security event", authed, h.securityEvent)
	// Sessions (v1) — idempotent on create/complete/progress
	h.route(mux, "POST /v1/sessions", "user", "sessions", "Compose a session; audio URLs are signed", authed, func(w http.ResponseWriter, r *http.Request) {
		idempotent(http.HandlerFunc(h.createSession)).ServeHTTP(w, r)
	})
	h.route(mux, "POST /v1/sessions/preview", "user", "sessions", "Preview a session (dry-run, no persistence)", authed, h.previewSession)
	h.route(mux, "GET /v1/sessions/{id}", "user", "sessions", "Read a session with freshly signed audio", authed, h.getSession)
	h.route(mux, "PATCH /v1/sessions/{id}", "user", "sessions", "Update session status", authed, h.updateSessionStatus)
	h.route(mux, "GET /v1/sessions", "user", "sessions", "List sessions", authed, h.listMySessions)
	// New §15 statuses: start/pause/resume/complete with state machine enforcement
	h.route(mux, "POST /v1/sessions/{id}/start", "user", "sessions", "Start a session (DRAFT→ACTIVE)", authed, func(w http.ResponseWriter, r *http.Request) {
		idempotent(http.HandlerFunc(h.startSession)).ServeHTTP(w, r)
	})
	h.route(mux, "POST /v1/sessions/{id}/pause", "user", "sessions", "Pause a session", authed, h.pauseSession)
	h.route(mux, "POST /v1/sessions/{id}/resume", "user", "sessions", "Resume a paused session", authed, h.resumeSession)
	h.route(mux, "POST /v1/sessions/{id}/complete", "user", "sessions", "Complete a session", authed, func(w http.ResponseWriter, r *http.Request) {
		idempotent(http.HandlerFunc(h.completeSession)).ServeHTTP(w, r)
	})
	h.route(mux, "GET /v1/sessions/{id}/queue", "user", "sessions", "Queue for a session", authed, h.getSessionQueue)
	h.route(mux, "POST /v1/sessions/{id}/progress", "user", "sessions", "Sync playback progress", authed, func(w http.ResponseWriter, r *http.Request) {
		idempotent(http.HandlerFunc(h.syncProgress)).ServeHTTP(w, r)
	})
	h.route(mux, "POST /v1/sessions/{id}/skip", "user", "sessions", "Skip queue item", authed, h.skipSessionItem)
	h.route(mux, "GET /v1/schedules", "user", "schedules", "List schedules", authed, h.listSchedules)
	h.route(mux, "POST /v1/schedules", "user", "schedules", "Create a schedule", authed, h.createSchedule)
	h.route(mux, "PATCH /v1/schedules/{id}", "user", "schedules", "Update a schedule", authed, h.updateSchedule)
	h.route(mux, "DELETE /v1/schedules/{id}", "user", "schedules", "Delete a schedule", authed, h.deleteSchedule)
	h.route(mux, "POST /v1/schedules/{id}/start", "user", "schedules", "Trigger a schedule — builds a session (deep link target)", authed, func(w http.ResponseWriter, r *http.Request) {
		idempotent(http.HandlerFunc(h.triggerSchedule)).ServeHTTP(w, r)
	})
	// Templates — custom sessions + shareable URLs
	h.route(mux, "POST /v1/templates", "user", "templates", "Create a template", authed, h.createTemplate)
	h.route(mux, "GET /v1/templates", "user", "templates", "List templates", authed, h.listTemplates)
	h.route(mux, "GET /v1/templates/{id}", "user", "templates", "Read a template", authed, h.getTemplate)
	h.route(mux, "PATCH /v1/templates/{id}", "user", "templates", "Update a template", authed, h.updateTemplate)
	h.route(mux, "DELETE /v1/templates/{id}", "user", "templates", "Delete a template", authed, h.deleteTemplate)
	h.route(mux, "POST /v1/templates/{id}/start", "user", "templates", "Start a template — builds a session", authed, func(w http.ResponseWriter, r *http.Request) {
		idempotent(http.HandlerFunc(h.startTemplate)).ServeHTTP(w, r)
	})
	h.route(mux, "GET /v1/t/{token}", "public", "templates", "Resolve a shareable template", nil, h.getTemplateByShareToken)
	h.route(mux, "POST /v1/me/favorites", "user", "library", "Add a favourite", authed, h.addFavorite)
	h.route(mux, "DELETE /v1/me/favorites", "user", "library", "Remove a favourite", authed, h.removeFavorite)
	h.route(mux, "GET /v1/me/favorites", "user", "library", "List favourites", authed, h.listFavorites)
	h.route(mux, "GET /v1/me/history", "user", "library", "Listening history", authed, h.history)
	h.route(mux, "POST /v1/me/history", "user", "library", "Record playback", authed, h.recordPlayback)
	h.route(mux, "POST /v1/me/confessions", "user", "library", "Create a personal confession", authed, h.createUserConfession)
	h.route(mux, "GET /v1/me/confessions", "user", "library", "List personal confessions", authed, h.listUserConfessions)
	h.route(mux, "GET /v1/recommendations", "user", "home", "Personalized recommendations (deterministic v1)", authed, h.recommendations)
	// Subscription & entitlements (v1)
	h.route(mux, "GET /v1/subscription", "user", "subscription", "Current plan and entitlements", authed, h.getSubscription)
	h.route(mux, "GET /v1/entitlements", "user", "subscription", "Entitlement flags", authed, h.getEntitlements)
	h.route(mux, "GET /v1/subscriptions/plans", "public", "subscription", "List plans with regional pricing (NGN/USD/GBP/EUR/PHP)", nil, h.listPlans)
	h.route(mux, "GET /v1/subscriptions/trial", "user", "subscription", "Trial journey (Day1..Day7)", authed, h.getTrial)
	h.route(mux, "POST /v1/subscriptions/verify", "user", "subscription", "Verify store receipt (server-side, billing.Verifier)", authed, func(w http.ResponseWriter, r *http.Request) {
		idempotent(http.HandlerFunc(h.verifySubscriptionV2)).ServeHTTP(w, r)
	})
	// Search (§41)
	h.route(mux, "POST /v1/community/posts", "user", "community", "Create community post (moderated, never auto-publish)", nil, h.createCommunityPost)
	h.route(mux, "GET /v1/community/feed", "public", "community", "Approved community feed", nil, h.feedCommunity)
	h.route(mux, "POST /v1/community/posts/{id}/react", "user", "community", "React amen/heart/pray", nil, h.reactCommunity)
	h.route(mux, "POST /v1/ai/parse", "user", "ai", "AI NLU → categories/duration (never invents theology)", nil, h.aiParse)
	h.route(mux, "POST /v1/analytics/batch", "user", "analytics", "Batch analytics events (no PII)", nil, h.analyticsBatch)
	h.route(mux, "GET /v1/search", "public", "content", "Search confessions, categories, voices, Scripture", nil, h.searchAll)
	// Admin (v1)
	h.route(mux, "GET /v1/admin/stats", "admin", "admin-ops", "Platform totals", admin, h.adminStats)
	h.route(mux, "POST /v1/admin/categories", "admin", "admin-content", "Create a category", admin, h.adminCreateCategory)
	h.route(mux, "GET /v1/admin/categories", "admin", "admin-content", "List all categories including drafts", admin, h.adminListCategories)
	h.route(mux, "POST /v1/admin/confessions", "admin", "admin-content", "Create a confession", admin, h.adminCreateConfession)
	h.route(mux, "GET /v1/admin/confessions", "admin", "admin-content", "List all confessions including drafts", admin, h.adminListConfessions)
	h.route(mux, "GET /v1/admin/confessions/{id}", "admin", "admin-content", "Read a confession with variants", admin, h.adminGetConfession)
	h.route(mux, "PATCH /v1/admin/confessions/{id}", "admin", "admin-content", "Update or publish a confession", admin, h.adminUpdateConfessionStatus)
	h.route(mux, "POST /v1/admin/confessions/{id}/qa", "admin", "admin-content", "Run Audio QA checklist (§75) before APPROVED", admin, h.adminQAConfession)
	h.route(mux, "GET /v1/admin/moderation/queue", "admin", "admin-content", "Moderation queue (UGC + editorial pending)", admin, h.adminListModerationQueue)
	h.route(mux, "POST /v1/admin/moderation/user-confessions/{id}/review", "admin", "admin-content", "Review a user confession (approved|rejected)", admin, h.adminReviewUserConfession)
	h.route(mux, "POST /v1/admin/voices", "admin", "admin-voice", "Create a voice", admin, h.adminCreateVoice)
	h.route(mux, "GET /v1/admin/voices", "admin", "admin-voice", "List voices", admin, h.adminListVoices)
	h.route(mux, "POST /v1/admin/audio", "admin", "admin-audio", "Attach an audio asset to a confession", admin, h.adminUpsertAudio)
	h.route(mux, "POST /v1/admin/users/role", "admin", "admin-users", "Grant an admin role", admin, h.adminSetUserRole)
	h.route(mux, "DELETE /v1/admin/users/role", "admin", "admin-users", "Revoke an admin role", admin, h.adminRemoveUserRole)
	h.route(mux, "GET /v1/admin/users/admins", "admin", "admin-users", "List admin accounts", admin, h.adminListAdmins)
	h.route(mux, "POST /v1/admin/users/status", "admin", "admin-users", "Suspend or restore an account", admin, h.adminSetUserStatus)
	h.route(mux, "GET /subscriptions/plans", "public", "subscription", "List plans", nil, h.listPlans)
	h.route(mux, "POST /v1/admin/users/subscription", "admin", "admin-users", "Set a subscription plan", admin, h.adminSetSubscription)
	h.route(mux, "GET /v1/admin/plans", "admin", "admin-content", "List pricing plans (admin-editable)", admin, h.adminListPlans)
	h.route(mux, "PUT /v1/admin/plans", "admin", "admin-content", "Create or update a pricing plan", admin, h.adminUpsertPlan)
	h.route(mux, "GET /v1/admin/metrics", "admin", "admin-ops", "Security event counters", admin, h.metricsHandler)
	h.route(mux, "GET /v1/admin/audit", "admin", "admin-ops", "Recent privileged actions", admin, h.adminAuditTrail)
	h.route(mux, "GET /v1/admin/voices/{id}/rights", "voice_manager", "admin-voice", "Rights record with a live evaluation", voiceMgr, h.adminGetVoiceRights)
	h.route(mux, "PUT /v1/admin/voices/{id}/rights", "voice_manager", "admin-voice", "Set rights; AI grants require an attestation", voiceMgr, h.adminUpsertVoiceRights)
	h.route(mux, "POST /v1/admin/voices/{id}/rights/revoke", "voice_manager", "admin-voice", "Revoke every use of a voice", voiceMgr, h.adminRevokeVoiceRights)
	h.route(mux, "POST /v1/admin/audio/generate", "audio_producer,voice_manager", "admin-audio", "Generate audio; refused 451 when voice rights disallow it", audioMgr, h.adminGenerateAudio)
	h.route(mux, "GET /v1/admin/queue", "admin", "admin-queue", "Background queue: counts by status and the job types this server runs", admin, h.adminQueueStats)
	h.route(mux, "POST /v1/admin/queue/requeue", "admin", "admin-queue", "Release dead-lettered jobs back into the queue", admin, h.adminQueueRequeue)
	h.route(mux, "GET /v1/admin/audio/jobs", "audio_producer,voice_manager", "admin-audio", "List generation requests, newest first", audioMgr, h.adminListAudioJobs)
	h.route(mux, "GET /v1/admin/audio/jobs/{id}", "audio_producer,voice_manager", "admin-audio", "One generation request with its outcome", audioMgr, h.adminGetAudioJob)
	h.route(mux, "POST /v1/admin/audio/{id}/qa/approve", "audio_producer,voice_manager", "admin-audio", "Approve a render for listeners", audioMgr, h.adminApproveAudio)
	h.route(mux, "POST /v1/admin/audio/{id}/qa/reject", "audio_producer,voice_manager", "admin-audio", "Reject a render; a note is required", audioMgr, h.adminRejectAudio)
	h.route(mux, "POST /v1/admin/audio/{id}/publish", "audio_producer,voice_manager", "admin-audio", "Surface an approved render in discovery", audioMgr, h.adminPublishAudio)
	h.route(mux, "POST /v1/admin/audio/{id}/archive", "audio_producer,voice_manager", "admin-audio", "Withdraw a render, including from existing sessions", audioMgr, h.adminArchiveAudio)

	return RequestIDMiddleware(tracing.Middleware(logRequests(mux)))
}
