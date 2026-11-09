package store

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

// Read limits, so one dashboard request cannot load unbounded data.
const (
	MaxLogsPerPage    = 200
	MaxMetricPoints   = 200_000
	MaxMetricsListed  = 1_000
	MaxServicesListed = 500
)

// attrLanguage is the OpenTelemetry attribute naming the SDK's language.
// It contains dots, so it is read with $getField rather than a path.
const attrLanguage = "telemetry.sdk.language"

// attrRuntime is the fallback when an SDK's resource leaves out the
// language: the runtime name every SDK reports, e.g. "nodejs" or "cpython".
const attrRuntime = "process.runtime.name"

// Services lists the services that sent logs or metrics since the given
// time, with counts and the language their SDK reported.
func (s *Store) Services(ctx context.Context, projectID bson.ObjectID, since time.Time) ([]model.ServiceInfo, error) {
	byName := map[string]*model.ServiceInfo{}
	get := func(name string) *model.ServiceInfo {
		if byName[name] == nil {
			byName[name] = &model.ServiceInfo{Name: name}
		}
		return byName[name]
	}

	type row struct {
		Name     string    `bson:"_id"`
		Count    int64     `bson:"count"`
		LastSeen time.Time `bson:"last_seen"`
		Language *string   `bson:"language"`
	}
	for _, coll := range []string{CollAppLogs, CollErrorLogs, CollTelemetryMetrics} {
		pipeline := mongo.Pipeline{
			{{Key: "$match", Value: bson.D{
				{Key: "project_id", Value: projectID},
				{Key: "timestamp", Value: bson.D{{Key: "$gte", Value: since}}},
			}}},
			{{Key: "$group", Value: bson.D{
				{Key: "_id", Value: "$service_name"},
				{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
				{Key: "last_seen", Value: bson.D{{Key: "$max", Value: "$received_at"}}},
				{Key: "language", Value: bson.D{{Key: "$max", Value: bson.D{{Key: "$ifNull", Value: bson.A{
					bson.D{{Key: "$getField", Value: bson.D{{Key: "field", Value: attrLanguage}, {Key: "input", Value: "$attributes"}}}},
					bson.D{{Key: "$getField", Value: bson.D{{Key: "field", Value: attrRuntime}, {Key: "input", Value: "$attributes"}}}},
				}}}}}},
			}}},
			{{Key: "$limit", Value: MaxServicesListed}},
		}
		var rows []row
		if err := s.aggregate(ctx, coll, pipeline, &rows); err != nil {
			return nil, fmt.Errorf("services from %s: %w", coll, err)
		}
		for _, r := range rows {
			svc := get(r.Name)
			switch coll {
			case CollAppLogs:
				svc.Logs += r.Count
			case CollErrorLogs:
				svc.Logs += r.Count
				svc.Errors += r.Count
			case CollTelemetryMetrics:
				svc.Metrics += r.Count
			}
			if r.LastSeen.After(svc.LastSeen) {
				svc.LastSeen = r.LastSeen
			}
			if r.Language != nil && *r.Language != "" {
				svc.Language = *r.Language
			}
		}
	}

	out := make([]model.ServiceInfo, 0, len(byName))
	for _, svc := range byName {
		out = append(out, *svc)
	}
	slices.SortFunc(out, func(a, b model.ServiceInfo) int { return cmp.Compare(a.Name, b.Name) })
	return out, nil
}

// Logs returns one page of logs matching q, newest first. ERROR and
// CRITICAL logs live in ErrorLogs and the rest in AppLogs, so both are read
// (when the severities need them) and merged.
func (s *Store) Logs(ctx context.Context, q model.LogQuery) ([]model.LogRecord, error) {
	limit := q.Limit
	if limit <= 0 || limit > MaxLogsPerPage {
		limit = MaxLogsPerPage
	}

	wantErrors, wantApp := len(q.Severities) == 0, len(q.Severities) == 0
	for _, sev := range q.Severities {
		if model.IsErrorSeverity(sev) {
			wantErrors = true
		} else {
			wantApp = true
		}
	}
	if q.DiagnosisStatus != "" {
		wantApp = false // only errors have a diagnosis
	}

	filter := bson.D{{Key: "project_id", Value: q.ProjectID}}
	timeRange := bson.D{}
	if !q.Since.IsZero() {
		timeRange = append(timeRange, bson.E{Key: "$gte", Value: q.Since})
	}
	if !q.Until.IsZero() {
		timeRange = append(timeRange, bson.E{Key: "$lt", Value: q.Until})
	}
	if len(timeRange) > 0 {
		filter = append(filter, bson.E{Key: "timestamp", Value: timeRange})
	}
	if len(q.Severities) > 0 {
		filter = append(filter, bson.E{Key: "severity", Value: bson.D{{Key: "$in", Value: q.Severities}}})
	}
	if q.Service != "" {
		filter = append(filter, bson.E{Key: "service_name", Value: q.Service})
	}
	if q.Search != "" {
		// QuoteMeta makes the text literal, so user input is never a pattern.
		filter = append(filter, bson.E{Key: "message", Value: bson.D{
			{Key: "$regex", Value: regexp.QuoteMeta(q.Search)}, {Key: "$options", Value: "i"},
		}})
	}
	if q.DiagnosisStatus != "" {
		filter = append(filter, bson.E{Key: "diagnosis_status", Value: q.DiagnosisStatus})
	}
	if !q.BeforeTime.IsZero() {
		filter = append(filter, bson.E{Key: "$or", Value: bson.A{
			bson.D{{Key: "timestamp", Value: bson.D{{Key: "$lt", Value: q.BeforeTime}}}},
			bson.D{{Key: "timestamp", Value: q.BeforeTime}, {Key: "_id", Value: bson.D{{Key: "$lt", Value: q.BeforeID}}}},
		}})
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "timestamp", Value: -1}, {Key: "_id", Value: -1}}).
		SetLimit(int64(limit))

	var all []model.LogRecord
	for coll, want := range map[string]bool{CollErrorLogs: wantErrors, CollAppLogs: wantApp} {
		if !want {
			continue
		}
		cur, err := s.Collection(coll).Find(ctx, filter, opts)
		if err != nil {
			return nil, fmt.Errorf("find logs in %s: %w", coll, err)
		}
		var page []model.LogRecord
		if err := cur.All(ctx, &page); err != nil {
			return nil, fmt.Errorf("read logs from %s: %w", coll, err)
		}
		all = append(all, page...)
	}

	slices.SortFunc(all, newestFirst)
	if len(all) > limit {
		all = all[:limit]
	}
	if all == nil {
		all = []model.LogRecord{}
	}
	return all, nil
}

// newestFirst orders logs by timestamp, then id, both descending.
func newestFirst(a, b model.LogRecord) int {
	if c := b.Timestamp.Compare(a.Timestamp); c != 0 {
		return c
	}
	return cmp.Compare(b.ID.Hex(), a.ID.Hex())
}

// ErrorByID returns one ERROR or CRITICAL log of the project.
// It returns model.ErrNotFound if there is none with that id.
func (s *Store) ErrorByID(ctx context.Context, projectID, id bson.ObjectID) (model.LogRecord, error) {
	var l model.LogRecord
	filter := bson.D{{Key: "_id", Value: id}, {Key: "project_id", Value: projectID}}
	err := s.Collection(CollErrorLogs).FindOne(ctx, filter).Decode(&l)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return model.LogRecord{}, model.ErrNotFound
	}
	if err != nil {
		return model.LogRecord{}, fmt.Errorf("find error: %w", err)
	}
	return l, nil
}

// CountErrors counts the project's errors since the given time, and how
// many of them are still waiting for an AI diagnosis.
func (s *Store) CountErrors(ctx context.Context, projectID bson.ObjectID, since time.Time) (total, pending int64, err error) {
	filter := bson.D{
		{Key: "project_id", Value: projectID},
		{Key: "timestamp", Value: bson.D{{Key: "$gte", Value: since}}},
	}
	coll := s.Collection(CollErrorLogs)
	if total, err = coll.CountDocuments(ctx, filter); err != nil {
		return 0, 0, fmt.Errorf("count errors: %w", err)
	}
	filter = append(filter, bson.E{Key: "diagnosis_status", Value: model.DiagnosisPending})
	if pending, err = coll.CountDocuments(ctx, filter); err != nil {
		return 0, 0, fmt.Errorf("count pending errors: %w", err)
	}
	return total, pending, nil
}

// MetricCatalog lists every metric of every service sent since the given
// time, with its type, unit, latest value and number of points.
func (s *Store) MetricCatalog(ctx context.Context, projectID bson.ObjectID, since time.Time) ([]model.MetricInfo, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{
			{Key: "project_id", Value: projectID},
			{Key: "timestamp", Value: bson.D{{Key: "$gte", Value: since}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "timestamp", Value: 1}}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{{Key: "name", Value: "$metric_name"}, {Key: "service", Value: "$service_name"}}},
			{Key: "type", Value: bson.D{{Key: "$last", Value: "$metric_type"}}},
			{Key: "unit", Value: bson.D{{Key: "$last", Value: "$unit"}}},
			{Key: "last_seen", Value: bson.D{{Key: "$last", Value: "$timestamp"}}},
			{Key: "last_value", Value: bson.D{{Key: "$last", Value: "$value"}}},
			{Key: "points", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id.name", Value: 1}, {Key: "_id.service", Value: 1}}}},
		{{Key: "$limit", Value: MaxMetricsListed}},
	}
	var rows []struct {
		ID struct {
			Name    string `bson:"name"`
			Service string `bson:"service"`
		} `bson:"_id"`
		Type      string    `bson:"type"`
		Unit      string    `bson:"unit"`
		LastSeen  time.Time `bson:"last_seen"`
		LastValue *float64  `bson:"last_value"`
		Points    int64     `bson:"points"`
	}
	if err := s.aggregate(ctx, CollTelemetryMetrics, pipeline, &rows); err != nil {
		return nil, fmt.Errorf("metric catalog: %w", err)
	}
	out := make([]model.MetricInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.MetricInfo{
			Name: r.ID.Name, Service: r.ID.Service, Type: r.Type, Unit: r.Unit,
			LastSeen: r.LastSeen, LastValue: r.LastValue, Points: r.Points,
		})
	}
	return out, nil
}

// MetricPoints returns the points matching q, oldest first. truncated is
// true when more points exist than the limit allowed.
func (s *Store) MetricPoints(ctx context.Context, q model.MetricQuery) (points []model.MetricPoint, truncated bool, err error) {
	limit := q.Limit
	if limit <= 0 || limit > MaxMetricPoints {
		limit = MaxMetricPoints
	}
	filter := bson.D{
		{Key: "project_id", Value: q.ProjectID},
		{Key: "timestamp", Value: bson.D{{Key: "$gte", Value: q.Since}, {Key: "$lt", Value: q.Until}}},
	}
	if len(q.Names) > 0 {
		filter = append(filter, bson.E{Key: "metric_name", Value: bson.D{{Key: "$in", Value: q.Names}}})
	}
	if q.Service != "" {
		filter = append(filter, bson.E{Key: "service_name", Value: q.Service})
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "timestamp", Value: 1}}).
		SetLimit(int64(limit) + 1) // one extra to detect truncation

	cur, err := s.Collection(CollTelemetryMetrics).Find(ctx, filter, opts)
	if err != nil {
		return nil, false, fmt.Errorf("find metric points: %w", err)
	}
	points = []model.MetricPoint{}
	if err := cur.All(ctx, &points); err != nil {
		return nil, false, fmt.Errorf("read metric points: %w", err)
	}
	if len(points) > limit {
		return points[:limit], true, nil
	}
	return points, false, nil
}

// aggregate runs a pipeline and decodes every result into out.
func (s *Store) aggregate(ctx context.Context, coll string, pipeline mongo.Pipeline, out any) error {
	cur, err := s.Collection(coll).Aggregate(ctx, pipeline, options.Aggregate().SetAllowDiskUse(true))
	if err != nil {
		return err
	}
	return cur.All(ctx, out)
}
