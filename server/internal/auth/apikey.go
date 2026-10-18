package auth

import "fmt"

// Project environments. They appear inside every API key, so a test key
// can never be mixed up with a live one.
const (
	EnvLive = "live"
	EnvTest = "test"
)

// keyPrefixLen is how many random characters are kept in the display
// prefix, e.g. "aidv_test_7Kq9". Enough to recognise a key, useless to an attacker.
const keyPrefixLen = 4

// APIKey is a newly generated API key.
type APIKey struct {
	Key    string // full key, shown to the user once and never stored
	Hash   string // SHA-256 hash, stored in the database
	Prefix string // short readable start of the key, stored for display
}

// ValidEnvironment reports whether env is "live" or "test".
func ValidEnvironment(env string) bool {
	return env == EnvLive || env == EnvTest
}

// NewAPIKey creates a key in the format aidv_<env>_<random>.
func NewAPIKey(env string) (APIKey, error) {
	if !ValidEnvironment(env) {
		return APIKey{}, fmt.Errorf("invalid environment %q", env)
	}
	start := "aidv_" + env + "_"
	random := randomString()
	key := start + random
	return APIKey{
		Key:    key,
		Hash:   HashToken(key),
		Prefix: start + random[:keyPrefixLen],
	}, nil
}
