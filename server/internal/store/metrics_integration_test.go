//go:build integration

package store

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

func TestInsertMetrics(t *testing.T) {
	s := newSchemaStore(t)
	ctx := testCtx(t)
	project := bson.NewObjectID()
	now := time.Now().UTC().Truncate(time.Millisecond)
	f := func(v float64) *float64 { return &v }
	n := func(v int64) *int64 { return &v }

	metrics := []model.MetricPoint{
		{ID: bson.NewObjectID(), ProjectID: project, ServiceName: "db", MetricName: "db_active_connections", MetricType: model.MetricGauge, Value: f(0), Timestamp: now, ReceivedAt: now},
		{ID: bson.NewObjectID(), ProjectID: project, ServiceName: "api", MetricName: "http.server.duration", MetricType: model.MetricHistogram, Unit: "ms",
			Count: n(10), Sum: f(1234.5), Min: f(12), Max: f(480), BucketBounds: []float64{100, 250, 500}, BucketCounts: []int64{4, 3, 3, 0}, Timestamp: now, ReceivedAt: now},
	}
	if err := s.InsertMetrics(ctx, metrics); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := s.InsertMetrics(ctx, metrics); err != nil { // a retry must not fail or duplicate
		t.Fatalf("retry: %v", err)
	}

	count, err := s.Collection(CollTelemetryMetrics).CountDocuments(ctx, bson.D{{Key: "project_id", Value: project}})
	if err != nil || count != 2 {
		t.Fatalf("count = %d, %v; want 2", count, err)
	}

	var gauge model.MetricPoint
	if err := s.Collection(CollTelemetryMetrics).FindOne(ctx, bson.D{{Key: "_id", Value: metrics[0].ID}}).Decode(&gauge); err != nil {
		t.Fatalf("find gauge: %v", err)
	}
	if gauge.Value == nil || *gauge.Value != 0 || gauge.Count != nil {
		t.Errorf("gauge zero value not kept: %+v", gauge)
	}

	var hist model.MetricPoint
	if err := s.Collection(CollTelemetryMetrics).FindOne(ctx, bson.D{{Key: "_id", Value: metrics[1].ID}}).Decode(&hist); err != nil {
		t.Fatalf("find histogram: %v", err)
	}
	if hist.Value != nil || *hist.Count != 10 || len(hist.BucketCounts) != 4 || hist.BucketBounds[2] != 500 {
		t.Errorf("histogram not stored correctly: %+v", hist)
	}
}
