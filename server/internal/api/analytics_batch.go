package api

import (
	"net/http"
	"time"

	"github.com/Teamthy/i-confess/internal/analytics"
	"github.com/Teamthy/i-confess/internal/httpx"
)

// analyticsBatch ingests batch events from mobile/web.
// Authenticated only: UserID is strictly stamped from the validated session claims.
// No PII: body/email/Scripture never accepted; only actor+entity IDs.
func (h *Handler) analyticsBatch(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r)
	if userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Events []analytics.Event `json:"events"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || len(req.Events) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "events required")
		return
	}
	if len(req.Events) > 100 {
		httpx.WriteError(w, http.StatusBadRequest, "max 100 events per batch")
		return
	}
	// Validate allowlist and stamp timestamp
	allowed := map[string]bool{}
	for _, n := range []string{
		analytics.EventAppOpened, analytics.EventSessionCreated, analytics.EventSessionStarted,
		analytics.EventSessionCompleted, analytics.EventScheduleCreated, analytics.EventDownloadCreated,
		analytics.EventTrialDayViewed, analytics.EventSubscriptionVerified, analytics.EventSubscriptionCancelled,
		analytics.EventSearchPerformed, analytics.EventCategoryViewed, analytics.EventTemplateCreated,
		analytics.EventPlaybackProgressSynced,
		// Client-observed funnel events. Day *completion* is deliberately not
		// here: it is written by the server from a real session completion, and
		// accepting it from a client would make the conversion rate a number
		// the app could set.
		analytics.EventTrialStarted, analytics.EventTrialConverted, analytics.EventTrialExpired,
	} {
		allowed[n] = true
	}
	for i := range req.Events {
		if !allowed[req.Events[i].Name] {
			httpx.WriteError(w, http.StatusBadRequest, "unknown event: "+req.Events[i].Name)
			return
		}
		// Always stamp authenticated caller's user id
		req.Events[i].UserID = userID
		if req.Events[i].Timestamp == "" {
			req.Events[i].Timestamp = time.Now().UTC().Format(time.RFC3339)
		}
		// strip any unexpected PII props keys
		if req.Events[i].Props != nil {
			delete(req.Events[i].Props, "body")
			delete(req.Events[i].Props, "email")
			delete(req.Events[i].Props, "scripture_text")
		}
	}
	// Persist. The endpoint used to answer 202 and hand the batch to a no-op
	// LogSink, so a client was told its event was accepted and the server then
	// had no record of it. An event that cannot be queried afterwards is not
	// tracking; it is a receipt for data the system does not hold. A store
	// failure is reported rather than acknowledged, because a silent drop is
	// exactly the defect this replaces. In prod this store is additionally
	// forwarded to an OTel/Segment sink behind the same writer.
	accepted := 0
	for _, ev := range req.Events {
		if err := h.analytics.Record(r.Context(), ev); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "failed to record events")
			return
		}
		accepted++
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"accepted": accepted})
}
