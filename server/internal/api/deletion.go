package api

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/deletion"
	"github.com/Teamthy/i-confess/internal/httpx"
)

// Account deletion endpoints (§40, §50, §84).
//
// Deletion is the most destructive thing a user can do, so it is gated by
// re-authentication and a grace period, and it always explains what will be
// kept before it happens.

// deletionPreview tells a user what deletion will and will not remove.
//
// Offered before the destructive call because informed consent is the point:
// discovering after the fact that a subscription record was retained is how
// trust is lost, even when the retention was lawful.
func (h *Handler) deletionPreview(w http.ResponseWriter, r *http.Request) {
	status, err := h.deletion.StatusFor(r.Context(), h.userID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load account status")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status": status,
		"will_be_deleted": []string{
			"your profile, photo and preferences",
			"your collections, favourites and personal confessions",
			"your sessions, schedules and listening history",
			"your devices and sign-in methods",
		},
		"will_be_retained":  deletion.RetainedCategories(),
		"grace_period_days": int(deletion.GracePeriod.Hours() / 24),
		"note": "Requesting deletion signs you out on every device immediately. " +
			"You can still sign in with your password during the grace period to cancel. " +
			"After that, deletion is permanent.",
	})
}

// requestDeletion schedules erasure.
//
// Re-authentication is required (§40): a session alone is not enough proof for
// an irreversible action, and it is exactly the situation where a borrowed or
// stolen session does most damage.
func (h *Handler) requestDeletion(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
		Reason   string `json:"reason"`
		// Confirm must be the literal word, so an accidental or replayed
		// request cannot destroy an account.
		Confirm string `json:"confirm"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeCode(w, http.StatusBadRequest, "PROFILE_INVALID", "invalid request body")
		return
	}
	if req.Confirm != "DELETE" {
		writeCode(w, http.StatusBadRequest, "DELETION_NOT_CONFIRMED",
			`send {"confirm":"DELETE"} to confirm this irreversible action`)
		return
	}

	userID := h.userID(r)
	u, err := h.users.ByID(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load account")
		return
	}

	// Re-authenticate. Federated accounts have no password, so possession of a
	// live session is the only proof available to them; requiring a password
	// they never set would lock them out of deleting their own account.
	hasPassword, err := h.users.HasPassword(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load account")
		return
	}
	if hasPassword {
		_, hash, err := h.users.ByEmail(r.Context(), u.Email)
		if err != nil || !auth.CheckPassword(hash, req.Password) {
			writeCode(w, http.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS",
				"your password is required to delete your account")
			return
		}
	}

	status, err := h.deletion.Request(r.Context(), userID, req.Reason)
	switch {
	case errors.Is(err, deletion.ErrAlreadyRequested):
		writeCode(w, http.StatusConflict, "DELETION_ALREADY_SCHEDULED", "deletion is already scheduled")
		return
	case errors.Is(err, deletion.ErrAlreadyDeleted):
		writeCode(w, http.StatusConflict, "DELETION_ALREADY_DONE", "this account is already deleted")
		return
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "failed to schedule deletion")
		return
	}

	// Always notify. If the request was not made by the owner, this email is
	// how they find out in time to cancel.
	h.notifySecurityEvent(u.Email, "Your account is scheduled for deletion",
		"Your account will be permanently deleted on "+status.ErasesAt+
			". If you did not request this, sign in to cancel it.")

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":           status,
		"will_be_retained": deletion.RetainedCategories(),
		"message":          "Your account is scheduled for deletion and has been signed out everywhere.",
	})
}

// cancelDeletion stops a scheduled erasure.
//
// Deliberately reachable with an ordinary session and no password: someone
// racing to save their account from an attacker must not be slowed down, and
// cancelling is not destructive.
func (h *Handler) cancelDeletion(w http.ResponseWriter, r *http.Request) {
	userID := h.userID(r)

	err := h.deletion.Cancel(r.Context(), userID)
	switch {
	case errors.Is(err, deletion.ErrNotRequested):
		writeCode(w, http.StatusConflict, "DELETION_NOT_SCHEDULED", "no deletion is scheduled")
		return
	case errors.Is(err, deletion.ErrAlreadyDeleted):
		writeCode(w, http.StatusGone, "DELETION_ALREADY_DONE", "this account has already been deleted")
		return
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "failed to cancel deletion")
		return
	}

	if u, uerr := h.users.ByID(r.Context(), userID); uerr == nil {
		h.notifySecurityEvent(u.Email, "Account deletion cancelled",
			"Your account will not be deleted. If you did not do this, change your password immediately.")
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "deletion cancelled, your account is active again",
	})
}

// adminEraseUser runs an immediate erasure.
//
// Restricted to SUPER_ADMIN and audited. It exists for regulator-ordered
// erasure and support escalations, where waiting out a grace period is not an
// option.
func (h *Handler) adminEraseUser(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")

	var req struct {
		Confirm string `json:"confirm"`
		Reason  string `json:"reason"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.Confirm != "ERASE" {
		writeCode(w, http.StatusBadRequest, "DELETION_NOT_CONFIRMED",
			`send {"confirm":"ERASE"} to confirm this irreversible action`)
		return
	}

	actor := ""
	if c := auth.FromContext(r); c != nil {
		actor = c.Email
	}

	// Record the request first, so an erasure ordered by an admin is traceable
	// even if the erasure itself then fails.
	if _, err := h.deletion.Request(r.Context(), targetID, "admin erasure by "+actor+": "+req.Reason); err != nil &&
		!errors.Is(err, deletion.ErrAlreadyRequested) {
		if errors.Is(err, deletion.ErrNotFound) {
			writeCode(w, http.StatusNotFound, "RESOURCE_NOT_FOUND", "account not found")
			return
		}
		if errors.Is(err, deletion.ErrAlreadyDeleted) {
			writeCode(w, http.StatusConflict, "DELETION_ALREADY_DONE", "already deleted")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "failed to start erasure")
		return
	}

	report, err := h.deletion.Erase(r.Context(), targetID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "erasure failed")
		return
	}

	log.Printf("audit action=admin_account_erased actor=%s target=%s rows=%d reason=%q",
		actor, targetID, report.RowsDeleted, req.Reason)

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"report": report,
		"note":   "Retained categories are listed in the report and were kept for stated legal reasons.",
	})
}

// RunDeletionSweep erases accounts whose grace period has elapsed.
//
// Exported so main can schedule it. Each account is erased in its own
// transaction, so one failure does not strand the rest of the batch.
func (h *Handler) RunDeletionSweep(ctx context.Context) (int, error) {
	due, err := h.deletion.DueForErasure(ctx, 100)
	if err != nil {
		return 0, err
	}
	erased := 0
	for _, id := range due {
		report, err := h.deletion.Erase(ctx, id)
		if err != nil {
			log.Printf("deletion: sweep failed for %s: %v", id, err)
			continue
		}
		log.Printf("deletion: erased account %s (%d rows)", id, report.RowsDeleted)
		erased++
	}
	return erased, nil
}
