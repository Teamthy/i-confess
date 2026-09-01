package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestHashPassword(t *testing.T) {
	pw := "mySecurePassword123!"
	hash, err := HashPassword(pw)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if hash == "" {
		t.Fatal("hash should not be empty")
	}
	if hash == pw {
		t.Fatal("hash should not be plaintext")
	}
}

func TestCheckPassword(t *testing.T) {
	pw := "mySecurePassword123!"
	hash, _ := HashPassword(pw)

	tests := []struct {
		name   string
		input  string
		expect bool
	}{
		{"correct password", pw, true},
		{"wrong password", "wrongPassword123!", false},
		{"empty password", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CheckPassword(hash, tt.input); got != tt.expect {
				t.Errorf("CheckPassword(%s) = %v, want %v", tt.input, got, tt.expect)
			}
		})
	}
}

func TestSignToken(t *testing.T) {
	secret := "test-secret-key"
	ttl := "24h"
	sub := "user-123"
	email := "user@example.com"
	role := "content_admin"

	token, err := SignToken(secret, ttl, sub, email, role)
	if err != nil {
		t.Fatalf("SignToken failed: %v", err)
	}
	if token == "" {
		t.Fatal("token should not be empty")
	}

	// Verify it's a valid JWT
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (any, error) {
		return []byte(secret), nil
	})
	if err != nil {
		t.Fatalf("failed to parse token: %v", err)
	}
	if !parsed.Valid {
		t.Fatal("token should be valid")
	}
}

func TestSignTokenExpiration(t *testing.T) {
	secret := "test-secret-key"
	sub := "user-123"
	email := "user@example.com"

	token, err := SignToken(secret, "1s", sub, email, "")
	if err != nil {
		t.Fatalf("SignToken failed: %v", err)
	}

	time.Sleep(2 * time.Second)

	claims, err := ParseToken(secret, token)
	if err == nil {
		t.Fatal("ParseToken should fail for expired token")
	}
	if claims != nil {
		t.Fatal("claims should be nil for expired token")
	}
}

func TestParseToken(t *testing.T) {
	secret := "test-secret-key"
	ttl := "24h"
	sub := "user-123"
	email := "user@example.com"
	role := "content_admin"

	token, _ := SignToken(secret, ttl, sub, email, role)

	tests := []struct {
		name      string
		token     string
		secret    string
		expectErr bool
	}{
		{"valid token", token, secret, false},
		{"wrong secret", token, "wrong-secret", true},
		{"empty token", "", secret, true},
		{"malformed token", "not.a.token", secret, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims, err := ParseToken(tt.secret, tt.token)
			if (err != nil) != tt.expectErr {
				t.Errorf("ParseToken error = %v, expectErr %v", err, tt.expectErr)
			}
			if !tt.expectErr && (claims.Sub != sub || claims.Email != email || claims.Role != role) {
				t.Errorf("claims mismatch: got %+v", claims)
			}
		})
	}
}

func TestMiddleware(t *testing.T) {
	secret := "test-secret-key"
	sub := "user-123"
	email := "user@example.com"
	role := "user"

	token, _ := SignToken(secret, "24h", sub, email, role)

	tests := []struct {
		name           string
		authHeader     string
		expectStatus   int
		expectClaims   bool
	}{
		{"valid bearer token", "Bearer " + token, http.StatusOK, true},
		{"missing bearer prefix", token, http.StatusUnauthorized, false},
		{"empty auth header", "", http.StatusUnauthorized, false},
		{"invalid token", "Bearer invalid-token", http.StatusUnauthorized, false},
		{"wrong bearer format", "Basic " + token, http.StatusUnauthorized, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			middleware := Middleware(secret)
			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				claims := FromContext(r)
				if tt.expectClaims {
					if claims == nil {
						t.Fatal("expected claims but got nil")
					}
					if claims.Sub != sub {
						t.Errorf("sub mismatch: got %s, want %s", claims.Sub, sub)
					}
				} else {
					if claims != nil {
						t.Fatal("expected nil claims but got non-nil")
					}
				}
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("GET", "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != tt.expectStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.expectStatus)
			}
		})
	}
}

func TestAdminMiddleware(t *testing.T) {
	secret := "test-secret-key"

	userToken, _ := SignToken(secret, "24h", "user-123", "user@example.com", "")
	adminToken, _ := SignToken(secret, "24h", "admin-123", "admin@example.com", "content_admin")

	tests := []struct {
		name         string
		token        string
		expectStatus int
	}{
		{"admin with role", adminToken, http.StatusOK},
		{"user without role", userToken, http.StatusForbidden},
		{"no token", "", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			middleware := AdminMiddleware(secret)
			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("GET", "/admin", nil)
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != tt.expectStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.expectStatus)
			}
		})
	}
}

func TestFromContext(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)

	// Without claims
	if c := FromContext(req); c != nil {
		t.Fatal("expected nil claims when none set")
	}

	// With claims
	claims := &Claims{Sub: "user-123", Email: "user@example.com"}
	req = req.WithContext(req.Context())
	key := claimsKey
	req = req.WithContext(context.WithValue(req.Context(), key, claims))
	if c := FromContext(req); c == nil || c.Sub != claims.Sub {
		t.Fatal("claims not properly stored in context")
	}
}

func TestTokenRoleValidation(t *testing.T) {
	secret := "test-secret-key"

	tests := []struct {
		name     string
		role     string
		isAdmin  bool
	}{
		{"empty role", "", false},
		{"user role", "user", false},
		{"super_admin role", "super_admin", true},
		{"content_admin role", "content_admin", true},
		{"audio_producer role", "audio_producer", true},
		{"theological_reviewer role", "theological_reviewer", true},
		{"support_admin role", "support_admin", true},
		{"analytics_admin role", "analytics_admin", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, _ := SignToken(secret, "24h", "user-123", "test@example.com", tt.role)
			claims, _ := ParseToken(secret, token)
			
			isAdmin := claims.Role != ""
			if isAdmin != tt.isAdmin {
				t.Errorf("isAdmin = %v, want %v for role %q", isAdmin, tt.isAdmin, tt.role)
			}
		})
	}
}
