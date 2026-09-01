package push

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"math/big"
)

// Thin wrappers kept separate so the signing path in providers.go reads as
// intent rather than as crypto plumbing.

func sha256Sum(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}

func ecdsaSign(key *ecdsa.PrivateKey, digest []byte) (r, s *big.Int, err error) {
	return ecdsa.Sign(rand.Reader, key, digest)
}
