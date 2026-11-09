package api

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

// httpRecorder shortens the recorder type in test helper signatures.
type httpRecorder = httptest.ResponseRecorder

// failingProjects is a ProjectStore whose every call fails.
type failingProjects struct{}

func (failingProjects) CreateProject(context.Context, *model.Project) error { return errDB }
func (failingProjects) ProjectsByOwner(context.Context, bson.ObjectID) ([]model.Project, error) {
	return nil, errDB
}
func (failingProjects) ProjectByID(context.Context, bson.ObjectID, bson.ObjectID) (model.Project, error) {
	return model.Project{}, errDB
}
func (failingProjects) RenameProject(context.Context, bson.ObjectID, bson.ObjectID, string) error {
	return errDB
}
func (failingProjects) RotateAPIKey(context.Context, bson.ObjectID, bson.ObjectID, string, string, time.Time) error {
	return errDB
}
func (failingProjects) DeleteProject(context.Context, bson.ObjectID, bson.ObjectID) error {
	return errDB
}

func TestRateLimiter(t *testing.T) {
	t.Parallel()

	newLimiter := func() (*rateLimiter, *fakeClock) {
		clock := &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
		return newRateLimiter(3, time.Minute, clock.Now), clock
	}

	t.Run("blocks a key at the limit and reports the wait", func(t *testing.T) {
		l, clock := newLimiter()
		for i := range 3 {
			if b, _ := l.blocked("k"); b {
				t.Fatalf("blocked after %d hits", i)
			}
			l.hit("k")
		}
		clock.Advance(20 * time.Second)
		b, retry := l.blocked("k")
		if !b || retry != 40*time.Second {
			t.Errorf("blocked = %v, retry = %s; want true, 40s", b, retry)
		}
	})

	t.Run("keys are independent", func(t *testing.T) {
		l, _ := newLimiter()
		for range 3 {
			l.hit("a")
		}
		if b, _ := l.blocked("b"); b {
			t.Error("key b blocked by key a")
		}
	})

	t.Run("unblocks when the window ends", func(t *testing.T) {
		l, clock := newLimiter()
		for range 3 {
			l.hit("k")
		}
		clock.Advance(time.Minute)
		if b, _ := l.blocked("k"); b {
			t.Error("still blocked after the window")
		}
		l.hit("k") // starts a fresh window with count 1
		if b, _ := l.blocked("k"); b {
			t.Error("blocked after one hit in a new window")
		}
	})

	t.Run("reset clears a key", func(t *testing.T) {
		l, _ := newLimiter()
		for range 3 {
			l.hit("k")
		}
		l.reset("k")
		if b, _ := l.blocked("k"); b {
			t.Error("still blocked after reset")
		}
	})

	t.Run("sweep removes finished windows", func(t *testing.T) {
		l, clock := newLimiter()
		for i := range 100 {
			l.hit(string(rune('a'+i%26)) + string(rune(i)))
		}
		clock.Advance(2 * time.Minute)
		l.hit("new")
		l.mu.Lock()
		n := len(l.counts)
		l.mu.Unlock()
		if n != 1 {
			t.Errorf("%d keys kept after sweep, want 1", n)
		}
	})

	t.Run("is safe under concurrent use", func(t *testing.T) {
		l, _ := newLimiter()
		l.limit = 1_000_000
		var wg sync.WaitGroup
		for range 50 {
			wg.Go(func() {
				for range 100 {
					l.hit("k")
					l.blocked("k")
				}
			})
		}
		wg.Wait()
		if c := l.counts["k"].count; c != 5000 {
			t.Errorf("count = %d, want 5000", c)
		}
	})
}
