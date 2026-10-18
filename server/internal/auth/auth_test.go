package auth

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"accepts 8 characters", "abcd1234", false},
		{"accepts 72 bytes", strings.Repeat("a", 72), false},
		{"accepts unicode within the byte limit", "pässwörd-ok", false},
		{"rejects an empty password", "", true},
		{"rejects 7 characters", "abc1234", true},
		{"rejects 73 bytes", strings.Repeat("a", 73), true},
		{"counts bytes, not letters", strings.Repeat("é", 37), true}, // 74 bytes
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePassword() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHashAndCheckPassword(t *testing.T) {
	const password = "correct horse battery"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	t.Run("hash does not contain the password", func(t *testing.T) {
		if strings.Contains(hash, password) {
			t.Error("hash contains the plain password")
		}
	})
	t.Run("same password gives a different hash each time", func(t *testing.T) {
		hash2, err := HashPassword(password)
		if err != nil {
			t.Fatalf("hash: %v", err)
		}
		if hash == hash2 {
			t.Error("two hashes are identical; salt is missing")
		}
	})
	t.Run("accepts the right password", func(t *testing.T) {
		if err := CheckPassword(hash, password); err != nil {
			t.Errorf("CheckPassword() = %v, want nil", err)
		}
	})
	t.Run("rejects a wrong password", func(t *testing.T) {
		if err := CheckPassword(hash, "wrong password"); !errors.Is(err, ErrPasswordMismatch) {
			t.Errorf("CheckPassword() = %v, want ErrPasswordMismatch", err)
		}
	})
	t.Run("returns a non-mismatch error for a corrupt hash", func(t *testing.T) {
		err := CheckPassword("not-a-bcrypt-hash", password)
		if err == nil || errors.Is(err, ErrPasswordMismatch) {
			t.Errorf("CheckPassword() = %v, want a different error", err)
		}
	})
	t.Run("dummy check does not panic", func(t *testing.T) {
		CheckDummyPassword(password)
	})
}

func TestSessionToken(t *testing.T) {
	seen := make(map[string]bool)
	for range 100 {
		tok := NewSessionToken()
		if len(tok) != 43 { // 32 bytes in unpadded base64url
			t.Fatalf("token length = %d, want 43", len(tok))
		}
		if seen[tok] {
			t.Fatal("duplicate token generated")
		}
		seen[tok] = true
	}
}

func TestHashToken(t *testing.T) {
	h1 := HashToken("abc")
	if h1 != HashToken("abc") {
		t.Error("same input gave different hashes")
	}
	if h1 == HashToken("abd") {
		t.Error("different inputs gave the same hash")
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(h1) {
		t.Errorf("hash %q is not 64 hex characters", h1)
	}
}

func TestNewAPIKey(t *testing.T) {
	for _, env := range []string{EnvLive, EnvTest} {
		t.Run("creates a valid "+env+" key", func(t *testing.T) {
			k, err := NewAPIKey(env)
			if err != nil {
				t.Fatalf("NewAPIKey: %v", err)
			}
			if !regexp.MustCompile(`^aidv_` + env + `_[A-Za-z0-9_-]{43}$`).MatchString(k.Key) {
				t.Errorf("key %q has the wrong format", k.Key)
			}
			if !strings.HasPrefix(k.Key, k.Prefix) || len(k.Prefix) != len("aidv_"+env+"_")+4 {
				t.Errorf("prefix %q does not match key %q", k.Prefix, k.Key)
			}
			if k.Hash != HashToken(k.Key) {
				t.Error("hash does not match the key")
			}
		})
	}
	t.Run("rejects an unknown environment", func(t *testing.T) {
		if _, err := NewAPIKey("prod"); err == nil {
			t.Error("expected an error, got nil")
		}
	})
	t.Run("keys are unique", func(t *testing.T) {
		a, _ := NewAPIKey(EnvTest)
		b, _ := NewAPIKey(EnvTest)
		if a.Key == b.Key {
			t.Error("two keys are identical")
		}
	})
}

func TestNormalizeEmail(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // empty means an error is expected
	}{
		{"keeps a plain address", "user@example.com", "user@example.com"},
		{"lowercases and trims", "  User@Example.COM \n", "user@example.com"},
		{"accepts plus and dots", "first.last+tag@mail.example.co.in", "first.last+tag@mail.example.co.in"},
		{"rejects empty", "", ""},
		{"rejects spaces only", "   ", ""},
		{"rejects missing @", "user.example.com", ""},
		{"rejects missing local part", "@example.com", ""},
		{"rejects a domain without a dot", "user@localhost", ""},
		{"rejects a domain ending in a dot", "user@example.", ""},
		{"rejects a display name", "Aditya <user@example.com>", ""},
		{"rejects two @", "a@b@example.com", ""},
		{"rejects spaces inside", "us er@example.com", ""},
		{"rejects over 254 characters", strings.Repeat("a", 250) + "@x.com", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeEmail(tt.input)
			if tt.want == "" {
				if !errors.Is(err, ErrInvalidEmail) {
					t.Errorf("NormalizeEmail(%q) = %q, %v; want ErrInvalidEmail", tt.input, got, err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("NormalizeEmail(%q) = %q, %v; want %q", tt.input, got, err, tt.want)
			}
		})
	}
}
