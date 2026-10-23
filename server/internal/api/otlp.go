package api

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/ingest"
	"github.com/adityakasralkar/aidevops-monitoring/server/internal/otlp"
)

// otlpMaxDecoded limits the body after gzip decompression, so a small
// compressed request cannot expand into gigabytes ("zip bomb").
const otlpMaxDecoded = 20 << 20 // 20 MiB

// OTLP body formats, chosen by the request's Content-Type.
const (
	formatProtobuf = "application/x-protobuf"
	formatJSON     = "application/json"
)

// handleOTLPLogs receives logs from OpenTelemetry SDKs and collectors
// (OTLP/HTTP, protobuf or JSON, optionally gzip). The logs go through the
// same queue and storage as the custom endpoint.
func (s *server) handleOTLPLogs(w http.ResponseWriter, r *http.Request) {
	req := &collogspb.ExportLogsServiceRequest{}
	format, ok := readOTLP(w, r, req)
	if !ok {
		return
	}

	records := otlp.Logs(req, currentProject(r).ID, s.now())
	if !s.enqueueOTLP(w, r, format, len(records), s.Logs.Capacity(), func() error { return s.Logs.Enqueue(records) }) {
		return
	}
	writeOTLP(w, format, http.StatusOK, &collogspb.ExportLogsServiceResponse{})
}

// handleOTLPMetrics receives metrics from OpenTelemetry SDKs and collectors.
// Gauge, sum and histogram are stored; other kinds are reported back as
// rejected in the response (OTLP "partial success").
func (s *server) handleOTLPMetrics(w http.ResponseWriter, r *http.Request) {
	req := &colmetricspb.ExportMetricsServiceRequest{}
	format, ok := readOTLP(w, r, req)
	if !ok {
		return
	}

	res := otlp.Metrics(req, currentProject(r).ID, s.now())
	if !s.enqueueOTLP(w, r, format, len(res.Points), s.Metrics.Capacity(), func() error { return s.Metrics.Enqueue(res.Points) }) {
		return
	}

	resp := &colmetricspb.ExportMetricsServiceResponse{}
	if res.Rejected > 0 {
		resp.PartialSuccess = &colmetricspb.ExportMetricsPartialSuccess{
			RejectedDataPoints: int64(res.Rejected),
			ErrorMessage:       "exponential histogram and summary metrics are not supported",
		}
	}
	writeOTLP(w, format, http.StatusOK, resp)
}

// enqueueOTLP puts n converted items on a queue and writes the OTLP error
// response if that fails. It returns false when a response was written.
func (s *server) enqueueOTLP(w http.ResponseWriter, r *http.Request, format string, n, capacity int, enqueue func() error) bool {
	if n == 0 {
		return true
	}
	if n > capacity {
		// It could never fit, so retrying would not help: 413 is final.
		writeOTLPError(w, format, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("export has %d items; at most %d per request (reduce the exporter batch size)", n, capacity))
		return false
	}

	switch err := enqueue(); {
	case err == nil:
		return true
	case errors.Is(err, ingest.ErrQueueFull):
		// OTLP clients retry 503 with back-off.
		w.Header().Set("Retry-After", "1")
		writeOTLPError(w, format, http.StatusServiceUnavailable, "server is busy; retry later")
	case errors.Is(err, ingest.ErrClosed):
		w.Header().Set("Retry-After", "5")
		writeOTLPError(w, format, http.StatusServiceUnavailable, "server is shutting down; retry later")
	default:
		s.Logger.ErrorContext(r.Context(), "otlp: enqueue", "error", err)
		writeOTLPError(w, format, http.StatusInternalServerError, "internal error")
	}
	return false
}

// readOTLP reads and decodes an OTLP request body into msg. It handles
// protobuf and JSON bodies and gzip compression. On failure it writes the
// error response and returns false.
func readOTLP(w http.ResponseWriter, r *http.Request, msg proto.Message) (string, bool) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || (mediaType != formatProtobuf && mediaType != formatJSON) {
		writeOTLPError(w, formatJSON, http.StatusUnsupportedMediaType,
			"Content-Type must be application/x-protobuf or application/json")
		return "", false
	}
	format := mediaType

	var body io.Reader = http.MaxBytesReader(w, r.Body, telemetryMaxBody)
	switch enc := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Encoding"))); enc {
	case "", "identity":
	case "gzip":
		gz, err := gzip.NewReader(body)
		if err != nil {
			writeOTLPError(w, format, http.StatusBadRequest, "body is not valid gzip")
			return "", false
		}
		defer gz.Close()
		body = io.LimitReader(gz, otlpMaxDecoded+1)
	default:
		writeOTLPError(w, format, http.StatusUnsupportedMediaType, "Content-Encoding must be gzip or none")
		return "", false
	}

	data, err := io.ReadAll(body)
	var maxErr *http.MaxBytesError
	switch {
	case errors.As(err, &maxErr):
		writeOTLPError(w, format, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("body must not be larger than %d bytes", telemetryMaxBody))
		return "", false
	case err != nil:
		writeOTLPError(w, format, http.StatusBadRequest, "could not read body")
		return "", false
	case len(data) > otlpMaxDecoded:
		writeOTLPError(w, format, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("decompressed body must not be larger than %d bytes", otlpMaxDecoded))
		return "", false
	}

	if format == formatProtobuf {
		err = proto.Unmarshal(data, msg)
	} else {
		// Newer OTLP fields this server does not know are ignored, as the
		// OTLP specification requires.
		err = protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(data, msg)
	}
	if err != nil {
		writeOTLPError(w, format, http.StatusBadRequest, "body is not a valid OTLP export request")
		return "", false
	}
	return format, true
}

// writeOTLP sends an OTLP message in the same format as the request.
func writeOTLP(w http.ResponseWriter, format string, status int, msg proto.Message) {
	var data []byte
	var err error
	if format == formatProtobuf {
		data, err = proto.Marshal(msg)
	} else {
		data, err = protojson.Marshal(msg)
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", format)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

// writeOTLPError sends an error as a google.rpc.Status message, the error
// body the OTLP/HTTP specification defines.
func writeOTLPError(w http.ResponseWriter, format string, status int, message string) {
	writeOTLP(w, format, status, &statuspb.Status{Message: message})
}
