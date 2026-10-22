package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/auth"
	"github.com/adityakasralkar/aidevops-monitoring/server/internal/ingest"
	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

// Limits for log ingestion.
const (
	telemetryMaxBody  = 5 << 20 // 5 MiB per request
	maxLogsPerRequest = 500
	maxServiceNameLen = 100
	maxMessageBytes   = 32 << 10 // 32 KiB
	maxStackBytes     = 64 << 10 // 64 KiB
	maxAttributes     = 50
	maxAttrKeyLen     = 100
	maxAttrValueBytes = 1 << 10 // 1 KiB for text values
	maxClockSkew      = 5 * time.Minute
)

// projectKey is the context key for the project resolved from an API key.
type projectKey struct{}

// currentProject returns the project resolved by requireProject.
func currentProject(r *http.Request) model.Project {
	return r.Context().Value(projectKey{}).(model.Project)
}

// requireProject only lets the request through if it carries a valid API
// key in the X-API-Key header. The project is found from the key's hash;
// senders never say which project they belong to.
func (s *server) requireProject(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(r.Header.Get("X-API-Key"))
		if key == "" {
			writeError(w, http.StatusUnauthorized, codeMissingAPIKey, "missing X-API-Key header")
			return
		}
		if len(key) > maxTokenLen {
			writeError(w, http.StatusUnauthorized, codeInvalidAPIKey, "API key is not valid")
			return
		}

		ctx := r.Context()
		p, err := s.Keys.ProjectByAPIKeyHash(ctx, auth.HashToken(key))
		if errors.Is(err, model.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, codeInvalidAPIKey, "API key is not valid")
			return
		}
		if err != nil {
			s.Logger.ErrorContext(ctx, "telemetry: find project by key", "error", err)
			writeInternalError(w)
			return
		}

		// The environment written in the key must match the project's.
		// Keys are generated this way, so a mismatch means stored data was
		// changed by hand; refuse rather than mix test and live data.
		if !strings.HasPrefix(key, "aidv_"+p.Environment+"_") {
			writeError(w, http.StatusUnauthorized, codeEnvMismatch, "API key environment does not match the project")
			return
		}

		next(w, r.WithContext(context.WithValue(ctx, projectKey{}, p)))
	}
}

// logInput is one log as sent by a client.
type logInput struct {
	ServiceName string         `json:"service_name"`
	Severity    string         `json:"severity"`
	Message     string         `json:"message"`
	StackTrace  string         `json:"stack_trace"`
	Timestamp   string         `json:"timestamp"`
	Attributes  map[string]any `json:"attributes"`
}

// acceptedResponse is returned for a single log; acceptedBatchResponse for an array.
type acceptedResponse struct {
	Status string `json:"status"`
	ID     string `json:"id"`
}

type acceptedBatchResponse struct {
	Status string   `json:"status"`
	Count  int      `json:"count"`
	IDs    []string `json:"ids"`
}

// handleLogs accepts one log object or an array of up to 500 logs, puts
// them on the ingest queue and replies 202 without waiting for MongoDB.
// The batch is all-or-nothing: one invalid log rejects the whole request,
// and if the queue has no room for every log, none are accepted (503).
func (s *server) handleLogs(w http.ResponseWriter, r *http.Request) {
	var raw json.RawMessage
	if !decodeJSONLimit(w, r, &raw, telemetryMaxBody) {
		return
	}

	items, isBatch, msg := splitLogItems(raw)
	if msg != "" {
		writeError(w, http.StatusBadRequest, codeValidationFailed, msg)
		return
	}

	project := currentProject(r)
	now := s.now()
	records := make([]model.LogRecord, len(items))
	for i, item := range items {
		rec, msg := parseLog(item, project.ID, now)
		if msg != "" {
			if isBatch {
				msg = fmt.Sprintf("logs[%d]: %s", i, msg)
			}
			writeError(w, http.StatusBadRequest, codeValidationFailed, msg)
			return
		}
		records[i] = rec
	}

	switch err := s.Logs.Enqueue(records); {
	case errors.Is(err, ingest.ErrQueueFull):
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusServiceUnavailable, codeQueueFull, "server is busy; please retry shortly")
		return
	case errors.Is(err, ingest.ErrClosed):
		w.Header().Set("Retry-After", "5")
		writeError(w, http.StatusServiceUnavailable, codeShuttingDown, "server is shutting down; please retry shortly")
		return
	case err != nil:
		s.Logger.ErrorContext(r.Context(), "telemetry: enqueue logs", "error", err)
		writeInternalError(w)
		return
	}

	if !isBatch {
		writeJSON(w, http.StatusAccepted, acceptedResponse{Status: "accepted", ID: records[0].ID.Hex()})
		return
	}
	ids := make([]string, len(records))
	for i, rec := range records {
		ids[i] = rec.ID.Hex()
	}
	writeJSON(w, http.StatusAccepted, acceptedBatchResponse{Status: "accepted", Count: len(ids), IDs: ids})
}

// splitLogItems turns the body into a list of raw log objects. It returns
// a non-empty message if the body is neither an object nor a valid array.
func splitLogItems(raw json.RawMessage) (items []json.RawMessage, isBatch bool, msg string) {
	trimmed := bytes.TrimSpace(raw)
	switch {
	case len(trimmed) > 0 && trimmed[0] == '{':
		return []json.RawMessage{trimmed}, false, ""
	case len(trimmed) > 0 && trimmed[0] == '[':
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return nil, true, "body must be a log object or an array of log objects"
		}
		if len(items) == 0 {
			return nil, true, "the array of logs must not be empty"
		}
		if len(items) > maxLogsPerRequest {
			return nil, true, fmt.Sprintf("at most %d logs per request, got %d", maxLogsPerRequest, len(items))
		}
		return items, true, ""
	default:
		return nil, false, "body must be a log object or an array of log objects"
	}
}

// parseLog decodes and validates one log. It returns a non-empty message
// describing the first problem found.
func parseLog(raw json.RawMessage, projectID bson.ObjectID, now time.Time) (model.LogRecord, string) {
	var in logInput
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		_, msg := describeDecodeError(err, telemetryMaxBody)
		return model.LogRecord{}, msg
	}

	service := strings.TrimSpace(in.ServiceName)
	if service == "" || len(service) > maxServiceNameLen {
		return model.LogRecord{}, fmt.Sprintf("service_name is required and must be at most %d characters", maxServiceNameLen)
	}

	severity := strings.ToUpper(strings.TrimSpace(in.Severity))
	switch severity {
	case model.SeverityInfo, model.SeverityWarn, model.SeverityError, model.SeverityCritical:
	default:
		return model.LogRecord{}, "severity must be one of INFO, WARN, ERROR, CRITICAL"
	}

	if strings.TrimSpace(in.Message) == "" {
		return model.LogRecord{}, "message is required"
	}
	if len(in.Message) > maxMessageBytes {
		return model.LogRecord{}, fmt.Sprintf("message must be at most %d bytes", maxMessageBytes)
	}
	if len(in.StackTrace) > maxStackBytes {
		return model.LogRecord{}, fmt.Sprintf("stack_trace must be at most %d bytes", maxStackBytes)
	}

	ts := now
	if t := strings.TrimSpace(in.Timestamp); t != "" {
		parsed, err := time.Parse(time.RFC3339Nano, t)
		if err != nil {
			return model.LogRecord{}, `timestamp must be RFC 3339, e.g. "2026-10-19T10:15:00Z"`
		}
		if parsed.After(now.Add(maxClockSkew)) {
			return model.LogRecord{}, "timestamp must not be more than 5 minutes in the future"
		}
		ts = parsed.UTC().Truncate(time.Millisecond)
	}

	if msg := validateAttributes(in.Attributes); msg != "" {
		return model.LogRecord{}, msg
	}

	return model.LogRecord{
		ID:          bson.NewObjectID(),
		ProjectID:   projectID,
		ServiceName: service,
		Severity:    severity,
		Message:     in.Message,
		StackTrace:  in.StackTrace,
		Attributes:  in.Attributes,
		Timestamp:   ts,
		ReceivedAt:  now,
	}, ""
}

// validateAttributes allows a flat map of text, number and true/false
// values. Nested objects, arrays and null are rejected to keep stored logs
// simple to query.
func validateAttributes(attrs map[string]any) string {
	if len(attrs) > maxAttributes {
		return fmt.Sprintf("at most %d attributes allowed, got %d", maxAttributes, len(attrs))
	}
	for k, v := range attrs {
		if k == "" || len(k) > maxAttrKeyLen || strings.HasPrefix(k, "$") || strings.ContainsRune(k, 0) {
			return fmt.Sprintf("attribute name %q is not allowed (1-%d characters, must not start with $)", truncate(k, 40), maxAttrKeyLen)
		}
		switch val := v.(type) {
		case string:
			if len(val) > maxAttrValueBytes {
				return fmt.Sprintf("attribute %q must be at most %d bytes", k, maxAttrValueBytes)
			}
		case float64, bool:
		default:
			return fmt.Sprintf("attribute %q must be text, a number or true/false", k)
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
