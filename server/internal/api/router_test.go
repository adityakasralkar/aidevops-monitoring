package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakePinger stands in for the database in tests.
type fakePinger struct {
	err   error
	delay time.Duration
}

func (f fakePinger) Ping(ctx context.Context) error {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return f.err
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestHealthz(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		db         fakePinger
		wantStatus int
		wantBody   *healthResponse
	}{
		{
			name:       "returns 200 when the database is reachable",
			method:     http.MethodGet,
			db:         fakePinger{},
			wantStatus: http.StatusOK,
			wantBody:   &healthResponse{Status: "ok", DB: "ok"},
		},
		{
			name:       "returns 503 when the database is unreachable",
			method:     http.MethodGet,
			db:         fakePinger{err: errors.New("server selection timeout: secret-host:27017")},
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   &healthResponse{Status: "degraded", DB: "unreachable"},
		},
		{
			name:       "returns 405 for POST",
			method:     http.MethodPost,
			db:         fakePinger{},
			wantStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, "/healthz", nil)

			NewRouter(discardLogger(), tt.db).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantBody == nil {
				return
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			var got healthResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode body %q: %v", rec.Body.String(), err)
			}
			if got != *tt.wantBody {
				t.Errorf("body = %+v, want %+v", got, *tt.wantBody)
			}
			if strings.Contains(rec.Body.String(), "secret-host") {
				t.Errorf("internal error details leaked to client: %s", rec.Body.String())
			}
		})
	}
}

func TestHealthzTimesOutOnSlowDatabase(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	start := time.Now()
	NewRouter(discardLogger(), fakePinger{delay: time.Minute}).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
	if elapsed := time.Since(start); elapsed > healthCheckTimeout+time.Second {
		t.Errorf("took %s; want it bounded by %s", elapsed, healthCheckTimeout)
	}
}
