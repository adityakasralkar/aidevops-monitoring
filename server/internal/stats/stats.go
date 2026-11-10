// Package stats holds the maths for the dashboard: turning OpenTelemetry's
// running totals into per-interval values, merging histograms, estimating
// percentiles from histogram buckets, and grouping times into chart steps.
// It has no database or HTTP code.
package stats

import (
	"math"
	"slices"
	"time"
)

// Hist is a histogram: Counts[i] values fell at or below Bounds[i] (and
// above Bounds[i-1]); the last count, with no bound, is everything above
// the last bound. So len(Counts) == len(Bounds)+1.
type Hist struct {
	Bounds []float64
	Counts []int64
	Count  int64
	Sum    float64
}

// Valid reports whether the counts fit the bounds.
func (h Hist) Valid() bool {
	return len(h.Counts) == len(h.Bounds)+1
}

// sameShape reports whether two histograms use identical buckets.
func sameShape(a, b Hist) bool {
	return a.Valid() && b.Valid() && slices.Equal(a.Bounds, b.Bounds)
}

// Sub returns what happened between two readings of a cumulative histogram
// (cur minus prev). It reports false when cur cannot follow prev: the
// buckets changed or a count went down, which means the app restarted and
// its totals began again from zero.
func Sub(cur, prev Hist) (Hist, bool) {
	if !sameShape(cur, prev) || cur.Count < prev.Count {
		return Hist{}, false
	}
	out := Hist{Bounds: cur.Bounds, Counts: make([]int64, len(cur.Counts)), Count: cur.Count - prev.Count, Sum: cur.Sum - prev.Sum}
	for i := range cur.Counts {
		if cur.Counts[i] < prev.Counts[i] {
			return Hist{}, false
		}
		out.Counts[i] = cur.Counts[i] - prev.Counts[i]
	}
	return out, true
}

// Add merges o into h. An empty h takes o's buckets. It reports false (and
// leaves h unchanged) if the buckets differ.
func (h *Hist) Add(o Hist) bool {
	if !o.Valid() {
		return false
	}
	if len(h.Counts) == 0 {
		h.Bounds = slices.Clone(o.Bounds)
		h.Counts = make([]int64, len(o.Counts))
	}
	if !sameShape(*h, o) {
		return false
	}
	for i, c := range o.Counts {
		h.Counts[i] += c
	}
	h.Count += o.Count
	h.Sum += o.Sum
	return true
}

// Quantile estimates the value below which a fraction q (0–1) of the
// recorded values fall, e.g. q=0.95 for p95. Inside the bucket that holds
// the target rank it assumes values are spread evenly (linear
// interpolation), which is the usual estimate for bucket histograms. The
// first bucket starts at 0 (or at its bound, if that is negative). Values
// in the overflow bucket are reported as the last bound, the most that is
// known about them. It reports false when the histogram is empty.
func Quantile(h Hist, q float64) (float64, bool) {
	if !h.Valid() || q < 0 || q > 1 {
		return 0, false
	}
	var total int64
	for _, c := range h.Counts {
		total += c
	}
	if total == 0 {
		return 0, false
	}
	if len(h.Bounds) == 0 {
		// One bucket and no bounds: only the average is known.
		if h.Count > 0 {
			return h.Sum / float64(h.Count), true
		}
		return 0, false
	}

	rank := q * float64(total)
	var seen float64
	for i, c := range h.Counts {
		if c == 0 {
			continue
		}
		if seen+float64(c) >= rank {
			if i == len(h.Bounds) { // overflow bucket
				return h.Bounds[len(h.Bounds)-1], true
			}
			upper := h.Bounds[i]
			lower := math.Min(0, upper)
			if i > 0 {
				lower = h.Bounds[i-1]
			}
			frac := (rank - seen) / float64(c)
			return lower + (upper-lower)*frac, true
		}
		seen += float64(c)
	}
	return h.Bounds[len(h.Bounds)-1], true
}

// CounterIncrease returns how much a cumulative counter grew from prev to
// cur. A drop means the app restarted and counting began again, so the
// increase since then is cur itself.
func CounterIncrease(prev, cur float64) float64 {
	if cur >= prev {
		return cur - prev
	}
	return cur
}

// Steps cuts the time range [since, until) into equal chart steps.
type Steps struct {
	Since time.Time
	Step  time.Duration
	N     int
}

// NewSteps divides [since, until) into steps of the given size.
func NewSteps(since, until time.Time, step time.Duration) Steps {
	n := int(math.Ceil(float64(until.Sub(since)) / float64(step)))
	return Steps{Since: since, Step: step, N: max(n, 1)}
}

// Index returns the step that t falls in, or -1 if t is outside the range.
func (s Steps) Index(t time.Time) int {
	if t.Before(s.Since) {
		return -1
	}
	i := int(t.Sub(s.Since) / s.Step)
	if i >= s.N {
		return -1
	}
	return i
}

// Start returns the start time of step i.
func (s Steps) Start(i int) time.Time {
	return s.Since.Add(time.Duration(i) * s.Step)
}

// StepFor picks a chart step for a time range so that a chart has at most
// 100 points, using round sizes people recognise.
//
// The shortest step is one minute: OpenTelemetry SDKs export metrics every
// 60 seconds by default, so shorter steps would leave most of them empty
// and turn a smooth line into spikes.
func StepFor(rng time.Duration) time.Duration {
	for _, step := range []time.Duration{
		time.Minute, 2 * time.Minute, 5 * time.Minute,
		10 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour,
		6 * time.Hour, 12 * time.Hour, 24 * time.Hour,
	} {
		if rng/step <= 100 {
			return step
		}
	}
	return 24 * time.Hour
}

// Mixed combines histograms that may use different buckets (e.g. one app's
// SDK uses 10 buckets and another's uses 15). Each bucket's count is kept at
// its upper bound; the result uses every bound seen. When all inputs share
// the same buckets this is an exact merge. It is the zero value ready to use.
type Mixed struct {
	byBound  map[float64]int64
	overflow int64 // values above an input's last bound
	Count    int64
	Sum      float64
}

// Add folds a histogram into m. Invalid histograms are ignored.
func (m *Mixed) Add(h Hist) {
	if !h.Valid() {
		return
	}
	if m.byBound == nil {
		m.byBound = map[float64]int64{}
	}
	for i, b := range h.Bounds {
		m.byBound[b] += h.Counts[i]
	}
	m.overflow += h.Counts[len(h.Counts)-1]
	m.Count += h.Count
	m.Sum += h.Sum
}

// Empty reports whether nothing was recorded.
func (m *Mixed) Empty() bool {
	return m.Count == 0 && m.overflow == 0 && len(m.byBound) == 0
}

// Hist returns the combined histogram.
func (m *Mixed) Hist() Hist {
	bounds := make([]float64, 0, len(m.byBound))
	for b := range m.byBound {
		bounds = append(bounds, b)
	}
	slices.Sort(bounds)
	counts := make([]int64, len(bounds)+1)
	for i, b := range bounds {
		counts[i] = m.byBound[b]
	}
	counts[len(bounds)] = m.overflow
	return Hist{Bounds: bounds, Counts: counts, Count: m.Count, Sum: m.Sum}
}
