package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

// readEnv is a test env with one logged-in user owning one project.
func readEnv(t *testing.T) (*testEnv, string, projectResponse) {
	t.Helper()
	e := newTestEnv(t)
	token := e.loggedIn(t, testEmail)
	p := e.newProject(t, token, "shop", "live")
	return e, token, p
}

func (e *testEnv) get(t *testing.T, token, path string) *httpRecorder {
	t.Helper()
	return e.do(t, request{method: "GET", path: path, token: token})
}

func TestReadRoutesCheckOwnershipAndRange(t *testing.T) {
	t.Parallel()
	e, _, p := readEnv(t)
	other := e.loggedIn(t, "other@example.com")
	base := "/api/v1/projects/" + p.ID
	routes := []string{"/services", "/overview", "/logs", "/errors", "/errors/" + bson.NewObjectID().Hex(), "/metrics", "/metrics/series?name=x"}

	for _, route := range routes {
		t.Run("another user gets 404 on "+route, func(t *testing.T) {
			wantError(t, e.get(t, other, base+route), http.StatusNotFound, codeNotFound)
		})
		t.Run("login required on "+route, func(t *testing.T) {
			wantError(t, e.get(t, "", base+route), http.StatusUnauthorized, codeMissingToken)
		})
	}
	t.Run("an unknown range is rejected", func(t *testing.T) {
		token := e.login(t, testEmail, testPassword)
		wantError(t, e.get(t, token, base+"/overview?range=2h"), http.StatusBadRequest, codeValidationFailed)
	})
}

func TestServicesRoute(t *testing.T) {
	t.Parallel()
	e, token, p := readEnv(t)
	e.reader.services = []model.ServiceInfo{{Name: "order-api", Language: "nodejs", LastSeen: e.clock.Now(), Logs: 4, Errors: 1}}
	rec := e.get(t, token, "/api/v1/projects/"+p.ID+"/services")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got struct {
		Range    string            `json:"range"`
		Services []serviceResponse `json:"services"`
	}
	decodeBody(t, rec, &got)
	if got.Range != "24h" || len(got.Services) != 1 || got.Services[0].Language != "nodejs" || got.Services[0].Errors != 1 {
		t.Errorf("got %+v", got)
	}
}

func makeLogs(n int, now time.Time) []model.LogRecord {
	out := make([]model.LogRecord, n)
	for i := range out {
		out[i] = model.LogRecord{ID: bson.NewObjectID(), ServiceName: "api", Severity: model.SeverityInfo, Message: "m", Timestamp: now.Add(-time.Duration(i) * time.Second)}
	}
	return out
}

func TestLogsRoute(t *testing.T) {
	t.Parallel()
	e, token, p := readEnv(t)
	base := "/api/v1/projects/" + p.ID + "/logs"
	e.reader.logs = makeLogs(3, e.clock.Now())

	t.Run("passes filters to the query", func(t *testing.T) {
		rec := e.get(t, token, base+"?level=warn,ERROR&service=order-api&q=timeout&range=24h&limit=10")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d %s", rec.Code, rec.Body)
		}
		q := e.reader.lastLogQuery
		if strings.Join(q.Severities, ",") != "WARN,ERROR" || q.Service != "order-api" || q.Search != "timeout" || q.Limit != 10 ||
			!q.Since.Equal(e.clock.Now().Add(-24*time.Hour)) || q.ProjectID.Hex() != p.ID {
			t.Errorf("query = %+v", q)
		}
	})
	t.Run("returns a next cursor only for a full page, and it round-trips", func(t *testing.T) {
		var page logPage
		decodeBody(t, e.get(t, token, base+"?limit=3"), &page)
		if len(page.Items) != 3 || page.NextCursor == "" {
			t.Fatalf("page = %+v", page)
		}
		e.get(t, token, base+"?limit=3&cursor="+page.NextCursor)
		last := e.reader.logs[2]
		if !e.reader.lastLogQuery.BeforeTime.Equal(last.Timestamp) || e.reader.lastLogQuery.BeforeID != last.ID {
			t.Errorf("cursor decoded as %v / %v", e.reader.lastLogQuery.BeforeTime, e.reader.lastLogQuery.BeforeID)
		}
		var partial logPage
		decodeBody(t, e.get(t, token, base+"?limit=10"), &partial)
		if partial.NextCursor != "" {
			t.Errorf("partial page has a cursor %q", partial.NextCursor)
		}
	})
	for name, query := range map[string]string{
		"an unknown level": "?level=DEBUG",
		"a bad limit":      "?limit=500",
		"a bad cursor":     "?cursor=nope",
		"a long search":    "?q=" + strings.Repeat("x", maxSearchLen+1),
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			wantError(t, e.get(t, token, base+query), http.StatusBadRequest, codeValidationFailed)
		})
	}
	t.Run("returns 500 without details when the store fails", func(t *testing.T) {
		e.reader.failWith = errDB
		defer func() { e.reader.failWith = nil }()
		wantError(t, e.get(t, token, base), http.StatusInternalServerError, codeInternalError)
	})
}

func TestErrorsRoutes(t *testing.T) {
	t.Parallel()
	e, token, p := readEnv(t)
	base := "/api/v1/projects/" + p.ID + "/errors"

	t.Run("list is errors only, last 24h by default, with status filter", func(t *testing.T) {
		if rec := e.get(t, token, base+"?status=pending"); rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		q := e.reader.lastLogQuery
		if strings.Join(q.Severities, ",") != "ERROR,CRITICAL" || q.DiagnosisStatus != model.DiagnosisPending || !q.Since.Equal(e.clock.Now().Add(-24*time.Hour)) {
			t.Errorf("query = %+v", q)
		}
		wantError(t, e.get(t, token, base+"?status=DONE"), http.StatusBadRequest, codeValidationFailed)
	})

	t.Run("detail includes metrics and related logs from before the error", func(t *testing.T) {
		at := e.clock.Now().Add(-time.Minute)
		errLog := model.LogRecord{ID: bson.NewObjectID(), ServiceName: "order-api", Severity: model.SeverityCritical, Message: "pool exhausted", Timestamp: at, StackTrace: "at pool.go:1", DiagnosisStatus: model.DiagnosisPending}
		e.reader.errByID[errLog.ID] = errLog
		e.reader.logs = []model.LogRecord{errLog, {ID: bson.NewObjectID(), ServiceName: "order-api", Severity: model.SeverityWarn, Message: "slow query", Timestamp: at.Add(-time.Minute)}}
		e.reader.points = []model.MetricPoint{
			{ServiceName: "db", MetricName: "db.client.connection.count", MetricType: model.MetricGauge, Value: f64p(48), Timestamp: at.Add(-2 * time.Minute)},
			{ServiceName: "db", MetricName: "db.client.connection.count", MetricType: model.MetricGauge, Value: f64p(50), Timestamp: at.Add(-30 * time.Second)},
		}

		rec := e.get(t, token, base+"/"+errLog.ID.Hex())
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d %s", rec.Code, rec.Body)
		}
		var got errorDetailResponse
		decodeBody(t, rec, &got)
		if got.Message != "pool exhausted" || got.StackTrace == "" || got.DiagnosisStatus != model.DiagnosisPending {
			t.Errorf("error = %+v", got.logResponse)
		}
		if !got.Context.WindowStart.Equal(at.Add(-errorContextWindow)) {
			t.Errorf("window start = %s", got.Context.WindowStart)
		}
		if len(got.Context.RelatedLogs) != 1 || got.Context.RelatedLogs[0].Message != "slow query" {
			t.Errorf("related logs = %+v (the error itself must be left out)", got.Context.RelatedLogs)
		}
		if len(got.Context.Metrics) != 1 {
			t.Fatalf("metrics = %+v", got.Context.Metrics)
		}
		var values []float64
		for _, pt := range got.Context.Metrics[0].Points {
			if pt.Value != nil {
				values = append(values, *pt.Value)
			}
		}
		if len(values) != 2 || values[0] != 48 || values[1] != 50 {
			t.Errorf("pool values = %v", values)
		}
		if q := e.reader.lastLogQuery; q.Service != "order-api" || !q.Since.Equal(at.Add(-errorContextWindow)) {
			t.Errorf("related logs query = %+v", q)
		}
	})

	t.Run("an unknown or invalid error id is 404", func(t *testing.T) {
		wantError(t, e.get(t, token, base+"/"+bson.NewObjectID().Hex()), http.StatusNotFound, codeNotFound)
		wantError(t, e.get(t, token, base+"/nope"), http.StatusNotFound, codeNotFound)
	})
}

func TestMetricRoutes(t *testing.T) {
	t.Parallel()
	e, token, p := readEnv(t)
	base := "/api/v1/projects/" + p.ID + "/metrics"
	now := e.clock.Now()

	t.Run("catalog", func(t *testing.T) {
		e.reader.catalog = []model.MetricInfo{{Name: "cpu", Service: "api", Type: model.MetricGauge, LastValue: f64p(0.7), Points: 5}}
		var got struct {
			Metrics []metricInfoResponse `json:"metrics"`
		}
		decodeBody(t, e.get(t, token, base), &got)
		if len(got.Metrics) != 1 || got.Metrics[0].Name != "cpu" || *got.Metrics[0].LastValue != 0.7 {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("series needs a name", func(t *testing.T) {
		wantError(t, e.get(t, token, base+"/series"), http.StatusBadRequest, codeValidationFailed)
	})

	t.Run("counter series shows what was added per step", func(t *testing.T) {
		mono := true
		e.reader.points = []model.MetricPoint{
			{ServiceName: "api", MetricName: "requests", MetricType: model.MetricSum, Temporality: model.TemporalityCumulative, Monotonic: &mono, Value: f64p(100), Timestamp: now.Add(-61 * time.Minute)},
			{ServiceName: "api", MetricName: "requests", MetricType: model.MetricSum, Temporality: model.TemporalityCumulative, Monotonic: &mono, Value: f64p(130), Timestamp: now.Add(-30 * time.Minute)},
			{ServiceName: "api", MetricName: "other", MetricType: model.MetricGauge, Value: f64p(1), Timestamp: now.Add(-30 * time.Minute)},
		}
		rec := e.get(t, token, base+"/series?name=requests")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		var got struct {
			Series metricSeriesOut `json:"series"`
		}
		decodeBody(t, rec, &got)
		s := got.Series
		if s.Name != "requests" || s.Type != model.MetricSum || s.StepSeconds != 60 || len(s.Points) != 60 {
			t.Fatalf("series = %+v", s)
		}
		var sum float64
		for _, pt := range s.Points {
			if pt.Value != nil {
				sum += *pt.Value
			}
		}
		if sum != 30 {
			t.Errorf("added in range = %v, want 30", sum)
		}
		if !e.reader.lastMetricQuery.Since.Equal(now.Add(-time.Hour - baselineLookback)) {
			t.Errorf("query since = %s (must include a baseline before the range)", e.reader.lastMetricQuery.Since)
		}
	})
}

func TestOverviewRoute(t *testing.T) {
	t.Parallel()
	e, token, p := readEnv(t)
	now := e.clock.Now()
	ok200 := map[string]any{"http.response.status_code": 200.0}
	fail500 := map[string]any{"http.response.status_code": 500.0}
	hp := func(at time.Time, attrs map[string]any, counts []int64) model.MetricPoint {
		pt := histPoint(at, model.TemporalityCumulative, counts, 0, attrs)
		pt.ServiceName = "order-api"
		return pt
	}
	// Two series (200 and 500), each with a baseline before the range.
	e.reader.points = []model.MetricPoint{
		hp(now.Add(-65*time.Minute), ok200, []int64{10, 0, 0}),
		hp(now.Add(-65*time.Minute), fail500, []int64{0, 1, 0}),
		hp(now.Add(-30*time.Minute), ok200, []int64{100, 0, 0}),  // +90 fast requests
		hp(now.Add(-30*time.Minute), fail500, []int64{0, 11, 0}), // +10 slow failures
	}
	e.reader.services = []model.ServiceInfo{{Name: "order-api", Language: "nodejs", Errors: 2}}
	e.reader.total, e.reader.pending = 7, 3

	rec := e.get(t, token, "/api/v1/projects/"+p.ID+"/overview")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
	var got overviewResponse
	decodeBody(t, rec, &got)

	if !got.HasHTTPMetrics || got.Requests != 100 || got.RequestsPerMin != round2(100.0/60) {
		t.Errorf("requests = %d (%v/min)", got.Requests, got.RequestsPerMin)
	}
	if got.ErrorRate == nil || *got.ErrorRate != 0.1 {
		t.Errorf("error rate = %v, want 0.1", got.ErrorRate)
	}
	// 90 requests ≤100ms, 10 in (100,500]ms: p95 falls in the second bucket.
	if got.P95 == nil || *got.P95 <= 100 || *got.P95 > 500 || got.P50 == nil || *got.P50 > 100 {
		t.Errorf("p50 = %v, p95 = %v", got.P50, got.P95)
	}
	if got.Errors != 7 || got.ErrorsPending != 3 || len(got.Series) != 60 || got.StepSeconds != 60 {
		t.Errorf("errors %d/%d, %d steps of %ds", got.Errors, got.ErrorsPending, len(got.Series), got.StepSeconds)
	}
	var stepTotal int64
	for _, st := range got.Series {
		stepTotal += st.Requests
	}
	if stepTotal != 100 {
		t.Errorf("chart adds up to %d requests, want 100", stepTotal)
	}
	if len(got.Services) != 1 || got.Services[0].Language != "nodejs" || got.Services[0].ErrorRate == nil || *got.Services[0].ErrorRate != 0.1 {
		t.Errorf("services = %+v", got.Services)
	}

	t.Run("no request metrics gives empty, not broken, numbers", func(t *testing.T) {
		e.reader.points = nil
		var empty overviewResponse
		decodeBody(t, e.get(t, token, "/api/v1/projects/"+p.ID+"/overview?range=15m"), &empty)
		if empty.HasHTTPMetrics || empty.Requests != 0 || empty.ErrorRate != nil || empty.P95 != nil || len(empty.Series) != 15 { // 15m in 1-minute steps
			t.Errorf("empty overview = %+v", empty)
		}
	})
}
