package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/db"
	"github.com/google/uuid"
)

// RBACStore manages custom roles and user-role assignments.
type RBACStore struct {
	db *db.DB
}

func NewRBACStore(db *db.DB) *RBACStore { return &RBACStore{db: db} }

// CustomRole represents a custom role stored in DB (system roles are code-defined).
type CustomRole struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	IsSystem    bool     `json:"is_system"`
	CreatedBy   string   `json:"created_by,omitempty"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

// UserRoleAssignment represents a user's role assignment.
type UserRoleAssignment struct {
	UserID     string `json:"user_id"`
	RoleID     string `json:"role_id"`
	RoleName   string `json:"role_name"`
	AssignedBy string `json:"assigned_by"`
	CreatedAt  string `json:"created_at"`
	UserEmail  string `json:"user_email,omitempty"`
}

// PermissionInfo for listing.
type PermissionInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

// ListSystemRoles returns all system roles (from code).
func (s *RBACStore) ListSystemRoles() []auth.RoleDefinition {
	return auth.ListSystemRoles()
}

// ListCustomRoles returns custom roles from DB.
func (s *RBACStore) ListCustomRoles(ctx context.Context) ([]CustomRole, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, display_name, COALESCE(description,''), COALESCE(permissions,'[]'), is_system, COALESCE(created_by,''), created_at, updated_at
		 FROM rbac_roles WHERE is_system = 0 OR is_system IS NULL ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CustomRole
	for rows.Next() {
		var r CustomRole
		var permsJSON string
		var isSystem int
		if err := rows.Scan(&r.ID, &r.Name, &r.DisplayName, &r.Description, &permsJSON, &isSystem, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.IsSystem = isSystem == 1
		_ = json.Unmarshal([]byte(permsJSON), &r.Permissions)
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListAllRoles returns both system and custom roles.
func (s *RBACStore) ListAllRoles(ctx context.Context) ([]CustomRole, error) {
	// System roles from code
	sysRoles := s.ListSystemRoles()
	out := make([]CustomRole, 0, len(sysRoles))
	for _, sr := range sysRoles {
		perms := make([]string, len(sr.Permissions))
		for i, p := range sr.Permissions {
			perms[i] = string(p)
		}
		out = append(out, CustomRole{
			ID:          "system:" + sr.Name,
			Name:        sr.Name,
			DisplayName: sr.DisplayName,
			Description: sr.Description,
			Permissions: perms,
			IsSystem:    true,
			CreatedAt:   time.Now().UTC().Format(time.RFC3339),
			UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
		})
	}

	// Custom roles from DB (if table exists)
	custom, err := s.ListCustomRoles(ctx)
	if err != nil {
		// If table doesn't exist yet, return only system roles
		if isNoSuchTable(err) {
			return out, nil
		}
		return nil, err
	}
	out = append(out, custom...)
	return out, nil
}

func isNoSuchTable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "no such table") || contains(msg, "does not exist")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}

// GetRole returns a role by name (system or custom).
func (s *RBACStore) GetRole(ctx context.Context, name string) (*CustomRole, error) {
	// Check system roles first
	if def, ok := auth.GetRoleDefinition(name); ok {
		perms := make([]string, len(def.Permissions))
		for i, p := range def.Permissions {
			perms[i] = string(p)
		}
		return &CustomRole{
			ID:          "system:" + def.Name,
			Name:        def.Name,
			DisplayName: def.DisplayName,
			Description: def.Description,
			Permissions: perms,
			IsSystem:    true,
		}, nil
	}

	// Check custom roles
	var r CustomRole
	var permsJSON string
	var isSystem int
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, display_name, COALESCE(description,''), COALESCE(permissions,'[]'), is_system, COALESCE(created_by,''), created_at, updated_at
		 FROM rbac_roles WHERE name = ?`, name).
		Scan(&r.ID, &r.Name, &r.DisplayName, &r.Description, &permsJSON, &isSystem, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.IsSystem = isSystem == 1
	_ = json.Unmarshal([]byte(permsJSON), &r.Permissions)
	return &r, nil
}

// CreateCustomRole creates a new custom role.
func (s *RBACStore) CreateCustomRole(ctx context.Context, name, displayName, description string, permissions []string, createdBy string) (*CustomRole, error) {
	// Validate name
	if name == "" || displayName == "" {
		return nil, errors.New("name and display_name are required")
	}
	// Check if system role exists with same name
	if _, ok := auth.GetRoleDefinition(name); ok {
		return nil, errors.New("role name conflicts with system role")
	}
	// Validate permissions
	for _, p := range permissions {
		if !isValidPermission(p) {
			return nil, errors.New("invalid permission: " + p)
		}
	}

	permsJSON, _ := json.Marshal(permissions)
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO rbac_roles (id, name, display_name, description, permissions, is_system, created_by, created_at, updated_at)
		 VALUES (?,?,?,?,?,0,?,?,?)`,
		id, name, displayName, description, string(permsJSON), createdBy, now, now)
	if err != nil {
		return nil, err
	}

	return &CustomRole{
		ID:          id,
		Name:        name,
		DisplayName: displayName,
		Description: description,
		Permissions: permissions,
		IsSystem:    false,
		CreatedBy:   createdBy,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// UpdateCustomRole updates a custom role.
func (s *RBACStore) UpdateCustomRole(ctx context.Context, name string, displayName, description string, permissions []string) (*CustomRole, error) {
	// Must be custom role
	existing, err := s.GetRole(ctx, name)
	if err != nil {
		return nil, err
	}
	if existing.IsSystem {
		return nil, errors.New("cannot modify system role")
	}

	for _, p := range permissions {
		if !isValidPermission(p) {
			return nil, errors.New("invalid permission: " + p)
		}
	}

	permsJSON, _ := json.Marshal(permissions)
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = s.db.ExecContext(ctx,
		`UPDATE rbac_roles SET display_name = ?, description = ?, permissions = ?, updated_at = ? WHERE name = ?`,
		displayName, description, string(permsJSON), now, name)
	if err != nil {
		return nil, err
	}

	existing.DisplayName = displayName
	existing.Description = description
	existing.Permissions = permissions
	existing.UpdatedAt = now
	return existing, nil
}

// DeleteCustomRole deletes a custom role.
func (s *RBACStore) DeleteCustomRole(ctx context.Context, name string) error {
	existing, err := s.GetRole(ctx, name)
	if err != nil {
		return err
	}
	if existing.IsSystem {
		return errors.New("cannot delete system role")
	}

	// Check if role is assigned to users
	var count int
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rbac_user_roles WHERE role_id = ?`, existing.ID).Scan(&count)
	if err != nil && !isNoSuchTable(err) {
		return err
	}
	if count > 0 {
		return errors.New("role is still assigned to users")
	}

	_, err = s.db.ExecContext(ctx, `DELETE FROM rbac_roles WHERE name = ?`, name)
	return err
}

// ListPermissions returns all permissions with descriptions.
func (s *RBACStore) ListPermissions() []PermissionInfo {
	infos := auth.GetAllPermissionInfos()
	out := make([]PermissionInfo, len(infos))
	for i, info := range infos {
		out[i] = PermissionInfo{
			Name:        info.Name,
			DisplayName: info.DisplayName,
			Description: info.Description,
			Category:    info.Category,
		}
	}
	return out
}

func isValidPermission(p string) bool {
	for _, all := range auth.AllPermissions {
		if string(all) == p {
			return true
		}
	}
	return false
}

// GetUserRoles returns all roles assigned to a user (including admin_users role).
func (s *RBACStore) GetUserRoles(ctx context.Context, userID string) ([]CustomRole, error) {
	var roles []CustomRole

	// Check admin_users table (legacy single role)
	var adminRole string
	err := s.db.QueryRowContext(ctx, `SELECT role FROM admin_users WHERE user_id = ?`, userID).Scan(&adminRole)
	if err == nil && adminRole != "" {
		if def, ok := auth.GetRoleDefinition(adminRole); ok {
			perms := make([]string, len(def.Permissions))
			for i, p := range def.Permissions {
				perms[i] = string(p)
			}
			roles = append(roles, CustomRole{
				ID:          "system:" + def.Name,
				Name:        def.Name,
				DisplayName: def.DisplayName,
				Description: def.Description,
				Permissions: perms,
				IsSystem:    true,
			})
		}
	}

	// Check rbac_user_roles (new multiple roles)
	rows, err := s.db.QueryContext(ctx,
		`SELECT r.id, r.name, r.display_name, COALESCE(r.description,''), COALESCE(r.permissions,'[]'), r.is_system
		 FROM rbac_user_roles ur JOIN rbac_roles r ON ur.role_id = r.id WHERE ur.user_id = ?`, userID)
	if err != nil {
		if isNoSuchTable(err) {
			return roles, nil
		}
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var r CustomRole
		var permsJSON string
		var isSystem int
		if err := rows.Scan(&r.ID, &r.Name, &r.DisplayName, &r.Description, &permsJSON, &isSystem); err != nil {
			return nil, err
		}
		r.IsSystem = isSystem == 1
		_ = json.Unmarshal([]byte(permsJSON), &r.Permissions)
		// Avoid duplicate if already from admin_users
		duplicate := false
		for _, existing := range roles {
			if existing.Name == r.Name {
				duplicate = true
				break
			}
		}
		if !duplicate {
			roles = append(roles, r)
		}
	}

	return roles, rows.Err()
}

// AssignRole assigns a role to a user.
func (s *RBACStore) AssignRole(ctx context.Context, userID, roleName, assignedBy string) error {
	role, err := s.GetRole(ctx, roleName)
	if err != nil {
		return err
	}

	// If it's a system role, use admin_users table for backward compat + rbac_user_roles
	if role.IsSystem && !contains(role.ID, "custom") {
		// For system roles, update admin_users (single role) for backward compat
		// But also insert into rbac_user_roles for multiple roles support
		// First, set admin_users role to the new role if user has no role or if assigning super_admin
		// For simplicity, we store in both places

		// Check if table exists
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO rbac_user_roles (id, user_id, role_id, assigned_by, created_at)
			 VALUES (?,?,?,?,?)
			 ON CONFLICT(user_id, role_id) DO NOTHING`,
			uuid.NewString(), userID, role.ID, assignedBy, time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			if isNoSuchTable(err) {
				// Fallback to admin_users only
				_, err = s.db.ExecContext(ctx,
					`INSERT INTO admin_users (id, user_id, role, created_at) VALUES (?,?,?,?)
					 ON CONFLICT(user_id) DO UPDATE SET role = excluded.role`,
					uuid.NewString(), userID, roleName, time.Now().UTC().Format(time.RFC3339))
				return err
			}
			return err
		}

		// Also update admin_users to keep legacy working (set to highest privilege role)
		// Get all roles for user and set admin_users to highest level
		allRoles, _ := s.GetUserRoles(ctx, userID)
		// Include the new role
		highestRole := roleName
		highestLevel := 0
		if def, ok := auth.GetRoleDefinition(roleName); ok {
			highestLevel = def.Level
		}
		for _, r := range allRoles {
			if def, ok := auth.GetRoleDefinition(r.Name); ok {
				if def.Level > highestLevel {
					highestLevel = def.Level
					highestRole = r.Name
				}
			}
		}

		_, err = s.db.ExecContext(ctx,
			`INSERT INTO admin_users (id, user_id, role, created_at) VALUES (?,?,?,?)
			 ON CONFLICT(user_id) DO UPDATE SET role = excluded.role`,
			uuid.NewString(), userID, highestRole, time.Now().UTC().Format(time.RFC3339))
		return err
	}

	// Custom role
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO rbac_user_roles (id, user_id, role_id, assigned_by, created_at)
		 VALUES (?,?,?,?,?)`,
		uuid.NewString(), userID, role.ID, assignedBy, time.Now().UTC().Format(time.RFC3339))
	return err
}

// RemoveRole removes a role from a user.
func (s *RBACStore) RemoveRole(ctx context.Context, userID, roleName string) error {
	role, err := s.GetRole(ctx, roleName)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `DELETE FROM rbac_user_roles WHERE user_id = ? AND role_id = ?`, userID, role.ID)
	if err != nil && !isNoSuchTable(err) {
		return err
	}

	// Also handle admin_users if it's the role stored there
	var currentAdminRole string
	err = s.db.QueryRowContext(ctx, `SELECT role FROM admin_users WHERE user_id = ?`, userID).Scan(&currentAdminRole)
	if err == nil && currentAdminRole == roleName {
		// Remove from admin_users and set to next highest role if any
		remainingRoles, _ := s.GetUserRoles(ctx, userID)
		if len(remainingRoles) == 0 {
			_, err = s.db.ExecContext(ctx, `DELETE FROM admin_users WHERE user_id = ?`, userID)
		} else {
			// Set to highest remaining
			highestRole := remainingRoles[0].Name
			highestLevel := 0
			for _, r := range remainingRoles {
				if def, ok := auth.GetRoleDefinition(r.Name); ok {
					if def.Level > highestLevel {
						highestLevel = def.Level
						highestRole = r.Name
					}
				}
			}
			_, err = s.db.ExecContext(ctx, `UPDATE admin_users SET role = ? WHERE user_id = ?`, highestRole, userID)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// ListUsersWithRoles returns users with their roles (paginated).
func (s *RBACStore) ListUsersWithRoles(ctx context.Context, search string, limit, offset int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	query := `SELECT u.id, u.email, COALESCE(u.display_name,''), u.status, u.created_at,
	                 COALESCE((SELECT role FROM admin_users WHERE user_id = u.id), '') as admin_role
	          FROM users u WHERE 1=1`
	args := []any{}

	if search != "" {
		query += ` AND (u.email LIKE ? OR u.display_name LIKE ?)`
		like := "%" + search + "%"
		args = append(args, like, like)
	}

	query += ` ORDER BY u.created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var id, email, displayName, status, createdAt, adminRole string
		if err := rows.Scan(&id, &email, &displayName, &status, &createdAt, &adminRole); err != nil {
			return nil, err
		}
		// Get additional roles from rbac_user_roles
		roles, _ := s.GetUserRoles(ctx, id)
		roleNames := make([]string, len(roles))
		for i, r := range roles {
			roleNames[i] = r.Name
		}

		out = append(out, map[string]any{
			"id":           id,
			"email":        email,
			"display_name": displayName,
			"status":       status,
			"created_at":   createdAt,
			"admin_role":   adminRole,
			"roles":        roleNames,
		})
	}
	return out, rows.Err()
}

// GetUserEffectivePermissions returns all effective permissions for a user.
func (s *RBACStore) GetUserEffectivePermissions(ctx context.Context, userID string) ([]string, error) {
	roles, err := s.GetUserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}

	permSet := map[string]bool{}
	for _, role := range roles {
		// Check if system role
		if def, ok := auth.GetRoleDefinition(role.Name); ok {
			if len(def.Permissions) == 1 && def.Permissions[0] == auth.PermSuperAll {
				// Super admin - return all
				all := make([]string, len(auth.AllPermissions))
				for i, p := range auth.AllPermissions {
					all[i] = string(p)
				}
				return all, nil
			}
			for _, p := range def.Permissions {
				permSet[string(p)] = true
			}
		} else {
			// Custom role
			for _, p := range role.Permissions {
				permSet[p] = true
			}
		}
	}

	out := make([]string, 0, len(permSet))
	for p := range permSet {
		out = append(out, p)
	}
	return out, nil
}
