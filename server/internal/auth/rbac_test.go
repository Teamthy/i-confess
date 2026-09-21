package auth

import (
	"net/http"
	"testing"
)

// TestEveryRoleOpensAtLeastOneModule is the reason PHASE 44 exists. Before it,
// four of the seven roles could be granted and opened nothing.
func TestEveryRoleOpensAtLeastOneModule(t *testing.T) {
	for _, role := range Roles() {
		held := ModulesFor(role)
		if len(held) == 0 {
			t.Errorf("role %s holds no module; a role that opens nothing is not a role", role)
		}
		if held[ModuleDashboard] == "" {
			t.Errorf("role %s cannot read the dashboard; every administrator sees the totals", role)
		}
	}
}

// TestSuperAdminHoldsEverythingAndIsNeverListed: super_admin is admitted by
// the middleware unconditionally, so it must not appear in RolesWith (the
// list the middleware is handed) and must hold Write everywhere.
func TestSuperAdminHoldsEverythingAndIsNeverListed(t *testing.T) {
	for _, m := range Modules() {
		if !Allowed(RoleSuperAdmin, m, Write) {
			t.Errorf("super_admin refused Write on %s", m)
		}
		for _, r := range RolesWith(m, Read) {
			if r == RoleSuperAdmin {
				t.Errorf("RolesWith(%s) lists super_admin; the middleware already admits it", m)
			}
		}
	}
	if got := ModulesFor(RoleSuperAdmin); len(got) != len(Modules()) {
		t.Errorf("super_admin holds %d modules, want all %d", len(got), len(Modules()))
	}
}

// TestRolesAndSystemAreSuperAdminOnly: nothing below super_admin may grant
// roles, erase accounts or touch the process.
func TestRolesAndSystemAreSuperAdminOnly(t *testing.T) {
	for _, m := range []Module{ModuleRoles, ModuleSystem} {
		if got := RolesWith(m, Read); len(got) != 0 {
			t.Errorf("%s is held by %v; it must be super_admin only", m, got)
		}
	}
}

// TestWriteIncludesReadAndReadDoesNotIncludeWrite pins the two-valued model.
func TestWriteIncludesReadAndReadDoesNotIncludeWrite(t *testing.T) {
	if !Allowed(RoleContentAdmin, ModuleContent, Read) || !Allowed(RoleContentAdmin, ModuleContent, Write) {
		t.Error("content_admin holds Write on content, which must include Read")
	}
	if !Allowed(RoleTheologicalRev, ModuleContent, Read) {
		t.Error("theological_reviewer must be able to read the content it reviews")
	}
	if Allowed(RoleTheologicalRev, ModuleContent, Write) {
		t.Error("theological_reviewer holds Read on content and must not be able to write it")
	}
	if Allowed(RoleContentAdmin, ModuleReview, Write) {
		t.Error("content_admin must not be able to record a theological review; that is the reviewer's step")
	}
	if !Allowed(RoleTheologicalRev, ModuleReview, Write) {
		t.Error("theological_reviewer must hold the review module")
	}
}

// TestTheMatrixMatchesTheTitles: each role's headline module is its own.
func TestTheMatrixMatchesTheTitles(t *testing.T) {
	cases := map[string]Module{
		RoleContentAdmin:   ModuleContent,
		RoleTheologicalRev: ModuleReview,
		RoleAudioProducer:  ModuleAudio,
		RoleVoiceManager:   ModuleVoices,
		RoleSupportAdmin:   ModuleUsers,
	}
	for role, m := range cases {
		if !Allowed(role, m, Write) {
			t.Errorf("%s must hold Write on %s", role, m)
		}
	}
	if !Allowed(RoleAnalyticsAdmin, ModuleAnalytics, Read) {
		t.Error("analytics_admin must read analytics")
	}
	if Allowed(RoleAnalyticsAdmin, ModuleUsers, Write) {
		t.Error("analytics_admin reads users and must not suspend them")
	}
	if Allowed(RoleSupportAdmin, ModuleSubscriptions, Write) {
		t.Error("support_admin reads plans and must not reprice them")
	}
	// Voice rights are held apart from audio production on purpose.
	if Allowed(RoleAudioProducer, ModuleVoices, Write) {
		t.Error("audio_producer must not be able to grant voice rights")
	}
	if !Allowed(RoleVoiceManager, ModuleAudio, Write) {
		t.Error("voice_manager keeps its pre-PHASE 44 access to audio production")
	}
}

func TestAccessForMethods(t *testing.T) {
	for _, m := range []string{http.MethodGet, http.MethodHead, "get"} {
		if AccessFor(m) != Read {
			t.Errorf("%s should need Read", m)
		}
	}
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, ""} {
		if AccessFor(m) != Write {
			t.Errorf("%q should need Write", m)
		}
	}
}

func TestAdminLabels(t *testing.T) {
	if m, ok := ParseAdminLabel("admin:content"); !ok || m != ModuleContent {
		t.Errorf("admin:content parsed as %q/%v", m, ok)
	}
	if _, ok := ParseAdminLabel("admin"); ok {
		t.Error("a bare \"admin\" label names no module and must be refused")
	}
	if _, ok := ParseAdminLabel("admin:everything"); ok {
		t.Error("an unknown module must be refused")
	}
	if _, ok := ParseAdminLabel("user"); ok {
		t.Error("\"user\" is not an admin label")
	}
	if !IsAdminLabel("admin:nope") || IsAdminLabel("user") {
		t.Error("IsAdminLabel should flag every admin-prefixed label, parseable or not")
	}
}

func TestRolesAreExactlySeven(t *testing.T) {
	if got := len(Roles()); got != 7 {
		t.Fatalf("roles = %d, want 7", got)
	}
	for _, r := range Roles() {
		if !ValidRole(r) {
			t.Errorf("%s not valid", r)
		}
	}
	if ValidRole("admin") || ValidRole("support") {
		t.Error("neither \"admin\" nor the baseline default \"support\" is a role")
	}
}

// TestRolesWithIsDeterministic: the middleware built for a route must be the
// same on every start, so the list is sorted rather than map-ordered.
func TestRolesWithIsDeterministic(t *testing.T) {
	first := RolesWith(ModuleContent, Read)
	for i := 0; i < 50; i++ {
		again := RolesWith(ModuleContent, Read)
		if len(again) != len(first) {
			t.Fatal("length changed")
		}
		for j := range first {
			if first[j] != again[j] {
				t.Fatalf("order changed: %v vs %v", first, again)
			}
		}
	}
}
