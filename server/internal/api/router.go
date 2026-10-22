// Package api holds the HTTP routes, handlers and middleware.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

// healthCheckTimeout is the longest /healthz waits for the database,
// so a slow or unreachable database cannot make the endpoint hang.
const healthCheckTimeout = 2 * time.Second

// Rate limits. Failed logins are counted per email (protects one account
// from password guessing) and per IP (stops one client trying many accounts).
const (
	loginFailuresPerEmail = 5
	loginFailuresPerIP    = 20
	loginWindow           = 15 * time.Minute
	signupsPerIP          = 10
	signupWindow          = time.Hour
)

// Pinger is anything that can check whether the database is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

// UserStore reads and writes user accounts.
type UserStore interface {
	CreateUser(ctx context.Context, u *model.User) error
	UserByEmail(ctx context.Context, email string) (model.User, error)
	UserByID(ctx context.Context, id bson.ObjectID) (model.User, error)
}

// SessionStore reads and writes login sessions.
type SessionStore interface {
	CreateSession(ctx context.Context, s *model.Session) error
	SessionByTokenHash(ctx context.Context, tokenHash string, now time.Time) (model.Session, error)
	DeleteSession(ctx context.Context, tokenHash string) error
}

// ProjectStore reads and writes projects.
type ProjectStore interface {
	CreateProject(ctx context.Context, p *model.Project) error
	ProjectsByOwner(ctx context.Context, ownerID bson.ObjectID) ([]model.Project, error)
}

// ProjectKeyStore finds the project that owns an API key.
type ProjectKeyStore interface {
	ProjectByAPIKeyHash(ctx context.Context, keyHash string) (model.Project, error)
}

// LogQueue accepts logs for background saving (the ingest.Queue).
type LogQueue interface {
	Enqueue(logs []model.LogRecord) error
}

// Deps are the things the HTTP handlers need. The real Store satisfies
// all the store interfaces; tests pass fakes.
type Deps struct {
	Logger   *slog.Logger
	DB       Pinger
	Users    UserStore
	Sessions SessionStore
	Projects ProjectStore
	Keys     ProjectKeyStore
	Logs     LogQueue
	Now      func() time.Time // current time; nil means time.Now
}

// server holds the dependencies and rate limiters shared by all handlers.
type server struct {
	Deps
	loginByEmail *rateLimiter
	loginByIP    *rateLimiter
	signupByIP   *rateLimiter
}

// NewRouter returns the HTTP handler with all routes registered.
func NewRouter(d Deps) http.Handler {
	if d.Now == nil {
		d.Now = time.Now
	}
	s := &server{
		Deps:         d,
		loginByEmail: newRateLimiter(loginFailuresPerEmail, loginWindow, d.Now),
		loginByIP:    newRateLimiter(loginFailuresPerIP, loginWindow, d.Now),
		signupByIP:   newRateLimiter(signupsPerIP, signupWindow, d.Now),
	}

	// Patterns like "GET /healthz" only match that method (GET also matches
	// HEAD); other methods automatically get 405 Method Not Allowed.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)

	mux.HandleFunc("POST /api/v1/auth/signup", s.handleSignup)
	mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/v1/auth/logout", s.requireUser(s.handleLogout))
	mux.HandleFunc("GET /api/v1/auth/me", s.requireUser(s.handleMe))

	mux.HandleFunc("POST /api/v1/projects", s.requireUser(s.handleCreateProject))
	mux.HandleFunc("GET /api/v1/projects", s.requireUser(s.handleListProjects))

	mux.HandleFunc("POST /api/v1/telemetry/logs", s.requireProject(s.handleLogs))
	return mux
}

// now returns the current time in UTC, rounded down to the millisecond,
// because MongoDB stores dates with millisecond precision. This keeps the
// times in API responses identical to the stored ones.
func (s *server) now() time.Time {
	return s.Now().UTC().Truncate(time.Millisecond)
}

// healthResponse is the JSON body returned by /healthz.
type healthResponse struct {
	Status string `json:"status"`
	DB     string `json:"db"`
}

// handleHealthz returns 200 when the server and database are working,
// and 503 when the database cannot be reached. The error details are
// written to the log only, never sent to the client.
func (s *server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
	defer cancel()

	if err := s.DB.Ping(ctx); err != nil {
		s.Logger.WarnContext(ctx, "health check: database unreachable", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, healthResponse{Status: "degraded", DB: "unreachable"})
		return
	}
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok", DB: "ok"})
}
