// Package ratelimit provides abuse controls for authentication endpoints
// (§20, §21, §66, §99).
//
// The design goal is to make credential stuffing expensive without handing an
// attacker a denial-of-service primitive. Locking an account after N failures
// lets anyone lock any user out by guessing badly on purpose, so this package
// throttles rather than locks, and keys on both the caller's address and the
// account so neither dimension alone can starve the other.
package ratelimit

import (
	"net/http"
	"sync"
	"time"

	"github.com/Teamthy/i-confess/internal/httpx"
)

// Enforcer is the limiter contract shared by the in-memory and distributed
// implementations, so callers are unaware which is deployed.
type Enforcer interface {
	// Allow records an event and reports whether it is within the rule,
	// plus how long to wait when it is not.
	Allow(key string, rule Rule) (bool, time.Duration)
	// Reset clears a key, called after a successful login.
	Reset(key string)
}

// Rule is one limit: at most Burst events per Window.
type Rule struct {
	Burst  int
	Window time.Duration
}

// Common rules. Deliberately generous for humans and expensive for scripts.
var (
	// LoginPerIP allows a shared office or NAT to sign several people in.
	LoginPerIP = Rule{Burst: 20, Window: time.Minute}
	// LoginPerAccount is tighter: one person does not legitimately attempt a
	// dozen passwords a minute.
	LoginPerAccount = Rule{Burst: 8, Window: time.Minute}
	// Registration limits automated signup floods.
	Registration = Rule{Burst: 5, Window: 10 * time.Minute}
	// EmailSend covers reset and verification resends, which cost money and
	// can be used to spam a third party's inbox (§17).
	EmailSend = Rule{Burst: 4, Window: 15 * time.Minute}
)

// Limiter is a fixed-window counter.
//
// A fixed window can allow up to 2x burst across a boundary. That is an
// acceptable trade for authentication throttling, where the goal is to make
// sustained guessing costly rather than to police exact rates. Redis with a
// sliding window is the production upgrade; the interface is unchanged.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	now     func() time.Time
}

type bucket struct {
	count   int
	resetAt time.Time
}

// New creates an in-memory limiter and starts no goroutines: expired buckets
// are reclaimed lazily on access, so an idle process costs nothing.
func New() *Limiter {
	return &Limiter{buckets: map[string]*bucket{}, now: time.Now}
}

// SetClock overrides the clock. Test-only.
func (l *Limiter) SetClock(f func() time.Time) { l.now = f }

// Allow records an event and reports whether it is within the rule. The second
// return value is how long to wait when it is not.
func (l *Limiter) Allow(key string, rule Rule) (bool, time.Duration) {
	if rule.Burst <= 0 || rule.Window <= 0 {
		return true, 0
	}
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	// Opportunistic sweep so the map cannot grow without bound under a
	// high-cardinality key attack.
	if len(l.buckets) > 10000 {
		for k, b := range l.buckets {
			if now.After(b.resetAt) {
				delete(l.buckets, k)
			}
		}
	}

	b, ok := l.buckets[key]
	if !ok || now.After(b.resetAt) {
		l.buckets[key] = &bucket{count: 1, resetAt: now.Add(rule.Window)}
		return true, 0
	}
	if b.count >= rule.Burst {
		return false, b.resetAt.Sub(now)
	}
	b.count++
	return true, 0
}

// Reset clears a key, called after a successful login so a user who finally
// remembers their password is not still throttled.
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.buckets, key)
}

// Middleware throttles a route by client address.
func (l *Limiter) Middleware(rule Rule, prefix string, clientIP func(*http.Request) string) func(http.Handler) http.Handler {
	return MiddlewareFor(l, rule, prefix, clientIP)
}

// MiddlewareFor throttles a route using any Enforcer.
func MiddlewareFor(e Enforcer, rule Rule, prefix string, clientIP func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ok, retry := e.Allow(prefix+":"+clientIP(r), rule)
			if !ok {
				TooManyRequests(w, retry)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// TooManyRequests writes a 429 with a Retry-After hint.
func TooManyRequests(w http.ResponseWriter, retry time.Duration) {
	secs := int(retry.Seconds())
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", itoa(secs))
	httpx.WriteJSON(w, http.StatusTooManyRequests, map[string]any{
		"error":       "too many attempts, please try again shortly",
		"code":        "AUTH_RATE_LIMITED",
		"retry_after": secs,
	})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
