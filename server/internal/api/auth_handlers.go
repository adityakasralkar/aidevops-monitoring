package api

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/auth"
	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

// sessionTTL is how long a login stays valid.
const sessionTTL = 24 * time.Hour

// credentials is the request body for signup and login.
type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// userResponse is how a user is shown to clients. It never includes the
// password hash.
type userResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

func toUserResponse(u model.User) userResponse {
	return userResponse{ID: u.ID.Hex(), Email: u.Email, CreatedAt: u.CreatedAt}
}

// loginResponse is returned by a successful login.
type loginResponse struct {
	Token     string       `json:"token"`
	TokenType string       `json:"token_type"`
	ExpiresAt time.Time    `json:"expires_at"`
	User      userResponse `json:"user"`
}

// handleSignup creates a new account.
// 201 on success · 400 bad input · 409 email taken · 429 too many signups.
func (s *server) handleSignup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ip := clientIP(r)

	if blocked, retry := s.signupByIP.blocked(ip); blocked {
		writeTooManyRequests(w, retry)
		return
	}

	var req credentials
	if !decodeJSON(w, r, &req) {
		return
	}
	// Count every signup attempt that reaches this point, successful or not,
	// so one client cannot create accounts in bulk.
	s.signupByIP.hit(ip)

	email, err := auth.NormalizeEmail(req.Email)
	if err != nil {
		writeError(w, http.StatusBadRequest, codeValidationFailed, "email address is not valid")
		return
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, codeValidationFailed, err.Error())
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.Logger.ErrorContext(ctx, "signup: hash password", "error", err)
		writeInternalError(w)
		return
	}

	user := model.User{Email: email, PasswordHash: hash, CreatedAt: s.now()}
	err = s.Users.CreateUser(ctx, &user)
	if errors.Is(err, model.ErrEmailTaken) {
		writeError(w, http.StatusConflict, codeEmailTaken, "an account with this email already exists")
		return
	}
	if err != nil {
		s.Logger.ErrorContext(ctx, "signup: create user", "error", err)
		writeInternalError(w)
		return
	}

	s.Logger.InfoContext(ctx, "user signed up", "user_id", user.ID.Hex())
	writeJSON(w, http.StatusCreated, toUserResponse(user))
}

// handleLogin checks email + password and returns a new session token.
// 200 on success · 400 bad input · 401 wrong email or password · 429 too many failures.
//
// Wrong email and wrong password give the same response and take the same
// time, so an attacker cannot find out which emails are registered.
func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ip := clientIP(r)

	var req credentials
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, codeValidationFailed, "email and password are required")
		return
	}

	// An invalid email cannot belong to an account; it is still treated as
	// a normal failed login so the response gives nothing away. All invalid
	// emails share one rate-limit key, so junk input cannot fill memory.
	email, emailErr := auth.NormalizeEmail(req.Email)
	emailKey := "email:" + email
	if emailErr != nil {
		emailKey = "email:<invalid>"
	}

	if blocked, retry := s.loginBlocked(ip, emailKey); blocked {
		s.Logger.WarnContext(ctx, "login rate limited", "ip", ip)
		writeTooManyRequests(w, retry)
		return
	}

	// No account can have a password over the limit (signup rejects it), and
	// bcrypt would silently compare only the first 72 bytes, so treat it as
	// a failed login without looking up the user.
	tooLong := len(req.Password) > auth.MaxPasswordLen

	var user model.User
	var err error
	if emailErr == nil && !tooLong {
		user, err = s.Users.UserByEmail(ctx, email)
	}
	switch {
	case emailErr != nil || tooLong || errors.Is(err, model.ErrNotFound):
		auth.CheckDummyPassword(req.Password) // same time cost as a real check
		s.loginFailed(w, r, ip, emailKey)
		return
	case err != nil:
		s.Logger.ErrorContext(ctx, "login: find user", "error", err)
		writeInternalError(w)
		return
	}

	err = auth.CheckPassword(user.PasswordHash, req.Password)
	if errors.Is(err, auth.ErrPasswordMismatch) {
		s.loginFailed(w, r, ip, emailKey)
		return
	}
	if err != nil {
		s.Logger.ErrorContext(ctx, "login: check password", "error", err, "user_id", user.ID.Hex())
		writeInternalError(w)
		return
	}

	// Success: forget earlier failures for this account and start a session.
	s.loginByEmail.reset(emailKey)

	token := auth.NewSessionToken()
	now := s.now()
	sess := model.Session{
		UserID:    user.ID,
		TokenHash: auth.HashToken(token),
		CreatedAt: now,
		ExpiresAt: now.Add(sessionTTL),
	}
	if err := s.Sessions.CreateSession(ctx, &sess); err != nil {
		s.Logger.ErrorContext(ctx, "login: create session", "error", err, "user_id", user.ID.Hex())
		writeInternalError(w)
		return
	}

	s.Logger.InfoContext(ctx, "user logged in", "user_id", user.ID.Hex())
	w.Header().Set("Cache-Control", "no-store") // the response contains a secret
	writeJSON(w, http.StatusOK, loginResponse{
		Token:     token,
		TokenType: "Bearer",
		ExpiresAt: sess.ExpiresAt,
		User:      toUserResponse(user),
	})
}

// loginBlocked reports whether either the IP or the email has too many
// recent failed logins, and the longer of the two waits.
func (s *server) loginBlocked(ip, emailKey string) (bool, time.Duration) {
	ipBlocked, ipRetry := s.loginByIP.blocked(ip)
	emailBlocked, emailRetry := s.loginByEmail.blocked(emailKey)
	return ipBlocked || emailBlocked, max(ipRetry, emailRetry)
}

// loginFailed records a failed attempt and sends the standard 401.
func (s *server) loginFailed(w http.ResponseWriter, r *http.Request, ip, emailKey string) {
	s.loginByIP.hit(ip)
	s.loginByEmail.hit(emailKey)
	s.Logger.InfoContext(r.Context(), "login failed", "ip", ip)
	writeError(w, http.StatusUnauthorized, codeInvalidCredentials, "invalid email or password")
}

// handleLogout ends the current session. 204 on success.
func (s *server) handleLogout(w http.ResponseWriter, r *http.Request) {
	a := currentAuth(r)
	if err := s.Sessions.DeleteSession(r.Context(), a.TokenHash); err != nil {
		s.Logger.ErrorContext(r.Context(), "logout: delete session", "error", err, "user_id", a.User.ID.Hex())
		writeInternalError(w)
		return
	}
	s.Logger.InfoContext(r.Context(), "user logged out", "user_id", a.User.ID.Hex())
	w.WriteHeader(http.StatusNoContent)
}

// handleMe returns the logged-in user's account.
func (s *server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, toUserResponse(currentAuth(r).User))
}

// writeTooManyRequests sends 429 with a Retry-After header in whole seconds.
func writeTooManyRequests(w http.ResponseWriter, retry time.Duration) {
	secs := int(math.Ceil(retry.Seconds()))
	w.Header().Set("Retry-After", strconv.Itoa(max(secs, 1)))
	writeError(w, http.StatusTooManyRequests, codeTooManyRequests, "too many attempts; please try again later")
}
