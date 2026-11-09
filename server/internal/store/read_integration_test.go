//go:build integration

package store

import (
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

func logAt(project bson.ObjectID, service, severity, msg string, ts time.Time, attrs map[string]any) model.LogRecord {
	return model.LogRecord{
		ID: bson.NewObjectID(), ProjectID: project, ServiceName: service, Severity: severity,
		Message: msg, Attributes: attrs, Timestamp: ts, ReceivedAt: ts,
	}
}

func TestReadLogsAndServices(t *testing.T) {
	s := newSchemaStore(t)
	ctx := testCtx(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	project, other := bson.NewObjectID(), bson.NewObjectID()
	node := map[string]any{"telemetry.sdk.language": "nodejs"}

	errs := []model.LogRecord{
		logAt(project, "order-api", model.SeverityError, "payment gateway timeout", now.Add(-1*time.Minute), node),
		logAt(project, "order-worker", model.SeverityCritical, "pool exhausted", now.Add(-3*time.Minute), nil),
		logAt(other, "order-api", model.SeverityError, "someone else's error", now.Add(-1*time.Minute), nil),
	}
	for i := range errs {
		errs[i].DiagnosisStatus = model.DiagnosisPending
	}
	apps := []model.LogRecord{
		logAt(project, "order-api", model.SeverityInfo, "order placed", now.Add(-2*time.Minute), node),
		logAt(project, "order-api", model.SeverityWarn, "slow query (a.b+c)", now.Add(-4*time.Minute), node),
		logAt(project, "order-api", model.SeverityInfo, "too old", now.Add(-3*time.Hour), node),
	}
	if err := s.InsertErrorLogs(ctx, errs); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertAppLogs(ctx, apps); err != nil {
		t.Fatal(err)
	}
	since := now.Add(-time.Hour)

	t.Run("merges both collections newest first, only this project and range", func(t *testing.T) {
		got, err := s.Logs(ctx, model.LogQuery{ProjectID: project, Since: since})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"payment gateway timeout", "order placed", "pool exhausted", "slow query (a.b+c)"}
		if len(got) != len(want) {
			t.Fatalf("got %d logs: %+v", len(got), got)
		}
		for i := range want {
			if got[i].Message != want[i] {
				t.Errorf("logs[%d] = %q, want %q", i, got[i].Message, want[i])
			}
		}
	})

	t.Run("filters by severity, service and search text", func(t *testing.T) {
		got, _ := s.Logs(ctx, model.LogQuery{ProjectID: project, Since: since, Severities: []string{model.SeverityWarn, model.SeverityCritical}})
		if len(got) != 2 {
			t.Errorf("severity filter: %d logs", len(got))
		}
		got, _ = s.Logs(ctx, model.LogQuery{ProjectID: project, Since: since, Service: "order-worker"})
		if len(got) != 1 || got[0].Message != "pool exhausted" {
			t.Errorf("service filter: %+v", got)
		}
		// Regex characters in the search are taken literally.
		got, _ = s.Logs(ctx, model.LogQuery{ProjectID: project, Since: since, Search: "QUERY (A.B+C)"})
		if len(got) != 1 {
			t.Errorf("search: %d logs", len(got))
		}
		got, _ = s.Logs(ctx, model.LogQuery{ProjectID: project, Since: since, Search: "a.b*"})
		if len(got) != 0 {
			t.Errorf("search treated input as a pattern: %d logs", len(got))
		}
	})

	t.Run("pages with a before-cursor without gaps or repeats", func(t *testing.T) {
		page1, _ := s.Logs(ctx, model.LogQuery{ProjectID: project, Since: since, Limit: 2})
		last := page1[len(page1)-1]
		page2, _ := s.Logs(ctx, model.LogQuery{ProjectID: project, Since: since, Limit: 2, BeforeTime: last.Timestamp, BeforeID: last.ID})
		if len(page1) != 2 || len(page2) != 2 || page2[0].Message != "pool exhausted" {
			t.Errorf("page1 %v, page2 %v", page1, page2)
		}
	})

	t.Run("filters errors by diagnosis status", func(t *testing.T) {
		got, _ := s.Logs(ctx, model.LogQuery{ProjectID: project, Since: since, DiagnosisStatus: model.DiagnosisPending})
		if len(got) != 2 {
			t.Errorf("pending errors: %d", len(got))
		}
	})

	t.Run("ErrorByID only finds this project's errors", func(t *testing.T) {
		got, err := s.ErrorByID(ctx, project, errs[0].ID)
		if err != nil || got.Message != "payment gateway timeout" {
			t.Errorf("got %+v, %v", got, err)
		}
		if _, err := s.ErrorByID(ctx, project, errs[2].ID); !errors.Is(err, model.ErrNotFound) {
			t.Errorf("other project's error: %v", err)
		}
		if _, err := s.ErrorByID(ctx, project, apps[0].ID); !errors.Is(err, model.ErrNotFound) {
			t.Errorf("an INFO log was returned as an error: %v", err)
		}
	})

	t.Run("CountErrors", func(t *testing.T) {
		total, pending, err := s.CountErrors(ctx, project, since)
		if err != nil || total != 2 || pending != 2 {
			t.Errorf("total %d, pending %d, %v", total, pending, err)
		}
	})

	t.Run("Services lists each service with counts and language", func(t *testing.T) {
		v := 1.0
		_ = s.InsertMetrics(ctx, []model.MetricPoint{{
			ID: bson.NewObjectID(), ProjectID: project, ServiceName: "order-cron", MetricName: "jobs", MetricType: "gauge",
			Value: &v, Attributes: map[string]any{"telemetry.sdk.language": "java"}, Timestamp: now, ReceivedAt: now,
		}})
		got, err := s.Services(ctx, project, since)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 3 {
			t.Fatalf("services = %+v", got)
		}
		api, cron, worker := got[0], got[1], got[2]
		if api.Name != "order-api" || api.Language != "nodejs" || api.Logs != 3 || api.Errors != 1 {
			t.Errorf("order-api = %+v", api)
		}
		if cron.Name != "order-cron" || cron.Language != "java" || cron.Metrics != 1 || !cron.LastSeen.Equal(now) {
			t.Errorf("order-cron = %+v", cron)
		}
		if worker.Name != "order-worker" || worker.Language != "" || worker.Errors != 1 {
			t.Errorf("order-worker = %+v", worker)
		}
	})
}

func TestReadMetrics(t *testing.T) {
	s := newSchemaStore(t)
	ctx := testCtx(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	project := bson.NewObjectID()
	f := func(v float64) *float64 { return &v }

	points := []model.MetricPoint{
		{ID: bson.NewObjectID(), ProjectID: project, ServiceName: "db", MetricName: "pool", MetricType: "gauge", Unit: "{conn}", Value: f(3), Timestamp: now.Add(-2 * time.Minute)},
		{ID: bson.NewObjectID(), ProjectID: project, ServiceName: "db", MetricName: "pool", MetricType: "gauge", Unit: "{conn}", Value: f(9), Timestamp: now.Add(-1 * time.Minute)},
		{ID: bson.NewObjectID(), ProjectID: project, ServiceName: "api", MetricName: "requests", MetricType: "sum", Value: f(10), Timestamp: now.Add(-1 * time.Minute)},
		{ID: bson.NewObjectID(), ProjectID: bson.NewObjectID(), ServiceName: "api", MetricName: "requests", MetricType: "sum", Value: f(99), Timestamp: now},
	}
	if err := s.InsertMetrics(ctx, points); err != nil {
		t.Fatal(err)
	}

	t.Run("catalog has one row per metric and service with the latest value", func(t *testing.T) {
		got, err := s.MetricCatalog(ctx, project, now.Add(-time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].Name != "pool" || got[0].Points != 2 || *got[0].LastValue != 9 || got[0].Unit != "{conn}" ||
			got[1].Name != "requests" || *got[1].LastValue != 10 {
			t.Errorf("catalog = %+v", got)
		}
	})

	t.Run("points are filtered, ordered and truncated", func(t *testing.T) {
		got, truncated, err := s.MetricPoints(ctx, model.MetricQuery{ProjectID: project, Names: []string{"pool"}, Since: now.Add(-time.Hour), Until: now.Add(time.Minute)})
		if err != nil || truncated || len(got) != 2 || *got[0].Value != 3 {
			t.Fatalf("got %+v, %v, %v", got, truncated, err)
		}
		got, truncated, _ = s.MetricPoints(ctx, model.MetricQuery{ProjectID: project, Since: now.Add(-time.Hour), Until: now.Add(time.Minute), Limit: 2})
		if len(got) != 2 || !truncated {
			t.Errorf("limit: %d points, truncated %v", len(got), truncated)
		}
	})
}
