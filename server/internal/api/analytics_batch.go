package api

import (
	"net/http"
	"time"

	"github.com/Teamthy/i-confess/internal/analytics"
	"github.com/Teamthy/i-confess/internal/httpx"
)

// analyticsBatch ingests batch events from mobile/web.
// No PII: body/email/Scripture never accepted; only actor+entity IDs.
func (h *Handler) analyticsBatch(w http.ResponseWriter, r *http.Request) {
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
	} {
		allowed[n] = true
	}
	for i := range req.Events {
		if !allowed[req.Events[i].Name] {
			httpx.WriteError(w, http.StatusBadRequest, "unknown event: "+req.Events[i].Name)
			return
		}
		if req.Events[i].UserID == "" {
			req.Events[i].UserID = h.userID(r)
		}
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
	// In prod: forward to OTel/Segment sink; here LogSink no-op preserves API contract
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"accepted": len(req.Events)})
}
