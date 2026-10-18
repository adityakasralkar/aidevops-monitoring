// Package api holds the HTTP routes, handlers and middleware.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// healthCheckTimeout is the longest /healthz waits for the database,
// so a slow or unreachable database cannot make the endpoint hang.
const healthCheckTimeout = 2 * time.Second

// Pinger is anything that can check whether the database is reachable.
// The real Store satisfies it; tests use a fake.
type Pinger interface {
	Ping(ctx context.Context) error
}

// NewRouter returns the HTTP handler with all routes registered.
func NewRouter(logger *slog.Logger, db Pinger) http.Handler {
	mux := http.NewServeMux()
	// "GET /healthz" only matches GET (and HEAD); other methods get 405.
	mux.HandleFunc("GET /healthz", handleHealthz(logger, db))
	return mux
}

// healthResponse is the JSON body returned by /healthz.
type healthResponse struct {
	Status string `json:"status"`
	DB     string `json:"db"`
}

// handleHealthz returns 200 when the server and database are working,
// and 503 when the database cannot be reached. The error details are
// written to the log only, never sent to the client.
func handleHealthz(logger *slog.Logger, db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
		defer cancel()

		if err := db.Ping(ctx); err != nil {
			logger.WarnContext(ctx, "health check: database unreachable", "error", err)
			writeJSON(w, http.StatusServiceUnavailable, healthResponse{Status: "degraded", DB: "unreachable"})
			return
		}
		writeJSON(w, http.StatusOK, healthResponse{Status: "ok", DB: "ok"})
	}
}
