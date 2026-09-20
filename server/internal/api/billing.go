package api

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/Teamthy/i-confess/internal/billing"
	"github.com/Teamthy/i-confess/internal/entitlements"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/store"
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

// verifySubscriptionV2 verifies a store receipt and records the entitlement
// (IC-003).
//
// The client never asserts a plan; only a verifier's verdict can grant premium.
// Three things this handler does that its predecessor did not:
//
//  1. It distinguishes a bad receipt (the caller's fault) from an outage or a
//     missing credential (ours). Reporting the second as the first sends a
//     paying customer to support with "invalid receipt" while the real problem
//     is a deployment that never got its service account.
//  2. It persists what the store said — provider, transaction id, expiry,
//     auto-renew — rather than a bare plan and a status string. Without an
//     expiry there is nothing for entitlement to be checked against, and a
//     lapsed subscription stays premium forever.
//  3. It refuses a receipt that is already bound to a different account. A
//     receipt is a bearer token: whoever holds the string can present it, so an
//     unbound purchase can be redeemed by every account that obtains a copy.
//
// In dev/test the no-op verifier accepts "valid_monthly_..." and
// "valid_annual_...", which is how the mobile purchase flow is exercised
// without store credentials.
func (h *Handler) verifySubscriptionV2(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider string `json:"provider"`
		Receipt  string `json:"receipt"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.Provider == "" || req.Receipt == "" {
		httpx.WriteError(w, http.StatusBadRequest, "provider and receipt are required")
		return
	}

	userID := h.userID(r)
	if userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	verifier := billing.VerifierFromEnv()
	ver, err := verifier.Verify(r.Context(), req.Provider, req.Receipt)
	if err != nil {
		status, code := http.StatusBadRequest, "RECEIPT_INVALID"
		switch {
		case errors.Is(err, billing.ErrUnconfigured):
			// Nothing can be verified, so nothing is granted - and the operator
			// is told, in a code the client can surface differently.
			log.Printf("billing: verification is not configured: %v", err)
			status, code = http.StatusServiceUnavailable, "VERIFIER_UNCONFIGURED"
		case errors.Is(err, billing.ErrInvalidReceipt):
			status, code = http.StatusBadRequest, "RECEIPT_INVALID"
		default:
			log.Printf("billing: provider verification failed: %v", err)
			status, code = http.StatusBadGateway, "PROVIDER_UNAVAILABLE"
		}
		httpx.WriteJSON(w, status, map[string]any{
			"error":    err.Error(),
			"code":     code,
			"verified": false,
		})
		return
	}

	// A purchase belongs to one account. This check runs before anything is
	// written, so the unique index behind it is a backstop rather than the
	// thing that reports the conflict.
	if ver.OriginalTransactionID != "" {
		owner, found, oerr := h.users.SubscriptionOwner(r.Context(), ver.Provider, ver.OriginalTransactionID)
		if oerr != nil {
			log.Printf("billing: cannot check receipt ownership: %v", oerr)
			httpx.WriteError(w, http.StatusInternalServerError, "could not record the subscription")
			return
		}
		if found && owner != userID {
			httpx.WriteJSON(w, http.StatusConflict, map[string]any{
				"error":    "this purchase is already linked to another account",
				"code":     "RECEIPT_ALREADY_REDEEMED",
				"verified": false,
			})
			return
		}
	}

	if !ver.Valid {
		// Genuine, but it does not entitle anything: expired, refunded, or
		// suspended. If it is this user's own store subscription, record what
		// the store said so a refund actually revokes access. A receipt for
		// someone else's purchase must not be able to rewrite this account.
		h.recordNonEntitlingVerdict(r, userID, ver)
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"verified":   false,
			"detail":     ver.Detail,
			"code":       "NOT_ENTITLED",
			"state":      ver.State,
			"expires_at": ver.ExpiresAt,
		})
		return
	}

	// The catalogue plan id (monthly/annual) describes what was bought; the
	// entitlement plan is what the product grants. Entitlements resolve from
	// "premium", so that is what is stored, and the product id keeps the
	// distinction that would otherwise be lost.
	plan := entitlements.PlanPremium
	status := ver.State
	if status == "" {
		status = models.SubscriptionActive
	}

	err = h.users.SaveVerifiedSubscription(r.Context(), userID, store.VerifiedSubscription{
		Plan:                  plan,
		Status:                status,
		ExpiresAt:             ver.ExpiresAt,
		Provider:              ver.Provider,
		ProviderTransactionID: ver.TransactionID,
		OriginalTransactionID: ver.OriginalTransactionID,
		ProductID:             ver.ProductID,
		StoreEnvironment:      ver.Environment,
		AutoRenew:             ver.AutoRenew,
	})
	if err != nil {
		log.Printf("billing: failed to record verified subscription for user %s: %v", userID, err)
		httpx.WriteError(w, http.StatusInternalServerError, "could not record the subscription")
		return
	}

	if ver.NeedsAcknowledgement {
		// Play refunds a purchase that is not acknowledged within three days.
		// The receipt is still honoured, because the customer paid; the log
		// line is what tells an operator that acknowledgement is missing.
		log.Printf("billing: user %s has an unacknowledged Play purchase (%s) - it will be refunded in 3 days unless acknowledged",
			userID, ver.ProductID)
	}

	ent := h.entitlementsFor(r.Context(), userID)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"verified":     true,
		"plan":         plan,
		"state":        status,
		"provider":     ver.Provider,
		"expires_at":   ver.ExpiresAt,
		"entitlements": ent,
	})
}

// recordNonEntitlingVerdict downgrades a subscription the store has ended.
//
// Only the user's own store subscription is touched: the row must already carry
// the same provider and original transaction id. That confines the effect to
// the purchase being reported, so presenting an expired receipt cannot disturb
// an unrelated active one, and an administrative grant is never overwritten by
// a store verdict about a different purchase.
func (h *Handler) recordNonEntitlingVerdict(r *http.Request, userID string, ver billing.Verification) {
	if ver.Provider == "" || ver.OriginalTransactionID == "" {
		return
	}
	existing, err := h.users.SubscriptionRecord(r.Context(), userID)
	if err != nil || existing == nil {
		return
	}
	if existing.Provider != ver.Provider || existing.OriginalTransactionID != ver.OriginalTransactionID {
		return
	}
	status := ver.State
	if status == "" {
		status = models.SubscriptionExpired
	}
	err = h.users.SaveVerifiedSubscription(r.Context(), userID, store.VerifiedSubscription{
		Plan:                  existing.Plan,
		Status:                status,
		ExpiresAt:             ver.ExpiresAt,
		Provider:              ver.Provider,
		ProviderTransactionID: ver.TransactionID,
		OriginalTransactionID: ver.OriginalTransactionID,
		ProductID:             ver.ProductID,
		StoreEnvironment:      ver.Environment,
		AutoRenew:             ver.AutoRenew,
	})
	if err != nil {
		log.Printf("billing: failed to record non-entitling verdict for user %s: %v", userID, err)
	}
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
