package api

import (
	"sync"
	"time"
)

// rateLimiter counts events per key (an IP address or an email) in fixed
// time windows, and blocks a key once it reaches the limit.
//
// It lives in memory, so counts reset when the server restarts and are not
// shared between several server instances. That is enough for a single
// server; a shared store (e.g. Redis) would be needed for several.
type rateLimiter struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu        sync.Mutex // protects the fields below; handlers run concurrently
	counts    map[string]*windowCount
	nextSweep time.Time
}

// windowCount is the number of events for one key in its current window.
type windowCount struct {
	count   int
	resetAt time.Time
}

func newRateLimiter(limit int, window time.Duration, now func() time.Time) *rateLimiter {
	return &rateLimiter{limit: limit, window: window, now: now, counts: make(map[string]*windowCount)}
}

// blocked reports whether key has reached the limit, and if so how long
// until it may try again.
func (l *rateLimiter) blocked(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	c, ok := l.counts[key]
	if !ok || !now.Before(c.resetAt) || c.count < l.limit {
		return false, 0
	}
	return true, c.resetAt.Sub(now)
}

// hit records one event for key.
func (l *rateLimiter) hit(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)

	c, ok := l.counts[key]
	if !ok || !now.Before(c.resetAt) {
		// No count yet, or the old window is over: start a new window.
		l.counts[key] = &windowCount{count: 1, resetAt: now.Add(l.window)}
		return
	}
	c.count++
}

// reset clears the count for key (e.g. after a successful login).
func (l *rateLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.counts, key)
}

// sweep removes finished windows, at most once per window, so memory does
// not grow with every key ever seen. The caller must hold l.mu.
func (l *rateLimiter) sweep(now time.Time) {
	if now.Before(l.nextSweep) {
		return
	}
	for key, c := range l.counts {
		if !now.Before(c.resetAt) {
			delete(l.counts, key)
		}
	}
	l.nextSweep = now.Add(l.window)
}
