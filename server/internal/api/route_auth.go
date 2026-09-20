package api

import (
	"net/http"
	"strings"

	"github.com/Teamthy/i-confess/internal/auth"
)

// Declared protection levels.
//
// These are the only values the `auth` argument of Handler.route accepts. The
// value is not documentation: it selects the middleware that is actually
// installed. Before this existed the two were separate arguments, so a route
// could claim "user" in the generated OpenAPI spec, in design/routes.json and
// in the IA contract test while being registered with no authentication at
// all. Eight routes were in exactly that state (see audit finding IC-004):
// POST /community/posts, POST /community/posts/{id}/react, POST /ai/parse and
// POST /analytics/batch, each with a /v1/ twin.
const (
	// AuthPublic requires no credential. Rate limiting still applies where the
	// route declares it.
	AuthPublic = "public"
	// AuthUser requires a valid access token whose session is still honoured.
	AuthUser = "user"
	// AuthAdmin requires the super-admin role.
	AuthAdmin = "admin"
)

// routeAuth returns the middleware that enforces a declared auth level.
//
// A nil return means "no protection", which is only ever correct for
// AuthPublic. Callers must not treat nil as an error: Handler.route installs
// the identity wrapper in that case.
//
// Wrappers are cached per level. There are ~290 registrations and a handful of
// distinct levels; building a fresh closure for each registration would be
// harmless but wasteful, and the cache also guarantees that every route
// declaring the same level shares one enforcement object.
func (h *Handler) routeAuth(level string) func(http.Handler) http.Handler {
	key := strings.ToLower(strings.TrimSpace(level))
	if key == "" || key == AuthPublic {
		return nil
	}

	h.authMWmu.Lock()
	defer h.authMWmu.Unlock()

	if h.authMW == nil {
		h.authMW = map[string]func(http.Handler) http.Handler{}
	}
	if mw, ok := h.authMW[key]; ok {
		return mw
	}

	// The session validator is the enforcement point for "the backend is the
	// ultimate authority": logout, suspension and demotion take effect on the
	// next request rather than when the token expires (PRD S3, S54).
	sv := sessionValidator{h: h}

	var mw func(http.Handler) http.Handler
	switch {
	case key == AuthUser:
		mw = auth.MiddlewareWithSessions(h.cfg.JWTSecret, sv)
	default:
		// Anything that is not "public" or "user" is a role list. "admin"
		// deliberately names no additional role, which admits super admins
		// only; RequireRoleWithSessions always admits RoleSuperAdmin.
		mw = auth.RequireRoleWithSessions(h.cfg.JWTSecret, sv, rolesFor(key)...)
	}

	h.authMW[key] = mw
	return mw
}

// rolesFor expands a declared level into the roles admitted besides
// super_admin. "admin" expands to nothing on purpose: the platform's ordinary
// administrative surface is super-admin only, and the narrower operational
// roles are named explicitly on the routes that need them
// ("voice_manager", "audio_producer,voice_manager").
func rolesFor(level string) []string {
	if level == AuthAdmin {
		return nil
	}
	parts := strings.Split(level, ",")
	roles := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" && p != auth.RoleSuperAdmin {
			roles = append(roles, p)
		}
	}
	return roles
}

// AuthLevels returns every distinct protection level in the route table,
// sorted. Used by the authorisation sweep so a level that appears nowhere is
// reported rather than silently untested.
func (h *Handler) AuthLevels() []string {
	routes := h.RouteTable()
	seen := map[string]bool{}
	var out []string
	for _, r := range routes {
		key := strings.ToLower(strings.TrimSpace(r.Auth))
		if key == "" {
			key = AuthPublic
		}
		if !seen[key] {
			seen[key] = true
			out = append(out, key)
		}
	}
	return out
}
