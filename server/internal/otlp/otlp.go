// Package otlp converts OpenTelemetry (OTLP) export requests into the
// server's own LogRecord and MetricPoint types, so OTLP data goes through
// the same queue and storage as the custom JSON endpoints.
//
// OTLP senders are standard SDKs that cannot fix individual records, so
// this conversion is lenient: oversized values are cut short instead of
// rejecting the whole export, and unsupported metric kinds are skipped
// and counted.
package otlp

import (
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

// Limits matching the custom endpoints. Values over a limit are cut short.
const (
	maxServiceNameLen = 100
	maxMetricNameLen  = 200
	maxUnitLen        = 30
	maxMessageBytes   = 32 << 10
	maxStackBytes     = 64 << 10
	maxAttributes     = 50
	maxAttrKeyLen     = 100
	maxAttrValueBytes = 1 << 10
	maxClockSkew      = 5 * time.Minute
)

// unknownService is the OpenTelemetry default when service.name is not set.
const unknownService = "unknown_service"

// Attribute keys from the OpenTelemetry semantic conventions.
const (
	attrServiceName      = "service.name"
	attrExceptionStack   = "exception.stacktrace"
	attrExceptionMessage = "exception.message"
	attrExceptionType    = "exception.type"
)

// Logs converts an OTLP logs export into log records for one project.
func Logs(req *collogspb.ExportLogsServiceRequest, projectID bson.ObjectID, now time.Time) []model.LogRecord {
	var out []model.LogRecord
	for _, rl := range req.GetResourceLogs() {
		resAttrs := rl.GetResource().GetAttributes()
		service := serviceName(resAttrs)
		for _, sl := range rl.GetScopeLogs() {
			for _, lr := range sl.GetLogRecords() {
				out = append(out, convertLog(lr, resAttrs, service, projectID, now))
			}
		}
	}
	return out
}

func convertLog(lr *logspb.LogRecord, resAttrs []*commonpb.KeyValue, service string, projectID bson.ObjectID, now time.Time) model.LogRecord {
	attrs := flatten(resAttrs, lr.GetAttributes())

	// The stack trace has its own field; keep it out of the attributes.
	stack, _ := attrs[attrExceptionStack].(string)
	delete(attrs, attrExceptionStack)

	message := anyToString(lr.GetBody())
	if message == "" {
		// Exception events often have no body; build one from the exception.
		excType, _ := attrs[attrExceptionType].(string)
		excMsg, _ := attrs[attrExceptionMessage].(string)
		message = strings.TrimSpace(strings.TrimSuffix(excType+": "+excMsg, ": "))
		message = strings.TrimPrefix(message, ": ")
	}
	if message == "" {
		message = "(empty log body)"
	}

	if id := lr.GetTraceId(); len(id) > 0 {
		attrs["trace_id"] = hex.EncodeToString(id)
	}
	if id := lr.GetSpanId(); len(id) > 0 {
		attrs["span_id"] = hex.EncodeToString(id)
	}
	if text := lr.GetSeverityText(); text != "" {
		attrs["otel.severity_text"] = cut(text, maxAttrValueBytes)
	}

	return model.LogRecord{
		ID:          bson.NewObjectID(),
		ProjectID:   projectID,
		ServiceName: service,
		Severity:    severity(lr.GetSeverityNumber(), lr.GetSeverityText()),
		Message:     cut(message, maxMessageBytes),
		StackTrace:  cut(stack, maxStackBytes),
		Attributes:  limitAttrs(attrs),
		Timestamp:   timestamp(lr.GetTimeUnixNano(), lr.GetObservedTimeUnixNano(), now),
		ReceivedAt:  now,
	}
}

// severity maps the OpenTelemetry severity number (1–24) onto the four
// levels this system stores. TRACE and DEBUG are kept as INFO (the exact
// text is kept in the otel.severity_text attribute); FATAL becomes CRITICAL.
func severity(num logspb.SeverityNumber, text string) string {
	switch {
	case num >= logspb.SeverityNumber_SEVERITY_NUMBER_FATAL:
		return model.SeverityCritical
	case num >= logspb.SeverityNumber_SEVERITY_NUMBER_ERROR:
		return model.SeverityError
	case num >= logspb.SeverityNumber_SEVERITY_NUMBER_WARN:
		return model.SeverityWarn
	case num > logspb.SeverityNumber_SEVERITY_NUMBER_UNSPECIFIED:
		return model.SeverityInfo
	}
	// No number: fall back to the text, e.g. "error" or "Warning".
	switch t := strings.ToUpper(strings.TrimSpace(text)); {
	case strings.HasPrefix(t, "FATAL"), strings.HasPrefix(t, "CRIT"):
		return model.SeverityCritical
	case strings.HasPrefix(t, "ERR"):
		return model.SeverityError
	case strings.HasPrefix(t, "WARN"):
		return model.SeverityWarn
	default:
		return model.SeverityInfo
	}
}

// MetricsResult is the outcome of converting a metrics export.
type MetricsResult struct {
	Points   []model.MetricPoint
	Rejected int // data points of unsupported kinds (exponential histogram, summary)
}

// Metrics converts an OTLP metrics export into metric points for one project.
// Gauge, sum and explicit-bucket histogram are supported.
func Metrics(req *colmetricspb.ExportMetricsServiceRequest, projectID bson.ObjectID, now time.Time) MetricsResult {
	var res MetricsResult
	for _, rm := range req.GetResourceMetrics() {
		resAttrs := rm.GetResource().GetAttributes()
		service := serviceName(resAttrs)
		for _, sm := range rm.GetScopeMetrics() {
			for _, m := range sm.GetMetrics() {
				base := model.MetricPoint{
					ProjectID:   projectID,
					ServiceName: service,
					MetricName:  cut(m.GetName(), maxMetricNameLen),
					Unit:        cut(m.GetUnit(), maxUnitLen),
					ReceivedAt:  now,
				}
				switch {
				case m.GetGauge() != nil:
					for _, dp := range m.GetGauge().GetDataPoints() {
						res.Points = append(res.Points, numberPoint(base, model.MetricGauge, dp, resAttrs, now))
					}
				case m.GetSum() != nil:
					sumBase := base
					sumBase.Temporality = temporality(m.GetSum().GetAggregationTemporality())
					monotonic := m.GetSum().GetIsMonotonic()
					sumBase.Monotonic = &monotonic
					for _, dp := range m.GetSum().GetDataPoints() {
						res.Points = append(res.Points, numberPoint(sumBase, model.MetricSum, dp, resAttrs, now))
					}
				case m.GetHistogram() != nil:
					histBase := base
					histBase.Temporality = temporality(m.GetHistogram().GetAggregationTemporality())
					for _, dp := range m.GetHistogram().GetDataPoints() {
						res.Points = append(res.Points, histogramPoint(histBase, dp, resAttrs, now))
					}
				case m.GetExponentialHistogram() != nil:
					res.Rejected += len(m.GetExponentialHistogram().GetDataPoints())
				case m.GetSummary() != nil:
					res.Rejected += len(m.GetSummary().GetDataPoints())
				}
			}
		}
	}
	return res
}

// temporality turns the OTLP enum into the stored text ("" if unspecified).
func temporality(t metricspb.AggregationTemporality) string {
	switch t {
	case metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE:
		return model.TemporalityCumulative
	case metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA:
		return model.TemporalityDelta
	default:
		return ""
	}
}

func numberPoint(base model.MetricPoint, kind string, dp *metricspb.NumberDataPoint, resAttrs []*commonpb.KeyValue, now time.Time) model.MetricPoint {
	p := base
	p.ID = bson.NewObjectID()
	p.MetricType = kind
	var v float64
	switch dp.GetValue().(type) {
	case *metricspb.NumberDataPoint_AsInt:
		v = float64(dp.GetAsInt())
	default:
		v = dp.GetAsDouble()
	}
	p.Value = &v
	p.Attributes = limitAttrs(flatten(resAttrs, dp.GetAttributes()))
	p.Timestamp = timestamp(dp.GetTimeUnixNano(), 0, now)
	p.StartTime = startTime(dp.GetStartTimeUnixNano())
	return p
}

// startTime converts a series' start time; nil when not sent.
func startTime(nano uint64) *time.Time {
	if nano == 0 {
		return nil
	}
	t := time.Unix(0, int64(nano)).UTC().Truncate(time.Millisecond)
	return &t
}

func histogramPoint(base model.MetricPoint, dp *metricspb.HistogramDataPoint, resAttrs []*commonpb.KeyValue, now time.Time) model.MetricPoint {
	p := base
	p.ID = bson.NewObjectID()
	p.MetricType = model.MetricHistogram

	count := int64(dp.GetCount())
	p.Count = &count
	if dp.Sum != nil {
		s := dp.GetSum()
		p.Sum = &s
	}
	if dp.Min != nil {
		v := dp.GetMin()
		p.Min = &v
	}
	if dp.Max != nil {
		v := dp.GetMax()
		p.Max = &v
	}

	bounds := dp.GetExplicitBounds()
	counts := dp.GetBucketCounts()
	if len(counts) == len(bounds)+1 {
		p.BucketBounds = append([]float64{}, bounds...)
		p.BucketCounts = make([]int64, len(counts))
		for i, c := range counts {
			p.BucketCounts[i] = int64(c)
		}
	} else {
		// No (or inconsistent) buckets: keep one bucket holding everything,
		// so count and sum are still usable.
		p.BucketCounts = []int64{count}
	}

	p.Attributes = limitAttrs(flatten(resAttrs, dp.GetAttributes()))
	p.Timestamp = timestamp(dp.GetTimeUnixNano(), 0, now)
	p.StartTime = startTime(dp.GetStartTimeUnixNano())
	return p
}

// serviceName reads the service.name resource attribute.
func serviceName(resAttrs []*commonpb.KeyValue) string {
	for _, kv := range resAttrs {
		if kv.GetKey() == attrServiceName {
			if s := strings.TrimSpace(kv.GetValue().GetStringValue()); s != "" {
				return cut(s, maxServiceNameLen)
			}
		}
	}
	return unknownService
}

// timestamp picks the event time, falling back to the observed time and
// then to now. Times too far in the future (a wrong clock) become now.
func timestamp(eventNano, observedNano uint64, now time.Time) time.Time {
	nano := eventNano
	if nano == 0 {
		nano = observedNano
	}
	if nano == 0 {
		return now
	}
	t := time.Unix(0, int64(nano)).UTC().Truncate(time.Millisecond)
	if t.After(now.Add(maxClockSkew)) {
		return now
	}
	return t
}

// flatten merges resource and record attributes into one flat map
// (record attributes win). Arrays and nested lists become JSON text.
// service.name is left out because it already has its own field.
func flatten(resAttrs, recAttrs []*commonpb.KeyValue) map[string]any {
	out := make(map[string]any, len(resAttrs)+len(recAttrs))
	for _, list := range [][]*commonpb.KeyValue{resAttrs, recAttrs} {
		for _, kv := range list {
			k := kv.GetKey()
			if k == "" || k == attrServiceName || strings.HasPrefix(k, "$") || strings.ContainsRune(k, 0) {
				continue
			}
			if v, ok := anyToValue(kv.GetValue()); ok {
				out[cut(k, maxAttrKeyLen)] = v
			}
		}
	}
	return out
}

// limitAttrs keeps at most maxAttributes keys (alphabetically first, so
// the result is stable) and returns nil for an empty map.
func limitAttrs(attrs map[string]any) map[string]any {
	if len(attrs) == 0 {
		return nil
	}
	if len(attrs) <= maxAttributes {
		return attrs
	}
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]any, maxAttributes)
	for _, k := range keys[:maxAttributes] {
		out[k] = attrs[k]
	}
	return out
}

// anyToValue turns an OTLP value into text, a number or true/false.
// It reports false for an empty value.
func anyToValue(v *commonpb.AnyValue) (any, bool) {
	switch x := v.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		return cut(x.StringValue, maxAttrValueBytes), true
	case *commonpb.AnyValue_BoolValue:
		return x.BoolValue, true
	case *commonpb.AnyValue_IntValue:
		return float64(x.IntValue), true
	case *commonpb.AnyValue_DoubleValue:
		return x.DoubleValue, true
	case nil:
		return nil, false
	default:
		return cut(anyToString(v), maxAttrValueBytes), true
	}
}

// anyToString renders any OTLP value as text (used for log bodies).
func anyToString(v *commonpb.AnyValue) string {
	switch x := v.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		return x.StringValue
	case nil:
		return ""
	default:
		b, err := json.Marshal(anyToJSON(v))
		if err != nil {
			return ""
		}
		return string(b)
	}
}

// anyToJSON converts an OTLP value into plain Go values for JSON output.
func anyToJSON(v *commonpb.AnyValue) any {
	switch x := v.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		return x.StringValue
	case *commonpb.AnyValue_BoolValue:
		return x.BoolValue
	case *commonpb.AnyValue_IntValue:
		return x.IntValue
	case *commonpb.AnyValue_DoubleValue:
		return x.DoubleValue
	case *commonpb.AnyValue_BytesValue:
		return hex.EncodeToString(x.BytesValue)
	case *commonpb.AnyValue_ArrayValue:
		out := make([]any, 0, len(x.ArrayValue.GetValues()))
		for _, e := range x.ArrayValue.GetValues() {
			out = append(out, anyToJSON(e))
		}
		return out
	case *commonpb.AnyValue_KvlistValue:
		out := make(map[string]any, len(x.KvlistValue.GetValues()))
		for _, kv := range x.KvlistValue.GetValues() {
			out[kv.GetKey()] = anyToJSON(kv.GetValue())
		}
		return out
	default:
		return nil
	}
}

// cut shortens s to at most n bytes without splitting a UTF-8 character.
func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && n < len(s) && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
}
