package api

import (
	"encoding/json"
	"net/http"
)

// writeJSON sends v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// The status code has already been sent, so an error here cannot be
	// reported to the client. It usually just means the client disconnected.
	_ = json.NewEncoder(w).Encode(v)
}
