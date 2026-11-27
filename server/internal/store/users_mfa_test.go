package store

import (
	"context"
	"testing"
	"time"

	"github.com/Teamthy/i-confess/internal/db/dbtest"
)

// TestAuthSessionRecordsMFAVerified covers §41/§42: a login session records
// whether it passed a second factor, and rotation preserves it.
func TestAuthSessionRecordsMFAVerified(t *testing.T) {
	db := dbtest.New(t)
	defer db.Close()
	users := NewUserStore(db)

	ctx := context.Background()
	u, err := users.Create(ctx, "mfa-store@test.com", "hash", "MFA Store", "UTC")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	userID := u.ID

	// Unverified session.
	unverifiedID, err := users.CreateAuthSession(ctx, userID, "web", "ua", "127.0.0.1", time.Hour, false)
	if err != nil {
		t.Fatalf("create unverified session: %v", err)
	}
	st, err := users.AuthSessionStatus(ctx, unverifiedID)
	if err != nil {
		t.Fatalf("status unverified: %v", err)
	}
	if !st.Exists || st.MFAVerified {
		t.Fatalf("unverified session marked verified: %+v", st)
	}

	// Verified session.
	verifiedID, err := users.CreateAuthSession(ctx, userID, "web", "ua", "127.0.0.1", time.Hour, true)
	if err != nil {
		t.Fatalf("create verified session: %v", err)
	}
	st, err = users.AuthSessionStatus(ctx, verifiedID)
	if err != nil {
		t.Fatalf("status verified: %v", err)
	}
	if !st.Exists || !st.MFAVerified {
		t.Fatalf("verified session not marked verified: %+v", st)
	}

	// Rotation preserves the MFA mark.
	rotatedID, err := users.RotateSession(ctx, userID, verifiedID, "web", "ua", "127.0.0.1", time.Hour)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	st, err = users.AuthSessionStatus(ctx, rotatedID)
	if err != nil {
		t.Fatalf("status rotated: %v", err)
	}
	if !st.Exists || !st.MFAVerified {
		t.Fatalf("rotated session lost MFA mark: %+v", st)
	}
}
