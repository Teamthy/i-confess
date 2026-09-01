package api

import (
	"net/http"

	"github.com/Teamthy/i-confess/internal/adminui"
	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/ratelimit"
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

	// Health
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

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
	mux.Handle("POST /auth/register", registerLimit(http.HandlerFunc(h.register)))
	mux.Handle("POST /auth/login", loginLimit(http.HandlerFunc(h.login)))
	mux.HandleFunc("POST /auth/verify-email", h.verifyEmail)
	mux.Handle("POST /auth/resend-verification", emailLimit(http.HandlerFunc(h.resendVerification)))
	mux.Handle("POST /auth/request-password-reset", emailLimit(http.HandlerFunc(h.requestPasswordReset)))
	mux.HandleFunc("POST /auth/reset-password", h.resetPassword)

	// Public content (read-only, published only)
	mux.HandleFunc("GET /collections", h.listCollections)
	mux.HandleFunc("GET /categories", h.listCategories)
	mux.HandleFunc("GET /categories/{id}/confessions", h.categoryConfessions)
	mux.HandleFunc("GET /confessions/{id}", h.getConfession)
	mux.HandleFunc("GET /voices", h.listVoices)

	// Authenticated user routes
	// Every authenticated route validates the session server-side, so logout,
	// suspension and password changes take effect immediately rather than when
	// the token happens to expire (PRD S3, S54).
	sv := sessionValidator{h: h}

	authed := auth.MiddlewareWithSessions(h.cfg.JWTSecret, sv)
	mux.Handle("GET /me", authed(http.HandlerFunc(h.me)))

	// Profile domain (PRD S51, S56). Separate resources rather than one giant
	// document, plus a bootstrap aggregate for cold start.
	mux.Handle("GET /me/bootstrap", authed(http.HandlerFunc(h.bootstrap)))
	mux.Handle("GET /me/profile", authed(http.HandlerFunc(h.getProfile)))
	mux.Handle("PATCH /me/profile", authed(http.HandlerFunc(h.updateProfile)))
	// Avatar upload is throttled: image processing is the most CPU-expensive
	// thing an authenticated user can trigger.
	mux.Handle("POST /me/avatar", registerLimit(authed(http.HandlerFunc(h.uploadAvatar))))
	mux.Handle("DELETE /me/avatar", authed(http.HandlerFunc(h.deleteAvatar)))
	mux.Handle("GET /me/preferences", authed(http.HandlerFunc(h.getPreferences)))
	mux.Handle("PATCH /me/preferences", authed(http.HandlerFunc(h.updatePreferences)))
	// User collections (PRD S53).
	mux.Handle("GET /me/collections", authed(http.HandlerFunc(h.listCollections2)))
	mux.Handle("POST /me/collections", authed(http.HandlerFunc(h.createCollection)))
	mux.Handle("GET /me/collections/{id}", authed(http.HandlerFunc(h.getCollection)))
	mux.Handle("PATCH /me/collections/{id}", authed(http.HandlerFunc(h.updateCollection)))
	mux.Handle("DELETE /me/collections/{id}", authed(http.HandlerFunc(h.deleteCollection)))
	mux.Handle("POST /me/collections/{id}/items", authed(http.HandlerFunc(h.addCollectionItem)))
	mux.Handle("DELETE /me/collections/{id}/items/{confessionId}", authed(http.HandlerFunc(h.removeCollectionItem)))
	mux.Handle("PATCH /me/collections/{id}/reorder", authed(http.HandlerFunc(h.reorderCollection)))

	// Devices and notifications (PRD S45, S46).
	mux.Handle("POST /me/devices", authed(http.HandlerFunc(h.registerDevice)))
	mux.Handle("GET /me/devices", authed(http.HandlerFunc(h.listDevices)))
	mux.Handle("DELETE /me/devices/{id}", authed(http.HandlerFunc(h.revokeDevice)))
	mux.Handle("GET /me/notifications", authed(http.HandlerFunc(h.getNotificationPreferences)))
	mux.Handle("PATCH /me/notifications", authed(http.HandlerFunc(h.updateNotificationPreferences)))

	// Account deletion (PRD S40, S50). Requesting is throttled and requires
	// re-authentication; cancelling deliberately is not, so a user racing to
	// save their account from an attacker is not slowed down.
	mux.Handle("GET /me/deletion", authed(http.HandlerFunc(h.deletionPreview)))
	mux.Handle("POST /me/deletion", registerLimit(authed(http.HandlerFunc(h.requestDeletion))))
	mux.Handle("DELETE /me/deletion", authed(http.HandlerFunc(h.cancelDeletion)))

	// Data export (PRD S49).
	mux.Handle("GET /me/export", authed(http.HandlerFunc(h.exportData)))

	mux.Handle("GET /me/interests", authed(http.HandlerFunc(h.getInterests)))
	mux.Handle("PUT /me/interests", authed(http.HandlerFunc(h.putInterests)))
	mux.Handle("POST /auth/change-password", authed(http.HandlerFunc(h.changePassword)))
	mux.Handle("POST /auth/refresh", authed(http.HandlerFunc(h.refreshToken)))
	mux.Handle("POST /auth/logout", authed(http.HandlerFunc(h.logout)))
	// Social sign-in (PRD S36, S37). Throttled like password login: an identity
	// token is a credential and the endpoint is equally attackable.
	mux.Handle("POST /auth/social/{provider}", loginLimit(http.HandlerFunc(h.socialSignIn)))

	// Linking requires an authenticated session, which is what proves the user
	// owns the account a new credential is being attached to (S38).
	mux.Handle("GET /auth/identities", authed(http.HandlerFunc(h.listIdentities)))
	mux.Handle("POST /auth/identities/{provider}", authed(http.HandlerFunc(h.linkIdentity)))
	mux.Handle("DELETE /auth/identities/{provider}", authed(http.HandlerFunc(h.unlinkIdentity)))

	// Two-factor authentication (PRD S41, S81).
	mux.Handle("GET /auth/mfa", authed(http.HandlerFunc(h.mfaStatus)))
	mux.Handle("POST /auth/mfa/begin", authed(http.HandlerFunc(h.beginMFAEnrolment)))
	mux.Handle("POST /auth/mfa/confirm", authed(http.HandlerFunc(h.confirmMFAEnrolment)))
	mux.Handle("POST /auth/mfa/disable", authed(http.HandlerFunc(h.disableMFA)))

	mux.Handle("GET /auth/sessions", authed(http.HandlerFunc(h.listAuthSessions)))
	mux.Handle("DELETE /auth/sessions/{id}", authed(http.HandlerFunc(h.revokeAuthSession)))
	mux.Handle("POST /auth/logout-all", authed(http.HandlerFunc(h.logoutAll)))
	mux.Handle("POST /auth/consent", authed(http.HandlerFunc(h.recordConsent)))
	mux.Handle("POST /auth/security-events", authed(http.HandlerFunc(h.securityEvent)))

	mux.Handle("POST /sessions", authed(http.HandlerFunc(h.createSession)))
	mux.Handle("GET /sessions/{id}", authed(http.HandlerFunc(h.getSession)))
	mux.Handle("PATCH /sessions/{id}", authed(http.HandlerFunc(h.updateSessionStatus)))
	mux.Handle("GET /sessions", authed(http.HandlerFunc(h.listMySessions)))

	mux.Handle("GET /schedules", authed(http.HandlerFunc(h.listSchedules)))
	mux.Handle("POST /schedules", authed(http.HandlerFunc(h.createSchedule)))
	mux.Handle("PATCH /schedules/{id}", authed(http.HandlerFunc(h.updateSchedule)))
	mux.Handle("DELETE /schedules/{id}", authed(http.HandlerFunc(h.deleteSchedule)))

	mux.Handle("POST /me/favorites", authed(http.HandlerFunc(h.addFavorite)))
	mux.Handle("DELETE /me/favorites", authed(http.HandlerFunc(h.removeFavorite)))
	mux.Handle("GET /me/favorites", authed(http.HandlerFunc(h.listFavorites)))

	mux.Handle("GET /me/history", authed(http.HandlerFunc(h.history)))
	mux.Handle("POST /me/history", authed(http.HandlerFunc(h.recordPlayback)))

	mux.Handle("POST /me/confessions", authed(http.HandlerFunc(h.createUserConfession)))
	mux.Handle("GET /me/confessions", authed(http.HandlerFunc(h.listUserConfessions)))

	// Admin routes
	admin := auth.RequireRoleWithSessions(h.cfg.JWTSecret, sv)
	mux.Handle("GET /admin/stats", admin(http.HandlerFunc(h.adminStats)))

	mux.Handle("POST /admin/categories", admin(http.HandlerFunc(h.adminCreateCategory)))
	mux.Handle("GET /admin/categories", admin(http.HandlerFunc(h.adminListCategories)))

	mux.Handle("POST /admin/confessions", admin(http.HandlerFunc(h.adminCreateConfession)))
	mux.Handle("GET /admin/confessions", admin(http.HandlerFunc(h.adminListConfessions)))
	mux.Handle("GET /admin/confessions/{id}", admin(http.HandlerFunc(h.adminGetConfession)))
	mux.Handle("PATCH /admin/confessions/{id}", admin(http.HandlerFunc(h.adminUpdateConfessionStatus)))

	mux.Handle("POST /admin/voices", admin(http.HandlerFunc(h.adminCreateVoice)))
	mux.Handle("GET /admin/voices", admin(http.HandlerFunc(h.adminListVoices)))

	mux.Handle("POST /admin/audio", admin(http.HandlerFunc(h.adminUpsertAudio)))

	mux.Handle("POST /admin/users/role", admin(http.HandlerFunc(h.adminSetUserRole)))
	mux.Handle("DELETE /admin/users/role", admin(http.HandlerFunc(h.adminRemoveUserRole)))
	mux.Handle("GET /admin/users/admins", admin(http.HandlerFunc(h.adminListAdmins)))
	mux.Handle("POST /admin/users/status", admin(http.HandlerFunc(h.adminSetUserStatus)))
	mux.Handle("POST /admin/users/subscription", admin(http.HandlerFunc(h.adminSetSubscription)))

	// Voice rights and synthesis (PRD S13, S14, S61).
	//
	// Restricted to the voice manager rather than any admin: a mistake here can
	// mean synthesizing a person's voice without permission. SUPER_ADMIN is
	// admitted by RoleBasedMiddleware.
	voiceMgr := auth.RequireRoleWithSessions(h.cfg.JWTSecret, sv, auth.RoleVoiceManager)
	mux.Handle("GET /admin/voices/{id}/rights", voiceMgr(http.HandlerFunc(h.adminGetVoiceRights)))
	mux.Handle("PUT /admin/voices/{id}/rights", voiceMgr(http.HandlerFunc(h.adminUpsertVoiceRights)))
	mux.Handle("POST /admin/voices/{id}/rights/revoke", voiceMgr(http.HandlerFunc(h.adminRevokeVoiceRights)))

	// Generation is an audio-team operation but consumes voice rights, so both
	// roles may run it.
	// Immediate erasure is SUPER_ADMIN only: RequireRoleWithSessions admits
	// super admins everywhere, and naming no other role keeps it to them.
	mux.Handle("POST /admin/users/{id}/erase", auth.RequireRoleWithSessions(h.cfg.JWTSecret, sv)(http.HandlerFunc(h.adminEraseUser)))

	audioMgr := auth.RequireRoleWithSessions(h.cfg.JWTSecret, sv, auth.RoleAudioProducer, auth.RoleVoiceManager)
	mux.Handle("POST /admin/audio/generate", audioMgr(http.HandlerFunc(h.adminGenerateAudio)))

	return logRequests(mux)
}
