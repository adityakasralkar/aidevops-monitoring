package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

// slowPinger simulates a database that takes too long to answer.
type slowPinger struct{}

func (slowPinger) Ping(ctx context.Context) error {
	select {
	case <-time.After(time.Minute):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestHealthz(t *testing.T) {
	t.Parallel()

	t.Run("returns 200 when the database is reachable", func(t *testing.T) {
		e := newTestEnv(t)
		rec := e.do(t, request{method: "GET", path: "/healthz"})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		var got healthResponse
		decodeBody(t, rec, &got)
		if got != (healthResponse{Status: "ok", DB: "ok"}) {
			t.Errorf("body = %+v", got)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q", ct)
		}
	})

	t.Run("returns 503 without leaking error details", func(t *testing.T) {
		e := newTestEnv(t)
		e.store.pingErr = errors.New("server selection timeout: secret-host:27017")
		rec := e.do(t, request{method: "GET", path: "/healthz"})
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", rec.Code)
		}
		if strings.Contains(rec.Body.String(), "secret-host") {
			t.Errorf("error details leaked: %s", rec.Body)
		}
	})

	t.Run("returns 503 within the timeout when the database is slow", func(t *testing.T) {
		e := newTestEnv(t)
		h := NewRouter(Deps{Logger: discardLogger(), DB: slowPinger{}, Users: e.store, Sessions: e.store, Projects: e.store})
		start := time.Now()
		rec := (&testEnv{h: h}).do(t, request{method: "GET", path: "/healthz"})
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want 503", rec.Code)
		}
		if elapsed := time.Since(start); elapsed > healthCheckTimeout+time.Second {
			t.Errorf("took %s; want it bounded by %s", elapsed, healthCheckTimeout)
		}
	})
}

func TestMethodNotAllowed(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	tests := []struct{ method, path string }{
		{"POST", "/healthz"},
		{"GET", "/api/v1/auth/signup"},
		{"GET", "/api/v1/auth/login"},
		{"GET", "/api/v1/auth/logout"},
		{"POST", "/api/v1/auth/me"},
		{"DELETE", "/api/v1/projects"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			if rec := e.do(t, request{method: tt.method, path: tt.path}); rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("status = %d, want 405", rec.Code)
			}
		})
	}
}

func TestUnknownPath(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	if rec := e.do(t, request{method: "GET", path: "/api/v1/nope"}); rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
