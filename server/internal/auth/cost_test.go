package auth

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func init() { hashCost = bcrypt.MinCost }

// The reduction must never reach production. Nothing outside a test binary
// sets the cost, and the package default stays at bcrypt.DefaultCost.
func TestProductionCostIsNotWeakened(t *testing.T) {
	if bcrypt.DefaultCost < 10 {
		t.Fatalf("bcrypt.DefaultCost is %d; too weak for password storage", bcrypt.DefaultCost)
	}
	// A hash produced at production cost must still verify, so the cost knob
	// cannot break existing stored passwords.
	SetHashCostForTesting(bcrypt.DefaultCost)
	h, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	SetHashCostForTesting(bcrypt.MinCost)
	if !CheckPassword(h, "correct horse battery staple") {
		t.Fatal("a hash created at production cost no longer verifies")
	}
}
