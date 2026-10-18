package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/auth"
	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

// maxTokenLen rejects absurdly long Authorization values before hashing
// them. Real session tokens are 43 characters.
const maxTokenLen = 256

// authInfo is what requireUser stores in the request context.
type authInfo struct {
	User      model.User
	TokenHash string
}

// authKey is the context key for authInfo. An unexported type means no
// other package can read or overwrite this value by accident.
type authKey struct{}

// currentAuth returns the logged-in user's info. Only call it inside a
// handler wrapped by requireUser.
func currentAuth(r *http.Request) authInfo {
	return r.Context().Value(authKey{}).(authInfo)
}

// requireUser only lets the request through if it carries a valid,
// unexpired session token ("Authorization: Bearer <token>"). Otherwise it
// responds 401 and the wrapped handler never runs.
func (s *server) requireUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer`)
			writeError(w, http.StatusUnauthorized, codeMissingToken, "missing or malformed Authorization header; use: Bearer <token>")
			return
		}

		ctx := r.Context()
		tokenHash := auth.HashToken(token)
		sess, err := s.Sessions.SessionByTokenHash(ctx, tokenHash, s.Now())
		if errors.Is(err, model.ErrNotFound) {
			writeInvalidToken(w)
			return
		}
		if err != nil {
			s.Logger.ErrorContext(ctx, "auth: find session", "error", err)
			writeInternalError(w)
			return
		}

		user, err := s.Users.UserByID(ctx, sess.UserID)
		if errors.Is(err, model.ErrNotFound) {
			// The session points at a user that no longer exists.
			writeInvalidToken(w)
			return
		}
		if err != nil {
			s.Logger.ErrorContext(ctx, "auth: find user", "error", err)
			writeInternalError(w)
			return
		}

		ctx = context.WithValue(ctx, authKey{}, authInfo{User: user, TokenHash: tokenHash})
		next(w, r.WithContext(ctx))
	}
}

func writeInvalidToken(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
	writeError(w, http.StatusUnauthorized, codeInvalidToken, "session is invalid or has expired; please log in again")
}

// bearerToken extracts the token from "Authorization: Bearer <token>".
// The word "Bearer" is case-insensitive, as the HTTP standard says.
func bearerToken(r *http.Request) (string, bool) {
	scheme, token, found := strings.Cut(r.Header.Get("Authorization"), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	if token == "" || len(token) > maxTokenLen {
		return "", false
	}
	return token, true
}

// clientIP returns the IP address of the client, used for rate limiting.
//
// It uses the TCP connection's address only. Headers such as
// X-Forwarded-For are ignored because any client can fake them; they
// should only be trusted when the server runs behind a known proxy.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
