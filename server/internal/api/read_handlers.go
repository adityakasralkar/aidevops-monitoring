package api

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
	"github.com/adityakasralkar/aidevops-monitoring/server/internal/stats"
)

// TelemetryReader reads a project's stored telemetry for the dashboard.
type TelemetryReader interface {
	Services(ctx context.Context, projectID bson.ObjectID, since time.Time) ([]model.ServiceInfo, error)
	Logs(ctx context.Context, q model.LogQuery) ([]model.LogRecord, error)
	ErrorByID(ctx context.Context, projectID, id bson.ObjectID) (model.LogRecord, error)
	CountErrors(ctx context.Context, projectID bson.ObjectID, since time.Time) (total, pending int64, err error)
	MetricCatalog(ctx context.Context, projectID bson.ObjectID, since time.Time) ([]model.MetricInfo, error)
	MetricPoints(ctx context.Context, q model.MetricQuery) ([]model.MetricPoint, bool, error)
	CILogs(ctx context.Context, projectID bson.ObjectID, since time.Time) ([]model.LogRecord, error)
}

// Time ranges the dashboard can ask for.
var timeRanges = map[string]time.Duration{
	"15m": 15 * time.Minute,
	"1h":  time.Hour,
	"6h":  6 * time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
}

// Page sizes for log and error lists.
const (
	defaultPageSize = 50
	maxPageSize     = 200
	maxSearchLen    = 200
)

// errorContextWindow is how much data before an error its detail shows,
// the same window the AI diagnosis reads.
const errorContextWindow = 10 * time.Minute

// timeWindow is a parsed ?range= value.
type timeWindow struct {
	name         string
	since, until time.Time
	step         time.Duration
}

// parseRange reads ?range= (15m, 1h, 6h, 24h, 7d, 30d), falling back to
// def. On a bad value it writes 400 and returns false.
func (s *server) parseRange(w http.ResponseWriter, r *http.Request, def string) (timeWindow, bool) {
	name := r.URL.Query().Get("range")
	if name == "" {
		name = def
	}
	d, ok := timeRanges[name]
	if !ok {
		writeError(w, http.StatusBadRequest, codeValidationFailed, "range must be one of 15m, 1h, 6h, 24h, 7d, 30d")
		return timeWindow{}, false
	}
	until := s.now()
	return timeWindow{name: name, since: until.Add(-d), until: until, step: stats.StepFor(d)}, true
}

// parsePageSize reads ?limit= (1–200, default 50).
func parsePageSize(w http.ResponseWriter, r *http.Request) (int, bool) {
	v := r.URL.Query().Get("limit")
	if v == "" {
		return defaultPageSize, true
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > maxPageSize {
		writeError(w, http.StatusBadRequest, codeValidationFailed, fmt.Sprintf("limit must be between 1 and %d", maxPageSize))
		return 0, false
	}
	return n, true
}

// A cursor continues a list after its last item: "<unix ms>_<id>".
func encodeCursor(l model.LogRecord) string {
	return fmt.Sprintf("%d_%s", l.Timestamp.UnixMilli(), l.ID.Hex())
}

func decodeCursor(c string) (time.Time, bson.ObjectID, bool) {
	msPart, idPart, found := strings.Cut(c, "_")
	ms, err := strconv.ParseInt(msPart, 10, 64)
	if !found || err != nil {
		return time.Time{}, bson.ObjectID{}, false
	}
	id, err := bson.ObjectIDFromHex(idPart)
	if err != nil {
		return time.Time{}, bson.ObjectID{}, false
	}
	return time.UnixMilli(ms).UTC(), id, true
}

// readFailed logs a store error and sends a generic 500.
func (s *server) readFailed(w http.ResponseWriter, r *http.Request, what string, err error, p model.Project) {
	s.Logger.ErrorContext(r.Context(), what, "error", err, "project_id", p.ID.Hex())
	writeInternalError(w)
}

// ── Services ────────────────────────────────────────────────────────────

type serviceResponse struct {
	Name     string    `json:"name"`
	Language string    `json:"language,omitempty"`
	LastSeen time.Time `json:"last_seen"`
	Logs     int64     `json:"logs"`
	Errors   int64     `json:"errors"`
	Metrics  int64     `json:"metrics"`
}

// handleServices lists the programs that sent data in the range (default
// 24h). The Connect page polls it to see a new service arrive.
func (s *server) handleServices(w http.ResponseWriter, r *http.Request) {
	p, ok := s.ownedProject(w, r)
	if !ok {
		return
	}
	win, ok := s.parseRange(w, r, "24h")
	if !ok {
		return
	}
	services, err := s.Reader.Services(r.Context(), p.ID, win.since)
	if err != nil {
		s.readFailed(w, r, "list services", err, p)
		return
	}
	out := make([]serviceResponse, 0, len(services))
	for _, svc := range services {
		out = append(out, serviceResponse{Name: svc.Name, Language: svc.Language, LastSeen: svc.LastSeen, Logs: svc.Logs, Errors: svc.Errors, Metrics: svc.Metrics})
	}
	writeJSON(w, http.StatusOK, map[string]any{"range": win.name, "services": out})
}

// ── Logs and errors ─────────────────────────────────────────────────────

type logResponse struct {
	ID              string         `json:"id"`
	Timestamp       time.Time      `json:"timestamp"`
	ReceivedAt      time.Time      `json:"received_at"`
	Severity        string         `json:"severity"`
	Service         string         `json:"service"`
	Message         string         `json:"message"`
	StackTrace      string         `json:"stack_trace,omitempty"`
	Attributes      map[string]any `json:"attributes,omitempty"`
	DiagnosisStatus string         `json:"diagnosis_status,omitempty"`
	AIRootCause     string         `json:"ai_root_cause,omitempty"`
	AISuggestedFix  string         `json:"ai_suggested_fix,omitempty"`
}

func toLogResponse(l model.LogRecord) logResponse {
	return logResponse{
		ID: l.ID.Hex(), Timestamp: l.Timestamp, ReceivedAt: l.ReceivedAt, Severity: l.Severity,
		Service: l.ServiceName, Message: l.Message, StackTrace: l.StackTrace, Attributes: l.Attributes,
		DiagnosisStatus: l.DiagnosisStatus, AIRootCause: l.AIRootCause, AISuggestedFix: l.AISuggestedFix,
	}
}

type logPage struct {
	Range      string        `json:"range"`
	Items      []logResponse `json:"items"`
	NextCursor string        `json:"next_cursor,omitempty"` // empty when there are no more
}

// buildLogQuery reads the filters shared by the logs and errors lists:
// ?range ?service ?q ?cursor ?limit. It writes 400 and returns false on bad input.
func (s *server) buildLogQuery(w http.ResponseWriter, r *http.Request, p model.Project) (model.LogQuery, timeWindow, bool) {
	win, ok := s.parseRange(w, r, "1h")
	if !ok {
		return model.LogQuery{}, win, false
	}
	limit, ok := parsePageSize(w, r)
	if !ok {
		return model.LogQuery{}, win, false
	}
	q := r.URL.Query()
	search := strings.TrimSpace(q.Get("q"))
	if len(search) > maxSearchLen {
		writeError(w, http.StatusBadRequest, codeValidationFailed, fmt.Sprintf("q must be at most %d characters", maxSearchLen))
		return model.LogQuery{}, win, false
	}
	lq := model.LogQuery{
		ProjectID: p.ID, Service: strings.TrimSpace(q.Get("service")), Search: search,
		Since: win.since, Until: win.until.Add(time.Millisecond), Limit: limit,
	}
	if c := q.Get("cursor"); c != "" {
		t, id, ok := decodeCursor(c)
		if !ok {
			writeError(w, http.StatusBadRequest, codeValidationFailed, "cursor is not valid")
			return model.LogQuery{}, win, false
		}
		lq.BeforeTime, lq.BeforeID = t, id
	}
	return lq, win, true
}

// writeLogPage runs the query and sends one page with its next cursor.
func (s *server) writeLogPage(w http.ResponseWriter, r *http.Request, p model.Project, lq model.LogQuery, win timeWindow) {
	logs, err := s.Reader.Logs(r.Context(), lq)
	if err != nil {
		s.readFailed(w, r, "list logs", err, p)
		return
	}
	page := logPage{Range: win.name, Items: make([]logResponse, 0, len(logs))}
	for _, l := range logs {
		page.Items = append(page.Items, toLogResponse(l))
	}
	if len(logs) == lq.Limit {
		page.NextCursor = encodeCursor(logs[len(logs)-1])
	}
	writeJSON(w, http.StatusOK, page)
}

// handleListLogs lists logs newest first. Filters: ?level=INFO,WARN,ERROR,CRITICAL
// ?service= ?q= (text in the message) ?range= (default 1h) ?limit= ?cursor=.
func (s *server) handleListLogs(w http.ResponseWriter, r *http.Request) {
	p, ok := s.ownedProject(w, r)
	if !ok {
		return
	}
	lq, win, ok := s.buildLogQuery(w, r, p)
	if !ok {
		return
	}
	if levels := r.URL.Query().Get("level"); levels != "" {
		for _, lv := range strings.Split(levels, ",") {
			lv = strings.ToUpper(strings.TrimSpace(lv))
			switch lv {
			case model.SeverityInfo, model.SeverityWarn, model.SeverityError, model.SeverityCritical:
				lq.Severities = append(lq.Severities, lv)
			default:
				writeError(w, http.StatusBadRequest, codeValidationFailed, "level must be INFO, WARN, ERROR or CRITICAL (comma-separated)")
				return
			}
		}
	}
	s.writeLogPage(w, r, p, lq, win)
}

// Diagnosis statuses that can be filtered on.
var diagnosisStatuses = []string{model.DiagnosisPending, "COMPLETED", "FAILED"}

// handleListErrors lists ERROR and CRITICAL logs newest first, with their AI
// diagnosis status. Filters as for logs (default range 24h), plus ?status=.
func (s *server) handleListErrors(w http.ResponseWriter, r *http.Request) {
	p, ok := s.ownedProject(w, r)
	if !ok {
		return
	}
	if r.URL.Query().Get("range") == "" {
		q := r.URL.Query()
		q.Set("range", "24h")
		r.URL.RawQuery = q.Encode()
	}
	lq, win, ok := s.buildLogQuery(w, r, p)
	if !ok {
		return
	}
	lq.Severities = []string{model.SeverityError, model.SeverityCritical}
	if st := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status"))); st != "" {
		if !slices.Contains(diagnosisStatuses, st) {
			writeError(w, http.StatusBadRequest, codeValidationFailed, "status must be PENDING, COMPLETED or FAILED")
			return
		}
		lq.DiagnosisStatus = st
	}
	s.writeLogPage(w, r, p, lq, win)
}

// errorDetailResponse is one error with what happened around it.
type errorDetailResponse struct {
	logResponse
	Context errorContext `json:"context"`
}

type errorContext struct {
	WindowStart time.Time         `json:"window_start"`
	WindowEnd   time.Time         `json:"window_end"`
	Metrics     []metricSeriesOut `json:"metrics"`
	RelatedLogs []logResponse     `json:"related_logs"`
}

// handleErrorDetail returns one error with the metrics of the whole project
// and the logs of the same service from the 10 minutes before it: the same
// context the AI diagnosis will read.
func (s *server) handleErrorDetail(w http.ResponseWriter, r *http.Request) {
	p, ok := s.ownedProject(w, r)
	if !ok {
		return
	}
	id, err := bson.ObjectIDFromHex(r.PathValue("errorId"))
	if err != nil {
		writeError(w, http.StatusNotFound, codeNotFound, "error not found")
		return
	}
	ctx := r.Context()
	e, err := s.Reader.ErrorByID(ctx, p.ID, id)
	if errors.Is(err, model.ErrNotFound) {
		writeError(w, http.StatusNotFound, codeNotFound, "error not found")
		return
	}
	if err != nil {
		s.readFailed(w, r, "find error", err, p)
		return
	}

	start := e.Timestamp.Add(-errorContextWindow)
	end := e.Timestamp.Add(time.Minute)
	points, _, err := s.Reader.MetricPoints(ctx, model.MetricQuery{ProjectID: p.ID, Since: start.Add(-baselineLookback), Until: end})
	if err != nil {
		s.readFailed(w, r, "error context metrics", err, p)
		return
	}
	related, err := s.Reader.Logs(ctx, model.LogQuery{
		ProjectID: p.ID, Service: e.ServiceName, Since: start, Until: e.Timestamp.Add(time.Millisecond), Limit: 30,
	})
	if err != nil {
		s.readFailed(w, r, "error context logs", err, p)
		return
	}

	out := errorDetailResponse{logResponse: toLogResponse(e)}
	out.Context = errorContext{WindowStart: start, WindowEnd: end, Metrics: summarizeMetrics(points, start, end, time.Minute)}
	out.Context.RelatedLogs = make([]logResponse, 0, len(related))
	for _, l := range related {
		if l.ID != e.ID {
			out.Context.RelatedLogs = append(out.Context.RelatedLogs, toLogResponse(l))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// ── Metrics ─────────────────────────────────────────────────────────────

type metricInfoResponse struct {
	Name      string    `json:"name"`
	Service   string    `json:"service"`
	Type      string    `json:"type"`
	Unit      string    `json:"unit,omitempty"`
	LastSeen  time.Time `json:"last_seen"`
	LastValue *float64  `json:"last_value,omitempty"`
	Points    int64     `json:"points"`
}

// handleMetricCatalog lists every metric each service sent in the range
// (default 1h), including custom ones.
func (s *server) handleMetricCatalog(w http.ResponseWriter, r *http.Request) {
	p, ok := s.ownedProject(w, r)
	if !ok {
		return
	}
	win, ok := s.parseRange(w, r, "1h")
	if !ok {
		return
	}
	list, err := s.Reader.MetricCatalog(r.Context(), p.ID, win.since)
	if err != nil {
		s.readFailed(w, r, "metric catalog", err, p)
		return
	}
	out := make([]metricInfoResponse, 0, len(list))
	for _, m := range list {
		out = append(out, metricInfoResponse{Name: m.Name, Service: m.Service, Type: m.Type, Unit: m.Unit, LastSeen: m.LastSeen, LastValue: m.LastValue, Points: m.Points})
	}
	writeJSON(w, http.StatusOK, map[string]any{"range": win.name, "metrics": out})
}

// chartPoint is one step of a metric chart. Gauges and sums fill Value;
// histograms fill Count, P50 and P95.
type chartPoint struct {
	T     time.Time `json:"t"`
	Value *float64  `json:"value,omitempty"`
	Count *int64    `json:"count,omitempty"`
	P50   *float64  `json:"p50,omitempty"`
	P95   *float64  `json:"p95,omitempty"`
}

type distribution struct {
	Bounds []float64 `json:"bounds"`
	Counts []int64   `json:"counts"`
}

// metricSeriesOut is one metric of one service as a chart.
type metricSeriesOut struct {
	Name         string        `json:"name"`
	Service      string        `json:"service"`
	Type         string        `json:"type"`
	Unit         string        `json:"unit,omitempty"`
	StepSeconds  int           `json:"step_seconds"`
	Points       []chartPoint  `json:"points"`
	Distribution *distribution `json:"distribution,omitempty"` // histograms only
}

// buildSeries turns the points of one metric of one service into chart
// steps. Gauges (and sums that can go down) show, per step, the latest
// value of each attribute set added together. Counters show what was
// added in the step. Histograms show the count and p50/p95 of the step,
// plus the distribution over the whole range.
func buildSeries(points []model.MetricPoint, since, until time.Time, step time.Duration) metricSeriesOut {
	steps := stats.NewSteps(since, until, step)
	out := metricSeriesOut{StepSeconds: int(step / time.Second), Points: make([]chartPoint, steps.N)}
	for i := range out.Points {
		out.Points[i].T = steps.Start(i)
	}
	if len(points) == 0 {
		return out
	}
	first := points[len(points)-1]
	out.Name, out.Service, out.Type, out.Unit = first.MetricName, first.ServiceName, first.MetricType, first.Unit

	if out.Type == model.MetricHistogram {
		stepHists := make([]stats.Mixed, steps.N)
		var total stats.Mixed
		for _, series := range groupSeries(points) {
			for _, th := range intervalHists(series, since, 1) {
				if i := steps.Index(th.at); i >= 0 {
					stepHists[i].Add(th.hist)
					total.Add(th.hist)
				}
			}
		}
		for i := range stepHists {
			if stepHists[i].Empty() {
				continue
			}
			c := stepHists[i].Count
			out.Points[i].Count = &c
			out.Points[i].P50 = quantile(&stepHists[i], 0.5)
			out.Points[i].P95 = quantile(&stepHists[i], 0.95)
		}
		if !total.Empty() {
			h := total.Hist()
			out.Distribution = &distribution{Bounds: h.Bounds, Counts: h.Counts}
		}
		return out
	}

	counter := isCounter(first)
	type cell struct {
		at time.Time
		v  float64
	}
	sums := make([]float64, steps.N)
	seen := make([]bool, steps.N)
	for _, series := range groupSeries(points) {
		latest := map[int]cell{} // gauges: the latest value of this series per step
		for _, tv := range intervalValues(series, since) {
			i := steps.Index(tv.at)
			if i < 0 {
				continue
			}
			if counter {
				sums[i] += tv.value
				seen[i] = true
			} else if c, ok := latest[i]; !ok || !tv.at.Before(c.at) {
				latest[i] = cell{tv.at, tv.value}
			}
		}
		for i, c := range latest {
			sums[i] += c.v
			seen[i] = true
		}
	}
	for i := range sums {
		if seen[i] {
			v := round2(sums[i])
			out.Points[i].Value = &v
		}
	}
	return out
}

// summarizeMetrics charts every metric of every service in [since, until),
// ordered by name then service (used for an error's context).
func summarizeMetrics(points []model.MetricPoint, since, until time.Time, step time.Duration) []metricSeriesOut {
	byMetric := map[[2]string][]model.MetricPoint{}
	for _, p := range points {
		k := [2]string{p.MetricName, p.ServiceName}
		byMetric[k] = append(byMetric[k], p)
	}
	out := make([]metricSeriesOut, 0, len(byMetric))
	for _, pts := range byMetric {
		out = append(out, buildSeries(pts, since, until, step))
	}
	slices.SortFunc(out, func(a, b metricSeriesOut) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Service, b.Service))
	})
	return out
}

// handleMetricSeries charts one metric: ?name= (required) ?service= ?range=.
func (s *server) handleMetricSeries(w http.ResponseWriter, r *http.Request) {
	p, ok := s.ownedProject(w, r)
	if !ok {
		return
	}
	win, ok := s.parseRange(w, r, "1h")
	if !ok {
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		writeError(w, http.StatusBadRequest, codeValidationFailed, "name is required")
		return
	}
	service := strings.TrimSpace(r.URL.Query().Get("service"))
	points, truncated, err := s.Reader.MetricPoints(r.Context(), model.MetricQuery{
		ProjectID: p.ID, Names: []string{name}, Service: service,
		Since: win.since.Add(-baselineLookback), Until: win.until.Add(time.Millisecond),
	})
	if err != nil {
		s.readFailed(w, r, "metric series", err, p)
		return
	}
	out := buildSeries(points, win.since, win.until, win.step)
	if out.Name == "" {
		out.Name, out.Service = name, service
	}
	writeJSON(w, http.StatusOK, map[string]any{"range": win.name, "truncated": truncated, "series": out})
}

// ── Overview ────────────────────────────────────────────────────────────

type overviewStep struct {
	T        time.Time `json:"t"`
	Requests int64     `json:"requests"`
	Errors   int64     `json:"errors"`
	P50      *float64  `json:"p50_ms"`
	P95      *float64  `json:"p95_ms"`
}

type overviewService struct {
	Name           string    `json:"name"`
	Language       string    `json:"language,omitempty"`
	LastSeen       time.Time `json:"last_seen"`
	RequestsPerMin float64   `json:"requests_per_min"`
	ErrorRate      *float64  `json:"error_rate"` // null without requests
	P95            *float64  `json:"p95_ms"`
	ErrorLogs      int64     `json:"error_logs"`
}

type overviewResponse struct {
	Range          string            `json:"range"`
	Since          time.Time         `json:"since"`
	Until          time.Time         `json:"until"`
	StepSeconds    int               `json:"step_seconds"`
	LastDataAt     *time.Time        `json:"last_data_at"`
	HasHTTPMetrics bool              `json:"has_http_metrics"` // false: the app sends no request metrics yet
	Requests       int64             `json:"requests"`
	RequestsPerMin float64           `json:"requests_per_min"`
	ErrorRate      *float64          `json:"error_rate"` // share of requests with a 5xx status
	P50            *float64          `json:"p50_ms"`
	P95            *float64          `json:"p95_ms"`
	Errors         int64             `json:"errors"`         // ERROR/CRITICAL logs in the range
	ErrorsPending  int64             `json:"errors_pending"` // of those, waiting for diagnosis
	Series         []overviewStep    `json:"series"`
	Services       []overviewService `json:"services"`
	Truncated      bool              `json:"truncated"`
}

// handleOverview returns a project's health for ?range= (default 1h): the
// golden signals (traffic, errors, latency) from OpenTelemetry's request
// duration metrics, a chart of them, error counts and a per-service table.
func (s *server) handleOverview(w http.ResponseWriter, r *http.Request) {
	p, ok := s.ownedProject(w, r)
	if !ok {
		return
	}
	win, ok := s.parseRange(w, r, "1h")
	if !ok {
		return
	}
	ctx := r.Context()

	points, truncated, err := s.Reader.MetricPoints(ctx, model.MetricQuery{
		ProjectID: p.ID, Names: httpDurationMetrics,
		Since: win.since.Add(-baselineLookback), Until: win.until.Add(time.Millisecond),
	})
	if err != nil {
		s.readFailed(w, r, "overview metrics", err, p)
		return
	}
	services, err := s.Reader.Services(ctx, p.ID, win.since)
	if err != nil {
		s.readFailed(w, r, "overview services", err, p)
		return
	}
	totalErrors, pending, err := s.Reader.CountErrors(ctx, p.ID, win.since)
	if err != nil {
		s.readFailed(w, r, "overview errors", err, p)
		return
	}

	out := buildOverview(points, services, win)
	out.LastDataAt, out.Errors, out.ErrorsPending, out.Truncated = p.LastUsedAt, totalErrors, pending, truncated
	writeJSON(w, http.StatusOK, out)
}

// buildOverview computes the golden signals from request duration points.
func buildOverview(points []model.MetricPoint, services []model.ServiceInfo, win timeWindow) overviewResponse {
	steps := stats.NewSteps(win.since, win.until, win.step)
	minutes := win.until.Sub(win.since).Minutes()

	type acc struct {
		hist          stats.Mixed
		requests, err int64
	}
	stepAcc := make([]acc, steps.N)
	var total acc
	bySvc := map[string]*acc{}

	for _, series := range groupSeries(points) {
		scale := msScale(series[0].Unit)
		svc := series[0].ServiceName
		if bySvc[svc] == nil {
			bySvc[svc] = &acc{}
		}
		for _, th := range intervalHists(series, win.since, scale) {
			i := steps.Index(th.at)
			if i < 0 {
				continue
			}
			failed := int64(0)
			if statusCode(th.attrs) >= 500 {
				failed = th.hist.Count
			}
			for _, a := range []*acc{&stepAcc[i], &total, bySvc[svc]} {
				a.hist.Add(th.hist)
				a.requests += th.hist.Count
				a.err += failed
			}
		}
	}

	out := overviewResponse{
		Range: win.name, Since: win.since, Until: win.until, StepSeconds: int(win.step / time.Second),
		HasHTTPMetrics: len(points) > 0,
		Requests:       total.requests, RequestsPerMin: round2(float64(total.requests) / minutes),
		ErrorRate: ratio(total.err, total.requests), P50: quantile(&total.hist, 0.5), P95: quantile(&total.hist, 0.95),
		Series: make([]overviewStep, steps.N), Services: []overviewService{},
	}
	for i, a := range stepAcc {
		out.Series[i] = overviewStep{T: steps.Start(i), Requests: a.requests, Errors: a.err, P50: quantile(&a.hist, 0.5), P95: quantile(&a.hist, 0.95)}
	}

	for _, svc := range services {
		row := overviewService{Name: svc.Name, Language: svc.Language, LastSeen: svc.LastSeen, ErrorLogs: svc.Errors}
		if a := bySvc[svc.Name]; a != nil {
			row.RequestsPerMin, row.ErrorRate, row.P95 = round2(float64(a.requests)/minutes), ratio(a.err, a.requests), quantile(&a.hist, 0.95)
		}
		out.Services = append(out.Services, row)
	}
	return out
}
