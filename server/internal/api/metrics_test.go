package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/ingest"
	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

func (e *testEnv) sendMetrics(t *testing.T, key, body string) *httpRecorder {
	t.Helper()
	return e.do(t, request{method: "POST", path: "/api/v1/telemetry/metrics", apiKey: key, body: body})
}

const (
	validGauge     = `{"service_name":"db","metric_name":"db_active_connections","metric_type":"gauge","value":42}`
	validHistogram = `{"service_name":"api","metric_name":"http.server.duration","metric_type":"histogram","unit":"ms",
		"count":10,"sum":1234.5,"min":12,"max":480,"bucket_bounds":[100,250,500],"bucket_counts":[4,3,3,0]}`
)

func TestMetricsAccepted(t *testing.T) {
	t.Parallel()

	t.Run("accepts a gauge and stamps the project from the key", func(t *testing.T) {
		e := newTestEnv(t)
		key := e.newProjectKey(t)
		rec := e.sendMetrics(t, key, validGauge)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		var resp acceptedResponse
		decodeBody(t, rec, &resp)
		got := e.metrics.queued()
		if len(got) != 1 || resp.ID != got[0].ID.Hex() {
			t.Fatalf("queued %+v, response %+v", got, resp)
		}
		m := got[0]
		if m.ProjectID != e.store.projects[0].ID || m.MetricType != model.MetricGauge || m.Value == nil || *m.Value != 42 ||
			!m.Timestamp.Equal(e.clock.Now()) || m.Count != nil {
			t.Errorf("queued metric = %+v", m)
		}
	})

	t.Run("keeps a real zero value", func(t *testing.T) {
		e := newTestEnv(t)
		key := e.newProjectKey(t)
		rec := e.sendMetrics(t, key, `{"service_name":"q","metric_name":"queue_depth","metric_type":"GAUGE","value":0}`)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		if v := e.metrics.queued()[0].Value; v == nil || *v != 0 {
			t.Errorf("value = %v, want pointer to 0", v)
		}
	})

	t.Run("accepts a sum", func(t *testing.T) {
		e := newTestEnv(t)
		key := e.newProjectKey(t)
		if rec := e.sendMetrics(t, key, `{"service_name":"api","metric_name":"http.requests","metric_type":"sum","value":1532}`); rec.Code != http.StatusAccepted {
			t.Errorf("status = %d, body %s", rec.Code, rec.Body)
		}
	})

	t.Run("accepts a histogram with all its fields", func(t *testing.T) {
		e := newTestEnv(t)
		key := e.newProjectKey(t)
		rec := e.sendMetrics(t, key, validHistogram)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		m := e.metrics.queued()[0]
		if m.Value != nil || *m.Count != 10 || *m.Sum != 1234.5 || *m.Min != 12 || *m.Max != 480 || m.Unit != "ms" ||
			len(m.BucketBounds) != 3 || len(m.BucketCounts) != 4 {
			t.Errorf("queued histogram = %+v", m)
		}
	})

	t.Run("accepts a histogram with a single bucket and zero count", func(t *testing.T) {
		e := newTestEnv(t)
		key := e.newProjectKey(t)
		body := `{"service_name":"api","metric_name":"latency","metric_type":"histogram","count":0,"sum":0,"bucket_counts":[0]}`
		if rec := e.sendMetrics(t, key, body); rec.Code != http.StatusAccepted {
			t.Errorf("status = %d, body %s", rec.Code, rec.Body)
		}
	})

	t.Run("accepts a mixed batch and returns ids in order", func(t *testing.T) {
		e := newTestEnv(t)
		key := e.newProjectKey(t)
		rec := e.sendMetrics(t, key, "["+validGauge+","+validHistogram+"]")
		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		var resp acceptedBatchResponse
		decodeBody(t, rec, &resp)
		got := e.metrics.queued()
		if resp.Count != 2 || resp.IDs[0] != got[0].ID.Hex() || resp.IDs[1] != got[1].ID.Hex() {
			t.Errorf("response %+v, queued %d", resp, len(got))
		}
	})

	t.Run("requires an API key", func(t *testing.T) {
		e := newTestEnv(t)
		wantError(t, e.sendMetrics(t, "", validGauge), http.StatusUnauthorized, codeMissingAPIKey)
	})
}

func TestMetricsValidation(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	key := e.newProjectKey(t)
	future := e.clock.Now().Add(10 * time.Minute).Format(time.RFC3339)
	hist := func(extra string) string {
		return `{"service_name":"api","metric_name":"lat","metric_type":"histogram",` + extra + `}`
	}

	tests := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{"missing service_name", `{"metric_name":"m","metric_type":"gauge","value":1}`, "service_name"},
		{"missing metric_name", `{"service_name":"a","metric_type":"gauge","value":1}`, "metric_name"},
		{"metric_name with spaces", `{"service_name":"a","metric_name":"cpu usage","metric_type":"gauge","value":1}`, "metric_name"},
		{"metric_name starting with a digit", `{"service_name":"a","metric_name":"1cpu","metric_type":"gauge","value":1}`, "metric_name"},
		{"unit too long", `{"service_name":"a","metric_name":"m","metric_type":"gauge","value":1,"unit":"` + strings.Repeat("u", 31) + `"}`, "unit"},
		{"unknown metric_type", `{"service_name":"a","metric_name":"m","metric_type":"counter","value":1}`, "metric_type"},
		{"gauge without value", `{"service_name":"a","metric_name":"m","metric_type":"gauge"}`, "value is required"},
		{"gauge with histogram fields", `{"service_name":"a","metric_name":"m","metric_type":"gauge","value":1,"count":3}`, "only allowed for histogram"},
		{"value as text", `{"service_name":"a","metric_name":"m","metric_type":"gauge","value":"42"}`, "wrong type"},
		{"histogram with value", hist(`"value":1,"count":1,"sum":1,"bucket_counts":[1]`), "value is not allowed"},
		{"histogram without count", hist(`"sum":1,"bucket_counts":[1]`), "count and sum are required"},
		{"histogram without sum", hist(`"count":1,"bucket_counts":[1]`), "count and sum are required"},
		{"histogram with fractional count", hist(`"count":1.5,"sum":1,"bucket_counts":[1]`), "wrong type"},
		{"histogram with negative count", hist(`"count":-1,"sum":1,"bucket_counts":[0]`), "must not be negative"},
		{"histogram without bucket_counts", hist(`"count":1,"sum":1`), "bucket_counts is required"},
		{"bucket counts length mismatch", hist(`"count":2,"sum":1,"bucket_bounds":[10],"bucket_counts":[2]`), "one more entry"},
		{"bounds not increasing", hist(`"count":3,"sum":1,"bucket_bounds":[10,10],"bucket_counts":[1,1,1]`), "strictly increasing"},
		{"negative bucket count", hist(`"count":0,"sum":1,"bucket_bounds":[10],"bucket_counts":[1,-1]`), "must not be negative"},
		{"buckets do not add up to count", hist(`"count":5,"sum":1,"bucket_bounds":[10],"bucket_counts":[1,1]`), "add up to 2 but count is 5"},
		{"min greater than max", hist(`"count":1,"sum":1,"min":9,"max":3,"bucket_counts":[1]`), "min must not be greater"},
		{"too many buckets", hist(`"count":0,"sum":0,"bucket_bounds":[` + bounds(101) + `],"bucket_counts":[` + zeros(102) + `]`), "at most 100"},
		{"timestamp in the future", `{"service_name":"a","metric_name":"m","metric_type":"gauge","value":1,"timestamp":"` + future + `"}`, "future"},
		{"nested attribute", `{"service_name":"a","metric_name":"m","metric_type":"gauge","value":1,"attributes":{"x":{}}}`, "attribute"},
		{"sender tries to set project_id", `{"service_name":"a","metric_name":"m","metric_type":"gauge","value":1,"project_id":"x"}`, `unknown field "project_id"`},
		{"empty array", `[]`, "array of metrics must not be empty"},
		{"a number instead of a metric", `7`, "metric object or an array of metric objects"},
	}
	for _, tt := range tests {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			rec := e.sendMetrics(t, key, tt.body)
			wantError(t, rec, http.StatusBadRequest, codeValidationFailed)
			var resp errorResponse
			decodeBody(t, rec, &resp)
			if !strings.Contains(resp.Error.Message, tt.wantMsg) {
				t.Errorf("message %q does not mention %q", resp.Error.Message, tt.wantMsg)
			}
		})
	}
	if n := len(e.metrics.queued()); n != 0 {
		t.Errorf("%d invalid metrics were queued", n)
	}
}

func bounds(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprint(i + 1)
	}
	return strings.Join(parts, ",")
}

func zeros(n int) string { return strings.TrimSuffix(strings.Repeat("0,", n), ",") }

func TestMetricsBatchIsAllOrNothing(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	key := e.newProjectKey(t)
	rec := e.sendMetrics(t, key, "["+validGauge+`,{"service_name":"a","metric_name":"m","metric_type":"gauge"}]`)
	wantError(t, rec, http.StatusBadRequest, codeValidationFailed)
	var resp errorResponse
	decodeBody(t, rec, &resp)
	if !strings.HasPrefix(resp.Error.Message, "metrics[1]: ") {
		t.Errorf("message %q does not name the bad item", resp.Error.Message)
	}
	if n := len(e.metrics.queued()); n != 0 {
		t.Errorf("%d metrics queued from a rejected batch", n)
	}
}

func TestMetricsQueueErrors(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		err    error
		status int
		code   string
	}{
		{ingest.ErrQueueFull, http.StatusServiceUnavailable, codeQueueFull},
		{ingest.ErrClosed, http.StatusServiceUnavailable, codeShuttingDown},
		{errDB, http.StatusInternalServerError, codeInternalError},
	} {
		t.Run(tt.code, func(t *testing.T) {
			e := newTestEnv(t)
			key := e.newProjectKey(t)
			e.metrics.err = tt.err
			wantError(t, e.sendMetrics(t, key, validGauge), tt.status, tt.code)
		})
	}
}
