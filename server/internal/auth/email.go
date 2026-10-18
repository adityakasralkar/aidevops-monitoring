package auth

import (
	"errors"
	"net/mail"
	"strings"
)

// maxEmailLen is the longest email address allowed by the standards.
const maxEmailLen = 254

// ErrInvalidEmail is returned for an email address that is not valid.
var ErrInvalidEmail = errors.New("email address is not valid")

// NormalizeEmail trims spaces, converts to lowercase and checks the format.
// Lowercase means "Aditya@X.com" and "aditya@x.com" are the same account.
func NormalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || len(email) > maxEmailLen {
		return "", ErrInvalidEmail
	}

	// ParseAddress also accepts forms like "Name <a@b.com>"; requiring the
	// parsed address to equal the input allows only a plain address.
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return "", ErrInvalidEmail
	}

	// Require a dot in the domain (e.g. "x.com"), which rules out
	// addresses like "a@localhost" that cannot be real users.
	at := strings.LastIndex(email, "@")
	domain := email[at+1:]
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return "", ErrInvalidEmail
	}
	return email, nil
}
