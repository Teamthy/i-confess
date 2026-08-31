package api

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/Teamthy/i-confess/internal/auth"
)

// Routes builds the full HTTP handler with all routes registered.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()

	// Health
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Placeholder/dev audio assets (generated locally; replaced by CDN in production).
	mediaPath := os.Getenv("MEDIA_DIR")
	if mediaPath == "" {
		mediaPath = "data/media"
	}
	if abs, err := filepath.Abs(mediaPath); err == nil {
		mux.Handle("GET /media/", http.StripPrefix("/media/", http.FileServer(http.Dir(abs))))
	}

	// Public auth
	mux.HandleFunc("POST /auth/register", h.register)
	mux.HandleFunc("POST /auth/login", h.login)

	// Public content (read-only, published only)
	mux.HandleFunc("GET /collections", h.listCollections)
	mux.HandleFunc("GET /categories", h.listCategories)
	mux.HandleFunc("GET /categories/{id}/confessions", h.categoryConfessions)
	mux.HandleFunc("GET /confessions/{id}", h.getConfession)
	mux.HandleFunc("GET /voices", h.listVoices)

	// Authenticated user routes
	authed := auth.Middleware(h.cfg.JWTSecret)
	mux.Handle("GET /me", authed(http.HandlerFunc(h.me)))

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
	admin := auth.AdminMiddleware(h.cfg.JWTSecret)
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

	mux.Handle("POST /admin/users/role", admin(http.HandlerFunc(h.adminSetRole)))
	mux.Handle("POST /admin/users/subscription", admin(http.HandlerFunc(h.adminSetSubscription)))

	return logRequests(mux)
}
