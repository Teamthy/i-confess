package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

// setupTestDB returns a fresh PostgreSQL database with the canonical schema
// loaded, dropped when the test ends. See internal/db/dbtest for why the suite
// runs against PostgreSQL rather than a lighter stand-in.
func setupTestDB(t *testing.T) *db.DB {
	return dbtest.New(t)
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
			// Registering an existing address must be indistinguishable from a
			// fresh signup. Returning 409 here made the endpoint an account
			// enumeration oracle (PRD S65); the real owner is emailed instead.
			"duplicate email is not disclosed",
			map[string]string{
				"email":    "user@example.com",
				"password": "password456",
			},
			http.StatusOK,
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
				_, hasToken := resp["token"]

				if resp["pending"] == true {
					// The duplicate-address path returns a neutral 200 so the
					// endpoint cannot be used to discover who has an account.
					// It must NOT return a token: that would sign the caller
					// into an account they do not own.
					if hasToken {
						t.Fatal("duplicate registration returned a session token")
					}
				} else if !hasToken {
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

func TestIdentityFlows(t *testing.T) {
	dbConn := setupTestDB(t)
	defer dbConn.Close()

	h := NewHandler(Config{
		JWTSecret: "test-secret",
		TokenTTL:  "24h",
	}, dbConn)
	h.BuildEngine()

	user, err := h.users.Create(context.Background(), "identity@example.com", "hash", "Identity User", "UTC")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := h.users.SetEmailVerified(context.Background(), user.ID, false); err != nil {
		t.Fatalf("mark unverified: %v", err)
	}
	if err := h.users.CreateVerificationToken(context.Background(), user.ID, "email", "verify-token", 60); err != nil {
		t.Fatalf("create verification token: %v", err)
	}
	if ok, err := h.users.VerifyEmailToken(context.Background(), user.ID, "verify-token"); err != nil || !ok {
		t.Fatalf("verify email token should succeed: ok=%v err=%v", ok, err)
	}

	if err := h.users.CreatePasswordReset(context.Background(), user.ID, "reset-token", 30); err != nil {
		t.Fatalf("create reset token: %v", err)
	}
	newHash, _ := auth.HashPassword("newSecret123")
	if err := h.users.ResetPassword(context.Background(), user.ID, "reset-token", newHash); err != nil {
		t.Fatalf("reset password: %v", err)
	}

	if _, err := h.users.CreateRefreshToken(context.Background(), "rt1", user.ID, "device-1", "ios"); err != nil {
		t.Fatalf("create refresh token: %v", err)
	}
	if _, err := h.users.CreateRefreshToken(context.Background(), "rt2", user.ID, "device-2", "android"); err != nil {
		t.Fatalf("create second refresh token: %v", err)
	}
	if _, err := h.users.RotateRefreshToken(context.Background(), "rt1", user.ID, "device-1", "ios"); err != nil {
		t.Fatalf("rotate refresh token: %v", err)
	}
	if err := h.users.RevokeAllSessions(context.Background(), user.ID); err != nil {
		t.Fatalf("revoke all sessions: %v", err)
	}
	if _, err := h.users.ListDevices(context.Background(), user.ID); err != nil {
		t.Fatalf("list devices should succeed: %v", err)
	}
}

func TestProfileAndPreferences(t *testing.T) {
	dbConn := setupTestDB(t)
	defer dbConn.Close()

	h := NewHandler(Config{JWTSecret: "test-secret", TokenTTL: "24h"}, dbConn)
	user, err := h.users.Create(context.Background(), "profile@example.com", "hash", "Profile User", "UTC")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := h.users.UpsertProfile(context.Background(), user.ID, "Profile User", "@profile", "Bio text", "https://cdn.example.com/avatar.png", "Africa/Lagos", "en-NG", "NG"); err != nil {
		t.Fatalf("upsert profile: %v", err)
	}
	if _, err := h.users.GetProfile(context.Background(), user.ID); err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if err := h.users.UpsertPreference(context.Background(), user.ID, "default_duration", "1800"); err != nil {
		t.Fatalf("set preference: %v", err)
	}
	if _, err := h.users.GetPreferences(context.Background(), user.ID); err != nil {
		t.Fatalf("get preferences: %v", err)
	}
	if err := h.users.UpsertInterest(context.Background(), user.ID, "cat-1", 0.8, "EXPLICIT_SELECTION"); err != nil {
		t.Fatalf("upsert interest: %v", err)
	}
	if _, err := h.users.ListInterests(context.Background(), user.ID); err != nil {
		t.Fatalf("list interests: %v", err)
	}
	if err := h.users.SetDefaultVoicePreference(context.Background(), user.ID, "voice-123"); err != nil {
		t.Fatalf("set default voice: %v", err)
	}
	if _, err := h.users.GetDefaultVoicePreference(context.Background(), user.ID); err != nil {
		t.Fatalf("get default voice: %v", err)
	}
}
