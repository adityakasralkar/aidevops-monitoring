package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORS(t *testing.T) {
	t.Parallel()
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	h := withCORS([]string{"http://localhost:5173"}, ok)

	send := func(method, origin string, preflight bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/v1/projects", nil)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if preflight {
			r.Header.Set("Access-Control-Request-Method", "POST")
			r.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}

	t.Run("allowed origin gets CORS headers and reaches the handler", func(t *testing.T) {
		rec := send(http.MethodGet, "http://localhost:5173", false)
		if rec.Code != http.StatusTeapot {
			t.Fatalf("status = %d", rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
			t.Errorf("Allow-Origin = %q", got)
		}
		if rec.Header().Get("Access-Control-Allow-Credentials") != "" {
			t.Error("credentials must not be allowed")
		}
	})

	t.Run("allowed preflight is answered with 204", func(t *testing.T) {
		rec := send(http.MethodOptions, "http://localhost:5173", true)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d", rec.Code)
		}
		for _, k := range []string{"Access-Control-Allow-Methods", "Access-Control-Allow-Headers", "Access-Control-Max-Age"} {
			if rec.Header().Get(k) == "" {
				t.Errorf("missing %s", k)
			}
		}
	})

	t.Run("other origins get no CORS headers", func(t *testing.T) {
		for _, method := range []string{http.MethodGet, http.MethodOptions} {
			rec := send(method, "https://evil.example.com", method == http.MethodOptions)
			if rec.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Errorf("%s: Allow-Origin set for a foreign origin", method)
			}
			if rec.Header().Get("Vary") == "" {
				t.Errorf("%s: missing Vary: Origin", method)
			}
		}
	})

	t.Run("requests without Origin are untouched", func(t *testing.T) {
		rec := send(http.MethodGet, "", false)
		if rec.Code != http.StatusTeapot || rec.Header().Get("Access-Control-Allow-Origin") != "" || rec.Header().Get("Vary") != "" {
			t.Errorf("status %d, headers %v", rec.Code, rec.Header())
		}
	})
}
