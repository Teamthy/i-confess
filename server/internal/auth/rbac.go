package auth

import (
	"net/http"
	"sort"
	"strings"
)

// Role-based access control: roles → modules → routes (master-plan 40).
//
// The seven roles have existed since the baseline schema, but until PHASE 44
// the router installed one generic gate for almost every admin route —
// RequireRoleWithSessions with no roles, which admits super_admin only — and
// named voice_manager and audio_producer on a handful of routes by hand. Four
// roles (content_admin, theological_reviewer, support_admin, analytics_admin)
// could be granted, appeared in the admin list, and could open nothing at all.
//
// This file is the single authority for who may do what. A route declares the
// module it belongs to ("admin:content") and the method it answers; the
// router asks RolesWith(module, access) for the roles to admit; the admin UI
// asks ModulesFor(role) for the surfaces to show. Both read the same table, so
// the UI cannot show a door the API keeps locked, or hide one it opens.
//
// Access is deliberately two-valued. Read is GET/HEAD; everything else is
// Write, and Write includes Read. Finer distinctions (this role may approve
// but not publish) belong in the handler for that action, where the
// distinction is visible next to the code that makes it — not in a matrix
// that would have to name individual routes.

// Module is one administrative surface. The names are the ones the admin
// command centre uses for its navigation.
type Module string

const (
	// ModuleDashboard is the platform totals every administrator sees.
	ModuleDashboard Module = "dashboard"
	// ModuleContent is the catalogue: categories, confessions, scriptures and
	// the editorial lifecycle.
	ModuleContent Module = "content"
	// ModuleReview is theological review, a distinct step in the lifecycle
	// held by a distinct role (directive §22).
	ModuleReview Module = "review"
	// ModuleModeration is the UGC queue, reports and appeals.
	ModuleModeration Module = "moderation"
	// ModuleVoices is voice records and voice rights — the highest-consequence
	// permission in the product.
	ModuleVoices Module = "voices"
	// ModuleAudio is generation, QA, publication and withdrawal of renders.
	ModuleAudio Module = "audio"
	// ModuleUsers is account support: listing, suspension, restoration.
	ModuleUsers Module = "users"
	// ModuleRoles grants and revokes administrative roles and erases
	// accounts. Nothing below super_admin holds it: a role that can grant
	// roles is super_admin under another name.
	ModuleRoles Module = "roles"
	// ModuleSubscriptions is plans, subscriptions and trials.
	ModuleSubscriptions Module = "subscriptions"
	// ModuleAnalytics is engagement and funnel reporting.
	ModuleAnalytics Module = "analytics"
	// ModuleSystem is the background queue, security counters, the audit log
	// and process metrics. Super_admin only.
	ModuleSystem Module = "system"
)

// Access is what a role may do within a module.
type Access string

const (
	// Read admits GET and HEAD.
	Read Access = "read"
	// Write admits every method, and therefore includes Read.
	Write Access = "write"
)

// modules lists every module in navigation order. Modules() and ValidModule()
// derive from it so there is one place to change.
var modules = []Module{
	ModuleDashboard, ModuleUsers, ModuleContent, ModuleReview, ModuleModeration,
	ModuleVoices, ModuleAudio, ModuleSubscriptions, ModuleAnalytics, ModuleRoles,
	ModuleSystem,
}

// roles lists every administrative role. super_admin is first and is the
// only role the grants table does not name: it holds Write on every module.
var roles = []string{
	RoleSuperAdmin, RoleContentAdmin, RoleTheologicalRev, RoleAudioProducer,
	RoleVoiceManager, RoleSupportAdmin, RoleAnalyticsAdmin,
}

// grants is the matrix. Every role reads the dashboard; beyond that each
// role holds the modules its title names, plus read access to whatever it
// needs to see to do that job (an audio producer reads the text being
// voiced; an analyst reads the catalogue and the plans the numbers describe).
var grants = map[string]map[Module]Access{
	RoleContentAdmin: {
		ModuleDashboard: Read, ModuleContent: Write, ModuleModeration: Write, ModuleReview: Read,
	},
	RoleTheologicalRev: {
		ModuleDashboard: Read, ModuleContent: Read, ModuleReview: Write,
	},
	RoleAudioProducer: {
		ModuleDashboard: Read, ModuleContent: Read, ModuleAudio: Write, ModuleVoices: Read,
	},
	RoleVoiceManager: {
		ModuleDashboard: Read, ModuleVoices: Write, ModuleAudio: Write,
	},
	RoleSupportAdmin: {
		ModuleDashboard: Read, ModuleUsers: Write, ModuleSubscriptions: Read,
	},
	RoleAnalyticsAdmin: {
		ModuleDashboard: Read, ModuleAnalytics: Read, ModuleContent: Read,
		ModuleSubscriptions: Read, ModuleUsers: Read,
	},
}

// Roles returns every administrative role, super_admin first.
func Roles() []string {
	out := make([]string, len(roles))
	copy(out, roles)
	return out
}

// ValidRole reports whether s names an administrative role.
func ValidRole(s string) bool {
	for _, r := range roles {
		if r == s {
			return true
		}
	}
	return false
}

// Modules returns every module in navigation order.
func Modules() []Module {
	out := make([]Module, len(modules))
	copy(out, modules)
	return out
}

// ValidModule reports whether m is a known module.
func ValidModule(m Module) bool {
	for _, known := range modules {
		if known == m {
			return true
		}
	}
	return false
}

// AccessFor maps an HTTP method to the access it needs.
func AccessFor(method string) Access {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead:
		return Read
	}
	return Write
}

// Allowed reports whether role may perform access on module. super_admin is
// allowed everything; an unknown role or module is allowed nothing.
func Allowed(role string, module Module, access Access) bool {
	if role == RoleSuperAdmin {
		return ValidModule(module)
	}
	held, ok := grants[role][module]
	if !ok {
		return false
	}
	return held == Write || access == Read
}

// RolesWith returns the roles, other than super_admin, that may perform
// access on module — the argument list RequireRoleWithSessions wants. The
// result is sorted so the middleware built for a route is the same on every
// start.
func RolesWith(module Module, access Access) []string {
	var out []string
	for role := range grants {
		if Allowed(role, module, access) {
			out = append(out, role)
		}
	}
	sort.Strings(out)
	return out
}

// ModulesFor returns the modules a role holds and at what access, in
// navigation order. It is what the admin UI renders its navigation from.
func ModulesFor(role string) map[Module]Access {
	out := map[Module]Access{}
	for _, m := range modules {
		switch {
		case role == RoleSuperAdmin:
			out[m] = Write
		case Allowed(role, m, Read):
			out[m] = grants[role][m]
		}
	}
	return out
}

// AdminLabelPrefix is how a route declares its module in its auth label:
// "admin:content", "admin:voices". The prefix alone ("admin") is not a
// module and is refused, so a route cannot be registered as merely
// "administrative" without saying which administrators.
const AdminLabelPrefix = "admin:"

// ParseAdminLabel extracts the module from a route's auth label. ok is false
// for labels that are not administrative or name no known module.
func ParseAdminLabel(label string) (Module, bool) {
	label = strings.ToLower(strings.TrimSpace(label))
	if !strings.HasPrefix(label, AdminLabelPrefix) {
		return "", false
	}
	m := Module(strings.TrimPrefix(label, AdminLabelPrefix))
	return m, ValidModule(m)
}

// IsAdminLabel reports whether a route's auth label is administrative at all,
// whether or not it parses. Sweeps use it to find every admin route.
func IsAdminLabel(label string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(label)), "admin")
}
