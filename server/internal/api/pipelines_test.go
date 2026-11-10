package api

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

func ciLog(run, severity, step string, at time.Time) model.LogRecord {
	attrs := map[string]any{
		"ci.run.id": run, "ci.pipeline": "CI", "ci.run.url": "https://github.com/o/r/actions/runs/" + run,
		"vcs.branch": "main", "vcs.commit.sha": "abc1234def", "vcs.commit.author": "aditya",
		"vcs.commit.message": "Add invoice totals",
	}
	if step != "" {
		attrs["ci.failed_step"] = step
	}
	return model.LogRecord{
		ID: bson.NewObjectID(), ServiceName: "shop-ci", Severity: severity,
		Message: "result of " + run, StackTrace: "output " + step, Attributes: attrs, Timestamp: at,
	}
}

func TestGroupRuns(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	logs := []model.LogRecord{
		ciLog("2", model.SeverityError, "Client lint", now),
		ciLog("2", model.SeverityCritical, "Server tests", now.Add(-time.Second)),
		ciLog("1", model.SeverityInfo, "", now.Add(-time.Hour)),
		{ID: bson.NewObjectID(), Severity: model.SeverityError, Message: "not from CI", Timestamp: now},
	}
	runs := groupRuns(logs)
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2 (the non-CI log is ignored)", len(runs))
	}
	failed, passed := runs[0], runs[1]
	if failed.RunID != "2" || failed.Result != "failed" || len(failed.FailedSteps) != 2 || !failed.At.Equal(now) {
		t.Errorf("failed run = %+v", failed)
	}
	if failed.FailedSteps[0].Name != "Server tests" {
		t.Errorf("most serious step should come first, got %q", failed.FailedSteps[0].Name)
	}
	if passed.RunID != "1" || passed.Result != "passed" || len(passed.FailedSteps) != 0 ||
		passed.Branch != "main" || passed.CommitSHA != "abc1234def" || passed.Author != "aditya" || passed.URL == "" {
		t.Errorf("passed run = %+v", passed)
	}
}

func TestGroupRunsCapsTheList(t *testing.T) {
	t.Parallel()
	now := time.Now()
	var logs []model.LogRecord
	for i := range maxPipelineRuns + 5 {
		logs = append(logs, ciLog(bson.NewObjectID().Hex(), model.SeverityInfo, "", now.Add(-time.Duration(i)*time.Minute)))
	}
	if got := groupRuns(logs); len(got) != maxPipelineRuns {
		t.Errorf("got %d runs, want %d", len(got), maxPipelineRuns)
	}
}

func TestPipelinesRoute(t *testing.T) {
	t.Parallel()
	e, token, p := readEnv(t)
	e.reader.ciLogs = []model.LogRecord{ciLog("7", model.SeverityCritical, "Server tests", e.clock.Now())}

	rec := e.get(t, token, "/api/v1/projects/"+p.ID+"/pipelines")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var got struct {
		Range string        `json:"range"`
		Runs  []pipelineRun `json:"runs"`
	}
	decodeBody(t, rec, &got)
	if got.Range != "7d" || len(got.Runs) != 1 || got.Runs[0].Result != "failed" || got.Runs[0].FailedSteps[0].Output == "" {
		t.Errorf("got %+v", got)
	}

	t.Run("store failure is a 500 without details", func(t *testing.T) {
		e.reader.failWith = errors.New("mongo-host:27017 down")
		rec := e.get(t, token, "/api/v1/projects/"+p.ID+"/pipelines")
		wantError(t, rec, http.StatusInternalServerError, codeInternalError)
	})
}
