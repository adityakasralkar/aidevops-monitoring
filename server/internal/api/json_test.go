package api

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// The signup endpoint is used to exercise decodeJSON through a real route.
func TestDecodeJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		body        string
		contentType string
		wantStatus  int
		wantMsg     string
	}{
		{"rejects a missing Content-Type", `{"email":"a@b.co","password":"12345678"}`, "-", http.StatusUnsupportedMediaType, "Content-Type"},
		{"rejects text/plain", `{"email":"a@b.co","password":"12345678"}`, "text/plain", http.StatusUnsupportedMediaType, "Content-Type"},
		{"rejects an empty body", "", "application/json", http.StatusBadRequest, "must not be empty"},
		{"rejects broken JSON", `{"email":`, "application/json", http.StatusBadRequest, "not valid JSON"},
		{"rejects a syntax error", `{"email" "x"}`, "application/json", http.StatusBadRequest, "not valid JSON"},
		{"rejects an unknown field", `{"email":"a@b.co","password":"12345678","admin":true}`, "application/json", http.StatusBadRequest, `unknown field "admin"`},
		{"rejects a wrong field type", `{"email":"a@b.co","password":12345678}`, "application/json", http.StatusBadRequest, `field "password" has the wrong type`},
		{"rejects a JSON array", `[1,2]`, "application/json", http.StatusBadRequest, "must be a JSON object"},
		{"rejects two objects", `{"email":"a@b.co","password":"12345678"}{}`, "application/json", http.StatusBadRequest, "single JSON object"},
		{"rejects a body over 1 MiB", `{"email":"` + strings.Repeat("a", maxBodyBytes) + `"}`, "application/json", http.StatusRequestEntityTooLarge, "larger than"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEnv(t)
			hdr := map[string]string{}
			if tt.contentType != "-" {
				hdr["Content-Type"] = tt.contentType
			}
			r := request{method: "POST", path: "/api/v1/auth/signup", body: tt.body, header: hdr}
			if tt.contentType == "-" {
				r.header = map[string]string{"Content-Type": ""}
			}
			rec := e.do(t, r)
			wantError(t, rec, tt.wantStatus, codeInvalidJSON)
			var errResp errorResponse
			decodeBody(t, rec, &errResp)
			if !strings.Contains(errResp.Error.Message, tt.wantMsg) {
				t.Errorf("message %q does not mention %q", errResp.Error.Message, tt.wantMsg)
			}
		})
	}

	t.Run("accepts Content-Type with a charset", func(t *testing.T) {
		e := newTestEnv(t)
		rec := e.do(t, request{
			method: "POST", path: "/api/v1/auth/signup",
			body:   `{"email":"a@b.co","password":"12345678"}`,
			header: map[string]string{"Content-Type": "application/json; charset=utf-8"},
		})
		if rec.Code != http.StatusCreated {
			t.Errorf("status = %d, want 201; body %s", rec.Code, rec.Body)
		}
	})
}
