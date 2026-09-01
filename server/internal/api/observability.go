package api

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/httpx"
)

// Authentication observability (§50, §83, §84, §85).
//
// Metrics here are counters an operator would actually act on: a spike in
// login failures, refresh-token reuse, or account locks. Latency histograms and
// request-rate graphs belong in the reverse proxy, which already sees every
// request without the application having to instrument itself.

// AuthMetrics counts security-relevant events.
type AuthMetrics struct {
	mu sync.Mutex
	c  map[string]int64
}

// Metric names. Stable strings, because dashboards and alerts reference them
// and renaming one silently breaks a page.
const (
	MetricLoginSuccess      = "login_success_total"
	MetricLoginFailure      = "login_failure_total"
	MetricRegistration      = "registration_total"
	MetricTokenRefresh      = "token_refresh_total"
	MetricTokenReuse        = "token_reuse_detected_total"
	MetricSessionRevoked    = "session_revocation_total"
	MetricRateLimited       = "rate_limited_total"
	MetricMFAChallenge      = "mfa_challenge_total"
	MetricMFAFailure        = "mfa_failure_total"
	MetricPasswordReset     = "password_reset_total"
	MetricAccountDeletion   = "account_deletion_requested_total"
	MetricSocialLogin       = "social_login_total"
	MetricEntitlementDenied = "entitlement_denied_total"
)

// NewAuthMetrics creates a counter set.
func NewAuthMetrics() *AuthMetrics { return &AuthMetrics{c: map[string]int64{}} }

// Inc increments a counter.
func (m *AuthMetrics) Inc(name string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.c[name]++
	m.mu.Unlock()
}

// Snapshot returns a copy of the counters.
func (m *AuthMetrics) Snapshot() map[string]int64 {
	if m == nil {
		return map[string]int64{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]int64, len(m.c))
	for k, v := range m.c {
		out[k] = v
	}
	return out
}

// metrics exposes counters for scraping.
//
// Restricted to admins: the failure counts reveal whether an attack is landing,
// which is exactly what an attacker wants to know.
func (h *Handler) metricsHandler(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"counters": h.metrics.Snapshot(),
		"at":       time.Now().UTC().Format(time.RFC3339),
	})
}

// livez reports that the process is running.
//
// Deliberately checks nothing: a liveness probe that fails on a dependency
// outage makes the orchestrator restart a healthy process, turning a database
// blip into a crash loop (§85).
func (h *Handler) livez(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// readyz reports whether the service can serve traffic.
//
// Checks the database because without it nothing works. Does not check the
// mail or push providers: those degrade gracefully, and failing readiness
// would pull a serving instance out of rotation over a non-essential outage.
func (h *Handler) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.db.PingContext(ctx); err != nil {
		// The error is logged, not returned: a readiness probe response is
		// visible to anyone who can reach the port.
		log.Printf("readyz: database unreachable: %v", err)
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "not ready", "reason": "database",
		})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "ready",
		// Reported so an operator can see degraded subsystems without them
		// affecting whether traffic is served.
		"subsystems": map[string]bool{
			"database": true,
			"email":    h.mail != nil,
			"push":     h.dispatcher != nil,
			"storage":  h.signer != nil,
		},
	})
}

// adminAuditTrail exposes recent privileged actions (§51).
func (h *Handler) adminAuditTrail(w http.ResponseWriter, r *http.Request) {
	entity := r.URL.Query().Get("entity")
	entityID := r.URL.Query().Get("entity_id")

	entries, err := h.audio.AuditTrail(r.Context(), entity, entityID, 100)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load audit trail")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, entries)
}

// recordAudit persists a privileged action.
//
// Best effort by design: the action already succeeded, and failing the response
// because the audit table was busy would be worse than a missing line. The
// failure is logged so it is at least visible.
func (h *Handler) recordAudit(r *http.Request, action, entity, entityID, detail, result string) {
	actor := ""
	if c := auth.FromContext(r); c != nil {
		actor = c.Email
	}
	if err := h.audio.RecordAudit(r.Context(), actor, action, entity, entityID, detail, result); err != nil {
		log.Printf("audit: failed to record %s on %s/%s: %v", action, entity, entityID, err)
	}
}
