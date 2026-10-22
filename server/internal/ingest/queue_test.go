package ingest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

// fakeWriter records saved logs. failures makes the next N writes fail;
// block makes writes wait until released (or their context ends).
type fakeWriter struct {
	mu        sync.Mutex
	errorLogs []model.LogRecord
	appLogs   []model.LogRecord
	writes    int
	failures  int
	block     chan struct{}
}

func (f *fakeWriter) insert(ctx context.Context, dst *[]model.LogRecord, logs []model.LogRecord) error {
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes++
	if f.failures > 0 {
		f.failures--
		return errors.New("mongo unavailable")
	}
	*dst = append(*dst, logs...)
	return nil
}

func (f *fakeWriter) InsertErrorLogs(ctx context.Context, logs []model.LogRecord) error {
	return f.insert(ctx, &f.errorLogs, logs)
}

func (f *fakeWriter) InsertAppLogs(ctx context.Context, logs []model.LogRecord) error {
	return f.insert(ctx, &f.appLogs, logs)
}

func (f *fakeWriter) counts() (errs, apps, writes int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.errorLogs), len(f.appLogs), f.writes
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newQueue(t *testing.T, w *fakeWriter, capacity, workers int) *Queue {
	t.Helper()
	q, err := New(Config{
		Capacity:    capacity,
		Workers:     workers,
		Writer:      w,
		Logger:      discardLogger(),
		RetryDelays: []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return q
}

func logs(n int, severity string) []model.LogRecord {
	out := make([]model.LogRecord, n)
	for i := range out {
		out[i] = model.LogRecord{ID: bson.NewObjectID(), Severity: severity, Message: "m"}
	}
	return out
}

func drain(t *testing.T, q *Queue) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := q.Drain(ctx); err != nil {
		t.Fatalf("Drain: %v", err)
	}
}

func TestNewValidatesConfig(t *testing.T) {
	bad := []Config{
		{Capacity: 0, Workers: 1, Writer: &fakeWriter{}, Logger: discardLogger()},
		{Capacity: 1, Workers: 0, Writer: &fakeWriter{}, Logger: discardLogger()},
		{Capacity: 1, Workers: 1, Logger: discardLogger()},
		{Capacity: 1, Workers: 1, Writer: &fakeWriter{}},
	}
	for i, cfg := range bad {
		if _, err := New(cfg); err == nil {
			t.Errorf("config %d: expected an error", i)
		}
	}
}

func TestRoutesBySeverity(t *testing.T) {
	w := &fakeWriter{}
	q := newQueue(t, w, 100, 2)
	q.Start()

	batch := append(append(append(logs(2, model.SeverityInfo), logs(3, model.SeverityWarn)...),
		logs(4, model.SeverityError)...), logs(1, model.SeverityCritical)...)
	if err := q.Enqueue(batch); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	drain(t, q)

	errs, apps, _ := w.counts()
	if errs != 5 || apps != 5 {
		t.Errorf("ErrorLogs = %d, AppLogs = %d; want 5 and 5", errs, apps)
	}
	for _, l := range w.errorLogs {
		if l.DiagnosisStatus != model.DiagnosisPending {
			t.Errorf("error log has diagnosis_status %q, want PENDING", l.DiagnosisStatus)
		}
	}
	for _, l := range w.appLogs {
		if l.DiagnosisStatus != "" {
			t.Errorf("app log has diagnosis_status %q, want empty", l.DiagnosisStatus)
		}
	}
	if s := q.Stats(); s != (Stats{Accepted: 10, Saved: 10}) {
		t.Errorf("stats = %+v", s)
	}
}

func TestMergesWaitingBatchesIntoOneWrite(t *testing.T) {
	w := &fakeWriter{}
	q := newQueue(t, w, 1000, 1)
	// Enqueue before starting the single worker, so all batches are waiting.
	for range 10 {
		if err := q.Enqueue(logs(5, model.SeverityInfo)); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}
	q.Start()
	drain(t, q)

	_, apps, writes := w.counts()
	if apps != 50 || writes != 1 {
		t.Errorf("saved %d logs in %d writes; want 50 in 1", apps, writes)
	}
}

func TestEnqueueIsAllOrNothing(t *testing.T) {
	w := &fakeWriter{}
	q := newQueue(t, w, 10, 1) // not started: nothing leaves the queue

	if err := q.Enqueue(logs(8, model.SeverityInfo)); err != nil {
		t.Fatalf("first batch: %v", err)
	}
	if err := q.Enqueue(logs(3, model.SeverityInfo)); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("batch that does not fit: err = %v, want ErrQueueFull", err)
	}
	if got := q.Stats(); got.Queued != 8 || got.Accepted != 8 {
		t.Errorf("a rejected batch changed the counters: %+v", got)
	}
	if err := q.Enqueue(logs(2, model.SeverityInfo)); err != nil {
		t.Errorf("batch that fits exactly: %v", err)
	}
	if err := q.Enqueue(logs(0, model.SeverityInfo)); err != nil {
		t.Errorf("empty batch: %v", err)
	}

	q.Start()
	drain(t, q)
	if _, apps, _ := w.counts(); apps != 10 {
		t.Errorf("saved %d logs, want 10", apps)
	}
	if q.Stats().Queued != 0 {
		t.Errorf("queued = %d after drain, want 0", q.Stats().Queued)
	}
}

func TestRoomIsFreedAfterSaving(t *testing.T) {
	w := &fakeWriter{}
	q := newQueue(t, w, 5, 1)
	q.Start()
	for i := range 20 {
		// Wait for room each time: proves capacity is released after writes.
		deadline := time.Now().Add(2 * time.Second)
		for {
			err := q.Enqueue(logs(5, model.SeverityInfo))
			if err == nil {
				break
			}
			if !errors.Is(err, ErrQueueFull) || time.Now().After(deadline) {
				t.Fatalf("round %d: %v", i, err)
			}
			time.Sleep(time.Millisecond)
		}
	}
	drain(t, q)
	if _, apps, _ := w.counts(); apps != 100 {
		t.Errorf("saved %d, want 100", apps)
	}
}

func TestEnqueueAfterDrainIsRejected(t *testing.T) {
	q := newQueue(t, &fakeWriter{}, 10, 1)
	q.Start()
	drain(t, q)
	if err := q.Enqueue(logs(1, model.SeverityInfo)); !errors.Is(err, ErrClosed) {
		t.Errorf("err = %v, want ErrClosed", err)
	}
	drain(t, q) // draining twice is safe
}

func TestRetriesThenSucceeds(t *testing.T) {
	w := &fakeWriter{failures: 2}
	q := newQueue(t, w, 10, 1)
	q.Start()
	_ = q.Enqueue(logs(3, model.SeverityError))
	drain(t, q)

	errs, _, writes := w.counts()
	if errs != 3 || writes != 3 {
		t.Errorf("saved %d after %d writes; want 3 after 3", errs, writes)
	}
	if s := q.Stats(); s.Saved != 3 || s.Dropped != 0 {
		t.Errorf("stats = %+v", s)
	}
}

func TestDropsAfterAllRetriesFail(t *testing.T) {
	w := &fakeWriter{failures: 100}
	q := newQueue(t, w, 10, 1)
	q.Start()
	_ = q.Enqueue(logs(4, model.SeverityInfo))
	drain(t, q)

	if _, _, writes := w.counts(); writes != 4 { // 1 try + 3 retries
		t.Errorf("writes = %d, want 4", writes)
	}
	if s := q.Stats(); s.Dropped != 4 || s.Saved != 0 || s.Queued != 0 {
		t.Errorf("stats = %+v", s)
	}
}

func TestDrainSavesEverythingQueued(t *testing.T) {
	w := &fakeWriter{block: make(chan struct{})}
	q := newQueue(t, w, 1000, 2)
	q.Start()
	for range 50 {
		if err := q.Enqueue(logs(10, model.SeverityWarn)); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}
	close(w.block) // database becomes available while draining
	drain(t, q)
	if _, apps, _ := w.counts(); apps != 500 {
		t.Errorf("saved %d, want 500", apps)
	}
}

func TestDrainTimeoutCountsLostLogs(t *testing.T) {
	w := &fakeWriter{block: make(chan struct{})} // database never answers
	q := newQueue(t, w, 100, 1)
	q.Start()
	_ = q.Enqueue(logs(7, model.SeverityInfo))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := q.Drain(ctx)
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("drain took %s after its deadline", time.Since(start))
	}
	if s := q.Stats(); s.Dropped != 7 || s.Queued != 0 {
		t.Errorf("stats = %+v; want 7 dropped, 0 queued", s)
	}
}

func TestConcurrentEnqueueAndDrain(t *testing.T) {
	w := &fakeWriter{}
	q := newQueue(t, w, 200, 4)
	q.Start()

	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted := 0
	for range 20 {
		wg.Go(func() {
			for range 50 {
				if err := q.Enqueue(logs(3, model.SeverityInfo)); err == nil {
					mu.Lock()
					accepted += 3
					mu.Unlock()
				}
			}
		})
	}
	// Start draining while senders are still running.
	go func() {
		time.Sleep(time.Millisecond)
		_ = q.Drain(context.Background())
	}()
	wg.Wait()
	drain(t, q)

	_, apps, _ := w.counts()
	s := q.Stats()
	if apps != accepted || int(s.Accepted) != accepted || int(s.Saved) != accepted || s.Queued != 0 {
		t.Errorf("accepted %d, saved %d, stats %+v", accepted, apps, s)
	}
}
