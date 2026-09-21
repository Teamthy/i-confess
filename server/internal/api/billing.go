package api

import (
	"errors"
	"log"
	"net/http"
	"strings"
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
//
// The day is read from the trial record when one exists — a claim, a clock and
// an end — and falls back to the account's age only for a legacy reader with
// no record yet. The fallback keeps PHASE 30's contract while the record
// below makes the journey answerable honestly for the states age cannot
// express: used, expiring, converted.
func (h *Handler) getTrial(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r)
	dayNum := 0
	if t, err := h.trials.Current(r.Context(), userID, time.Now()); err == nil {
		dayNum = t.Day
	} else {
		// Trial start is the user's created_at; fallback to now if not found
		var start time.Time
		if u, err := h.users.ByID(r.Context(), userID); err == nil {
			if t, perr := time.Parse(time.RFC3339, u.CreatedAt); perr == nil {
				start = t
			}
		}
		dayNum = billing.DayFor(start, time.Now())
	}
	day := billing.Day(dayNum)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"journey":     billing.TrialJourney,
		"current_day": dayNum,
		"today":       day,
	})
}

// getTrialLifecycle is the §36 record itself: where the account stands in the
// trial, and from the state, what the client should be showing.
func (h *Handler) getTrialLifecycle(w http.ResponseWriter, r *http.Request) {
	t, err := h.trials.Current(r.Context(), h.userID(r), time.Now())
	if err != nil {
		log.Printf("trial: cannot read lifecycle for %s: %v", h.userID(r), err)
		httpx.WriteError(w, http.StatusInternalServerError, "could not load the trial state")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, t)
}

// startTrial claims the offer: eligible→started, seven-day clock set.
// Everything else is a refusal with a reason a client can act on.
func (h *Handler) startTrial(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r)
	if userID == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	// A premium account does not get a trial countdown: the offer exists to
	// convert, and a paying subscriber is past that question. (Restoring an
	// old purchase lands here too, which is correct — restoring is not
	// re-offering.) The plan test is load-bearing: every account carries an
	// active FREE subscription row, and a guard that only looked at status
	// would deny the trial to everyone.
	if rec, err := h.users.SubscriptionRecord(r.Context(), userID); err == nil && rec != nil &&
		rec.Status == models.SubscriptionActive && rec.Plan == entitlements.PlanPremium {
		httpx.WriteJSON(w, http.StatusConflict, map[string]any{
			"error":  "this account already has an active subscription; there is no trial to start",
			"code":   "TRIAL_UNAVAILABLE",
			"status": string(billing.TrialConverted),
		})
		return
	}
	// Deliberate scope note, recorded in docs/35-TRIAL-LIFECYCLE.md as a
	// condition: STARTING a trial is now a real, once-per-account event, but
	// an ACTIVE trial does not yet flip entitlements to premium — that gate
	// belongs to the store-verification phase (PHASE 36), which rewrites how
	// plan state is decided. Granting it here would decide entitlements twice,
	// in two places, which is the bug pattern this project keeps closing.
	t, err := h.trials.Start(r.Context(), userID, time.Now())
	var trans *billing.TrialTransitionError
	switch {
	case errors.As(err, &trans):
		// The honest 409 G-3 was about: the trial is not a countdown to be
		// re-armed, it is a consumed offer.
		httpx.WriteJSON(w, http.StatusConflict, map[string]any{
			"error":   trans.Error(),
			"code":    "TRIAL_ALREADY_USED",
			"status":  string(trans.From),
			"allowed": billing.TrialAllowedFrom(trans.From),
		})
	case err != nil:
		log.Printf("trial: cannot start for %s: %v", userID, err)
		httpx.WriteError(w, http.StatusInternalServerError, "could not start the trial")
	default:
		httpx.WriteJSON(w, http.StatusOK, t)
	}
}

// patchTrial moves the record along the graph explicitly. Marked legacy:
// reads advance the clock on their own and verification converts, so nothing
// in the app should need this — it exists for the worker, for ops, and for
// tests that want to fast-forward without sleeping. It cannot jump backwards
// or invent an edge: same graph, same CAS.
func (h *Handler) patchTrial(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Status string `json:"status"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	to := billing.TrialStatus(strings.TrimSpace(req.Status))
	if !billing.ValidTrialStatus(to) {
		writeCode(w, http.StatusBadRequest, "TRIAL_INVALID",
			"status must be one of: "+joinTrialStatuses())
		return
	}
	t, err := h.trials.SetStatus(r.Context(), h.userID(r), to, time.Now())
	var trans *billing.TrialTransitionError
	switch {
	case errors.As(err, &trans):
		httpx.WriteJSON(w, http.StatusConflict, map[string]any{
			"error":   trans.Error(),
			"status":  string(trans.From),
			"allowed": billing.TrialAllowedFrom(trans.From),
		})
	case err != nil:
		log.Printf("trial: cannot move to %s for %s: %v", to, h.userID(r), err)
		httpx.WriteError(w, http.StatusInternalServerError, "could not move the trial")
	default:
		httpx.WriteJSON(w, http.StatusOK, t)
	}
}

func joinTrialStatuses() string {
	names := make([]string, 0, 6)
	for _, s := range billing.TrialStatuses() {
		names = append(names, string(s))
	}
	return strings.Join(names, ", ")
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
		var (
			status int
			code   string
		)
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
	if ver.OriginalTransactionID != "" || ver.PurchaseToken != "" {
		owner, found, oerr := h.users.SubscriptionOwnerFor(r.Context(), ver.Provider, ver.OriginalTransactionID, ver.PurchaseToken)
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
		PurchaseToken:         ver.PurchaseToken,
	})
	if err != nil {
		log.Printf("billing: failed to record verified subscription for user %s: %v", userID, err)
		httpx.WriteError(w, http.StatusInternalServerError, "could not record the subscription")
		return
	}

	// The purchase the offer existed to produce: if this account is inside —
	// or just outside — its trial, that trial is now converted. Best effort
	// and logged, never fatal: the subscription is already recorded, and
	// failing the customer's receipt over a trial bookkeeping row would
	// punish the paying party for our own housekeeping.
	if _, cerr := h.trials.Convert(r.Context(), userID, time.Now()); cerr != nil {
		// Two silences are correct here: a graph refusal (no running trial,
		// or none at all) is the ordinary case for direct purchasers, and any
		// other failure is ours, logged but never fatal to the receipt.
		var trans *billing.TrialTransitionError
		if !errors.As(cerr, &trans) && !errors.Is(cerr, store.ErrNoTrial) {
			log.Printf("billing: could not mark trial converted for %s: %v", userID, cerr)
		}
	}

	acknowledged := false
	if ver.NeedsAcknowledgement {
		// Play refunds a purchase that is not acknowledged within three days,
		// so this is not a courtesy: an unacknowledged purchase is money taken
		// and then returned, three days later, with the customer still holding
		// an entitlement. The receipt is honoured either way, because the
		// customer paid - but the acknowledgement is attempted here, and its
		// outcome is reported rather than assumed.
		acknowledged = h.acknowledgePlayPurchase(r.Context(), userID, ver.ProductID, ver.PurchaseToken)
	}

	ent := h.entitlementsFor(r.Context(), userID)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"verified":     true,
		"plan":         plan,
		"state":        status,
		"provider":     ver.Provider,
		"expires_at":   ver.ExpiresAt,
		"entitlements": ent,
		// Only meaningful for a Play purchase that arrived unacknowledged;
		// false for every other provider, and false when Google refused it.
		"acknowledged": acknowledged,
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
