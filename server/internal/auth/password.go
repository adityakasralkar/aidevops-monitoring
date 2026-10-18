// Package auth holds the security helpers: password hashing, session
// tokens, API keys and email checks. It has no database or HTTP code.
package auth

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// Password length limits. bcrypt only uses the first 72 bytes of a
// password, so longer ones are rejected instead of being silently cut.
const (
	MinPasswordLen = 8
	MaxPasswordLen = 72
)

// bcryptCost controls how slow hashing is. Each +1 doubles the work.
// 12 takes roughly 200–300 ms, which is slow for an attacker guessing
// millions of passwords but fine for one login.
const bcryptCost = 12

// ErrPasswordMismatch is returned when a password does not match its hash.
var ErrPasswordMismatch = errors.New("password does not match")

// ValidatePassword checks the password length rules.
// The length is counted in bytes, because that is what bcrypt limits.
func ValidatePassword(password string) error {
	n := len(password)
	if n < MinPasswordLen || n > MaxPasswordLen {
		return fmt.Errorf("password must be between %d and %d characters", MinPasswordLen, MaxPasswordLen)
	}
	return nil
}

// HashPassword returns the bcrypt hash of password. The hash includes a
// random salt, so the same password gives a different hash every time.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// CheckPassword compares password with a bcrypt hash.
// It returns ErrPasswordMismatch when they do not match.
func CheckPassword(hash, password string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return ErrPasswordMismatch
	}
	if err != nil {
		return fmt.Errorf("check password: %w", err)
	}
	return nil
}

// dummyHash is a real bcrypt hash of a random value. When a login uses an
// email that does not exist, the password is still checked against this
// hash, so the response takes the same time as for a real account.
// Otherwise an attacker could tell which emails are registered by timing.
var dummyHash = mustHash("dummy-password-for-timing-only")

// CheckDummyPassword does the same amount of work as CheckPassword and
// always fails. Use it when the account does not exist.
func CheckDummyPassword(password string) {
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
}

func mustHash(s string) []byte {
	h, err := bcrypt.GenerateFromPassword([]byte(s), bcryptCost)
	if err != nil {
		panic(fmt.Sprintf("auth: create dummy hash: %v", err))
	}
	return h
}
