package api

import (
	"net/http"

	"github.com/Teamthy/i-confess/internal/entitlements"
	"github.com/Teamthy/i-confess/internal/httpx"
)

// writePlanLimit answers 402 for a session length the caller's plan does not
// allow.
//
// It exists because this response used to be written out inline in each
// handler that builds a session, and PHASE 13 found the copies had drifted:
// three of the four Build call sites had the check, and the fourth — the
// schedule trigger in home.go — did not, so a free user could obtain a
// three-hour session by saving it as a schedule instead of asking for it
// directly.
//
// One function means one response shape and one place to add a call site.
func writePlanLimit(w http.ResponseWriter, ent entitlements.Entitlements) {
	httpx.WriteJSON(w, http.StatusPaymentRequired, map[string]any{
		"error":       "session length exceeds your plan limit",
		"reason":      "session_duration_exceeds_plan_limit",
		"max_seconds": ent.MaxSessionSeconds(),
		"plan":        ent.Plan,
	})
}
