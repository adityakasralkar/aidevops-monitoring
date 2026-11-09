package otlp

import (
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

var (
	now     = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	project = bson.NewObjectID()
)

func str(k, v string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}}}
}

func intKV(k string, v int64) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: v}}}
}

func strBody(s string) *commonpb.AnyValue {
	return &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: s}}
}

func logsRequest(resAttrs []*commonpb.KeyValue, records ...*logspb.LogRecord) *collogspb.ExportLogsServiceRequest {
	return &collogspb.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{
		Resource:  &resourcepb.Resource{Attributes: resAttrs},
		ScopeLogs: []*logspb.ScopeLogs{{LogRecords: records}},
	}}}
}

func nanos(t time.Time) uint64 { return uint64(t.UnixNano()) }

func TestLogsBasicConversion(t *testing.T) {
	ts := now.Add(-time.Minute)
	req := logsRequest(
		[]*commonpb.KeyValue{str("service.name", "checkout"), str("deployment.environment", "test")},
		&logspb.LogRecord{
			TimeUnixNano:   nanos(ts),
			SeverityNumber: logspb.SeverityNumber_SEVERITY_NUMBER_ERROR,
			SeverityText:   "ERROR",
			Body:           strBody("payment failed"),
			Attributes:     []*commonpb.KeyValue{str("http.route", "/pay"), intKV("retries", 3), str("exception.stacktrace", "at pay.go:42")},
			TraceId:        []byte{0x01, 0x02},
			SpanId:         []byte{0xab},
		},
	)

	got := Logs(req, project, now)
	if len(got) != 1 {
		t.Fatalf("got %d records, want 1", len(got))
	}
	l := got[0]
	if l.ProjectID != project || l.ServiceName != "checkout" || l.Severity != model.SeverityError ||
		l.Message != "payment failed" || l.StackTrace != "at pay.go:42" || !l.Timestamp.Equal(ts) || !l.ReceivedAt.Equal(now) || l.ID.IsZero() {
		t.Errorf("record = %+v", l)
	}
	wantAttrs := map[string]any{
		"deployment.environment": "test", "http.route": "/pay", "retries": float64(3),
		"trace_id": "0102", "span_id": "ab", "otel.severity_text": "ERROR",
	}
	for k, v := range wantAttrs {
		if l.Attributes[k] != v {
			t.Errorf("attribute %s = %v, want %v", k, l.Attributes[k], v)
		}
	}
	for _, k := range []string{"service.name", "exception.stacktrace"} {
		if _, ok := l.Attributes[k]; ok {
			t.Errorf("attribute %s should not be kept", k)
		}
	}
}

func TestSeverityMapping(t *testing.T) {
	tests := []struct {
		num  logspb.SeverityNumber
		text string
		want string
	}{
		{logspb.SeverityNumber_SEVERITY_NUMBER_TRACE, "", model.SeverityInfo},
		{logspb.SeverityNumber_SEVERITY_NUMBER_DEBUG4, "", model.SeverityInfo},
		{logspb.SeverityNumber_SEVERITY_NUMBER_INFO, "", model.SeverityInfo},
		{logspb.SeverityNumber_SEVERITY_NUMBER_WARN, "", model.SeverityWarn},
		{logspb.SeverityNumber_SEVERITY_NUMBER_WARN4, "", model.SeverityWarn},
		{logspb.SeverityNumber_SEVERITY_NUMBER_ERROR, "", model.SeverityError},
		{logspb.SeverityNumber_SEVERITY_NUMBER_ERROR4, "", model.SeverityError},
		{logspb.SeverityNumber_SEVERITY_NUMBER_FATAL, "", model.SeverityCritical},
		{logspb.SeverityNumber_SEVERITY_NUMBER_FATAL4, "", model.SeverityCritical},
		{0, "Warning", model.SeverityWarn},
		{0, "error", model.SeverityError},
		{0, "CRITICAL", model.SeverityCritical},
		{0, "fatal", model.SeverityCritical},
		{0, "", model.SeverityInfo},
		{0, "notice", model.SeverityInfo},
	}
	for _, tt := range tests {
		if got := severity(tt.num, tt.text); got != tt.want {
			t.Errorf("severity(%v, %q) = %s, want %s", tt.num, tt.text, got, tt.want)
		}
	}
}

func TestLogsEdgeCases(t *testing.T) {
	t.Run("missing service.name uses the OTel default", func(t *testing.T) {
		got := Logs(logsRequest(nil, &logspb.LogRecord{Body: strBody("x")}), project, now)
		if got[0].ServiceName != unknownService {
			t.Errorf("service = %q", got[0].ServiceName)
		}
	})
	t.Run("structured body becomes JSON text", func(t *testing.T) {
		body := &commonpb.AnyValue{Value: &commonpb.AnyValue_KvlistValue{KvlistValue: &commonpb.KeyValueList{
			Values: []*commonpb.KeyValue{str("event", "login"), intKV("user", 7)},
		}}}
		got := Logs(logsRequest(nil, &logspb.LogRecord{Body: body}), project, now)
		if got[0].Message != `{"event":"login","user":7}` {
			t.Errorf("message = %q", got[0].Message)
		}
	})
	t.Run("empty body is built from the exception", func(t *testing.T) {
		rec := &logspb.LogRecord{Attributes: []*commonpb.KeyValue{str("exception.type", "TypeError"), str("exception.message", "x is undefined")}}
		got := Logs(logsRequest(nil, rec), project, now)
		if got[0].Message != "TypeError: x is undefined" {
			t.Errorf("message = %q", got[0].Message)
		}
	})
	t.Run("completely empty log still gets a message", func(t *testing.T) {
		got := Logs(logsRequest(nil, &logspb.LogRecord{}), project, now)
		if got[0].Message == "" {
			t.Error("message is empty")
		}
	})
	t.Run("timestamp falls back to observed time, then now", func(t *testing.T) {
		obs := now.Add(-time.Hour)
		got := Logs(logsRequest(nil, &logspb.LogRecord{ObservedTimeUnixNano: nanos(obs)}, &logspb.LogRecord{}), project, now)
		if !got[0].Timestamp.Equal(obs) || !got[1].Timestamp.Equal(now) {
			t.Errorf("timestamps = %s, %s", got[0].Timestamp, got[1].Timestamp)
		}
	})
	t.Run("future timestamp from a wrong clock becomes now", func(t *testing.T) {
		got := Logs(logsRequest(nil, &logspb.LogRecord{TimeUnixNano: nanos(now.Add(time.Hour))}), project, now)
		if !got[0].Timestamp.Equal(now) {
			t.Errorf("timestamp = %s", got[0].Timestamp)
		}
	})
	t.Run("oversized message is cut, not rejected", func(t *testing.T) {
		got := Logs(logsRequest(nil, &logspb.LogRecord{Body: strBody(strings.Repeat("m", maxMessageBytes+100))}), project, now)
		if len(got[0].Message) != maxMessageBytes {
			t.Errorf("message length = %d", len(got[0].Message))
		}
	})
	t.Run("more than 50 attributes are limited", func(t *testing.T) {
		var attrs []*commonpb.KeyValue
		for i := range 80 {
			attrs = append(attrs, intKV("k"+strings.Repeat("x", i%10)+string(rune('a'+i%26))+string(rune('a'+i/26)), int64(i)))
		}
		got := Logs(logsRequest(nil, &logspb.LogRecord{Body: strBody("x"), Attributes: attrs}), project, now)
		if n := len(got[0].Attributes); n != maxAttributes {
			t.Errorf("attributes = %d, want %d", n, maxAttributes)
		}
	})
	t.Run("array attribute becomes JSON text and $ keys are dropped", func(t *testing.T) {
		arr := &commonpb.KeyValue{Key: "tags", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{ArrayValue: &commonpb.ArrayValue{
			Values: []*commonpb.AnyValue{strBody("a"), strBody("b")},
		}}}}
		got := Logs(logsRequest(nil, &logspb.LogRecord{Body: strBody("x"), Attributes: []*commonpb.KeyValue{arr, str("$where", "1")}}), project, now)
		if got[0].Attributes["tags"] != `["a","b"]` {
			t.Errorf("tags = %v", got[0].Attributes["tags"])
		}
		if _, ok := got[0].Attributes["$where"]; ok {
			t.Error("$ key kept")
		}
	})
	t.Run("empty request gives no records", func(t *testing.T) {
		if got := Logs(&collogspb.ExportLogsServiceRequest{}, project, now); len(got) != 0 {
			t.Errorf("got %d records", len(got))
		}
	})
}

func TestCutKeepsUTF8Valid(t *testing.T) {
	s := strings.Repeat("é", 10) // 2 bytes each
	if got := cut(s, 5); got != "éé" {
		t.Errorf("cut = %q, want two whole characters", got)
	}
}

func metricsRequest(metrics ...*metricspb.Metric) *colmetricspb.ExportMetricsServiceRequest {
	return &colmetricspb.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{
		Resource:     &resourcepb.Resource{Attributes: []*commonpb.KeyValue{str("service.name", "api")}},
		ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: metrics}},
	}}}
}

func f64(v float64) *float64 { return &v }

func TestMetrics(t *testing.T) {
	ts := now.Add(-10 * time.Second)
	req := metricsRequest(
		&metricspb.Metric{Name: "db_active_connections", Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{
			DataPoints: []*metricspb.NumberDataPoint{{StartTimeUnixNano: nanos(ts.Add(-time.Hour)), TimeUnixNano: nanos(ts), Value: &metricspb.NumberDataPoint_AsInt{AsInt: 42}}},
		}}},
		&metricspb.Metric{Name: "http.server.requests", Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{
			AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, IsMonotonic: true,
			DataPoints: []*metricspb.NumberDataPoint{{TimeUnixNano: nanos(ts), Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: 1532.5}},
				{TimeUnixNano: nanos(ts), Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: 0}, Attributes: []*commonpb.KeyValue{str("http.route", "/pay")}}},
		}}},
		&metricspb.Metric{Name: "http.server.duration", Unit: "ms", Data: &metricspb.Metric_Histogram{Histogram: &metricspb.Histogram{
			DataPoints: []*metricspb.HistogramDataPoint{{
				TimeUnixNano: nanos(ts), Count: 10, Sum: f64(1234.5), Min: f64(12), Max: f64(480),
				ExplicitBounds: []float64{100, 250, 500}, BucketCounts: []uint64{4, 3, 3, 0},
			}},
		}}},
		&metricspb.Metric{Name: "exp", Data: &metricspb.Metric_ExponentialHistogram{ExponentialHistogram: &metricspb.ExponentialHistogram{
			DataPoints: []*metricspb.ExponentialHistogramDataPoint{{}, {}},
		}}},
		&metricspb.Metric{Name: "summary", Data: &metricspb.Metric_Summary{Summary: &metricspb.Summary{
			DataPoints: []*metricspb.SummaryDataPoint{{}},
		}}},
	)

	res := Metrics(req, project, now)
	if len(res.Points) != 4 || res.Rejected != 3 {
		t.Fatalf("points = %d, rejected = %d; want 4 and 3", len(res.Points), res.Rejected)
	}

	gauge, sum, zero, hist := res.Points[0], res.Points[1], res.Points[2], res.Points[3]
	if gauge.MetricType != model.MetricGauge || *gauge.Value != 42 || gauge.ServiceName != "api" || gauge.ProjectID != project || !gauge.Timestamp.Equal(ts) ||
		gauge.StartTime == nil || !gauge.StartTime.Equal(ts.Add(-time.Hour)) {
		t.Errorf("gauge = %+v", gauge)
	}
	if sum.MetricType != model.MetricSum || *sum.Value != 1532.5 || sum.Temporality != model.TemporalityCumulative || sum.Monotonic == nil || !*sum.Monotonic {
		t.Errorf("sum = %+v", sum)
	}
	if zero.Value == nil || *zero.Value != 0 || zero.Attributes["http.route"] != "/pay" {
		t.Errorf("zero-value sum = %+v", zero)
	}
	if hist.MetricType != model.MetricHistogram || hist.Unit != "ms" || *hist.Count != 10 || *hist.Sum != 1234.5 ||
		*hist.Min != 12 || *hist.Max != 480 || len(hist.BucketBounds) != 3 || hist.BucketCounts[0] != 4 || hist.Value != nil {
		t.Errorf("histogram = %+v", hist)
	}
}

func TestHistogramEdgeCases(t *testing.T) {
	t.Run("without buckets keeps one bucket with the count", func(t *testing.T) {
		req := metricsRequest(&metricspb.Metric{Name: "lat", Data: &metricspb.Metric_Histogram{Histogram: &metricspb.Histogram{
			DataPoints: []*metricspb.HistogramDataPoint{{Count: 5}},
		}}})
		p := Metrics(req, project, now).Points[0]
		if len(p.BucketCounts) != 1 || p.BucketCounts[0] != 5 || p.BucketBounds != nil || p.Sum != nil || p.Min != nil {
			t.Errorf("point = %+v", p)
		}
	})
	t.Run("inconsistent buckets fall back to one bucket", func(t *testing.T) {
		req := metricsRequest(&metricspb.Metric{Name: "lat", Data: &metricspb.Metric_Histogram{Histogram: &metricspb.Histogram{
			DataPoints: []*metricspb.HistogramDataPoint{{Count: 5, ExplicitBounds: []float64{1, 2}, BucketCounts: []uint64{5}}},
		}}})
		p := Metrics(req, project, now).Points[0]
		if len(p.BucketCounts) != 1 || p.BucketCounts[0] != 5 || p.BucketBounds != nil {
			t.Errorf("point = %+v", p)
		}
	})
}
