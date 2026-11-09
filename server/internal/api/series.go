package api

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
	"github.com/adityakasralkar/aidevops-monitoring/server/internal/stats"
)

// httpDurationMetrics are the OpenTelemetry metrics for server request
// duration: the current name (seconds) and the older one (milliseconds).
// SDKs record every request in them, so they give request count, error
// count (by status code) and latency.
var httpDurationMetrics = []string{"http.server.request.duration", "http.server.duration"}

// baselineLookback is how far before a range points are read, so the first
// cumulative reading inside the range has an earlier one to subtract.
const baselineLookback = 10 * time.Minute

// seriesKey identifies one time series: one metric of one service with one
// exact set of attributes (e.g. route and status code).
func seriesKey(p model.MetricPoint) string {
	var b strings.Builder
	b.WriteString(p.ServiceName)
	b.WriteByte(0)
	b.WriteString(p.MetricName)
	for _, k := range slices.Sorted(maps.Keys(p.Attributes)) {
		fmt.Fprintf(&b, "\x00%s=%v", k, p.Attributes[k])
	}
	return b.String()
}

// groupSeries splits points (oldest first) into their series, keeping order.
func groupSeries(points []model.MetricPoint) [][]model.MetricPoint {
	index := map[string]int{}
	var out [][]model.MetricPoint
	for _, p := range points {
		k := seriesKey(p)
		i, ok := index[k]
		if !ok {
			i = len(out)
			index[k] = i
			out = append(out, nil)
		}
		out[i] = append(out[i], p)
	}
	return out
}

// histOf converts a histogram point, multiplying bounds and sum by scale.
func histOf(p model.MetricPoint, scale float64) stats.Hist {
	h := stats.Hist{Counts: p.BucketCounts}
	if p.Count != nil {
		h.Count = *p.Count
	}
	if p.Sum != nil {
		h.Sum = *p.Sum * scale
	}
	h.Bounds = make([]float64, len(p.BucketBounds))
	for i, b := range p.BucketBounds {
		h.Bounds[i] = b * scale
	}
	return h
}

// timedHist is what a histogram recorded during one export interval.
type timedHist struct {
	at    time.Time
	attrs map[string]any
	hist  stats.Hist
}

// restarted reports whether a cumulative reading began counting afresh
// compared with the previous one (its start time moved on).
func restarted(cur model.MetricPoint, prevStart *time.Time) bool {
	return cur.StartTime != nil && prevStart != nil && !cur.StartTime.Equal(*prevStart)
}

// countsFromZero reports whether a first cumulative reading holds only
// data from inside the range: its series started counting at or after since.
func countsFromZero(p model.MetricPoint, since time.Time) bool {
	return p.StartTime != nil && !p.StartTime.Before(since)
}

// intervalHists turns one series' histogram points (oldest first) into what
// happened in each export interval, keeping intervals that end at or after
// since. Cumulative (or unknown) points are subtracted from the previous
// reading. A reading with a new start time, or one that cannot follow the
// previous one, means the app restarted, so it counts from zero. The first
// reading counts in full only if its series started inside the range;
// otherwise it holds older data and is only the starting point.
func intervalHists(series []model.MetricPoint, since time.Time, scale float64) []timedHist {
	var out []timedHist
	var prev *stats.Hist
	var prevStart *time.Time
	for _, p := range series {
		if p.MetricType != model.MetricHistogram {
			continue
		}
		cur := histOf(p, scale)
		var d stats.Hist
		switch {
		case p.Temporality == model.TemporalityDelta:
			d = cur
		case prev == nil && !countsFromZero(p, since):
			prev, prevStart = &cur, p.StartTime
			continue
		case prev == nil, restarted(p, prevStart):
			d = cur
		default:
			var ok bool
			if d, ok = stats.Sub(cur, *prev); !ok {
				d = cur
			}
		}
		prev, prevStart = &cur, p.StartTime
		if !p.Timestamp.Before(since) {
			out = append(out, timedHist{at: p.Timestamp, attrs: p.Attributes, hist: d})
		}
	}
	return out
}

// timedValue is a gauge's value, or what a counter added, at a time.
type timedValue struct {
	at    time.Time
	value float64
}

// intervalValues turns one series of gauge or sum points (oldest first)
// into chart values. A gauge (or a sum that can go down) is its value. A
// counter is what it added in each interval: a delta sum as it is, a
// cumulative one minus the previous reading (a drop or a new start time
// means the app restarted). A first reading counts in full only if its
// series started inside the range; otherwise it is only the starting point.
func intervalValues(series []model.MetricPoint, since time.Time) []timedValue {
	var out []timedValue
	var prev *float64
	var prevStart *time.Time
	for _, p := range series {
		if p.Value == nil {
			continue
		}
		cur := *p.Value
		v := cur
		if isCounter(p) && p.Temporality != model.TemporalityDelta {
			switch {
			case prev == nil && !countsFromZero(p, since):
				prev, prevStart = &cur, p.StartTime
				continue
			case prev == nil, restarted(p, prevStart):
				// v = cur: counting began inside the range or started again
			default:
				v = stats.CounterIncrease(*prev, cur)
			}
			prev, prevStart = &cur, p.StartTime
		}
		if !p.Timestamp.Before(since) {
			out = append(out, timedValue{at: p.Timestamp, value: v})
		}
	}
	return out
}

// isCounter reports whether a metric's values add up over time (so a step
// shows the sum of increases) rather than being a level (a step shows the
// latest value).
func isCounter(p model.MetricPoint) bool {
	return p.MetricType == model.MetricSum && (p.Monotonic == nil || *p.Monotonic)
}

// msScale returns the factor that converts a time unit to milliseconds.
// Unknown or empty units are assumed to be milliseconds.
func msScale(unit string) float64 {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "s", "sec", "second", "seconds":
		return 1000
	case "us", "µs", "μs", "microsecond", "microseconds":
		return 0.001
	case "ns", "nanosecond", "nanoseconds":
		return 0.000001
	default:
		return 1
	}
}

// statusCode reads the HTTP status code attribute (current or older name).
func statusCode(attrs map[string]any) int {
	for _, k := range []string{"http.response.status_code", "http.status_code"} {
		switch v := attrs[k].(type) {
		case float64:
			return int(v)
		case int64:
			return int(v)
		case int32:
			return int(v)
		case string:
			var n int
			if _, err := fmt.Sscan(v, &n); err == nil {
				return n
			}
		}
	}
	return 0
}

// quantile returns the q-quantile of m, or nil if it holds no data.
func quantile(m *stats.Mixed, q float64) *float64 {
	if m.Empty() {
		return nil
	}
	v, ok := stats.Quantile(m.Hist(), q)
	if !ok {
		return nil
	}
	v = round2(v)
	return &v
}

// ratio returns a/b, or nil when b is zero.
func ratio(a, b int64) *float64 {
	if b == 0 {
		return nil
	}
	v := round4(float64(a) / float64(b))
	return &v
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
func round4(v float64) float64 { return math.Round(v*10000) / 10000 }
