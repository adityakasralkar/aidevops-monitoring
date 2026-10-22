//go:build integration

package store

import (
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

func testLogs(n int, severity string, project bson.ObjectID) []model.LogRecord {
	now := time.Now().UTC().Truncate(time.Millisecond)
	out := make([]model.LogRecord, n)
	for i := range out {
		out[i] = model.LogRecord{
			ID: bson.NewObjectID(), ProjectID: project, ServiceName: "svc", Severity: severity,
			Message: "m", Attributes: map[string]any{"http.route": "/pay", "n": float64(i)},
			Timestamp: now, ReceivedAt: now,
		}
	}
	return out
}

func TestInsertLogs(t *testing.T) {
	s := newSchemaStore(t)
	ctx := testCtx(t)
	project := bson.NewObjectID()

	errs := testLogs(3, model.SeverityError, project)
	for i := range errs {
		errs[i].DiagnosisStatus = model.DiagnosisPending
	}
	apps := testLogs(5, model.SeverityInfo, project)

	if err := s.InsertErrorLogs(ctx, errs); err != nil {
		t.Fatalf("insert error logs: %v", err)
	}
	if err := s.InsertAppLogs(ctx, apps); err != nil {
		t.Fatalf("insert app logs: %v", err)
	}

	count := func(coll string) int64 {
		n, err := s.Collection(coll).CountDocuments(ctx, bson.D{{Key: "project_id", Value: project}})
		if err != nil {
			t.Fatalf("count %s: %v", coll, err)
		}
		return n
	}
	if count(CollErrorLogs) != 3 || count(CollAppLogs) != 5 {
		t.Fatalf("ErrorLogs=%d AppLogs=%d, want 3 and 5", count(CollErrorLogs), count(CollAppLogs))
	}

	t.Run("stores fields including dotted attribute names", func(t *testing.T) {
		var got model.LogRecord
		if err := s.Collection(CollErrorLogs).FindOne(ctx, bson.D{{Key: "_id", Value: errs[0].ID}}).Decode(&got); err != nil {
			t.Fatalf("find: %v", err)
		}
		if got.DiagnosisStatus != model.DiagnosisPending || got.Attributes["http.route"] != "/pay" || !got.Timestamp.Equal(errs[0].Timestamp) {
			t.Errorf("stored log = %+v", got)
		}
	})

	t.Run("retrying a fully saved batch succeeds without duplicates", func(t *testing.T) {
		if err := s.InsertAppLogs(ctx, apps); err != nil {
			t.Fatalf("retry: %v", err)
		}
		if n := count(CollAppLogs); n != 5 {
			t.Errorf("AppLogs = %d after retry, want 5", n)
		}
	})

	t.Run("retrying a partly saved batch saves only the missing logs", func(t *testing.T) {
		more := testLogs(2, model.SeverityWarn, project)
		mixed := append(append([]model.LogRecord{}, apps...), more...)
		if err := s.InsertAppLogs(ctx, mixed); err != nil {
			t.Fatalf("partial retry: %v", err)
		}
		if n := count(CollAppLogs); n != 7 {
			t.Errorf("AppLogs = %d, want 7", n)
		}
	})

	t.Run("an empty batch is a no-op", func(t *testing.T) {
		if err := s.InsertAppLogs(ctx, nil); err != nil {
			t.Errorf("err = %v", err)
		}
	})
}

func TestProjectByAPIKeyHash(t *testing.T) {
	s := newSchemaStore(t)
	ctx := testCtx(t)
	p := model.Project{OwnerID: bson.NewObjectID(), Name: "shop", Environment: "test", APIKeyHash: "keyhash", KeyPrefix: "aidv_test_abcd", CreatedAt: time.Now().UTC().Truncate(time.Millisecond)}
	if err := s.CreateProject(ctx, &p); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := s.ProjectByAPIKeyHash(ctx, "keyhash")
	if err != nil || got.ID != p.ID || got.Environment != "test" {
		t.Errorf("got %+v, %v", got, err)
	}
	if _, err := s.ProjectByAPIKeyHash(ctx, "nope"); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("unknown hash: err = %v, want ErrNotFound", err)
	}
}
