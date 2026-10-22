// Package ingest holds the in-memory queues between the HTTP handlers and
// MongoDB. Handlers put items (logs or metrics) on a queue and reply at
// once; background workers save them in batches.
package ingest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
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

// Config sets up a Queue of items of type T.
// Name, Capacity, Workers, Write and Logger are required.
type Config[T any] struct {
	Name          string                                     // shown in log messages, e.g. "logs"
	Capacity      int                                        // most items held at once (queued or being written)
	Workers       int                                        // number of background writer goroutines
	Write         func(ctx context.Context, items []T) error // saves one batch; must be safe to retry
	Logger        *slog.Logger                               // for retry and drop messages
	MaxWriteBatch int                                        // most items per write; default 500
	WriteTimeout  time.Duration                              // limit for one write attempt; default 10s
	RetryDelays   []time.Duration                            // pauses between attempts; default 100ms, 500ms, 2s
}

// Stats are the queue's counters since start.
type Stats struct {
	Accepted int64 // items put on the queue
	Saved    int64 // items written to MongoDB
	Dropped  int64 // items given up on after all retries (or at shutdown timeout)
	Queued   int64 // items currently waiting or being written
}

// Queue is a bounded, concurrency-safe queue of T with background writers.
// T is the item type, e.g. model.LogRecord or model.MetricPoint.
type Queue[T any] struct {
	cfg Config[T]

	// items carries one accepted batch per element. Its buffer is Capacity,
	// and every batch has at least one item, so a send never blocks once
	// room has been reserved in queued.
	items chan []T

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
func New[T any](cfg Config[T]) (*Queue[T], error) {
	if cfg.Name == "" || cfg.Capacity < 1 || cfg.Workers < 1 || cfg.Write == nil || cfg.Logger == nil {
		return nil, errors.New("ingest: Name, Capacity, Workers, Write and Logger are required")
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
	return &Queue[T]{
		cfg:         cfg,
		items:       make(chan []T, cfg.Capacity),
		writeCtx:    ctx,
		cancelWrite: cancel,
	}, nil
}

// Start launches the worker goroutines.
func (q *Queue[T]) Start() {
	for range q.cfg.Workers {
		q.workers.Go(q.work)
	}
}

// Capacity returns the most items the queue can hold.
func (q *Queue[T]) Capacity() int { return q.cfg.Capacity }

// Enqueue adds a batch of items. The whole batch is accepted or none of it:
// it returns ErrQueueFull if there is not room for every item, and
// ErrClosed once shutdown has started. It never blocks.
func (q *Queue[T]) Enqueue(batch []T) error {
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

// Drain stops accepting items and waits until every queued item has been
// saved (or dropped after retries). If ctx ends first, in-flight writes
// are cancelled, the remaining items are counted as dropped, and an error
// reports how many were lost.
func (q *Queue[T]) Drain(ctx context.Context) error {
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
		return fmt.Errorf("%s drain timed out: %d dropped in total", q.cfg.Name, q.dropped.Load())
	}
}

// Stats returns a snapshot of the counters.
func (q *Queue[T]) Stats() Stats {
	return Stats{
		Accepted: q.accepted.Load(),
		Saved:    q.saved.Load(),
		Dropped:  q.dropped.Load(),
		Queued:   q.queued.Load(),
	}
}

// close stops new items from being accepted. Workers keep running until
// the channel is empty. Calling it more than once is safe.
func (q *Queue[T]) close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.closed {
		q.closed = true
		close(q.items)
	}
}

// work is one worker goroutine: it takes batches off the queue, merges
// whatever else is waiting (up to MaxWriteBatch items) into one write, and
// exits when the queue is closed and empty.
func (q *Queue[T]) work() {
	for batch := range q.items {
		items := make([]T, 0, max(len(batch), q.cfg.MaxWriteBatch))
		items = append(items, batch...)

		// Under load, combine more waiting batches into the same write.
		// Under light load nothing is waiting and the item is saved at once.
	collect:
		for len(items) < q.cfg.MaxWriteBatch {
			select {
			case more, ok := <-q.items:
				if !ok {
					break collect
				}
				items = append(items, more...)
			default:
				break collect
			}
		}

		q.writeWithRetry(items)
		q.queued.Add(-int64(len(items)))
	}
}

// writeWithRetry tries to save items, pausing between attempts. If every
// attempt fails, the items are counted as dropped and an error is logged.
func (q *Queue[T]) writeWithRetry(items []T) {
	var err error
	for attempt := 0; ; attempt++ {
		ctx, cancel := context.WithTimeout(q.writeCtx, q.cfg.WriteTimeout)
		err = q.cfg.Write(ctx, items)
		cancel()
		if err == nil {
			q.saved.Add(int64(len(items)))
			return
		}
		if attempt >= len(q.cfg.RetryDelays) || q.writeCtx.Err() != nil {
			break
		}

		delay := q.cfg.RetryDelays[attempt]
		q.cfg.Logger.Warn("ingest: write failed, retrying",
			"queue", q.cfg.Name, "items", len(items), "attempt", attempt+1, "retry_in", delay, "error", err)
		select {
		case <-time.After(delay):
		case <-q.writeCtx.Done():
		}
	}

	q.dropped.Add(int64(len(items)))
	q.cfg.Logger.Error("ingest: dropping items after failed writes",
		"queue", q.cfg.Name, "items", len(items), "error", err)
}
