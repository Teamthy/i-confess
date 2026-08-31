package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var ErrUnauthorized = errors.New("unauthorized")

// Claims are the JWT claims issued to authenticated users and admins.
type Claims struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Role  string `json:"role,omitempty"` // admin role if present, else ""
	jwt.RegisteredClaims
}

func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

func SignToken(secret, ttl, sub, email, role string) (string, error) {
	d, err := time.ParseDuration(ttl)
	if err != nil {
		d = 720 * time.Hour
	}
	claims := Claims{
		Sub:   sub,
		Email: email,
		Role:  role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(d)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "i-confess",
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func ParseToken(secret, tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return nil, ErrUnauthorized
	}
	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, ErrUnauthorized
	}
	return claims, nil
}

// Middleware returns an http middleware that requires a valid bearer token.
func Middleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := fromRequest(secret, r)
			if err != nil {
				httpx.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			next.ServeHTTP(w, withClaims(r, c))
		})
	}
}

// AdminMiddleware requires a valid token AND an admin role.
func AdminMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := fromRequest(secret, r)
			if err != nil || c.Role == "" {
				httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "admin access required"})
				return
			}
			next.ServeHTTP(w, withClaims(r, c))
		})
	}
}

func fromRequest(secret string, r *http.Request) (*Claims, error) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return nil, ErrUnauthorized
	}
	return ParseToken(secret, strings.TrimPrefix(h, "Bearer "))
}

type ctxKey int

const claimsKey ctxKey = 0

func withClaims(r *http.Request, c *Claims) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), claimsKey, c))
}

// FromContext returns the claims attached by the middleware (nil if absent).
func FromContext(r *http.Request) *Claims {
	c, _ := r.Context().Value(claimsKey).(*Claims)
	return c
}
