package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// tokenBytes is the amount of randomness in session tokens and API keys.
// 32 bytes (256 bits) cannot be guessed.
const tokenBytes = 32

// NewSessionToken returns a new random session token for the client.
// Only its hash (see HashToken) is stored in the database.
func NewSessionToken() string {
	return randomString()
}

// HashToken returns the SHA-256 hash of a session token or API key, as hex.
// A fast hash is fine here (unlike passwords) because the token is fully
// random, so there is nothing to guess from a dictionary.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// randomString returns tokenBytes of secure random data, URL-safe encoded.
func randomString() string {
	b := make([]byte, tokenBytes)
	// crypto/rand.Read never fails on supported platforms (Go 1.24+);
	// it crashes the program instead of returning weak randomness.
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
