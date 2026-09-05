package api

import (
	"net/http"
	"time"

	"github.com/Teamthy/i-confess/internal/billing"
	"github.com/Teamthy/i-confess/internal/store"
	"github.com/Teamthy/i-confess/internal/httpx"
)

// plans returns the subscription catalog with regional pricing.
// DB (subscription_plans) is source of truth; fallback is billing.DefaultPlans.
// Prices are never hard-coded in mobile; this is the single source.
func (h *Handler) listPlans(w http.ResponseWriter, r *http.Request) {
	plans := billing.DefaultPlans
	if h.plans != nil {
		if dbPlans, err := h.plans.List(r.Context()); err == nil && len(dbPlans) > 0 {
			plans = dbPlans
		}
	}
	currency := r.URL.Query().Get("currency")
	if currency == "" {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"plans":      plans,
			"currencies": billing.Currencies,
		})
		return
	}
	filtered := make([]billing.Plan, len(plans))
	for i, p := range plans {
		filtered[i] = p
		if amt, ok := p.AmountFor(currency); ok {
			filtered[i].Prices = map[string]billing.Amount{amt.Currency: amt}
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"plans":      filtered,
		"currencies": billing.Currencies,
	})
}

// trial returns the 7-day journey and the caller's current day.
func (h *Handler) getTrial(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r)
	// Trial start is the user's created_at; fallback to now if not found
	var start time.Time
	if u, err := h.users.ByID(r.Context(), userID); err == nil {
		if t, perr := time.Parse(time.RFC3339, u.CreatedAt); perr == nil {
			start = t
		}
	}
	dayNum := billing.DayFor(start, time.Now())
	day := billing.Day(dayNum)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"journey":     billing.TrialJourney,
		"current_day": dayNum,
		"today":       day,
	})
}

// verifySubscription now does server-side verification via billing.Verifier.
// The client never asserts a plan; only a valid receipt can grant premium.
// In dev/test NoopVerifier accepts "valid_monthly_..." and "valid_annual_...".
func (h *Handler) verifySubscriptionV2(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
		Receipt  string `json:"receipt"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.Provider == "" || req.Receipt == "" {
		httpx.WriteError(w, http.StatusBadRequest, "provider and receipt are required")
		return
	}
	verifier := billing.VerifierFromEnv()
	ver, err := verifier.Verify(r.Context(), req.Provider, req.Receipt)
	if err != nil {
		httpx.WriteJSON(w, http.StatusBadRequest, map[string]any{
			"error":  err.Error(),
			"code":   "RECEIPT_INVALID",
			"verified": false,
		})
		return
	}
	if !ver.Valid {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"verified": false,
			"detail":   ver.Detail,
		})
		return
	}
	// Grant the plan — server is the authority, not the client payload
	userID := h.userID(r)
	plan := ver.PlanID
	if plan != "monthly" && plan != "annual" {
		plan = "premium"
	}
	// For MVP, map monthly/annual to "premium" plan string used by entitlements
	if plan == "monthly" || plan == "annual" {
		plan = "premium"
	}
	_ = h.users.SetSubscription(r.Context(), userID, plan, "active")
	ent := h.entitlementsFor(r.Context(), userID)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"verified":     true,
		"plan":         plan,
		"provider":     ver.Provider,
		"entitlements": ent,
	})
}

// Admin plans — pricing is admin-editable; mobile never hard-codes.
func (h *Handler) adminListPlans(w http.ResponseWriter, r *http.Request) {
	plans := billing.DefaultPlans
	if h.plans != nil {
		if dbPlans, err := h.plans.List(r.Context()); err == nil && len(dbPlans) > 0 {
			plans = dbPlans
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plans": plans, "currencies": billing.Currencies})
}

func (h *Handler) adminUpsertPlan(w http.ResponseWriter, r *http.Request) {
	var p billing.Plan
	if err := httpx.DecodeJSON(r, &p); err != nil || p.ID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "plan id and body are required")
		return
	}
	if err := p.Validate(); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.plans == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "plans store not configured")
		return
	}
	if err := h.plans.Upsert(r.Context(), p); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to save plan")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"plan": p})
}

