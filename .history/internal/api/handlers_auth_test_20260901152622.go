package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/db"
)

func setupTestDB(t *testing.T) *sql.DB {
	dbConn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Create schema
	if err := db.InitSchema(dbConn, db.SchemaSQL); err != nil {
		t.Fatalf("failed to initialize schema: %v", err)
	}

	return dbConn
}

func TestRegister(t *testing.T) {
	dbConn := setupTestDB(t)
	defer dbConn.Close()

	h := NewHandler(Config{
		JWTSecret: "test-secret",
		TokenTTL:  "24h",
	}, dbConn)
	h.BuildEngine()

	tests := []struct {
		name       string
		payload    map[string]string
		expectCode int
		expectErr  bool
	}{
		{
			"valid registration",
			map[string]string{
				"email":        "user@example.com",
				"password":     "password123",
				"display_name": "Test User",
				"timezone":     "UTC",
			},
			http.StatusOK,
			false,
		},
		{
			"duplicate email",
			map[string]string{
				"email":    "user@example.com",
				"password": "password456",
			},
			http.StatusConflict,
			true,
		},
		{
			"password too short",
			map[string]string{
				"email":    "short@example.com",
				"password": "short",
			},
			http.StatusBadRequest,
			true,
		},
		{
			"missing email",
			map[string]string{
				"password": "password123",
			},
			http.StatusBadRequest,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.payload)
			req := httptest.NewRequest("POST", "/auth/register", bytes.NewReader(body))
			w := httptest.NewRecorder()

			h.register(w, req)

			if w.Code != tt.expectCode {
				t.Errorf("status = %d, want %d", w.Code, tt.expectCode)
			}

			if tt.expectCode == http.StatusOK {
				var resp map[string]any
				json.NewDecoder(w.Body).Decode(&resp)
				if _, ok := resp["token"]; !ok {
					t.Fatal("expected token in response")
				}
			}
		})
	}
}

func TestLogin(t *testing.T) {
	dbConn := setupTestDB(t)
	defer dbConn.Close()

	h := NewHandler(Config{
		JWTSecret: "test-secret",
		TokenTTL:  "24h",
	}, dbConn)
	h.BuildEngine()

	// Create a test user
	email := "login@example.com"
	password := "password123"
	hash, _ := auth.HashPassword(password)
	h.users.Create(context.Background(), email, hash, "Test User", "UTC")

	tests := []struct {
		name       string
		email      string
		password   string
		expectCode int
	}{
		{
			"valid login",
			email,
			password,
			http.StatusOK,
		},
		{
			"wrong password",
			email,
			"wrongpassword",
			http.StatusUnauthorized,
		},
		{
			"nonexistent email",
			"notfound@example.com",
			password,
			http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := map[string]string{
				"email":    tt.email,
				"password": tt.password,
			}
			body, _ := json.Marshal(payload)
			req := httptest.NewRequest("POST", "/auth/login", bytes.NewReader(body))
			w := httptest.NewRecorder()

			h.login(w, req)

			if w.Code != tt.expectCode {
				t.Errorf("status = %d, want %d", w.Code, tt.expectCode)
			}
		})
	}
}

func TestChangePassword(t *testing.T) {
	dbConn := setupTestDB(t)
	defer dbConn.Close()

	h := NewHandler(Config{
		JWTSecret: "test-secret",
		TokenTTL:  "24h",
	}, dbConn)
	h.BuildEngine()

	// Create a test user
	email := "change@example.com"
	password := "password123"
	hash, _ := auth.HashPassword(password)
	user, _ := h.users.Create(context.Background(), email, hash, "Test User", "UTC")

	// Create a token for the user
	token, _ := auth.SignToken("test-secret", "24h", user.ID, email, "")

	payload := map[string]string{
		"current_password": password,
		"new_password":     "newpassword456",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/auth/change-password", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.changePassword(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRefreshToken(t *testing.T) {
	dbConn := setupTestDB(t)
	defer dbConn.Close()

	h := NewHandler(Config{
		JWTSecret: "test-secret",
		TokenTTL:  "24h",
	}, dbConn)
	h.BuildEngine()

	// Create a test user
	email := "refresh@example.com"
	password := "password123"
	hash, _ := auth.HashPassword(password)
	user, _ := h.users.Create(context.Background(), email, hash, "Test User", "UTC")

	// Create a token for the user
	token, _ := auth.SignToken("test-secret", "24h", user.ID, email, "")

	req := httptest.NewRequest("POST", "/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	// Add the token to context manually (middleware would do this)
	claims, _ := auth.ParseToken("test-secret", token)
	req = req.WithContext(context.WithValue(req.Context(), auth.ClaimsKey, claims))

	h.refreshToken(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if _, ok := resp["token"]; !ok {
		t.Fatal("expected token in response")
	}
}

func TestAdminSetUserRole(t *testing.T) {
	dbConn := setupTestDB(t)
	defer dbConn.Close()

	h := NewHandler(Config{
		JWTSecret: "test-secret",
		TokenTTL:  "24h",
	}, dbConn)
	h.BuildEngine()

	// Create test users
	email1 := "user1@example.com"
	hash1, _ := auth.HashPassword("password123")
	user1, _ := h.users.Create(context.Background(), email1, hash1, "User 1", "UTC")

	email2 := "admin@example.com"
	hash2, _ := auth.HashPassword("password123")
	user2, _ := h.users.Create(context.Background(), email2, hash2, "Admin", "UTC")

	// Make user2 an admin
	h.users.SetAdminRole(context.Background(), user2.ID, auth.RoleContentAdmin)

	// Create admin token
	token, _ := auth.SignToken("test-secret", "24h", user2.ID, email2, auth.RoleContentAdmin)

	payload := map[string]string{
		"user_id": user1.ID,
		"role":    auth.RoleAudioProducer,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/admin/users/role", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	// Add the token to context manually
	claims, _ := auth.ParseToken("test-secret", token)
	req = req.WithContext(context.WithValue(req.Context(), auth.ClaimsKey, claims))

	h.adminSetUserRole(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	// Verify the role was set
	role, _ := h.users.AdminRole(context.Background(), user1.ID)
	if role != auth.RoleAudioProducer {
		t.Errorf("role = %s, want %s", role, auth.RoleAudioProducer)
	}
}

func TestAdminRemoveUserRole(t *testing.T) {
	dbConn := setupTestDB(t)
	defer dbConn.Close()

	h := NewHandler(Config{
		JWTSecret: "test-secret",
		TokenTTL:  "24h",
	}, dbConn)
	h.BuildEngine()

	// Create test users
	email1 := "user1@example.com"
	hash1, _ := auth.HashPassword("password123")
	user1, _ := h.users.Create(context.Background(), email1, hash1, "User 1", "UTC")

	email2 := "admin@example.com"
	hash2, _ := auth.HashPassword("password123")
	user2, _ := h.users.Create(context.Background(), email2, hash2, "Admin", "UTC")

	// Make both users admins
	h.users.SetAdminRole(context.Background(), user1.ID, auth.RoleAudioProducer)
	h.users.SetAdminRole(context.Background(), user2.ID, auth.RoleContentAdmin)

	// Create admin token
	token, _ := auth.SignToken("test-secret", "24h", user2.ID, email2, auth.RoleContentAdmin)

	payload := map[string]string{
		"user_id": user1.ID,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("DELETE", "/admin/users/role", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	// Add the token to context manually
	claims, _ := auth.ParseToken("test-secret", token)
	req = req.WithContext(context.WithValue(req.Context(), auth.ClaimsKey, claims))

	h.adminRemoveUserRole(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	// Verify the role was removed
	role, _ := h.users.AdminRole(context.Background(), user1.ID)
	if role != "" {
		t.Errorf("role = %s, want empty", role)
	}
}

func TestAdminListAdmins(t *testing.T) {
	dbConn := setupTestDB(t)
	defer dbConn.Close()

	h := NewHandler(Config{
		JWTSecret: "test-secret",
		TokenTTL:  "24h",
	}, dbConn)
	h.BuildEngine()

	// Create test users
	email1 := "user1@example.com"
	hash1, _ := auth.HashPassword("password123")
	user1, _ := h.users.Create(context.Background(), email1, hash1, "User 1", "UTC")

	email2 := "admin@example.com"
	hash2, _ := auth.HashPassword("password123")
	user2, _ := h.users.Create(context.Background(), email2, hash2, "Admin", "UTC")

	// Make both users admins
	h.users.SetAdminRole(context.Background(), user1.ID, auth.RoleAudioProducer)
	h.users.SetAdminRole(context.Background(), user2.ID, auth.RoleContentAdmin)

	// Create admin token
	token, _ := auth.SignToken("test-secret", "24h", user2.ID, email2, auth.RoleContentAdmin)

	req := httptest.NewRequest("GET", "/admin/users/admins", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	// Add the token to context manually
	claims, _ := auth.ParseToken("test-secret", token)
	req = req.WithContext(context.WithValue(req.Context(), auth.ClaimsKey, claims))

	h.adminListAdmins(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var admins []any
	json.NewDecoder(w.Body).Decode(&admins)
	if len(admins) < 2 {
		t.Errorf("expected at least 2 admins, got %d", len(admins))
	}
}
