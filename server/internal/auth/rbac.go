package auth

import (
	"strings"
)

// Additional role constants not in auth.go
const (
	RoleModerator  = "moderator"
	RoleBibleAdmin = "bible_admin"
)

// Permission is a granular capability in the system.
type Permission string

const (
	// Content permissions
	PermContentRead    Permission = "content:read"
	PermContentWrite   Permission = "content:write"
	PermContentPublish Permission = "content:publish"
	PermContentReview  Permission = "content:review"
	PermContentDelete  Permission = "content:delete"
	PermCategoryManage Permission = "category:manage"

	// Audio permissions
	PermAudioRead     Permission = "audio:read"
	PermAudioWrite    Permission = "audio:write"
	PermAudioQA       Permission = "audio:qa"
	PermAudioPublish  Permission = "audio:publish"
	PermAudioArchive  Permission = "audio:archive"
	PermAudioGenerate Permission = "audio:generate"

	// Voice permissions
	PermVoiceRead         Permission = "voice:read"
	PermVoiceWrite        Permission = "voice:write"
	PermVoiceRightsRead   Permission = "voice:rights:read"
	PermVoiceRightsManage Permission = "voice:rights:manage"
	PermVoiceTrain        Permission = "voice:train"

	// Bible permissions
	PermBibleRead    Permission = "bible:read"
	PermBibleWrite   Permission = "bible:write"
	PermBibleReview  Permission = "bible:review"
	PermBiblePublish Permission = "bible:publish"
	PermBibleAudio   Permission = "bible:audio:manage"

	// User management
	PermUserRead        Permission = "user:read"
	PermUserWrite       Permission = "user:write"
	PermUserSuspend     Permission = "user:suspend"
	PermUserDelete      Permission = "user:delete"
	PermUserImpersonate Permission = "user:impersonate"

	// Role & RBAC management
	PermRoleRead   Permission = "role:read"
	PermRoleWrite  Permission = "role:write"
	PermRoleAssign Permission = "role:assign"
	PermRoleDelete Permission = "role:delete"

	// Moderation
	PermModerationRead   Permission = "moderation:read"
	PermModerationWrite  Permission = "moderation:write"
	PermModerationDecide Permission = "moderation:decide"

	// Billing & pricing
	PermBillingRead   Permission = "billing:read"
	PermBillingWrite  Permission = "billing:write"
	PermPricingManage Permission = "pricing:manage"

	// System & observability
	PermSystemRead    Permission = "system:read"
	PermSystemWrite   Permission = "system:write"
	PermSystemMetrics Permission = "system:metrics"
	PermSystemHealth  Permission = "system:health"
	PermQueueRead     Permission = "queue:read"
	PermQueueWrite    Permission = "queue:write"
	PermCacheManage   Permission = "cache:manage"

	// Audit & security
	PermAuditRead     Permission = "audit:read"
	PermAuditExport   Permission = "audit:export"
	PermSecurityRead  Permission = "security:read"
	PermSecurityWrite Permission = "security:write"

	// Super admin - all permissions
	PermSuperAll Permission = "*"
)

// AllPermissions lists every permission in the system.
var AllPermissions = []Permission{
	PermContentRead, PermContentWrite, PermContentPublish, PermContentReview, PermContentDelete, PermCategoryManage,
	PermAudioRead, PermAudioWrite, PermAudioQA, PermAudioPublish, PermAudioArchive, PermAudioGenerate,
	PermVoiceRead, PermVoiceWrite, PermVoiceRightsRead, PermVoiceRightsManage, PermVoiceTrain,
	PermBibleRead, PermBibleWrite, PermBibleReview, PermBiblePublish, PermBibleAudio,
	PermUserRead, PermUserWrite, PermUserSuspend, PermUserDelete, PermUserImpersonate,
	PermRoleRead, PermRoleWrite, PermRoleAssign, PermRoleDelete,
	PermModerationRead, PermModerationWrite, PermModerationDecide,
	PermBillingRead, PermBillingWrite, PermPricingManage,
	PermSystemRead, PermSystemWrite, PermSystemMetrics, PermSystemHealth, PermQueueRead, PermQueueWrite, PermCacheManage,
	PermAuditRead, PermAuditExport, PermSecurityRead, PermSecurityWrite,
}

// RoleDefinition describes a role and its permissions.
type RoleDefinition struct {
	Name        string       `json:"name"`
	DisplayName string       `json:"display_name"`
	Description string       `json:"description"`
	Permissions []Permission `json:"permissions"`
	IsSystem    bool         `json:"is_system"`
	Level       int          `json:"level"` // Higher level = more privilege, super_admin is 100
	Category    string       `json:"category"`
}

// SystemRoles defines all built-in roles and their permissions.
var SystemRoles = map[string]RoleDefinition{
	RoleSuperAdmin: {
		Name:        RoleSuperAdmin,
		DisplayName: "Super Admin",
		Description: "Full platform access, including RBAC management, user erasure, and system operations. The ultimate authority.",
		Permissions: []Permission{PermSuperAll},
		IsSystem:    true,
		Level:       100,
		Category:    "administration",
	},
	RoleContentAdmin: {
		Name:        RoleContentAdmin,
		DisplayName: "Content Admin",
		Description: "Manages categories, confessions, and editorial lifecycle. Can publish and archive content.",
		Permissions: []Permission{
			PermContentRead, PermContentWrite, PermContentPublish, PermContentReview, PermContentDelete,
			PermCategoryManage,
			PermBibleRead, PermBibleWrite,
			PermAudioRead,
			PermModerationRead,
			PermSystemRead,
		},
		IsSystem: true,
		Level:    70,
		Category: "content",
	},
	RoleAudioProducer: {
		Name:        RoleAudioProducer,
		DisplayName: "Audio Producer",
		Description: "Produces, QA reviews, and publishes audio assets. Manages voice catalog.",
		Permissions: []Permission{
			PermAudioRead, PermAudioWrite, PermAudioQA, PermAudioPublish, PermAudioArchive, PermAudioGenerate,
			PermVoiceRead, PermVoiceWrite,
			PermContentRead,
			PermSystemRead, PermQueueRead,
		},
		IsSystem: true,
		Level:    60,
		Category: "audio",
	},
	RoleVoiceManager: {
		Name:        RoleVoiceManager,
		DisplayName: "Voice Manager",
		Description: "Manages voice rights, licensing, and authorization. Highest-consequence permission for voice synthesis.",
		Permissions: []Permission{
			PermVoiceRead, PermVoiceWrite, PermVoiceRightsRead, PermVoiceRightsManage, PermVoiceTrain,
			PermAudioRead, PermAudioWrite, PermAudioQA, PermAudioGenerate,
			PermSystemRead,
		},
		IsSystem: true,
		Level:    75,
		Category: "audio",
	},
	RoleMLEngineer: {
		Name:        RoleMLEngineer,
		DisplayName: "ML Engineer",
		Description: "Freezes voice datasets, runs and cancels training, reads models and evaluations. Cannot change voice rights.",
		Permissions: []Permission{PermVoiceRead, PermAudioRead, PermVoiceTrain, PermSystemRead, PermQueueRead},
		IsSystem:    true,
		Level:       55,
		Category:    "audio",
	},
	RoleAuditor: {
		Name:        RoleAuditor,
		DisplayName: "Auditor",
		Description: "Read-only access to voice rights audit logs, voice metrics and model history.",
		Permissions: []Permission{PermVoiceRead, PermVoiceRightsRead, PermAudioRead, PermAuditRead, PermSystemRead, PermSystemMetrics},
		IsSystem:    true,
		Level:       40,
		Category:    "audit",
	},
	RoleTheologicalRev: {
		Name:        RoleTheologicalRev,
		DisplayName: "Theological Reviewer",
		Description: "Reviews content for theological accuracy. Can approve or reject in theological review stage.",
		Permissions: []Permission{
			PermContentRead, PermContentReview,
			PermBibleRead, PermBibleReview,
			PermModerationRead,
		},
		IsSystem: true,
		Level:    60,
		Category: "content",
	},
	RoleSupportAdmin: {
		Name:        RoleSupportAdmin,
		DisplayName: "Support Admin",
		Description: "Supports users: views accounts, manages blocks, appeals, and moderation decisions.",
		Permissions: []Permission{
			PermUserRead, PermUserWrite,
			PermModerationRead, PermModerationWrite, PermModerationDecide,
			PermContentRead,
			PermBibleRead,
			PermSystemRead,
			PermAuditRead,
		},
		IsSystem: true,
		Level:    50,
		Category: "support",
	},
	RoleAnalyticsAdmin: {
		Name:        RoleAnalyticsAdmin,
		DisplayName: "Analytics Admin",
		Description: "Views analytics, metrics, and audit logs. Read-only operational visibility.",
		Permissions: []Permission{
			PermSystemRead, PermSystemMetrics, PermSystemHealth, PermQueueRead,
			PermAuditRead, PermAuditExport,
			PermSecurityRead,
			PermContentRead, PermUserRead, PermBillingRead,
			PermBibleRead,
		},
		IsSystem: true,
		Level:    40,
		Category: "analytics",
	},
	// Legacy admin role for backward compatibility
	"admin": {
		Name:        "admin",
		DisplayName: "Admin (Legacy)",
		Description: "Legacy admin role with broad access. Migrate to specific roles.",
		Permissions: []Permission{
			PermContentRead, PermContentWrite, PermContentPublish,
			PermAudioRead, PermAudioWrite,
			PermVoiceRead,
			PermUserRead, PermUserWrite,
			PermModerationRead, PermModerationWrite,
			PermBibleRead, PermBibleWrite,
			PermSystemRead,
			PermAuditRead,
		},
		IsSystem: true,
		Level:    60,
		Category: "administration",
	},
	RoleModerator: {
		Name:        RoleModerator,
		DisplayName: "Moderator",
		Description: "Moderates community content, reports, and user-generated confessions.",
		Permissions: []Permission{
			PermModerationRead, PermModerationWrite, PermModerationDecide,
			PermContentRead,
			PermUserRead,
			PermAuditRead,
		},
		IsSystem: true,
		Level:    45,
		Category: "moderation",
	},
	RoleBibleAdmin: {
		Name:        RoleBibleAdmin,
		DisplayName: "Bible Admin",
		Description: "Manages Bible translations, rights, reading plans, and audio.",
		Permissions: []Permission{
			PermBibleRead, PermBibleWrite, PermBibleReview, PermBiblePublish, PermBibleAudio,
			PermContentRead,
			PermSystemRead,
		},
		IsSystem: true,
		Level:    65,
		Category: "content",
	},
}

// HasPermission checks if a role grants a specific permission.
func HasPermission(role string, perm Permission) bool {
	if role == "" {
		return false
	}
	role = strings.TrimSpace(role)
	if role == RoleSuperAdmin {
		return true
	}
	def, ok := SystemRoles[role]
	if !ok {
		return false
	}
	for _, p := range def.Permissions {
		if p == PermSuperAll || p == perm {
			return true
		}
		if strings.HasSuffix(string(p), ":*") {
			prefix := strings.TrimSuffix(string(p), "*")
			if strings.HasPrefix(string(perm), prefix) {
				return true
			}
		}
	}
	return false
}

// HasAnyPermission checks if role has any of the given permissions.
func HasAnyPermission(role string, perms ...Permission) bool {
	for _, p := range perms {
		if HasPermission(role, p) {
			return true
		}
	}
	return false
}

// HasAllPermissions checks if role has all given permissions.
func HasAllPermissions(role string, perms ...Permission) bool {
	for _, p := range perms {
		if !HasPermission(role, p) {
			return false
		}
	}
	return true
}

// GetPermissionsForRole returns all permissions for a role.
func GetPermissionsForRole(role string) []Permission {
	if role == RoleSuperAdmin {
		return AllPermissions
	}
	def, ok := SystemRoles[role]
	if !ok {
		return nil
	}
	return def.Permissions
}

// GetRoleDefinition returns the definition for a role, if system role.
func GetRoleDefinition(role string) (RoleDefinition, bool) {
	def, ok := SystemRoles[role]
	return def, ok
}

// ListSystemRoles returns all system roles sorted by level descending.
func ListSystemRoles() []RoleDefinition {
	roles := make([]RoleDefinition, 0, len(SystemRoles))
	for _, def := range SystemRoles {
		roles = append(roles, def)
	}
	for i := 0; i < len(roles); i++ {
		for j := i + 1; j < len(roles); j++ {
			if roles[j].Level > roles[i].Level {
				roles[i], roles[j] = roles[j], roles[i]
			}
		}
	}
	return roles
}

// IsValidRole checks if a role name is known (system role).
func IsValidRole(role string) bool {
	role = strings.TrimSpace(role)
	if role == "" {
		return false
	}
	_, ok := SystemRoles[role]
	return ok
}

// CanAssignRole checks if assigner role can assign target role.
func CanAssignRole(assignerRole, targetRole string) bool {
	if assignerRole == RoleSuperAdmin {
		return true
	}
	assignerDef, ok := SystemRoles[assignerRole]
	if !ok {
		return false
	}
	targetDef, ok := SystemRoles[targetRole]
	if !ok {
		return false
	}
	return targetDef.Level < assignerDef.Level
}

// PermissionCategories groups permissions for UI display.
var PermissionCategories = map[string][]Permission{
	"content": {
		PermContentRead, PermContentWrite, PermContentPublish, PermContentReview, PermContentDelete, PermCategoryManage,
	},
	"audio": {
		PermAudioRead, PermAudioWrite, PermAudioQA, PermAudioPublish, PermAudioArchive, PermAudioGenerate,
		PermVoiceRead, PermVoiceWrite, PermVoiceRightsRead, PermVoiceRightsManage, PermVoiceTrain,
	},
	"bible": {
		PermBibleRead, PermBibleWrite, PermBibleReview, PermBiblePublish, PermBibleAudio,
	},
	"users": {
		PermUserRead, PermUserWrite, PermUserSuspend, PermUserDelete, PermUserImpersonate,
	},
	"rbac": {
		PermRoleRead, PermRoleWrite, PermRoleAssign, PermRoleDelete,
	},
	"moderation": {
		PermModerationRead, PermModerationWrite, PermModerationDecide,
	},
	"billing": {
		PermBillingRead, PermBillingWrite, PermPricingManage,
	},
	"system": {
		PermSystemRead, PermSystemWrite, PermSystemMetrics, PermSystemHealth, PermQueueRead, PermQueueWrite, PermCacheManage,
	},
	"audit": {
		PermAuditRead, PermAuditExport, PermSecurityRead, PermSecurityWrite,
	},
}

// PermissionInfo describes a permission for UI.
type PermissionInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

// PermissionDescriptions provides human-readable info for each permission.
var PermissionDescriptions = map[Permission]PermissionInfo{
	PermContentRead:    {Name: string(PermContentRead), DisplayName: "View Content", Description: "View categories and confessions", Category: "content"},
	PermContentWrite:   {Name: string(PermContentWrite), DisplayName: "Edit Content", Description: "Create and edit categories and confessions", Category: "content"},
	PermContentPublish: {Name: string(PermContentPublish), DisplayName: "Publish Content", Description: "Publish, unpublish, and archive content", Category: "content"},
	PermContentReview:  {Name: string(PermContentReview), DisplayName: "Review Content", Description: "Review content in editorial stages", Category: "content"},
	PermContentDelete:  {Name: string(PermContentDelete), DisplayName: "Delete Content", Description: "Permanently delete content", Category: "content"},
	PermCategoryManage: {Name: string(PermCategoryManage), DisplayName: "Manage Categories", Description: "Create, edit, delete categories", Category: "content"},

	PermAudioRead:     {Name: string(PermAudioRead), DisplayName: "View Audio", Description: "View audio assets and jobs", Category: "audio"},
	PermAudioWrite:    {Name: string(PermAudioWrite), DisplayName: "Manage Audio", Description: "Create and manage audio assets", Category: "audio"},
	PermAudioQA:       {Name: string(PermAudioQA), DisplayName: "Audio QA", Description: "Approve or reject audio renders", Category: "audio"},
	PermAudioPublish:  {Name: string(PermAudioPublish), DisplayName: "Publish Audio", Description: "Publish approved audio", Category: "audio"},
	PermAudioArchive:  {Name: string(PermAudioArchive), DisplayName: "Archive Audio", Description: "Archive audio assets", Category: "audio"},
	PermAudioGenerate: {Name: string(PermAudioGenerate), DisplayName: "Generate Audio", Description: "Trigger audio generation", Category: "audio"},

	PermVoiceRead:         {Name: string(PermVoiceRead), DisplayName: "View Voices", Description: "View voice catalog", Category: "audio"},
	PermVoiceWrite:        {Name: string(PermVoiceWrite), DisplayName: "Manage Voices", Description: "Create and edit voices", Category: "audio"},
	PermVoiceRightsRead:   {Name: string(PermVoiceRightsRead), DisplayName: "View Voice Rights", Description: "View voice licensing and rights", Category: "audio"},
	PermVoiceRightsManage: {Name: string(PermVoiceRightsManage), DisplayName: "Manage Voice Rights", Description: "Manage voice rights and authorizations - high consequence", Category: "audio"},
	PermVoiceTrain:        {Name: string(PermVoiceTrain), DisplayName: "Train Voices", Description: "Freeze voice datasets and run or cancel fine-tuning. Requires a grant that permits training", Category: "audio"},

	PermBibleRead:    {Name: string(PermBibleRead), DisplayName: "View Bible", Description: "View Bible catalog and translations", Category: "bible"},
	PermBibleWrite:   {Name: string(PermBibleWrite), DisplayName: "Manage Bible", Description: "Manage Bible translations", Category: "bible"},
	PermBibleReview:  {Name: string(PermBibleReview), DisplayName: "Review Bible Rights", Description: "Review Bible translation rights", Category: "bible"},
	PermBiblePublish: {Name: string(PermBiblePublish), DisplayName: "Publish Bible", Description: "Publish Bible content", Category: "bible"},
	PermBibleAudio:   {Name: string(PermBibleAudio), DisplayName: "Manage Bible Audio", Description: "Manage Bible audio assets", Category: "bible"},

	PermUserRead:        {Name: string(PermUserRead), DisplayName: "View Users", Description: "View user accounts", Category: "users"},
	PermUserWrite:       {Name: string(PermUserWrite), DisplayName: "Edit Users", Description: "Edit user profiles and preferences", Category: "users"},
	PermUserSuspend:     {Name: string(PermUserSuspend), DisplayName: "Suspend Users", Description: "Suspend and restore user accounts", Category: "users"},
	PermUserDelete:      {Name: string(PermUserDelete), DisplayName: "Delete Users", Description: "Erase user accounts - irreversible", Category: "users"},
	PermUserImpersonate: {Name: string(PermUserImpersonate), DisplayName: "Impersonate Users", Description: "Sign in as another user for support", Category: "users"},

	PermRoleRead:   {Name: string(PermRoleRead), DisplayName: "View Roles", Description: "View roles and permissions", Category: "rbac"},
	PermRoleWrite:  {Name: string(PermRoleWrite), DisplayName: "Manage Roles", Description: "Create and edit custom roles", Category: "rbac"},
	PermRoleAssign: {Name: string(PermRoleAssign), DisplayName: "Assign Roles", Description: "Assign and revoke roles", Category: "rbac"},
	PermRoleDelete: {Name: string(PermRoleDelete), DisplayName: "Delete Roles", Description: "Delete custom roles", Category: "rbac"},

	PermModerationRead:   {Name: string(PermModerationRead), DisplayName: "View Moderation", Description: "View moderation queue and reports", Category: "moderation"},
	PermModerationWrite:  {Name: string(PermModerationWrite), DisplayName: "Manage Moderation", Description: "Manage moderation cases", Category: "moderation"},
	PermModerationDecide: {Name: string(PermModerationDecide), DisplayName: "Decide Moderation", Description: "Make moderation decisions", Category: "moderation"},

	PermBillingRead:   {Name: string(PermBillingRead), DisplayName: "View Billing", Description: "View subscriptions and billing", Category: "billing"},
	PermBillingWrite:  {Name: string(PermBillingWrite), DisplayName: "Manage Billing", Description: "Manage subscriptions", Category: "billing"},
	PermPricingManage: {Name: string(PermPricingManage), DisplayName: "Manage Pricing", Description: "Manage pricing plans", Category: "billing"},

	PermSystemRead:    {Name: string(PermSystemRead), DisplayName: "View System", Description: "View system health and status", Category: "system"},
	PermSystemWrite:   {Name: string(PermSystemWrite), DisplayName: "Manage System", Description: "Manage system configuration", Category: "system"},
	PermSystemMetrics: {Name: string(PermSystemMetrics), DisplayName: "View Metrics", Description: "View system metrics and counters", Category: "system"},
	PermSystemHealth:  {Name: string(PermSystemHealth), DisplayName: "View Health", Description: "View health checks and readiness", Category: "system"},
	PermQueueRead:     {Name: string(PermQueueRead), DisplayName: "View Queue", Description: "View background job queue", Category: "system"},
	PermQueueWrite:    {Name: string(PermQueueWrite), DisplayName: "Manage Queue", Description: "Manage job queue - requeue, cancel", Category: "system"},
	PermCacheManage:   {Name: string(PermCacheManage), DisplayName: "Manage Cache", Description: "Invalidate and manage caches", Category: "system"},

	PermAuditRead:     {Name: string(PermAuditRead), DisplayName: "View Audit", Description: "View audit logs", Category: "audit"},
	PermAuditExport:   {Name: string(PermAuditExport), DisplayName: "Export Audit", Description: "Export audit logs", Category: "audit"},
	PermSecurityRead:  {Name: string(PermSecurityRead), DisplayName: "View Security", Description: "View security events and sessions", Category: "audit"},
	PermSecurityWrite: {Name: string(PermSecurityWrite), DisplayName: "Manage Security", Description: "Manage security settings", Category: "audit"},

	PermSuperAll: {Name: string(PermSuperAll), DisplayName: "All Permissions", Description: "Grants all permissions", Category: "admin"},
}

// GetAllPermissionInfos returns all permissions with descriptions.
func GetAllPermissionInfos() []PermissionInfo {
	infos := make([]PermissionInfo, 0, len(AllPermissions))
	for _, p := range AllPermissions {
		if info, ok := PermissionDescriptions[p]; ok {
			infos = append(infos, info)
		} else {
			infos = append(infos, PermissionInfo{Name: string(p), DisplayName: string(p), Description: "", Category: "other"})
		}
	}
	return infos
}
