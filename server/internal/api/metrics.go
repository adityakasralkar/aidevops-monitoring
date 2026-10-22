package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

// Limits for metric ingestion (the batch limit is shared with logs).
const (
	maxBuckets = 100
	maxUnitLen = 30
)

// metricNamePattern allows names such as "http.server.duration" or
// "db_active_connections": 1–200 characters of letters, digits, ".", "_",
// "-" and "/", starting with a letter.
var metricNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._/-]{0,199}$`)

// metricInput is one metric as sent by a client. Pointers tell "not sent"
// apart from a real zero.
type metricInput struct {
	ServiceName  string         `json:"service_name"`
	MetricName   string         `json:"metric_name"`
	MetricType   string         `json:"metric_type"`
	Unit         string         `json:"unit"`
	Value        *float64       `json:"value"`
	Count        *int64         `json:"count"`
	Sum          *float64       `json:"sum"`
	Min          *float64       `json:"min"`
	Max          *float64       `json:"max"`
	BucketBounds []float64      `json:"bucket_bounds"`
	BucketCounts []int64        `json:"bucket_counts"`
	Timestamp    string         `json:"timestamp"`
	Attributes   map[string]any `json:"attributes"`
}

// handleMetrics accepts one metric object or an array of up to 500, puts
// them on the metrics queue and replies 202 without waiting for MongoDB.
// Like logs, a batch is all-or-nothing.
func (s *server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	var raw json.RawMessage
	if !decodeJSONLimit(w, r, &raw, telemetryMaxBody) {
		return
	}

	items, isBatch, msg := splitItems(raw, "metric")
	if msg != "" {
		writeError(w, http.StatusBadRequest, codeValidationFailed, msg)
		return
	}

	project := currentProject(r)
	now := s.now()
	points := make([]model.MetricPoint, len(items))
	for i, item := range items {
		p, msg := parseMetric(item, project.ID, now)
		if msg != "" {
			if isBatch {
				msg = fmt.Sprintf("metrics[%d]: %s", i, msg)
			}
			writeError(w, http.StatusBadRequest, codeValidationFailed, msg)
			return
		}
		points[i] = p
	}

	if err := s.Metrics.Enqueue(points); err != nil {
		s.writeEnqueueError(w, r, err)
		return
	}
	ids := make([]string, len(points))
	for i, p := range points {
		ids[i] = p.ID.Hex()
	}
	writeAccepted(w, isBatch, ids)
}

// parseMetric decodes and validates one metric. It returns a non-empty
// message describing the first problem found.
func parseMetric(raw json.RawMessage, projectID bson.ObjectID, now time.Time) (model.MetricPoint, string) {
	var in metricInput
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		_, msg := describeDecodeError(err, telemetryMaxBody)
		return model.MetricPoint{}, msg
	}

	service := strings.TrimSpace(in.ServiceName)
	if service == "" || len(service) > maxServiceNameLen {
		return model.MetricPoint{}, fmt.Sprintf("service_name is required and must be at most %d characters", maxServiceNameLen)
	}
	name := strings.TrimSpace(in.MetricName)
	if !metricNamePattern.MatchString(name) {
		return model.MetricPoint{}, "metric_name must be 1-200 characters: letters, digits, '.', '_', '-' or '/', starting with a letter"
	}
	unit := strings.TrimSpace(in.Unit)
	if len(unit) > maxUnitLen {
		return model.MetricPoint{}, fmt.Sprintf("unit must be at most %d characters", maxUnitLen)
	}

	metricType := strings.ToLower(strings.TrimSpace(in.MetricType))
	var msg string
	switch metricType {
	case model.MetricGauge, model.MetricSum:
		msg = checkSingleValue(in)
	case model.MetricHistogram:
		msg = checkHistogram(in)
	default:
		msg = "metric_type must be one of gauge, sum, histogram"
	}
	if msg != "" {
		return model.MetricPoint{}, msg
	}

	ts, msg := parseTimestamp(in.Timestamp, now)
	if msg != "" {
		return model.MetricPoint{}, msg
	}
	if msg := validateAttributes(in.Attributes); msg != "" {
		return model.MetricPoint{}, msg
	}

	return model.MetricPoint{
		ID:           bson.NewObjectID(),
		ProjectID:    projectID,
		ServiceName:  service,
		MetricName:   name,
		MetricType:   metricType,
		Unit:         unit,
		Value:        in.Value,
		Count:        in.Count,
		Sum:          in.Sum,
		Min:          in.Min,
		Max:          in.Max,
		BucketBounds: in.BucketBounds,
		BucketCounts: in.BucketCounts,
		Attributes:   in.Attributes,
		Timestamp:    ts,
		ReceivedAt:   now,
	}, ""
}

// checkSingleValue validates a gauge or sum: value is required and the
// histogram fields must not be sent.
func checkSingleValue(in metricInput) string {
	if in.Value == nil {
		return "value is required for gauge and sum metrics"
	}
	if in.Count != nil || in.Sum != nil || in.Min != nil || in.Max != nil || in.BucketBounds != nil || in.BucketCounts != nil {
		return "count, sum, min, max and buckets are only allowed for histogram metrics"
	}
	return ""
}

// checkHistogram validates a histogram so percentiles can be computed from
// it later: buckets are consistent and add up to count.
func checkHistogram(in metricInput) string {
	if in.Value != nil {
		return "value is not allowed for histogram metrics; use count, sum and buckets"
	}
	if in.Count == nil || in.Sum == nil {
		return "count and sum are required for histogram metrics"
	}
	if *in.Count < 0 {
		return "count must not be negative"
	}
	if in.BucketCounts == nil {
		return "bucket_counts is required for histogram metrics"
	}
	if len(in.BucketBounds) > maxBuckets {
		return fmt.Sprintf("at most %d bucket_bounds allowed", maxBuckets)
	}
	if len(in.BucketCounts) != len(in.BucketBounds)+1 {
		return fmt.Sprintf("bucket_counts must have one more entry than bucket_bounds (%d), got %d",
			len(in.BucketBounds)+1, len(in.BucketCounts))
	}
	for i := 1; i < len(in.BucketBounds); i++ {
		if in.BucketBounds[i] <= in.BucketBounds[i-1] {
			return "bucket_bounds must be strictly increasing"
		}
	}
	var total int64
	for _, c := range in.BucketCounts {
		if c < 0 {
			return "bucket_counts must not be negative"
		}
		total += c
	}
	if total != *in.Count {
		return fmt.Sprintf("bucket_counts add up to %d but count is %d", total, *in.Count)
	}
	if in.Min != nil && in.Max != nil && *in.Min > *in.Max {
		return "min must not be greater than max"
	}
	return ""
}
