package api

import (
	"net/http"
	"sort"
	"strings"
	"sync"
)

// Route registration bookkeeping.
//
// The API spec is generated from this table, which is populated by the same
// calls that register the handlers. A hand-written spec drifts the moment
// someone adds an endpoint and forgets to document it; this cannot, because
// there is only one place a route can come from.

// Route describes one registered endpoint.
type Route struct {
	Method string
	Path   string
	// Auth is the protection applied: "public", "user", or an admin role list.
	Auth string
	// Summary is a short human description used in the generated spec.
	Summary string
	// Tag groups related endpoints in the spec.
	Tag string
}

// routeRecorder collects routes as they are registered.
type routeRecorder struct {
	mu     sync.Mutex
	routes []Route
}

func (rr *routeRecorder) add(r Route) {
	rr.mu.Lock()
	rr.routes = append(rr.routes, r)
	rr.mu.Unlock()
}

// Routes returns a sorted copy of the registered route table.
func (rr *routeRecorder) snapshot() []Route {
	rr.mu.Lock()
	defer rr.mu.Unlock()

	out := make([]Route, len(rr.routes))
	copy(out, rr.routes)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Method < out[j].Method
	})
	return out
}

// RouteTable exposes the registered routes, for spec generation and for the
// contract test that keeps the spec honest.
func (h *Handler) RouteTable() []Route {
	if h.routes == nil {
		return nil
	}
	return h.routes.snapshot()
}

// route wires a handler and records it in one step.
//
// Taking both together is the point: a route cannot be served without also
// being described, so the spec cannot silently omit an endpoint.
func (h *Handler) route(mux *http.ServeMux, pattern, auth, tag, summary string,
	wrap func(http.Handler) http.Handler, fn http.HandlerFunc) {

	var handler http.Handler = fn
	if wrap != nil {
		handler = wrap(fn)
	}
	mux.Handle(pattern, handler)

	method, path := splitPattern(pattern)
	h.routes.add(Route{
		Method: method, Path: path, Auth: auth, Tag: tag, Summary: summary,
	})
}

// splitPattern breaks "GET /me/profile" into its method and path.
func splitPattern(pattern string) (method, path string) {
	parts := strings.SplitN(strings.TrimSpace(pattern), " ", 2)
	if len(parts) == 2 {
		return parts[0], strings.TrimSpace(parts[1])
	}
	return "GET", strings.TrimSpace(pattern)
}
