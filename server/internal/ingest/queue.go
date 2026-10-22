// Package ingest holds the in-memory queue between the HTTP handlers and
// MongoDB. Handlers put logs on the queue and reply at once; background
// workers save them in batches, routed by severity.
package ingest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

// Errors returned by Enqueue.
var (
	ErrQueueFull = errors.New("ingest queue is full")
	ErrClosed    = errors.New("ingest queue is closed")
)

// Defaults for optional Config fields.
const (
	defaultMaxWriteBatch = 500
	defaultWriteTimeout  = 10 * time.Second
)

// defaultRetryDelays are the pauses between write attempts: a failed write
// is tried up to 4 times in total over roughly 2.6 seconds.
var defaultRetryDelays = []time.Duration{100 * time.Millisecond, 500 * time.Millisecond, 2 * time.Second}

// Writer saves logs. The MongoDB Store satisfies it; tests use a fake.
type Writer interface {
	InsertErrorLogs(ctx context.Context, logs []model.LogRecord) error
	InsertAppLogs(ctx context.Context, logs []model.LogRecord) error
}

// Config sets up a Queue. Capacity, Workers, Writer and Logger are required.
type Config struct {
	Capacity      int             // most logs held at once (queued or being written)
	Workers       int             // number of background writer goroutines
	Writer        Writer          // where logs are saved
	Logger        *slog.Logger    // for retry and drop messages
	MaxWriteBatch int             // most logs per database write; default 500
	WriteTimeout  time.Duration   // limit for one write attempt; default 10s
	RetryDelays   []time.Duration // pauses between attempts; default 100ms, 500ms, 2s
}

// Stats are the queue's counters since start.
type Stats struct {
	Accepted int64 // logs put on the queue
	Saved    int64 // logs written to MongoDB
	Dropped  int64 // logs given up on after all retries (or at shutdown timeout)
	Queued   int64 // logs currently waiting or being written
}

// Queue is a bounded, concurrency-safe log queue with background writers.
type Queue struct {
	cfg Config

	// items carries one accepted batch per element. Its buffer is Capacity,
	// and every batch has at least one log, so a send never blocks once
	// room has been reserved in queued.
	items chan []model.LogRecord

	// mu makes Enqueue and Close safe together: Enqueue holds a read lock
	// while sending, Close takes the write lock before closing items, so a
	// send can never hit a closed channel.
	mu     sync.RWMutex
	closed bool

	queued   atomic.Int64
	accepted atomic.Int64
	saved    atomic.Int64
	dropped  atomic.Int64

	workers sync.WaitGroup

	// writeCtx is the context for all database writes. It is cancelled only
	// if draining runs out of time, which makes in-flight writes give up.
	writeCtx    context.Context
	cancelWrite context.CancelFunc
}

// New creates a queue. Call Start to begin saving.
func New(cfg Config) (*Queue, error) {
	if cfg.Capacity < 1 || cfg.Workers < 1 || cfg.Writer == nil || cfg.Logger == nil {
		return nil, errors.New("ingest: Capacity, Workers, Writer and Logger are required")
	}
	if cfg.MaxWriteBatch < 1 {
		cfg.MaxWriteBatch = defaultMaxWriteBatch
	}
	if cfg.WriteTimeout <= 0 {
		cfg.WriteTimeout = defaultWriteTimeout
	}
	if cfg.RetryDelays == nil {
		cfg.RetryDelays = defaultRetryDelays
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Queue{
		cfg:         cfg,
		items:       make(chan []model.LogRecord, cfg.Capacity),
		writeCtx:    ctx,
		cancelWrite: cancel,
	}, nil
}

// Start launches the worker goroutines.
func (q *Queue) Start() {
	for range q.cfg.Workers {
		q.workers.Go(q.work)
	}
}

// Capacity returns the most logs the queue can hold.
func (q *Queue) Capacity() int { return q.cfg.Capacity }

// Enqueue adds a batch of logs. The whole batch is accepted or none of it:
// it returns ErrQueueFull if there is not room for every log, and
// ErrClosed once shutdown has started. It never blocks.
func (q *Queue) Enqueue(batch []model.LogRecord) error {
	n := int64(len(batch))
	if n == 0 {
		return nil
	}

	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.closed {
		return ErrClosed
	}

	// Reserve room for the whole batch. CompareAndSwap makes the check and
	// the update one step, so two requests cannot both take the last space.
	for {
		cur := q.queued.Load()
		if cur+n > int64(q.cfg.Capacity) {
			return ErrQueueFull
		}
		if q.queued.CompareAndSwap(cur, cur+n) {
			break
		}
	}

	q.items <- batch
	q.accepted.Add(n)
	return nil
}

// Drain stops accepting logs and waits until every queued log has been
// saved (or dropped after retries). If ctx ends first, in-flight writes
// are cancelled, the remaining logs are counted as dropped, and an error
// reports how many were lost.
func (q *Queue) Drain(ctx context.Context) error {
	q.close()

	done := make(chan struct{})
	go func() {
		q.workers.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		q.cancelWrite()
		<-done // workers now fail fast and exit
		return fmt.Errorf("drain timed out: %d logs dropped in total", q.dropped.Load())
	}
}

// Stats returns a snapshot of the counters.
func (q *Queue) Stats() Stats {
	return Stats{
		Accepted: q.accepted.Load(),
		Saved:    q.saved.Load(),
		Dropped:  q.dropped.Load(),
		Queued:   q.queued.Load(),
	}
}

// close stops new logs from being accepted. Workers keep running until
// the channel is empty. Calling it more than once is safe.
func (q *Queue) close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.closed {
		q.closed = true
		close(q.items)
	}
}

// work is one worker goroutine: it takes batches off the queue, merges
// whatever else is waiting (up to MaxWriteBatch logs) into one write, and
// exits when the queue is closed and empty.
func (q *Queue) work() {
	for batch := range q.items {
		logs := make([]model.LogRecord, 0, max(len(batch), q.cfg.MaxWriteBatch))
		logs = append(logs, batch...)

		// Under load, combine more waiting batches into the same write.
		// Under light load nothing is waiting and the log is saved at once.
	collect:
		for len(logs) < q.cfg.MaxWriteBatch {
			select {
			case more, ok := <-q.items:
				if !ok {
					break collect
				}
				logs = append(logs, more...)
			default:
				break collect
			}
		}

		q.write(logs)
		q.queued.Add(-int64(len(logs)))
	}
}

// write routes logs by severity and saves each group.
func (q *Queue) write(logs []model.LogRecord) {
	var errorLogs, appLogs []model.LogRecord
	for _, l := range logs {
		if model.IsErrorSeverity(l.Severity) {
			l.DiagnosisStatus = model.DiagnosisPending
			errorLogs = append(errorLogs, l)
		} else {
			appLogs = append(appLogs, l)
		}
	}
	if len(errorLogs) > 0 {
		q.writeWithRetry("ErrorLogs", q.cfg.Writer.InsertErrorLogs, errorLogs)
	}
	if len(appLogs) > 0 {
		q.writeWithRetry("AppLogs", q.cfg.Writer.InsertAppLogs, appLogs)
	}
}

// writeWithRetry tries to save logs, pausing between attempts. If every
// attempt fails, the logs are counted as dropped and an error is logged.
func (q *Queue) writeWithRetry(target string, insert func(context.Context, []model.LogRecord) error, logs []model.LogRecord) {
	var err error
	for attempt := 0; ; attempt++ {
		ctx, cancel := context.WithTimeout(q.writeCtx, q.cfg.WriteTimeout)
		err = insert(ctx, logs)
		cancel()
		if err == nil {
			q.saved.Add(int64(len(logs)))
			return
		}
		if attempt >= len(q.cfg.RetryDelays) || q.writeCtx.Err() != nil {
			break
		}

		delay := q.cfg.RetryDelays[attempt]
		q.cfg.Logger.Warn("ingest: write failed, retrying",
			"target", target, "logs", len(logs), "attempt", attempt+1, "retry_in", delay, "error", err)
		select {
		case <-time.After(delay):
		case <-q.writeCtx.Done():
		}
	}

	q.dropped.Add(int64(len(logs)))
	q.cfg.Logger.Error("ingest: dropping logs after failed writes",
		"target", target, "logs", len(logs), "error", err)
}
