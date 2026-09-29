package auth

import "testing"

func TestHasPermission(t *testing.T) {
	tests := []struct {
		role string
		perm Permission
		want bool
	}{
		{RoleSuperAdmin, PermContentRead, true},
		{RoleSuperAdmin, PermUserDelete, true},
		{RoleSuperAdmin, "nonexistent:perm", true}, // super admin has all
		{RoleContentAdmin, PermContentRead, true},
		{RoleContentAdmin, PermContentWrite, true},
		{RoleContentAdmin, PermAudioQA, false},
		{RoleContentAdmin, PermUserDelete, false},
		{RoleAudioProducer, PermAudioQA, true},
		{RoleAudioProducer, PermVoiceRightsManage, false},
		{RoleVoiceManager, PermVoiceRightsManage, true},
		{RoleVoiceManager, PermContentDelete, false},
		{RoleSupportAdmin, PermUserRead, true},
		{RoleSupportAdmin, PermContentPublish, false},
		{RoleAnalyticsAdmin, PermSystemMetrics, true},
		{RoleAnalyticsAdmin, PermContentWrite, false},
		{"", PermContentRead, false},
		{"nonexistent", PermContentRead, false},
	}

	for _, tc := range tests {
		if got := HasPermission(tc.role, tc.perm); got != tc.want {
			t.Errorf("HasPermission(%q, %q) = %v, want %v", tc.role, tc.perm, got, tc.want)
		}
	}
}

func TestCanAssignRole(t *testing.T) {
	tests := []struct {
		assigner string
		target   string
		want     bool
	}{
		{RoleSuperAdmin, RoleContentAdmin, true},
		{RoleSuperAdmin, RoleSuperAdmin, true},
		{RoleContentAdmin, RoleSupportAdmin, true},   // 70 > 50
		{RoleContentAdmin, RoleSuperAdmin, false},    // cannot assign higher
		{RoleContentAdmin, RoleVoiceManager, false},  // 70 < 75, cannot
		{RoleSupportAdmin, RoleAnalyticsAdmin, true}, // 50 > 40
		{RoleSupportAdmin, RoleContentAdmin, false},
		{RoleAnalyticsAdmin, RoleSupportAdmin, false},
		{"", RoleContentAdmin, false},
		{RoleContentAdmin, "custom_role", false}, // custom roles only super admin for now
	}

	for _, tc := range tests {
		if got := CanAssignRole(tc.assigner, tc.target); got != tc.want {
			t.Errorf("CanAssignRole(%q, %q) = %v, want %v", tc.assigner, tc.target, got, tc.want)
		}
	}
}

func TestListSystemRoles(t *testing.T) {
	roles := ListSystemRoles()
	if len(roles) == 0 {
		t.Fatal("expected at least one system role")
	}
	// Should be sorted descending by level
	for i := 1; i < len(roles); i++ {
		if roles[i].Level > roles[i-1].Level {
			t.Errorf("roles not sorted descending: %s (%d) after %s (%d)", roles[i].Name, roles[i].Level, roles[i-1].Name, roles[i-1].Level)
		}
	}
	// Super admin should be first
	if roles[0].Name != RoleSuperAdmin {
		t.Errorf("expected super_admin first, got %s", roles[0].Name)
	}
}

func TestAllPermissionsDescribed(t *testing.T) {
	for _, p := range AllPermissions {
		if _, ok := PermissionDescriptions[p]; !ok {
			t.Errorf("permission %s has no description", p)
		}
	}
}
