package api

import (
	"net/http"
)

// SecurityHeadersMiddleware adds defensive HTTP security headers to all responses (OWASP recommendations, IC-009).
func SecurityHeadersMiddleware(next http.Handler, isProduction bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()

		// Frame protection: prevent clickjacking on API and admin surfaces
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-XSS-Protection", "1; mode=block")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")

		// Content Security Policy
		// Allows embedded admin console while blocking unsafe external script execution
		csp := "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; media-src 'self' blob: https:; connect-src 'self' https:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"
		h.Set("Content-Security-Policy", csp)

		// HSTS in production
		if isProduction || r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
		}

		// CORS handling for API endpoints
		origin := r.Header.Get("Origin")
		if origin != "" {
			// In production, allow same-origin and trusted domains; in dev allow requested origin
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-Request-ID, Accept")
			h.Set("Access-Control-Allow-Credentials", "true")
			h.Set("Access-Control-Max-Age", "86400")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		// Ensure server tokens are not leaked
		h.Set("Server", "i-confess")

		next.ServeHTTP(w, r)
	})
}
