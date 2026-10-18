package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/auth"
)

const (
	testEmail    = "user@example.com"
	testPassword = "s3cure-Passw0rd"
)

var errDB = errors.New("db exploded at mongo-host:27017")

func TestSignup(t *testing.T) {
	t.Parallel()

	t.Run("creates an account and never returns the password hash", func(t *testing.T) {
		e := newTestEnv(t)
		rec := e.do(t, request{method: "POST", path: "/api/v1/auth/signup", body: jsonBody(t, credentials{Email: "  New.User@Example.COM ", Password: testPassword})})
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		var got userResponse
		decodeBody(t, rec, &got)
		if got.Email != "new.user@example.com" || got.ID == "" || !got.CreatedAt.Equal(e.clock.Now()) {
			t.Errorf("response = %+v", got)
		}
		if strings.Contains(rec.Body.String(), "password") || strings.Contains(rec.Body.String(), "$2a$") {
			t.Errorf("response leaks password data: %s", rec.Body)
		}

		stored, err := e.store.UserByEmail(t.Context(), "new.user@example.com")
		if err != nil {
			t.Fatalf("user not stored: %v", err)
		}
		if stored.PasswordHash == testPassword || auth.CheckPassword(stored.PasswordHash, testPassword) != nil {
			t.Error("password not stored as a valid bcrypt hash")
		}
	})

	t.Run("rejects an email that is already registered, ignoring case", func(t *testing.T) {
		e := newTestEnv(t)
		e.signup(t, testEmail, testPassword)
		rec := e.do(t, request{method: "POST", path: "/api/v1/auth/signup", body: jsonBody(t, credentials{Email: "USER@example.com", Password: "another-password"})})
		wantError(t, rec, http.StatusConflict, codeEmailTaken)
	})

	validation := []struct {
		name     string
		email    string
		password string
	}{
		{"rejects an invalid email", "not-an-email", testPassword},
		{"rejects an empty email", "", testPassword},
		{"rejects a short password", testEmail, "short"},
		{"rejects an empty password", testEmail, ""},
		{"rejects a password over 72 bytes", testEmail, strings.Repeat("x", 73)},
	}
	for _, tt := range validation {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEnv(t)
			rec := e.do(t, request{method: "POST", path: "/api/v1/auth/signup", body: jsonBody(t, credentials{Email: tt.email, Password: tt.password})})
			wantError(t, rec, http.StatusBadRequest, codeValidationFailed)
		})
	}

	t.Run("returns 500 without leaking details when the store fails", func(t *testing.T) {
		e := newTestEnv(t)
		e.store.setFail(errDB)
		rec := e.do(t, request{method: "POST", path: "/api/v1/auth/signup", body: jsonBody(t, credentials{Email: testEmail, Password: testPassword})})
		wantError(t, rec, http.StatusInternalServerError, codeInternalError)
		if strings.Contains(rec.Body.String(), "mongo-host") {
			t.Errorf("internal error leaked: %s", rec.Body)
		}
		if !strings.Contains(e.logs.String(), "mongo-host") {
			t.Error("internal error was not logged")
		}
	})

	t.Run("limits signups per IP and recovers after the window", func(t *testing.T) {
		e := newTestEnv(t)
		for i := range signupsPerIP {
			// Invalid passwords keep the test fast (no bcrypt) but still count.
			rec := e.do(t, request{method: "POST", path: "/api/v1/auth/signup", ip: "203.0.113.9", body: jsonBody(t, credentials{Email: fmt.Sprintf("u%d@example.com", i), Password: "x"})})
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("attempt %d: status %d", i+1, rec.Code)
			}
		}
		rec := e.do(t, request{method: "POST", path: "/api/v1/auth/signup", ip: "203.0.113.9", body: jsonBody(t, credentials{Email: testEmail, Password: testPassword})})
		wantError(t, rec, http.StatusTooManyRequests, codeTooManyRequests)
		if ra, _ := strconv.Atoi(rec.Header().Get("Retry-After")); ra < 1 || ra > int(signupWindow.Seconds()) {
			t.Errorf("Retry-After = %q", rec.Header().Get("Retry-After"))
		}

		// A different IP is not affected.
		other := e.do(t, request{method: "POST", path: "/api/v1/auth/signup", ip: "203.0.113.10", body: jsonBody(t, credentials{Email: testEmail, Password: testPassword})})
		if other.Code != http.StatusCreated {
			t.Errorf("other IP: status %d", other.Code)
		}

		e.clock.Advance(signupWindow)
		again := e.do(t, request{method: "POST", path: "/api/v1/auth/signup", ip: "203.0.113.9", body: jsonBody(t, credentials{Email: "later@example.com", Password: testPassword})})
		if again.Code != http.StatusCreated {
			t.Errorf("after window: status %d", again.Code)
		}
	})
}

func TestLogin(t *testing.T) {
	t.Parallel()

	loginReq := func(email, password, ip string) request {
		return request{method: "POST", path: "/api/v1/auth/login", ip: ip, body: fmt.Sprintf(`{"email":%q,"password":%q}`, email, password)}
	}

	t.Run("returns a token that is stored only as a hash", func(t *testing.T) {
		e := newTestEnv(t)
		e.signup(t, testEmail, testPassword)
		rec := e.do(t, loginReq("  USER@example.com ", testPassword, ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Error("missing Cache-Control: no-store")
		}
		var got loginResponse
		decodeBody(t, rec, &got)
		if got.Token == "" || got.TokenType != "Bearer" || got.User.Email != testEmail {
			t.Errorf("response = %+v", got)
		}
		if !got.ExpiresAt.Equal(e.clock.Now().Add(sessionTTL)) {
			t.Errorf("expires_at = %s, want now + %s", got.ExpiresAt, sessionTTL)
		}
		if _, ok := e.store.sessions[got.Token]; ok {
			t.Error("plain token stored as session key")
		}
		if _, ok := e.store.sessions[auth.HashToken(got.Token)]; !ok {
			t.Error("session not stored under the token hash")
		}
	})

	t.Run("gives the same answer for a wrong password and an unknown email", func(t *testing.T) {
		e := newTestEnv(t)
		e.signup(t, testEmail, testPassword)
		wrongPass := e.do(t, loginReq(testEmail, "wrong-password", "192.0.2.50"))
		unknown := e.do(t, loginReq("nobody@example.com", testPassword, "192.0.2.51"))
		invalid := e.do(t, loginReq("not-an-email", testPassword, "192.0.2.52"))
		wantError(t, wrongPass, http.StatusUnauthorized, codeInvalidCredentials)
		wantError(t, unknown, http.StatusUnauthorized, codeInvalidCredentials)
		wantError(t, invalid, http.StatusUnauthorized, codeInvalidCredentials)
		if wrongPass.Body.String() != unknown.Body.String() || unknown.Body.String() != invalid.Body.String() {
			t.Errorf("responses differ:\n%s\n%s\n%s", wrongPass.Body, unknown.Body, invalid.Body)
		}
	})

	t.Run("takes similar time for an unknown email and a wrong password", func(t *testing.T) {
		e := newTestEnv(t)
		e.signup(t, testEmail, testPassword)
		measure := func(email, ip string) time.Duration {
			start := time.Now()
			e.do(t, loginReq(email, "wrong-password", ip))
			return time.Since(start)
		}
		known := measure(testEmail, "192.0.2.60")
		unknown := measure("nobody@example.com", "192.0.2.61")
		// bcrypt dominates both; the unknown-email path must not be much faster.
		if unknown < known/3 {
			t.Errorf("unknown email took %s vs %s for a known one; timing reveals accounts", unknown, known)
		}
	})

	t.Run("rejects a password over 72 bytes even if its first 72 bytes match", func(t *testing.T) {
		e := newTestEnv(t)
		long := strings.Repeat("p", 72)
		e.signup(t, testEmail, long)
		wantError(t, e.do(t, loginReq(testEmail, long+"extra", "")), http.StatusUnauthorized, codeInvalidCredentials)
		if rec := e.do(t, loginReq(testEmail, long, "")); rec.Code != http.StatusOK {
			t.Errorf("exact password: status %d", rec.Code)
		}
	})

	t.Run("requires email and password", func(t *testing.T) {
		e := newTestEnv(t)
		wantError(t, e.do(t, loginReq("", testPassword, "")), http.StatusBadRequest, codeValidationFailed)
		wantError(t, e.do(t, loginReq(testEmail, "", "")), http.StatusBadRequest, codeValidationFailed)
	})

	t.Run("returns 500 when the store fails", func(t *testing.T) {
		e := newTestEnv(t)
		e.signup(t, testEmail, testPassword)
		e.store.setFail(errDB)
		wantError(t, e.do(t, loginReq(testEmail, testPassword, "")), http.StatusInternalServerError, codeInternalError)
	})

	t.Run("blocks an account after repeated failures, from any IP", func(t *testing.T) {
		e := newTestEnv(t)
		e.signup(t, testEmail, testPassword)
		for i := range loginFailuresPerEmail {
			ip := fmt.Sprintf("192.0.2.%d", 100+i) // different IP each time
			wantError(t, e.do(t, loginReq(testEmail, "wrong-password", ip)), http.StatusUnauthorized, codeInvalidCredentials)
		}
		// Even the right password is refused while blocked.
		rec := e.do(t, loginReq(testEmail, testPassword, "192.0.2.200"))
		wantError(t, rec, http.StatusTooManyRequests, codeTooManyRequests)
		if rec.Header().Get("Retry-After") == "" {
			t.Error("missing Retry-After header")
		}

		e.clock.Advance(loginWindow)
		if rec := e.do(t, loginReq(testEmail, testPassword, "192.0.2.200")); rec.Code != http.StatusOK {
			t.Errorf("after window: status %d", rec.Code)
		}
	})

	t.Run("a successful login clears earlier failures for the account", func(t *testing.T) {
		e := newTestEnv(t)
		e.signup(t, testEmail, testPassword)
		for range loginFailuresPerEmail - 1 {
			e.do(t, loginReq(testEmail, "wrong-password", "192.0.2.70"))
		}
		if rec := e.do(t, loginReq(testEmail, testPassword, "192.0.2.70")); rec.Code != http.StatusOK {
			t.Fatalf("login: status %d", rec.Code)
		}
		for range loginFailuresPerEmail - 1 {
			e.do(t, loginReq(testEmail, "wrong-password", "192.0.2.70"))
		}
		if rec := e.do(t, loginReq(testEmail, testPassword, "192.0.2.70")); rec.Code != http.StatusOK {
			t.Errorf("counter was not reset: status %d", rec.Code)
		}
	})

	t.Run("blocks an IP that tries many accounts", func(t *testing.T) {
		e := newTestEnv(t)
		e.signup(t, testEmail, testPassword)
		for i := range loginFailuresPerIP {
			// A different, unknown account each time, all from one IP.
			e.do(t, loginReq(fmt.Sprintf("victim%d@example.com", i), "wrong-password", "203.0.113.50"))
		}
		rec := e.do(t, loginReq(testEmail, testPassword, "203.0.113.50"))
		wantError(t, rec, http.StatusTooManyRequests, codeTooManyRequests)

		// The same account from another IP still works.
		if rec := e.do(t, loginReq(testEmail, testPassword, "203.0.113.51")); rec.Code != http.StatusOK {
			t.Errorf("other IP: status %d", rec.Code)
		}
	})

	t.Run("never logs the password or email", func(t *testing.T) {
		e := newTestEnv(t)
		e.signup(t, testEmail, testPassword)
		e.do(t, loginReq(testEmail, "wrong-password", ""))
		e.login(t, testEmail, testPassword)
		logs := e.logs.String()
		for _, secret := range []string{testPassword, "wrong-password", testEmail} {
			if strings.Contains(logs, secret) {
				t.Errorf("logs contain %q:\n%s", secret, logs)
			}
		}
	})
}

func TestRequireUser(t *testing.T) {
	t.Parallel()

	t.Run("rejects requests without a valid Authorization header", func(t *testing.T) {
		e := newTestEnv(t)
		tests := []struct {
			name   string
			header string
		}{
			{"no header", ""},
			{"wrong scheme", "Basic dXNlcjpwYXNz"},
			{"bearer without token", "Bearer "},
			{"token without scheme", "sometoken"},
			{"token too long", "Bearer " + strings.Repeat("a", maxTokenLen+1)},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				rec := e.do(t, request{method: "GET", path: "/api/v1/auth/me", header: map[string]string{"Authorization": tt.header}})
				wantError(t, rec, http.StatusUnauthorized, codeMissingToken)
				if rec.Header().Get("WWW-Authenticate") == "" {
					t.Error("missing WWW-Authenticate header")
				}
			})
		}
	})

	t.Run("rejects an unknown token", func(t *testing.T) {
		e := newTestEnv(t)
		wantError(t, e.do(t, request{method: "GET", path: "/api/v1/auth/me", token: "made-up-token"}), http.StatusUnauthorized, codeInvalidToken)
	})

	t.Run("accepts the scheme in any letter case", func(t *testing.T) {
		e := newTestEnv(t)
		e.signup(t, testEmail, testPassword)
		token := e.login(t, testEmail, testPassword)
		rec := e.do(t, request{method: "GET", path: "/api/v1/auth/me", header: map[string]string{"Authorization": "bearer " + token}})
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d", rec.Code)
		}
	})

	t.Run("rejects a token once the session has expired", func(t *testing.T) {
		e := newTestEnv(t)
		e.signup(t, testEmail, testPassword)
		token := e.login(t, testEmail, testPassword)

		e.clock.Advance(sessionTTL - time.Second)
		if rec := e.do(t, request{method: "GET", path: "/api/v1/auth/me", token: token}); rec.Code != http.StatusOK {
			t.Fatalf("just before expiry: status %d", rec.Code)
		}
		e.clock.Advance(time.Second)
		wantError(t, e.do(t, request{method: "GET", path: "/api/v1/auth/me", token: token}), http.StatusUnauthorized, codeInvalidToken)
	})

	t.Run("rejects a token whose user no longer exists", func(t *testing.T) {
		e := newTestEnv(t)
		e.signup(t, testEmail, testPassword)
		token := e.login(t, testEmail, testPassword)
		e.store.mu.Lock()
		clear(e.store.users)
		e.store.mu.Unlock()
		wantError(t, e.do(t, request{method: "GET", path: "/api/v1/auth/me", token: token}), http.StatusUnauthorized, codeInvalidToken)
	})

	t.Run("returns 500 when the store fails", func(t *testing.T) {
		e := newTestEnv(t)
		e.signup(t, testEmail, testPassword)
		token := e.login(t, testEmail, testPassword)
		e.store.setFail(errDB)
		wantError(t, e.do(t, request{method: "GET", path: "/api/v1/auth/me", token: token}), http.StatusInternalServerError, codeInternalError)
	})
}

func TestMeAndLogout(t *testing.T) {
	t.Parallel()

	t.Run("me returns the logged-in user", func(t *testing.T) {
		e := newTestEnv(t)
		e.signup(t, testEmail, testPassword)
		token := e.login(t, testEmail, testPassword)
		rec := e.do(t, request{method: "GET", path: "/api/v1/auth/me", token: token})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		var got userResponse
		decodeBody(t, rec, &got)
		if got.Email != testEmail {
			t.Errorf("email = %q", got.Email)
		}
	})

	t.Run("logout ends only the current session", func(t *testing.T) {
		e := newTestEnv(t)
		e.signup(t, testEmail, testPassword)
		laptop := e.login(t, testEmail, testPassword)
		phone := e.login(t, testEmail, testPassword)

		rec := e.do(t, request{method: "POST", path: "/api/v1/auth/logout", token: laptop})
		if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
			t.Fatalf("logout: status %d, body %q", rec.Code, rec.Body)
		}
		wantError(t, e.do(t, request{method: "GET", path: "/api/v1/auth/me", token: laptop}), http.StatusUnauthorized, codeInvalidToken)
		if rec := e.do(t, request{method: "GET", path: "/api/v1/auth/me", token: phone}); rec.Code != http.StatusOK {
			t.Errorf("other session: status %d", rec.Code)
		}
		// Logging out twice with the same token is rejected.
		wantError(t, e.do(t, request{method: "POST", path: "/api/v1/auth/logout", token: laptop}), http.StatusUnauthorized, codeInvalidToken)
	})

	t.Run("logout returns 500 when the store fails to delete", func(t *testing.T) {
		e := newTestEnv(t)
		e.signup(t, testEmail, testPassword)
		token := e.login(t, testEmail, testPassword)
		// Fail only the delete: wrap the store.
		h := NewRouter(Deps{Logger: discardLogger(), DB: e.store, Users: e.store, Sessions: failingDelete{e.store}, Projects: e.store, Now: e.clock.Now})
		rec := (&testEnv{h: h}).do(t, request{method: "POST", path: "/api/v1/auth/logout", token: token})
		wantError(t, rec, http.StatusInternalServerError, codeInternalError)
	})
}

// failingDelete is a SessionStore whose DeleteSession always fails.
type failingDelete struct{ *fakeStore }

func (failingDelete) DeleteSession(context.Context, string) error {
	return errDB
}
