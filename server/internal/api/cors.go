package api

import (
	"net/http"
	"strings"
)

// corsMaxAge is how long (seconds) a browser may cache a preflight answer.
const corsMaxAge = "600"

// withCORS lets dashboards on the allowed origins call the API from a
// browser. Requests from other origins get no CORS headers, so the browser
// blocks them. Requests without an Origin header (curl, SDKs, servers) are
// not affected at all.
//
// The dashboard sends its session token in the Authorization header, not in
// a cookie, so credentials mode is not needed and is not enabled.
func withCORS(allowed []string, next http.Handler) http.Handler {
	allowedSet := make(map[string]bool, len(allowed))
	for _, o := range allowed {
		allowedSet[o] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			// The answer depends on Origin, so caches must keep them apart.
			w.Header().Add("Vary", "Origin")
		}
		if origin == "" || !allowedSet[origin] {
			next.ServeHTTP(w, r)
			return
		}

		h := w.Header()
		h.Set("Access-Control-Allow-Origin", origin)
		// Let the dashboard read Retry-After on 429 and 503 responses.
		h.Set("Access-Control-Expose-Headers", "Retry-After")

		// A preflight asks permission before the real request.
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			h.Add("Vary", "Access-Control-Request-Method")
			h.Add("Vary", "Access-Control-Request-Headers")
			h.Set("Access-Control-Allow-Methods", strings.Join([]string{
				http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete,
			}, ", "))
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			h.Set("Access-Control-Max-Age", corsMaxAge)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
