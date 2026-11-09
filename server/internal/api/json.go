package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

// maxBodyBytes is the largest request body accepted by most endpoints (1 MiB).
const maxBodyBytes = 1 << 20

// writeJSON sends v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	// The status code has already been sent, so an error here cannot be
	// reported to the client. It usually just means the client disconnected.
	_ = json.NewEncoder(w).Encode(v)
}

// errorResponse is the standard error body:
// {"error": {"code": "...", "message": "..."}}
type errorResponse struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error codes sent to clients (see conventions.md §5).
const (
	codeInvalidJSON        = "invalid_json"
	codeValidationFailed   = "validation_failed"
	codeEmailTaken         = "email_taken"
	codeInvalidCredentials = "invalid_credentials"
	codeWrongPassword      = "wrong_password"
	codeMissingToken       = "missing_token"
	codeInvalidToken       = "invalid_token"
	codeTooManyRequests    = "too_many_requests"
	codeProjectExists      = "project_exists"
	codeNotFound           = "not_found"
	codeMissingAPIKey      = "missing_api_key"
	codeInvalidAPIKey      = "invalid_api_key"
	codeEnvMismatch        = "env_mismatch"
	codeQueueFull          = "queue_full"
	codeShuttingDown       = "shutting_down"
	codeInternalError      = "internal_error"
)

// writeError sends an error in the standard format.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Error: errorDetail{Code: code, Message: message}})
}

// writeInternalError sends a generic 500. The real cause is logged by the
// caller and never shown to the client.
func writeInternalError(w http.ResponseWriter) {
	writeError(w, http.StatusInternalServerError, codeInternalError, "something went wrong, please try again")
}

// decodeJSON reads the request body as JSON into dst.
// It allows exactly one JSON value, rejects unknown fields and bodies over
// maxBodyBytes. On failure it writes the error response and returns false.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	return decodeJSONLimit(w, r, dst, maxBodyBytes)
}

// decodeJSONLimit is decodeJSON with a custom body size limit in bytes.
func decodeJSONLimit(w http.ResponseWriter, r *http.Request, dst any, limit int64) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, codeInvalidJSON, "Content-Type must be application/json")
		return false
	}

	r.Body = http.MaxBytesReader(w, r.Body, limit)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		status, msg := describeDecodeError(err, limit)
		writeError(w, status, codeInvalidJSON, msg)
		return false
	}
	// Anything after the first object (e.g. "{}{}") is rejected.
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, codeInvalidJSON, "request body must contain a single JSON object")
		return false
	}
	return true
}

// describeDecodeError turns a JSON decoding error into a status code and a
// message that is safe and useful to show to the client.
func describeDecodeError(err error, limit int64) (int, string) {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	var maxErr *http.MaxBytesError

	switch {
	case errors.As(err, &maxErr):
		return http.StatusRequestEntityTooLarge, fmt.Sprintf("request body must not be larger than %d bytes", limit)
	case errors.Is(err, io.EOF):
		return http.StatusBadRequest, "request body must not be empty"
	case errors.Is(err, io.ErrUnexpectedEOF), errors.As(err, &syntaxErr):
		return http.StatusBadRequest, "request body is not valid JSON"
	case errors.As(err, &typeErr):
		if typeErr.Field != "" {
			return http.StatusBadRequest, fmt.Sprintf("field %q has the wrong type", typeErr.Field)
		}
		return http.StatusBadRequest, "request body must be a JSON object"
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		// encoding/json has no error type for this; the message is stable.
		return http.StatusBadRequest, "unknown field " + strings.TrimPrefix(err.Error(), "json: unknown field ")
	default:
		return http.StatusBadRequest, "request body is not valid JSON"
	}
}
