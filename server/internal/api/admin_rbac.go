package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/store"
)

// ---------- RBAC: roles & permissions ----------

func (h *Handler) adminListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.rbac.ListAllRoles(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load roles")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"roles": roles,
		"count": len(roles),
	})
}

func (h *Handler) adminGetRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "role name required")
		return
	}
	role, err := h.rbac.GetRole(r.Context(), name)
	if err != nil {
		if err == store.ErrNotFound {
			httpx.WriteError(w, http.StatusNotFound, "role not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load role")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, role)
}

func (h *Handler) adminCreateRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string   `json:"name"`
		DisplayName string   `json:"display_name"`
		Description string   `json:"description"`
		Permissions []string `json:"permissions"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.DisplayName) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "name and display_name are required")
		return
	}
	// Name validation: lowercase, alphanumeric, underscore
	if !isValidRoleName(req.Name) {
		httpx.WriteError(w, http.StatusBadRequest, "role name must be lowercase alphanumeric with underscores, 3-40 chars")
		return
	}
	if len(req.Permissions) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "at least one permission is required")
		return
	}

	actor := actorID(r)
	role, err := h.rbac.CreateCustomRole(r.Context(), strings.TrimSpace(req.Name), strings.TrimSpace(req.DisplayName), strings.TrimSpace(req.Description), req.Permissions, actor)
	if err != nil {
		if strings.Contains(err.Error(), "conflicts with system role") {
			httpx.WriteError(w, http.StatusConflict, err.Error())
			return
		}
		if strings.Contains(err.Error(), "invalid permission") {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate") {
			httpx.WriteError(w, http.StatusConflict, "role with this name already exists")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create role")
		return
	}

	h.recordAudit(r, "rbac_role_created", "rbac_role", role.ID, "name="+role.Name, "ok")
	httpx.WriteJSON(w, http.StatusCreated, role)
}

func (h *Handler) adminUpdateRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "role name required")
		return
	}
	var req struct {
		DisplayName string   `json:"display_name"`
		Description string   `json:"description"`
		Permissions []string `json:"permissions"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Permissions) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "at least one permission is required")
		return
	}

	role, err := h.rbac.UpdateCustomRole(r.Context(), name, strings.TrimSpace(req.DisplayName), strings.TrimSpace(req.Description), req.Permissions)
	if err != nil {
		if strings.Contains(err.Error(), "cannot modify system role") {
			httpx.WriteError(w, http.StatusForbidden, "system roles cannot be modified")
			return
		}
		if err == store.ErrNotFound {
			httpx.WriteError(w, http.StatusNotFound, "role not found")
			return
		}
		if strings.Contains(err.Error(), "invalid permission") {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "failed to update role")
		return
	}

	h.recordAudit(r, "rbac_role_updated", "rbac_role", role.ID, "name="+role.Name, "ok")
	httpx.WriteJSON(w, http.StatusOK, role)
}

func (h *Handler) adminDeleteRole(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "role name required")
		return
	}

	err := h.rbac.DeleteCustomRole(r.Context(), name)
	if err != nil {
		if strings.Contains(err.Error(), "cannot delete system role") {
			httpx.WriteError(w, http.StatusForbidden, "system roles cannot be deleted")
			return
		}
		if strings.Contains(err.Error(), "still assigned") {
			httpx.WriteError(w, http.StatusConflict, "role is still assigned to users, revoke assignments first")
			return
		}
		if err == store.ErrNotFound {
			httpx.WriteError(w, http.StatusNotFound, "role not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "failed to delete role")
		return
	}

	h.recordAudit(r, "rbac_role_deleted", "rbac_role", name, "name="+name, "ok")
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "role deleted"})
}

func (h *Handler) adminListPermissions(w http.ResponseWriter, r *http.Request) {
	perms := h.rbac.ListPermissions()
	// Group by category
	grouped := map[string][]any{}
	for _, p := range perms {
		grouped[p.Category] = append(grouped[p.Category], p)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"permissions": perms,
		"grouped":     grouped,
		"count":       len(perms),
		"categories":  auth.PermissionCategories,
	})
}

// ---------- RBAC: user role assignments ----------

func (h *Handler) adminGetUserRoles(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	if userID == "" {
		userID = r.PathValue("userId")
	}
	if userID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "user id required")
		return
	}

	roles, err := h.rbac.GetUserRoles(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load user roles")
		return
	}

	perms, err := h.rbac.GetUserEffectivePermissions(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load permissions")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"user_id":     userID,
		"roles":       roles,
		"permissions": perms,
	})
}

func (h *Handler) adminAssignUserRole(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	if userID == "" {
		userID = r.PathValue("userId")
	}
	if userID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "user id required")
		return
	}

	var req struct {
		Role string `json:"role"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || strings.TrimSpace(req.Role) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "role is required")
		return
	}

	// Check if assigner can assign this role
	assignerRole := currentRole(r)
	if !auth.CanAssignRole(assignerRole, req.Role) {
		httpx.WriteError(w, http.StatusForbidden, "you cannot assign a role with equal or higher privilege than your own")
		return
	}

	// Verify user exists
	if _, err := h.users.ByID(r.Context(), userID); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "user not found")
		return
	}

	actor := actorID(r)
	if err := h.rbac.AssignRole(r.Context(), userID, strings.TrimSpace(req.Role), actor); err != nil {
		if err == store.ErrNotFound {
			httpx.WriteError(w, http.StatusNotFound, "role not found")
			return
		}
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate") {
			httpx.WriteError(w, http.StatusConflict, "user already has this role")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "failed to assign role")
		return
	}

	h.recordAudit(r, "rbac_role_assigned", "user", userID, "role="+req.Role, "ok")
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"user_id": userID, "role": req.Role, "message": "role assigned"})
}

func (h *Handler) adminRemoveUserRole(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	if userID == "" {
		userID = r.PathValue("userId")
	}
	if userID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "user id required")
		return
	}

	roleName := r.URL.Query().Get("role")
	if roleName == "" {
		// Try body
		var req struct {
			Role string `json:"role"`
		}
		_ = httpx.DecodeJSON(r, &req)
		roleName = req.Role
	}
	if strings.TrimSpace(roleName) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "role is required")
		return
	}

	// Prevent self-demotion of last super_admin? Allow but audit.
	if actorID(r) == userID && roleName == auth.RoleSuperAdmin {
		// Check if there is at least one other super_admin
		admins, err := h.users.ListAdminsByRole(r.Context(), auth.RoleSuperAdmin, 2)
		if err == nil && len(admins) <= 1 {
			httpx.WriteError(w, http.StatusConflict, "cannot remove your own super_admin role when you are the last super admin")
			return
		}
	}

	if err := h.rbac.RemoveRole(r.Context(), userID, strings.TrimSpace(roleName)); err != nil {
		if err == store.ErrNotFound {
			httpx.WriteError(w, http.StatusNotFound, "role assignment not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "failed to remove role")
		return
	}

	h.recordAudit(r, "rbac_role_revoked", "user", userID, "role="+roleName, "ok")
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "role removed"})
}

// ---------- Enhanced user management ----------

func (h *Handler) adminListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	search := strings.TrimSpace(q.Get("q"))
	search = strings.TrimSpace(q.Get("search"))
	if search == "" {
		search = strings.TrimSpace(q.Get("query"))
	}

	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	offset, _ := strconv.Atoi(q.Get("offset"))

	users, err := h.rbac.ListUsersWithRoles(r.Context(), search, limit, offset)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load users")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"users":  users,
		"count":  len(users),
		"limit":  limit,
		"offset": offset,
		"search": search,
	})
}

func (h *Handler) adminGetUserDetail(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	if userID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "user id required")
		return
	}

	user, err := h.users.ByID(r.Context(), userID)
	if err != nil {
		if err == store.ErrNotFound {
			httpx.WriteError(w, http.StatusNotFound, "user not found")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load user")
		return
	}

	roles, _ := h.rbac.GetUserRoles(r.Context(), userID)
	perms, _ := h.rbac.GetUserEffectivePermissions(r.Context(), userID)
	sub, _ := h.users.SubscriptionRecord(r.Context(), userID)
	sessions, _ := h.users.ListAuthSessions(r.Context(), userID)

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"user":        user,
		"roles":       roles,
		"permissions": perms,
		"subscription": sub,
		"sessions":    sessions,
	})
}

func (h *Handler) adminListUserSessions(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	if userID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "user id required")
		return
	}

	sessions, err := h.users.ListAuthSessions(r.Context(), userID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load sessions")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"user_id":  userID,
		"sessions": sessions,
		"count":    len(sessions),
	})
}

func (h *Handler) adminRevokeUserSessions(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	if userID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "user id required")
		return
	}

	if err := h.users.RevokeAllSessions(r.Context(), userID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to revoke sessions")
		return
	}

	h.recordAudit(r, "user_sessions_revoked", "user", userID, "all sessions revoked", "ok")
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"message": "all sessions revoked"})
}

// ---------- System & security ----------

func (h *Handler) adminSystemHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dbOK := h.db.PingContext(ctx) == nil

	var catCount, voiceCount, userCount, adminCount int
	_ = h.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM categories WHERE status='published'`).Scan(&catCount)
	_ = h.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM voices WHERE status='active'`).Scan(&voiceCount)
	_ = h.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&userCount)
	_ = h.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_users`).Scan(&adminCount)

	// Cache stats
	var cacheStats any
	if h.cacheStats != nil {
		cacheStats = h.cacheStats()
	}

	// Queue stats
	var queueStats map[string]int
	if h.queue != nil {
		if qs, err := h.queue.Stats(ctx); err == nil {
			queueStats = qs
		}
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"database": map[string]any{"healthy": dbOK},
		"inventory": map[string]int{
			"published_categories": catCount,
			"active_voices":        voiceCount,
			"total_users":          userCount,
			"admin_users":          adminCount,
		},
		"cache": cacheStats,
		"queue": queueStats,
		"subsystems": map[string]bool{
			"database": dbOK,
			"email":    h.mail != nil,
			"storage":  h.signer != nil,
			"queue":    h.queue != nil,
		},
	})
}

func (h *Handler) adminSecurityOverview(w http.ResponseWriter, r *http.Request) {
	// Security counters from metrics
	counters := h.metrics.Snapshot()

	// Recent security events
	var recentEvents []map[string]any
	rows, err := h.db.QueryContext(r.Context(),
		`SELECT id, user_id, event_type, ip_address, created_at FROM security_events ORDER BY created_at DESC LIMIT 50`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, userID, eventType, ip, createdAt string
			_ = rows.Scan(&id, &userID, &eventType, &ip, &createdAt)
			recentEvents = append(recentEvents, map[string]any{
				"id":         id,
				"user_id":    userID,
				"event_type": eventType,
				"ip_address": ip,
				"created_at": createdAt,
			})
		}
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"counters":       counters,
		"recent_events":  recentEvents,
		"failed_logins":  counters["login_failure_total"],
		"token_reuse":    counters["token_reuse_detected_total"],
		"rate_limited":   counters["rate_limited_total"],
		"mfa_failures":   counters["mfa_failure_total"],
	})
}

func (h *Handler) adminAuditExport(w http.ResponseWriter, r *http.Request) {
	// Only super_admin can export
	if currentRole(r) != auth.RoleSuperAdmin {
		httpx.WriteError(w, http.StatusForbidden, "only super_admin can export audit logs")
		return
	}

	entity := r.URL.Query().Get("entity")
	entityID := r.URL.Query().Get("entity_id")
	limitStr := r.URL.Query().Get("limit")
	limit := 1000
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 10000 {
		limit = l
	}

	entries, err := h.audio.AuditTrail(r.Context(), entity, entityID, limit)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load audit trail")
		return
	}

	// If format=json, return JSON, else CSV-like JSON
	format := r.URL.Query().Get("format")
	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=audit-export.csv")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("id,actor,action,entity,entity_id,detail,result,created_at\n"))
		for _, e := range entries {
			line := ""
			if m, ok := e.(map[string]any); ok {
				line = toCSVLine(m)
			} else {
				b, _ := json.Marshal(e)
				var m map[string]any
				_ = json.Unmarshal(b, &m)
				line = toCSVLine(m)
			}
			_, _ = w.Write([]byte(line + "\n"))
		}
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"entries": entries,
		"count":   len(entries),
		"exported_at": nowString(),
	})
}

func toCSVLine(m map[string]any) string {
	// Simple CSV escaping
	esc := func(v any) string {
		s := ""
		if v != nil {
			s = strings.ReplaceAll(strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(
				jsonString(v), "\"", "\"\""), "\n", " "), "\r", " ")), ",", ";")
		}
		return "\"" + s + "\""
	}
	return strings.Join([]string{
		esc(m["id"]),
		esc(m["actor"]),
		esc(m["action"]),
		esc(m["entity"]),
		esc(m["entity_id"]),
		esc(m["detail"]),
		esc(m["result"]),
		esc(m["created_at"]),
	}, ",")
}

func jsonString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func nowString() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// ---------- Helpers ----------

func isValidRoleName(name string) bool {
	if len(name) < 3 || len(name) > 40 {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

func currentRole(r *http.Request) string {
	if c := auth.FromContext(r); c != nil {
		return c.Role
	}
	return ""
}

func actorID(r *http.Request) string {
	if c := auth.FromContext(r); c != nil {
		return c.Sub
	}
	return ""
}

func actor(r *http.Request) string {
	if c := auth.FromContext(r); c != nil {
		if c.Email != "" {
			return c.Email
		}
		return c.Sub
	}
	return "unknown"
}
