package offline

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

// Seal encrypts plaintext with key (32 bytes for AES-256) using AES-GCM.
// The returned string is base64(nonce+ciphertext). Metadata at rest is never plaintext.
func Seal(key, plaintext []byte) (string, error) {
	if len(key) != 32 {
		return "", errors.New("offline: key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// Open decrypts a Seal payload.
func Open(key []byte, sealed string) ([]byte, error) {
	if len(key) != 32 {
		return nil, errors.New("offline: key must be 32 bytes")
	}
	ct, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ct) < gcm.NonceSize() {
		return nil, errors.New("offline: ciphertext too short")
	}
	nonce, ciphertext := ct[:gcm.NonceSize()], ct[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertext, nil)
}
