package stats

import (
	"math"
	"slices"
	"testing"
	"time"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestQuantile(t *testing.T) {
	// 10 values: 4 in (0,100], 3 in (100,250], 3 in (250,500], 0 above.
	h := Hist{Bounds: []float64{100, 250, 500}, Counts: []int64{4, 3, 3, 0}, Count: 10, Sum: 2140}

	tests := []struct {
		q    float64
		want float64
	}{
		{0.5, 100 + 150*(1.0/3)}, // rank 5: 1st of 3 in (100,250]
		{0.4, 100},               // rank 4: end of the first bucket
		{0.95, 250 + 250*(2.5/3)},
		{1, 500},
		{0.1, 25}, // rank 1 of 4 in (0,100]
	}
	for _, tt := range tests {
		got, ok := Quantile(h, tt.q)
		if !ok || !near(got, tt.want) {
			t.Errorf("Quantile(%v) = %v, %v; want %v", tt.q, got, ok, tt.want)
		}
	}

	t.Run("overflow bucket reports the last bound", func(t *testing.T) {
		h := Hist{Bounds: []float64{10}, Counts: []int64{1, 9}, Count: 10}
		if got, _ := Quantile(h, 0.95); got != 10 {
			t.Errorf("got %v", got)
		}
	})
	t.Run("empty or invalid histograms report false", func(t *testing.T) {
		for _, h := range []Hist{
			{Bounds: []float64{10}, Counts: []int64{0, 0}},
			{Bounds: []float64{10}, Counts: []int64{1}},
			{},
		} {
			if _, ok := Quantile(h, 0.5); ok {
				t.Errorf("Quantile(%+v) reported ok", h)
			}
		}
	})
	t.Run("no bounds falls back to the average", func(t *testing.T) {
		got, ok := Quantile(Hist{Counts: []int64{4}, Count: 4, Sum: 100}, 0.95)
		if !ok || got != 25 {
			t.Errorf("got %v, %v", got, ok)
		}
	})
}

func TestSubAndAdd(t *testing.T) {
	prev := Hist{Bounds: []float64{1, 2}, Counts: []int64{1, 2, 3}, Count: 6, Sum: 10}
	cur := Hist{Bounds: []float64{1, 2}, Counts: []int64{2, 4, 3}, Count: 9, Sum: 16}

	d, ok := Sub(cur, prev)
	if !ok || d.Count != 3 || d.Sum != 6 || d.Counts[0] != 1 || d.Counts[1] != 2 || d.Counts[2] != 0 {
		t.Fatalf("Sub = %+v, %v", d, ok)
	}

	t.Run("a count going down means a restart", func(t *testing.T) {
		restarted := Hist{Bounds: []float64{1, 2}, Counts: []int64{0, 1, 0}, Count: 1}
		if _, ok := Sub(restarted, prev); ok {
			t.Error("Sub accepted a restart")
		}
	})
	t.Run("changed buckets cannot be subtracted or added", func(t *testing.T) {
		other := Hist{Bounds: []float64{5}, Counts: []int64{9, 9}, Count: 18}
		if _, ok := Sub(other, prev); ok {
			t.Error("Sub accepted different buckets")
		}
		h := prev
		h.Counts = append([]int64(nil), prev.Counts...)
		if h.Add(other) {
			t.Error("Add accepted different buckets")
		}
	})

	var total Hist
	if !total.Add(d) || !total.Add(d) || total.Count != 6 || total.Counts[1] != 4 {
		t.Errorf("Add = %+v", total)
	}
	d.Counts[0] = 99 // total must not share memory with d's bounds/counts
	if total.Counts[0] == 99 {
		t.Error("Add kept a reference to the input counts")
	}
}

func TestCounterIncrease(t *testing.T) {
	if CounterIncrease(10, 15) != 5 || CounterIncrease(10, 3) != 3 || CounterIncrease(5, 5) != 0 {
		t.Error("wrong increase")
	}
}

func TestSteps(t *testing.T) {
	since := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	s := NewSteps(since, since.Add(time.Hour), time.Minute)
	if s.N != 60 {
		t.Fatalf("N = %d", s.N)
	}
	if s.Index(since) != 0 || s.Index(since.Add(59*time.Second)) != 0 || s.Index(since.Add(time.Minute)) != 1 ||
		s.Index(since.Add(-time.Second)) != -1 || s.Index(since.Add(time.Hour)) != -1 {
		t.Error("wrong step index")
	}
	if !s.Start(2).Equal(since.Add(2 * time.Minute)) {
		t.Error("wrong step start")
	}
}

func TestStepFor(t *testing.T) {
	tests := map[time.Duration]time.Duration{
		15 * time.Minute:   15 * time.Second,
		time.Hour:          time.Minute,
		6 * time.Hour:      5 * time.Minute,
		24 * time.Hour:     15 * time.Minute,
		7 * 24 * time.Hour: 2 * time.Hour,
	}
	for rng, want := range tests {
		if got := StepFor(rng); got != want {
			t.Errorf("StepFor(%s) = %s, want %s", rng, got, want)
		}
	}
}

func TestMixed(t *testing.T) {
	t.Run("same buckets is an exact merge", func(t *testing.T) {
		a := Hist{Bounds: []float64{10, 20}, Counts: []int64{1, 2, 3}, Count: 6, Sum: 60}
		var m Mixed
		m.Add(a)
		m.Add(a)
		h := m.Hist()
		if !slices.Equal(h.Bounds, []float64{10, 20}) || !slices.Equal(h.Counts, []int64{2, 4, 6}) || h.Count != 12 || h.Sum != 120 {
			t.Errorf("merged = %+v", h)
		}
	})
	t.Run("different buckets use every bound", func(t *testing.T) {
		var m Mixed
		m.Add(Hist{Bounds: []float64{10, 50}, Counts: []int64{4, 4, 0}, Count: 8})
		m.Add(Hist{Bounds: []float64{25}, Counts: []int64{2, 0}, Count: 2})
		h := m.Hist()
		if !slices.Equal(h.Bounds, []float64{10, 25, 50}) || !slices.Equal(h.Counts, []int64{4, 2, 4, 0}) || h.Count != 10 {
			t.Errorf("merged = %+v", h)
		}
		if p, ok := Quantile(h, 0.5); !ok || p <= 10 || p > 50 {
			t.Errorf("p50 = %v, %v", p, ok)
		}
	})
	t.Run("empty", func(t *testing.T) {
		var m Mixed
		m.Add(Hist{Bounds: []float64{1}, Counts: []int64{1}}) // invalid, ignored
		if !m.Empty() {
			t.Error("not empty")
		}
	})
}
