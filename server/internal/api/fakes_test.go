package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

// fakeStore is an in-memory stand-in for the MongoDB store.
// Setting failWith makes every call return that error.
type fakeStore struct {
	mu       sync.Mutex
	users    map[bson.ObjectID]model.User
	sessions map[string]model.Session // keyed by token hash
	projects []model.Project
	pingErr  error
	failWith error
}

func newFakeStore() *fakeStore {
	return &fakeStore{users: map[bson.ObjectID]model.User{}, sessions: map[string]model.Session{}}
}

func (f *fakeStore) Ping(ctx context.Context) error { return f.pingErr }

func (f *fakeStore) CreateUser(_ context.Context, u *model.User) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failWith != nil {
		return f.failWith
	}
	for _, existing := range f.users {
		if existing.Email == u.Email {
			return model.ErrEmailTaken
		}
	}
	u.ID = bson.NewObjectID()
	f.users[u.ID] = *u
	return nil
}

func (f *fakeStore) UserByEmail(_ context.Context, email string) (model.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failWith != nil {
		return model.User{}, f.failWith
	}
	for _, u := range f.users {
		if u.Email == email {
			return u, nil
		}
	}
	return model.User{}, model.ErrNotFound
}

func (f *fakeStore) UserByID(_ context.Context, id bson.ObjectID) (model.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failWith != nil {
		return model.User{}, f.failWith
	}
	u, ok := f.users[id]
	if !ok {
		return model.User{}, model.ErrNotFound
	}
	return u, nil
}

func (f *fakeStore) CreateSession(_ context.Context, s *model.Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failWith != nil {
		return f.failWith
	}
	s.ID = bson.NewObjectID()
	f.sessions[s.TokenHash] = *s
	return nil
}

func (f *fakeStore) SessionByTokenHash(_ context.Context, hash string, now time.Time) (model.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failWith != nil {
		return model.Session{}, f.failWith
	}
	s, ok := f.sessions[hash]
	if !ok || !s.ExpiresAt.After(now) {
		return model.Session{}, model.ErrNotFound
	}
	return s, nil
}

func (f *fakeStore) DeleteSession(_ context.Context, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failWith != nil {
		return f.failWith
	}
	delete(f.sessions, hash)
	return nil
}

func (f *fakeStore) CreateProject(_ context.Context, p *model.Project) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failWith != nil {
		return f.failWith
	}
	for _, existing := range f.projects {
		if existing.OwnerID == p.OwnerID && existing.Name == p.Name && existing.Environment == p.Environment {
			return model.ErrProjectExists
		}
	}
	p.ID = bson.NewObjectID()
	f.projects = append(f.projects, *p)
	return nil
}

func (f *fakeStore) ProjectsByOwner(_ context.Context, owner bson.ObjectID) ([]model.Project, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failWith != nil {
		return nil, f.failWith
	}
	out := []model.Project{}
	for i := len(f.projects) - 1; i >= 0; i-- { // newest first
		if p := f.projects[i]; p.OwnerID == owner {
			p.APIKeyHash = ""
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakeStore) ProjectByAPIKeyHash(_ context.Context, hash string) (model.Project, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failWith != nil {
		return model.Project{}, f.failWith
	}
	for _, p := range f.projects {
		if p.APIKeyHash == hash {
			return p, nil
		}
	}
	return model.Project{}, model.ErrNotFound
}

// fakeQueue records enqueued logs; err makes Enqueue fail.
type fakeQueue struct {
	mu   sync.Mutex
	logs []model.LogRecord
	err  error
}

func (q *fakeQueue) Enqueue(logs []model.LogRecord) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return q.err
	}
	q.logs = append(q.logs, logs...)
	return nil
}

// fakeMetricQueue records enqueued metrics; err makes Enqueue fail.
type fakeMetricQueue struct {
	mu      sync.Mutex
	metrics []model.MetricPoint
	err     error
}

func (q *fakeMetricQueue) Enqueue(m []model.MetricPoint) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return q.err
	}
	q.metrics = append(q.metrics, m...)
	return nil
}

func (q *fakeMetricQueue) Capacity() int { return 5000 }

func (q *fakeMetricQueue) queued() []model.MetricPoint {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]model.MetricPoint(nil), q.metrics...)
}

func (q *fakeQueue) Capacity() int { return 5000 }

func (q *fakeQueue) queued() []model.LogRecord {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]model.LogRecord(nil), q.logs...)
}

func (f *fakeStore) setFail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failWith = err
}

// fakeClock is a controllable time source.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// testEnv bundles a router with its fake store, queue, clock and captured logs.
type testEnv struct {
	h       http.Handler
	store   *fakeStore
	queue   *fakeQueue
	metrics *fakeMetricQueue
	clock   *fakeClock
	logs    *strings.Builder
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	store := newFakeStore()
	clock := &fakeClock{t: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)}
	logs := &strings.Builder{}
	var logMu sync.Mutex
	logger := slog.New(slog.NewTextHandler(lockedWriter{w: logs, mu: &logMu}, nil))
	queue := &fakeQueue{}
	metrics := &fakeMetricQueue{}
	h := NewRouter(Deps{Logger: logger, DB: store, Users: store, Sessions: store, Projects: store, Keys: store, Logs: queue, Metrics: metrics, Now: clock.Now})
	return &testEnv{h: h, store: store, queue: queue, metrics: metrics, clock: clock, logs: logs}
}

// lockedWriter makes a strings.Builder safe for concurrent log writes.
type lockedWriter struct {
	w  io.Writer
	mu *sync.Mutex
}

func (l lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// request describes one HTTP call in a test.
type request struct {
	method string
	path   string
	body   string
	token  string // sent as "Authorization: Bearer <token>" when set
	apiKey string // sent as "X-API-Key" when set
	ip     string // client IP; default 192.0.2.1
	header map[string]string
}

func (e *testEnv) do(t *testing.T, req request) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader
	if req.body != "" {
		body = strings.NewReader(req.body)
	}
	r := httptest.NewRequest(req.method, req.path, body)
	if req.body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if req.token != "" {
		r.Header.Set("Authorization", "Bearer "+req.token)
	}
	if req.apiKey != "" {
		r.Header.Set("X-API-Key", req.apiKey)
	}
	ip := req.ip
	if ip == "" {
		ip = "192.0.2.1"
	}
	r.RemoteAddr = ip + ":40000"
	for k, v := range req.header {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	return rec
}

// signup creates an account through the API and fails the test if it does not work.
func (e *testEnv) signup(t *testing.T, email, password string) {
	t.Helper()
	rec := e.do(t, request{method: "POST", path: "/api/v1/auth/signup", body: jsonBody(t, credentials{Email: email, Password: password}), ip: "198.51.100.200"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("signup %s: status %d, body %s", email, rec.Code, rec.Body)
	}
}

// login logs in through the API and returns the session token.
func (e *testEnv) login(t *testing.T, email, password string) string {
	t.Helper()
	rec := e.do(t, request{method: "POST", path: "/api/v1/auth/login", body: jsonBody(t, credentials{Email: email, Password: password}), ip: "198.51.100.201"})
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s: status %d, body %s", email, rec.Code, rec.Body)
	}
	var resp loginResponse
	decodeBody(t, rec, &resp)
	return resp.Token
}

func jsonBody(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
}

// wantError checks the status code and the standard error code.
func wantError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body %s", rec.Code, status, rec.Body)
	}
	var e errorResponse
	decodeBody(t, rec, &e)
	if e.Error.Code != code {
		t.Errorf("error code = %q, want %q", e.Error.Code, code)
	}
	if e.Error.Message == "" {
		t.Error("error message is empty")
	}
}
