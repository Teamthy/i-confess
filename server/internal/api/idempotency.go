package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"
)

// idempotencyHeader is how a client asks for a mutation to be replay-safe (§47).
//
// It is opt-in. A client that does not send the header gets ordinary
// at-least-once behaviour, which is correct for reads and for mutations that
// are naturally idempotent; the endpoints that wrap themselves in this
// middleware are the ones where a duplicated request would corrupt state.
const idempotencyHeader = "Idempotency-Key"

// idempotencyTTL is how long a recorded response is replayable. A key that
// lives forever is a table that only grows, and a client retrying a request
// from last week is not a retry — it is a new request.
const idempotencyTTL = 24 * time.Hour

// idempotencyMiddleware replays the stored response for a repeated key instead
// of executing the handler a second time.
//
// This is the difference between "the client retried" and "the user did it
// twice". Starting a session twice, or completing it twice, must not produce
// two sets of side effects or move recorded metrics twice.
//
// The stored key is scoped to the authenticated user. The table's primary key
// is the key alone, so two listeners who both send "abc" would otherwise
// collide and one would be handed the other's response — which is both wrong
// and a data leak. Hashing the pair keeps the primary key unique per user
// without a schema change.
func (h *Handler) idempotencyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(r.Header.Get(idempotencyHeader))
		if key == "" || len(key) > 200 {
			next.ServeHTTP(w, r)
			return
		}

		userID := h.userID(r)
		if userID == "" {
			// Unauthenticated requests have no scope to key on; replaying
			// across users would be worse than not deduplicating at all.
			next.ServeHTTP(w, r)
			return
		}
		storedKey := scopedIdempotencyKey(userID, key)

		if rec, ok := h.idem.Lookup(r.Context(), storedKey, r.Method, r.URL.Path); ok {
			w.Header().Set("Idempotent-Replay", "1")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(rec.Status)
			_, _ = w.Write([]byte(rec.Body))
			return
		}

		rec := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		// Only successful, cacheable outcomes are recorded. Storing a 500
		// would pin a transient failure to the key for a day and stop the
		// client from ever succeeding.
		if rec.status >= 200 && rec.status < 500 {
			h.idem.Store(r.Context(), storedKey, userID, r.Method, r.URL.Path,
				rec.status, rec.body.String(), time.Now().UTC().Add(idempotencyTTL))
		}
	})
}

// scopedIdempotencyKey binds a client-supplied key to one account. The hash
// keeps the value a fixed length regardless of what the client sent, so a
// long key cannot overflow the primary key column.
func scopedIdempotencyKey(userID, key string) string {
	sum := sha256.Sum256([]byte(userID + "\x00" + key))
	return hex.EncodeToString(sum[:])
}

// statusRecorder captures what the wrapped handler wrote so it can be stored
// and replayed. It deliberately does not implement Flush or Hijack: these
// endpoints return small JSON bodies, never streamed ones.
type responseRecorder struct {
	http.ResponseWriter
	status      int
	body        strings.Builder
	wroteHeader bool
}

func (r *responseRecorder) WriteHeader(status int) {
	if !r.wroteHeader {
		r.status = status
		r.wroteHeader = true
		r.ResponseWriter.WriteHeader(status)
	}
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.wroteHeader = true
		r.ResponseWriter.WriteHeader(r.status)
	}
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}
