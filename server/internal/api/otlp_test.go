package api

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/ingest"
	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

func otlpLogs(n int) *collogspb.ExportLogsServiceRequest {
	recs := make([]*logspb.LogRecord, n)
	for i := range recs {
		recs[i] = &logspb.LogRecord{
			SeverityNumber: logspb.SeverityNumber_SEVERITY_NUMBER_ERROR,
			Body:           &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "db timeout"}},
		}
	}
	return &collogspb.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{
		Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{{
			Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "orders"}},
		}}},
		ScopeLogs: []*logspb.ScopeLogs{{LogRecords: recs}},
	}}}
}

// sendOTLP posts body to path with the given content type, optional gzip and API key.
func (e *testEnv) sendOTLP(t *testing.T, path, key, contentType string, body []byte, gz bool) *httptest.ResponseRecorder {
	t.Helper()
	if gz {
		var buf bytes.Buffer
		w := gzip.NewWriter(&buf)
		_, _ = w.Write(body)
		_ = w.Close()
		body = buf.Bytes()
	}
	r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	r.Header.Set("Content-Type", contentType)
	if key != "" {
		r.Header.Set("X-API-Key", key)
	}
	if gz {
		r.Header.Set("Content-Encoding", "gzip")
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	return rec
}

func mustProto(t *testing.T, m proto.Message) []byte {
	t.Helper()
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mustJSON(t *testing.T, m proto.Message) []byte {
	t.Helper()
	b, err := protojson.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestOTLPLogsFormats(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		contentType string
		encode      func(*testing.T, proto.Message) []byte
		gzip        bool
	}{
		{"protobuf", formatProtobuf, mustProto, false},
		{"JSON", formatJSON, mustJSON, false},
		{"gzip protobuf", formatProtobuf, mustProto, true},
		{"gzip JSON", formatJSON, mustJSON, true},
	}
	for _, tt := range tests {
		t.Run("accepts "+tt.name, func(t *testing.T) {
			e := newTestEnv(t)
			key := e.newProjectKey(t)
			rec := e.sendOTLP(t, "/v1/logs", key, tt.contentType, tt.encode(t, otlpLogs(3)), tt.gzip)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body %q", rec.Code, rec.Body)
			}
			if ct := rec.Header().Get("Content-Type"); ct != tt.contentType {
				t.Errorf("response Content-Type = %q, want %q", ct, tt.contentType)
			}
			got := e.queue.queued()
			if len(got) != 3 {
				t.Fatalf("queued %d logs, want 3", len(got))
			}
			if got[0].ProjectID != e.store.projects[0].ID || got[0].ServiceName != "orders" || got[0].Severity != model.SeverityError {
				t.Errorf("queued log = %+v", got[0])
			}
		})
	}
}

func TestOTLPLogsJSONFromSDKShape(t *testing.T) {
	t.Parallel()
	// The shape an OpenTelemetry SDK sends: camelCase names, 64-bit numbers
	// as strings, hex trace ids, and fields this server does not know.
	body := `{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"web"}}]},
	  "scopeLogs":[{"scope":{"name":"my-lib"},"logRecords":[{
	    "timeUnixNano":"1792310400000000000","severityNumber":13,"severityText":"WARN",
	    "body":{"stringValue":"slow request"},"attributes":[{"key":"http.route","value":{"stringValue":"/home"}}],
	    "traceId":"5b8efff798038103d269b633813fc60c","spanId":"eee19b7ec3c1b174","someFutureField":true}]}]}]}`
	e := newTestEnv(t)
	key := e.newProjectKey(t)
	rec := e.sendOTLP(t, "/v1/logs", key, "application/json; charset=utf-8", []byte(body), false)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body)
	}
	got := e.queue.queued()
	if len(got) != 1 || got[0].ServiceName != "web" || got[0].Severity != model.SeverityWarn ||
		got[0].Message != "slow request" || got[0].Attributes["http.route"] != "/home" {
		t.Errorf("queued = %+v", got)
	}
}

func TestOTLPErrors(t *testing.T) {
	t.Parallel()
	valid := func(t *testing.T) []byte { return mustProto(t, otlpLogs(1)) }

	t.Run("rejects a missing API key", func(t *testing.T) {
		e := newTestEnv(t)
		rec := e.sendOTLP(t, "/v1/logs", "", formatProtobuf, valid(t), false)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d", rec.Code)
		}
	})
	t.Run("rejects an unknown content type", func(t *testing.T) {
		e := newTestEnv(t)
		key := e.newProjectKey(t)
		if rec := e.sendOTLP(t, "/v1/logs", key, "text/plain", valid(t), false); rec.Code != http.StatusUnsupportedMediaType {
			t.Errorf("status = %d", rec.Code)
		}
	})
	t.Run("rejects an unknown content encoding", func(t *testing.T) {
		e := newTestEnv(t)
		key := e.newProjectKey(t)
		r := httptest.NewRequest(http.MethodPost, "/v1/logs", bytes.NewReader(valid(t)))
		r.Header.Set("Content-Type", formatProtobuf)
		r.Header.Set("Content-Encoding", "br")
		r.Header.Set("X-API-Key", key)
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, r)
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Errorf("status = %d", rec.Code)
		}
	})
	t.Run("rejects garbage protobuf with a Status body", func(t *testing.T) {
		e := newTestEnv(t)
		key := e.newProjectKey(t)
		rec := e.sendOTLP(t, "/v1/logs", key, formatProtobuf, []byte{0xff, 0xff, 0xff}, false)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d", rec.Code)
		}
		var st statuspb.Status
		if err := proto.Unmarshal(rec.Body.Bytes(), &st); err != nil || st.Message == "" {
			t.Errorf("body is not a google.rpc.Status: %v %q", err, rec.Body)
		}
	})
	t.Run("rejects invalid JSON", func(t *testing.T) {
		e := newTestEnv(t)
		key := e.newProjectKey(t)
		if rec := e.sendOTLP(t, "/v1/logs", key, formatJSON, []byte(`{"resourceLogs":`), false); rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d", rec.Code)
		}
	})
	t.Run("rejects a body that is not gzip", func(t *testing.T) {
		e := newTestEnv(t)
		key := e.newProjectKey(t)
		r := httptest.NewRequest(http.MethodPost, "/v1/logs", strings.NewReader("not gzip"))
		r.Header.Set("Content-Type", formatProtobuf)
		r.Header.Set("Content-Encoding", "gzip")
		r.Header.Set("X-API-Key", key)
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, r)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d", rec.Code)
		}
	})
	t.Run("rejects a gzip bomb", func(t *testing.T) {
		e := newTestEnv(t)
		key := e.newProjectKey(t)
		huge := bytes.Repeat([]byte{0}, otlpMaxDecoded+10) // compresses to a few KiB
		if rec := e.sendOTLP(t, "/v1/logs", key, formatProtobuf, huge, true); rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("status = %d", rec.Code)
		}
	})
	t.Run("rejects an export bigger than the queue with 413", func(t *testing.T) {
		e := newTestEnv(t)
		key := e.newProjectKey(t)
		if rec := e.sendOTLP(t, "/v1/logs", key, formatProtobuf, mustProto(t, otlpLogs(5001)), false); rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("status = %d", rec.Code)
		}
	})
	t.Run("asks to retry when the queue is full", func(t *testing.T) {
		e := newTestEnv(t)
		key := e.newProjectKey(t)
		e.queue.err = ingest.ErrQueueFull
		rec := e.sendOTLP(t, "/v1/logs", key, formatProtobuf, valid(t), false)
		if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") == "" {
			t.Errorf("status = %d, Retry-After = %q", rec.Code, rec.Header().Get("Retry-After"))
		}
	})
	t.Run("an empty export succeeds without queuing", func(t *testing.T) {
		e := newTestEnv(t)
		key := e.newProjectKey(t)
		e.queue.err = ingest.ErrQueueFull // would fail if Enqueue were called
		rec := e.sendOTLP(t, "/v1/logs", key, formatProtobuf, mustProto(t, &collogspb.ExportLogsServiceRequest{}), false)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d", rec.Code)
		}
	})
}

func TestOTLPMetrics(t *testing.T) {
	t.Parallel()
	req := &colmetricspb.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{
		ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: []*metricspb.Metric{
			{Name: "cpu", Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{
				{Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: 0.73}},
			}}}},
			{Name: "latency", Data: &metricspb.Metric_Summary{Summary: &metricspb.Summary{DataPoints: []*metricspb.SummaryDataPoint{{}}}}},
		}}},
	}}}

	e := newTestEnv(t)
	key := e.newProjectKey(t)
	rec := e.sendOTLP(t, "/v1/metrics", key, formatProtobuf, mustProto(t, req), false)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body)
	}
	var resp colmetricspb.ExportMetricsServiceResponse
	if err := proto.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.GetPartialSuccess().GetRejectedDataPoints() != 1 {
		t.Errorf("rejected = %d, want 1 (the summary)", resp.GetPartialSuccess().GetRejectedDataPoints())
	}
	got := e.metrics.queued()
	if len(got) != 1 || got[0].MetricName != "cpu" || *got[0].Value != 0.73 || got[0].ProjectID != e.store.projects[0].ID {
		t.Errorf("queued = %+v", got)
	}
}
