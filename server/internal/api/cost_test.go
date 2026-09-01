package api

import (
	"testing"

	"github.com/Teamthy/i-confess/internal/auth"
	"golang.org/x/crypto/bcrypt"
)

// This suite registers hundreds of accounts. At production bcrypt cost the
// race-enabled run takes over five minutes, which is long enough that people
// stop running it — a slow security check is one that does not get run.
func TestMain(m *testing.M) {
	auth.SetHashCostForTesting(bcrypt.MinCost)
	m.Run()
}
