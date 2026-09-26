package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/Teamthy/i-confess/internal/analytics"
	"github.com/Teamthy/i-confess/internal/billing"
	"github.com/Teamthy/i-confess/internal/entitlements"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/store"
	trialdomain "github.com/Teamthy/i-confess/internal/trial"
	"github.com/lib/pq"
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

// getTrial returns the deterministic journey and the persisted trial state.
//
// The trial start is never inferred from users.created_at. An account can be
// eligible for months before choosing to try Premium, and inferring a start
// from registration would silently spend the trial before the customer asks
// for it.
func (h *Handler) getTrial(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r)
	current, err := h.trials.Current(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load trial")
		return
	}
	dayNum := trialDay(current, time.Now().UTC())
	// Completed days come from persisted completion rows, so the journey list
	// can mark a day done only if a session was actually finished on it.
	completed, err := h.trials.CompletedDayNumbers(r.Context(), userID)
	if err != nil {
		completed = []int{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"journey":        h.personalizedJourney(r, userID),
		"current_day":    dayNum,
		"today":          billing.Day(dayNum),
		"state":          current.State,
		"trial":          current,
		"completed_days": completed,
	})
}

// startTrial is the only HTTP entry point that can begin a catalogue trial.
// TrialStore performs the state transition and writes the premium/trial
// projection atomically; the handler never writes plan truth itself.
func (h *Handler) startTrial(w http.ResponseWriter, r *http.Request) {
	current, err := h.trials.Start(r.Context(), h.userID(r), time.Now().UTC())
	if err != nil {
		switch {
		case errors.Is(err, store.ErrTrialNotEligible):
			httpx.WriteError(w, http.StatusConflict, "trial is no longer available")
		case errors.Is(err, store.ErrNotFound):
			httpx.WriteError(w, http.StatusNotFound, "account not found")
		default:
			log.Printf("trial: start failed: %v", err)
			httpx.WriteError(w, http.StatusInternalServerError, "failed to start trial")
		}
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"state":       current.State,
		"trial":       current,
		"plan":        entitlements.PlanPremium,
		"current_day": trialDay(current, time.Now().UTC()),
	})
}

// getTrialStatus advances clock-derived states through the same store method
// used by workers. Reads therefore cannot report ACTIVE after its expiry.
func (h *Handler) getTrialStatus(w http.ResponseWriter, r *http.Request) {
	current, err := h.trials.Refresh(r.Context(), h.userID(r), time.Now().UTC())
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "trial has not been created")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load trial status")
		return
	}
	plan, planErr := h.users.Subscription(r.Context(), h.userID(r))
	if planErr != nil {
		plan = entitlements.PlanFree
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"state":       current.State,
		"trial":       current,
		"current_day": trialDay(current, time.Now().UTC()),
		"today":       billing.Day(trialDay(current, time.Now().UTC())),
		"plan":        plan,
		"entitled":    plan == entitlements.PlanPremium,
	})
}

// convertTrial is intentionally not a billing shortcut. It records the trial
// terminal state and removes the temporary projection; a paid subscription is
// created only by POST /subscriptions/verify after a real store verdict.
func (h *Handler) convertTrial(w http.ResponseWriter, r *http.Request) {
	current, err := h.trials.Convert(r.Context(), h.userID(r), time.Now().UTC())
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			httpx.WriteError(w, http.StatusNotFound, "trial has not been created")
		case errors.Is(err, store.ErrTrialTransition):
			httpx.WriteError(w, http.StatusConflict, "trial cannot be converted from its current state")
		default:
			httpx.WriteError(w, http.StatusInternalServerError, "failed to convert trial")
		}
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"state": current.State,
		"trial": current,
		"plan":  entitlements.PlanFree,
	})
}

func trialDay(current *store.Trial, now time.Time) int {
	if current == nil || current.State == trialdomain.Eligible || current.State == trialdomain.Expired || current.State == trialdomain.Converted {
		return 0
	}
	start, startErr := time.Parse(time.RFC3339, current.StartedAt)
	expires, expiresErr := time.Parse(time.RFC3339, current.ExpiresAt)
	if startErr != nil || expiresErr != nil {
		return 0
	}
	return trialdomain.DayFor(start, expires, now)
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
		case errors.Is(err, billing.ErrPaymentsDisabled):
			// Purchases are switched off on purpose (staging without store
			// products). Tell the client that, not that a provider is broken.
			status, code = http.StatusServiceUnavailable, "PAYMENTS_DISABLED"
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

// recordTrialDayCompletion derives a trial journey day from a completed
// session. It is called only from the session completion path, so a day is
// never completed by a client asserting it.
//
// Every outcome is deliberately silent to the caller: a listener who is not on
// a trial, whose trial has expired, or who already completed today's day has
// done nothing wrong, and a completion they earned must not fail because of
// analytics bookkeeping.
func (h *Handler) recordTrialDayCompletion(ctx context.Context, userID, sessionID string) {
	rec, created, err := h.trials.CompleteDay(ctx, userID, sessionID, time.Now().UTC())
	if err != nil || rec == nil || !created {
		return
	}
	_ = h.analytics.Record(ctx, analytics.Event{
		Name:   analytics.EventTrialDayCompleted,
		UserID: userID,
		Props: map[string]any{
			"day":        rec.Day,
			"session_id": sessionID,
			"trial_id":   rec.TrialID,
		},
	})
}

// getTrialEngagement reports the measured journey: which days were actually
// completed, the completion rate, and the funnel events behind it.
//
// Days completed is counted from persisted completion rows, so the number
// cannot be produced by a clock. That is the difference between "the listener
// is on day 5" — which is true of an account that never opened the app — and
// "the listener finished four days", which is the only version that answers
// whether the trial worked.
func (h *Handler) getTrialEngagement(w http.ResponseWriter, r *http.Request) {
	eng, err := h.trials.Engagement(r.Context(), h.userID(r), time.Now().UTC())
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "trial has not been created")
			return
		}
		log.Printf("trial: engagement failed: %v", err)
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load trial engagement")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, eng)
}

// personalizedJourney resolves Day 3 against the listener's own interests.
//
// Day 3 is the personalization day, so shipping it with a hard-coded category
// would make the day's stated purpose false. The stored journey keeps a
// fallback so the day is never empty; when the listener has stated interests,
// their categories replace it. This is the only day that varies per account,
// and the variation is a lookup rather than a model, so the journey stays
// deterministic and testable.
func (h *Handler) personalizedJourney(r *http.Request, userID string) []billing.TrialDay {
	journey := make([]billing.TrialDay, len(billing.TrialJourney))
	copy(journey, billing.TrialJourney)
	for i := range journey {
		if !journey[i].Personalized {
			continue
		}
		if slugs := h.interestSlugs(r.Context(), userID); len(slugs) > 0 {
			journey[i].Categories = slugs
		}
	}
	return journey
}

// interestSlugs returns the listener's interest categories as slugs, strongest
// weight first, capped so a day's session stays buildable.
func (h *Handler) interestSlugs(ctx context.Context, userID string) []string {
	interests, err := h.profiles.Interests(ctx, userID)
	if err != nil || len(interests) == 0 {
		return nil
	}
	ids := make([]string, 0, len(interests))
	for _, i := range interests {
		if i.CategoryID != "" {
			ids = append(ids, i.CategoryID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := h.db.QueryContext(ctx,
		`SELECT slug FROM categories WHERE id = ANY(?) AND deleted_at IS NULL ORDER BY name`, pq.Array(ids))
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil
		}
		out = append(out, slug)
	}
	if len(out) > 4 {
		out = out[:4]
	}
	return out
}
