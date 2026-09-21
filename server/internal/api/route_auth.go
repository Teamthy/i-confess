package api

import (
	"fmt"
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
)

// Administrative routes declare their module instead of a level:
// "admin:content", "admin:voices", "admin:system". The module and the route's
// method select the roles admitted through auth.RolesWith, so the matrix in
// internal/auth/rbac.go is the only place that says who may do what
// (PHASE 44). A bare "admin" label is refused at registration: before this
// existed it meant "super_admin only" for forty routes and left four roles
// with nothing to open. See auth.AdminLabelPrefix.

// routeAuth returns the middleware that enforces a declared auth level for a
// route answering method.
//
// A nil return means "no protection", which is only ever correct for
// AuthPublic. Callers must not treat nil as an error: Handler.route installs
// the identity wrapper in that case.
//
// Wrappers are cached per (level, access). There are ~320 registrations and a
// few dozen distinct keys; building a fresh closure for each registration would
// be harmless but wasteful, and the cache also guarantees that every route
// declaring the same level shares one enforcement object.
//
// An unparseable admin label panics. Registration happens once at start-up and
// in every API test, so a typo in a module name fails the build rather than
// silently admitting or refusing the wrong roles.
func (h *Handler) routeAuth(level, method string) func(http.Handler) http.Handler {
	key := strings.ToLower(strings.TrimSpace(level))
	if key == "" || key == AuthPublic {
		return nil
	}
	access := auth.AccessFor(method)
	cacheKey := key + "|" + string(access)

	h.authMWmu.Lock()
	defer h.authMWmu.Unlock()

	if h.authMW == nil {
		h.authMW = map[string]func(http.Handler) http.Handler{}
	}
	if mw, ok := h.authMW[cacheKey]; ok {
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
	case auth.IsAdminLabel(key):
		module, ok := auth.ParseAdminLabel(key)
		if !ok {
			panic(fmt.Sprintf("route auth %q: administrative routes must name a module (%s<module>)", level, auth.AdminLabelPrefix))
		}
		// RequireRoleWithSessions always admits RoleSuperAdmin; the matrix
		// supplies everyone else. A module nobody below super_admin holds
		// (roles, system) yields an empty list, which is super_admin only.
		mw = auth.RequireRoleWithSessions(h.cfg.JWTSecret, sv, auth.RolesWith(module, access)...)
	default:
		panic(fmt.Sprintf("route auth %q is not public, user or an admin module", level))
	}

	h.authMW[cacheKey] = mw
	return mw
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
