package api

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/sessions"
)

// errBadCursor marks a cursor that decoded but does not hold a position.
var errBadCursor = errors.New("malformed cursor")

// deleteSession removes a session from the listener's history.
//
// It is a soft delete (§25). The row stays, because it is the record of what
// someone listened to and streaks and completion metrics are derived from it; a
// deletion a listener can perform must not silently edit those numbers. What
// changes is that history stops returning it, and it can no longer be resumed.
//
// A session that is still live is cancelled first, and through the state machine
// rather than by writing a status. Deleting an ACTIVE session without cancelling
// it would leave a player somewhere holding a session it believes is playing and
// nothing that can tell it otherwise.
func (h *Handler) deleteSession(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.ownSession(w, r)
	if !ok {
		return
	}

	if from, ok := sessionState(sess); ok && !sessions.IsTerminal(from) {
		if !h.transition(w, r, sess, sessions.Cancelled) {
			return
		}
	}

	deleted, err := h.sess.SoftDelete(r.Context(), sess.ID, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to delete session")
		return
	}
	if !deleted {
		// Between the read and the write somebody - or a second tap on the same
		// button - already deleted it. Say so rather than confirming twice.
		httpx.WriteError(w, http.StatusNotFound, "session not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listMySessions returns one page of the caller's history.
//
// The list used to be capped at a hardcoded 50 with no way past it, which for a
// daily listener is a few weeks of history and then silence. Paging is by
// keyset rather than offset, so a session created while the listener scrolls
// cannot shift the pages and repeat one they have already seen.
func (h *Handler) listMySessions(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			httpx.WriteError(w, http.StatusBadRequest, "limit must be between 1 and 100")
			return
		}
		limit = n
	}

	cursorCreatedAt, cursorID, err := decodeSessionCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "cursor is not valid")
		return
	}

	page, err := h.sess.ListByUserPage(r.Context(), h.userID(r), limit, cursorCreatedAt, cursorID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load sessions")
		return
	}
	if page == nil {
		page = []models.Session{}
	}

	// A cursor is offered only when the page was full. A short page means the
	// end of the history, and handing back a cursor that returns nothing makes
	// a client page once more for no reason.
	next := ""
	if len(page) == limit {
		last := page[len(page)-1]
		next = encodeSessionCursor(last.CreatedAt, last.ID)
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"sessions":    page,
		"next_cursor": next,
		"limit":       limit,
	})
}

// The cursor is opaque to clients on purpose: it encodes the position, not a
// page number, so the encoding can change without breaking anyone who stored one.
func encodeSessionCursor(createdAt, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(createdAt + "|" + id))
}

func decodeSessionCursor(raw string) (createdAt, id string, err error) {
	if raw == "" {
		return "", "", nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errBadCursor
	}
	return parts[0], parts[1], nil
}
