package api

import (
	"testing"
	"time"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

var t0 = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

func f64p(v float64) *float64 { return &v }
func i64p(v int64) *int64     { return &v }
func boolp(v bool) *bool      { return &v }

func histPoint(at time.Time, temporality string, counts []int64, sum float64, attrs map[string]any) model.MetricPoint {
	var total int64
	for _, c := range counts {
		total += c
	}
	return model.MetricPoint{
		ServiceName: "api", MetricName: "http.server.request.duration", MetricType: model.MetricHistogram,
		Unit: "s", Temporality: temporality, Timestamp: at, Attributes: attrs,
		Count: i64p(total), Sum: f64p(sum), BucketBounds: []float64{0.1, 0.5}, BucketCounts: counts,
	}
}

func TestIntervalHistsCumulative(t *testing.T) {
	series := []model.MetricPoint{
		histPoint(t0.Add(-1*time.Minute), model.TemporalityCumulative, []int64{1, 0, 0}, 0.05, nil), // before range: baseline
		histPoint(t0.Add(15*time.Second), model.TemporalityCumulative, []int64{3, 1, 0}, 0.5, nil),
		histPoint(t0.Add(30*time.Second), model.TemporalityCumulative, []int64{3, 1, 0}, 0.5, nil), // nothing new
		histPoint(t0.Add(45*time.Second), model.TemporalityCumulative, []int64{1, 0, 1}, 0.9, nil), // restart
	}
	got := intervalHists(series, t0, 1000)
	if len(got) != 3 {
		t.Fatalf("got %d intervals", len(got))
	}
	if got[0].hist.Count != 3 || got[0].hist.Counts[0] != 2 || got[0].hist.Bounds[0] != 100 || got[0].hist.Sum != 450 {
		t.Errorf("first interval = %+v (want 3 new requests, bounds in ms)", got[0].hist)
	}
	if got[1].hist.Count != 0 {
		t.Errorf("unchanged reading added %d", got[1].hist.Count)
	}
	if got[2].hist.Count != 2 || got[2].hist.Counts[2] != 1 {
		t.Errorf("after restart = %+v (want the new totals as they are)", got[2].hist)
	}
}

func TestIntervalHistsFirstReadingIsOnlyABaseline(t *testing.T) {
	series := []model.MetricPoint{histPoint(t0.Add(time.Minute), "", []int64{5, 0, 0}, 1, nil)}
	if got := intervalHists(series, t0, 1); len(got) != 0 {
		t.Errorf("first cumulative reading counted: %+v", got)
	}
}

func TestIntervalHistsDelta(t *testing.T) {
	series := []model.MetricPoint{
		histPoint(t0.Add(time.Minute), model.TemporalityDelta, []int64{2, 0, 0}, 0.1, nil),
		histPoint(t0.Add(2*time.Minute), model.TemporalityDelta, []int64{1, 0, 0}, 0.05, nil),
	}
	got := intervalHists(series, t0, 1)
	if len(got) != 2 || got[0].hist.Count != 2 || got[1].hist.Count != 1 {
		t.Errorf("delta intervals = %+v", got)
	}
}

func valuePoint(at time.Time, typ, temporality string, monotonic *bool, v float64) model.MetricPoint {
	return model.MetricPoint{ServiceName: "api", MetricName: "m", MetricType: typ, Temporality: temporality, Monotonic: monotonic, Value: f64p(v), Timestamp: at}
}

func TestIntervalValues(t *testing.T) {
	t.Run("gauge values are taken as they are", func(t *testing.T) {
		got := intervalValues([]model.MetricPoint{
			valuePoint(t0.Add(-time.Minute), model.MetricGauge, "", nil, 9),
			valuePoint(t0.Add(time.Minute), model.MetricGauge, "", nil, 4),
		}, t0)
		if len(got) != 1 || got[0].value != 4 {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("cumulative counter becomes increases, restart counts from zero", func(t *testing.T) {
		got := intervalValues([]model.MetricPoint{
			valuePoint(t0.Add(-time.Minute), model.MetricSum, model.TemporalityCumulative, boolp(true), 100),
			valuePoint(t0.Add(time.Minute), model.MetricSum, model.TemporalityCumulative, boolp(true), 130),
			valuePoint(t0.Add(2*time.Minute), model.MetricSum, model.TemporalityCumulative, boolp(true), 5),
		}, t0)
		if len(got) != 2 || got[0].value != 30 || got[1].value != 5 {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("up/down counter is a level, not increases", func(t *testing.T) {
		got := intervalValues([]model.MetricPoint{
			valuePoint(t0.Add(time.Minute), model.MetricSum, model.TemporalityCumulative, boolp(false), 7),
			valuePoint(t0.Add(2*time.Minute), model.MetricSum, model.TemporalityCumulative, boolp(false), 3),
		}, t0)
		if len(got) != 2 || got[0].value != 7 || got[1].value != 3 {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("delta counter values are already increases", func(t *testing.T) {
		got := intervalValues([]model.MetricPoint{
			valuePoint(t0.Add(time.Minute), model.MetricSum, model.TemporalityDelta, boolp(true), 4),
		}, t0)
		if len(got) != 1 || got[0].value != 4 {
			t.Errorf("got %+v", got)
		}
	})
}

func TestGroupSeriesSplitsByAttributes(t *testing.T) {
	a := histPoint(t0, "", []int64{1, 0, 0}, 0, map[string]any{"http.route": "/a", "http.response.status_code": 200.0})
	b := histPoint(t0, "", []int64{1, 0, 0}, 0, map[string]any{"http.route": "/b"})
	a2 := histPoint(t0.Add(time.Minute), "", []int64{2, 0, 0}, 0, map[string]any{"http.response.status_code": 200.0, "http.route": "/a"})
	got := groupSeries([]model.MetricPoint{a, b, a2})
	if len(got) != 2 || len(got[0]) != 2 || len(got[1]) != 1 {
		t.Errorf("series = %d groups (%d, %d)", len(got), len(got[0]), len(got[1]))
	}
}

func TestMsScaleAndStatusCode(t *testing.T) {
	if msScale("s") != 1000 || msScale("ms") != 1 || msScale("") != 1 || msScale("us") != 0.001 {
		t.Error("wrong unit scale")
	}
	for attrs, want := range map[*map[string]any]int{
		{"http.response.status_code": 503.0}: 503,
		{"http.status_code": "404"}:          404,
		{"other": 1.0}:                       0,
	} {
		if got := statusCode(*attrs); got != want {
			t.Errorf("statusCode(%v) = %d, want %d", *attrs, got, want)
		}
	}
}

func withStart(p model.MetricPoint, start time.Time) model.MetricPoint {
	p.StartTime = &start
	return p
}

func TestStartTimeRules(t *testing.T) {
	t.Run("first reading counts in full when the app started inside the range", func(t *testing.T) {
		series := []model.MetricPoint{withStart(histPoint(t0.Add(time.Minute), model.TemporalityCumulative, []int64{30, 0, 0}, 1, nil), t0.Add(10*time.Second))}
		got := intervalHists(series, t0, 1)
		if len(got) != 1 || got[0].hist.Count != 30 {
			t.Errorf("got %+v, want the 30 requests since the app started", got)
		}
	})
	t.Run("first reading is only a baseline when the app started before the range", func(t *testing.T) {
		series := []model.MetricPoint{withStart(histPoint(t0.Add(time.Minute), model.TemporalityCumulative, []int64{30, 0, 0}, 1, nil), t0.Add(-time.Hour))}
		if got := intervalHists(series, t0, 1); len(got) != 0 {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("a new start time is a restart even if counts went up", func(t *testing.T) {
		series := []model.MetricPoint{
			withStart(histPoint(t0.Add(time.Minute), model.TemporalityCumulative, []int64{5, 0, 0}, 1, nil), t0.Add(-time.Hour)),
			withStart(histPoint(t0.Add(2*time.Minute), model.TemporalityCumulative, []int64{8, 0, 0}, 1, nil), t0.Add(90*time.Second)),
		}
		got := intervalHists(series, t0, 1)
		if len(got) != 1 || got[0].hist.Count != 8 {
			t.Errorf("got %+v, want all 8 requests of the restarted app", got)
		}
	})
	t.Run("counters follow the same rules", func(t *testing.T) {
		start := t0.Add(5 * time.Second)
		p := valuePoint(t0.Add(time.Minute), model.MetricSum, model.TemporalityCumulative, boolp(true), 12)
		got := intervalValues([]model.MetricPoint{withStart(p, start)}, t0)
		if len(got) != 1 || got[0].value != 12 {
			t.Errorf("got %+v", got)
		}
	})
}
