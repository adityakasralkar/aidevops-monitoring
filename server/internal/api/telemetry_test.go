package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/ingest"
	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

// newProjectKey signs up, logs in and creates a test project, returning its API key.
func (e *testEnv) newProjectKey(t *testing.T) string {
	t.Helper()
	e.signup(t, testEmail, testPassword)
	token := e.login(t, testEmail, testPassword)
	rec := e.do(t, request{method: "POST", path: "/api/v1/projects", token: token, body: `{"name":"shop","environment":"test"}`})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create project: %d %s", rec.Code, rec.Body)
	}
	var p projectResponse
	decodeBody(t, rec, &p)
	return p.APIKey
}

func (e *testEnv) sendLogs(t *testing.T, key, body string) *httpRecorder {
	t.Helper()
	return e.do(t, request{method: "POST", path: "/api/v1/telemetry/logs", apiKey: key, body: body})
}

const validLog = `{"service_name":"checkout","severity":"ERROR","message":"pool exhausted"}`

func TestLogsAuth(t *testing.T) {
	t.Parallel()

	t.Run("rejects a missing key", func(t *testing.T) {
		e := newTestEnv(t)
		wantError(t, e.sendLogs(t, "", validLog), http.StatusUnauthorized, codeMissingAPIKey)
	})
	t.Run("rejects an unknown key", func(t *testing.T) {
		e := newTestEnv(t)
		wantError(t, e.sendLogs(t, "aidv_test_not-a-real-key", validLog), http.StatusUnauthorized, codeInvalidAPIKey)
	})
	t.Run("rejects an absurdly long key", func(t *testing.T) {
		e := newTestEnv(t)
		wantError(t, e.sendLogs(t, strings.Repeat("k", maxTokenLen+1), validLog), http.StatusUnauthorized, codeInvalidAPIKey)
	})
	t.Run("rejects a key whose environment does not match the project", func(t *testing.T) {
		e := newTestEnv(t)
		key := e.newProjectKey(t)
		e.store.mu.Lock()
		e.store.projects[0].Environment = "live" // stored data changed by hand
		e.store.mu.Unlock()
		wantError(t, e.sendLogs(t, key, validLog), http.StatusUnauthorized, codeEnvMismatch)
	})
	t.Run("returns 500 when the key lookup fails", func(t *testing.T) {
		e := newTestEnv(t)
		key := e.newProjectKey(t)
		e.store.setFail(errDB)
		wantError(t, e.sendLogs(t, key, validLog), http.StatusInternalServerError, codeInternalError)
	})
	t.Run("a session token is not accepted as an API key", func(t *testing.T) {
		e := newTestEnv(t)
		e.signup(t, testEmail, testPassword)
		token := e.login(t, testEmail, testPassword)
		wantError(t, e.sendLogs(t, token, validLog), http.StatusUnauthorized, codeInvalidAPIKey)
	})
}

func TestLogsAccepted(t *testing.T) {
	t.Parallel()

	t.Run("accepts a single log and stamps the project from the key", func(t *testing.T) {
		e := newTestEnv(t)
		key := e.newProjectKey(t)
		rec := e.sendLogs(t, key, `{
			"service_name": "  checkout ",
			"severity": "error",
			"message": "pool exhausted",
			"stack_trace": "at pool.go:88",
			"timestamp": "2026-10-18T11:59:30.123456Z",
			"attributes": {"http.route": "/pay", "retries": 3, "cached": false}
		}`)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		var resp acceptedResponse
		decodeBody(t, rec, &resp)

		got := e.queue.queued()
		if len(got) != 1 {
			t.Fatalf("queued %d logs, want 1", len(got))
		}
		l := got[0]
		if resp.Status != "accepted" || resp.ID != l.ID.Hex() {
			t.Errorf("response %+v does not match queued id %s", resp, l.ID.Hex())
		}
		if l.ProjectID != e.store.projects[0].ID {
			t.Error("project_id not taken from the API key")
		}
		wantTS := time.Date(2026, 10, 18, 11, 59, 30, 123_000_000, time.UTC)
		if l.ServiceName != "checkout" || l.Severity != "ERROR" || l.StackTrace != "at pool.go:88" ||
			!l.Timestamp.Equal(wantTS) || !l.ReceivedAt.Equal(e.clock.Now()) {
			t.Errorf("queued log = %+v", l)
		}
		if l.Attributes["http.route"] != "/pay" || l.Attributes["retries"] != float64(3) || l.Attributes["cached"] != false {
			t.Errorf("attributes = %#v", l.Attributes)
		}
	})

	t.Run("uses the server time when no timestamp is sent", func(t *testing.T) {
		e := newTestEnv(t)
		key := e.newProjectKey(t)
		if rec := e.sendLogs(t, key, validLog); rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d", rec.Code)
		}
		if ts := e.queue.queued()[0].Timestamp; !ts.Equal(e.clock.Now()) {
			t.Errorf("timestamp = %s, want server time", ts)
		}
	})

	t.Run("accepts a batch and returns ids in order", func(t *testing.T) {
		e := newTestEnv(t)
		key := e.newProjectKey(t)
		rec := e.sendLogs(t, key, `[
			{"service_name":"a","severity":"INFO","message":"one"},
			{"service_name":"b","severity":"WARN","message":"two"},
			{"service_name":"c","severity":"CRITICAL","message":"three"}
		]`)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		var resp acceptedBatchResponse
		decodeBody(t, rec, &resp)
		got := e.queue.queued()
		if resp.Count != 3 || len(resp.IDs) != 3 || len(got) != 3 {
			t.Fatalf("response %+v, queued %d", resp, len(got))
		}
		for i := range got {
			if resp.IDs[i] != got[i].ID.Hex() {
				t.Errorf("ids[%d] = %s, queued %s", i, resp.IDs[i], got[i].ID.Hex())
			}
		}
	})

	t.Run("accepts exactly 500 logs", func(t *testing.T) {
		e := newTestEnv(t)
		key := e.newProjectKey(t)
		if rec := e.sendLogs(t, key, logArray(maxLogsPerRequest)); rec.Code != http.StatusAccepted {
			t.Errorf("status = %d, body %s", rec.Code, rec.Body)
		}
	})

	t.Run("accepts a timestamp up to 5 minutes ahead (clock skew)", func(t *testing.T) {
		e := newTestEnv(t)
		key := e.newProjectKey(t)
		ts := e.clock.Now().Add(4 * time.Minute).Format(time.RFC3339)
		body := fmt.Sprintf(`{"service_name":"a","severity":"INFO","message":"m","timestamp":%q}`, ts)
		if rec := e.sendLogs(t, key, body); rec.Code != http.StatusAccepted {
			t.Errorf("status = %d, body %s", rec.Code, rec.Body)
		}
	})
}

func logArray(n int) string {
	items := make([]string, n)
	for i := range items {
		items[i] = fmt.Sprintf(`{"service_name":"svc","severity":"INFO","message":"log %d"}`, i)
	}
	return "[" + strings.Join(items, ",") + "]"
}

func TestLogsValidation(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	key := e.newProjectKey(t)
	future := e.clock.Now().Add(10 * time.Minute).Format(time.RFC3339)

	tests := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{"missing service_name", `{"severity":"INFO","message":"m"}`, "service_name"},
		{"blank service_name", `{"service_name":"  ","severity":"INFO","message":"m"}`, "service_name"},
		{"service_name too long", `{"service_name":"` + strings.Repeat("s", 101) + `","severity":"INFO","message":"m"}`, "service_name"},
		{"missing severity", `{"service_name":"a","message":"m"}`, "severity"},
		{"unknown severity", `{"service_name":"a","severity":"DEBUG","message":"m"}`, "severity"},
		{"missing message", `{"service_name":"a","severity":"INFO"}`, "message is required"},
		{"blank message", `{"service_name":"a","severity":"INFO","message":"   "}`, "message is required"},
		{"message too long", `{"service_name":"a","severity":"INFO","message":"` + strings.Repeat("m", maxMessageBytes+1) + `"}`, "message must be at most"},
		{"stack trace too long", `{"service_name":"a","severity":"INFO","message":"m","stack_trace":"` + strings.Repeat("s", maxStackBytes+1) + `"}`, "stack_trace"},
		{"bad timestamp", `{"service_name":"a","severity":"INFO","message":"m","timestamp":"yesterday"}`, "RFC 3339"},
		{"timestamp far in the future", `{"service_name":"a","severity":"INFO","message":"m","timestamp":"` + future + `"}`, "future"},
		{"nested attribute", `{"service_name":"a","severity":"INFO","message":"m","attributes":{"x":{"y":1}}}`, "attribute \"x\""},
		{"array attribute", `{"service_name":"a","severity":"INFO","message":"m","attributes":{"x":[1]}}`, "attribute \"x\""},
		{"null attribute", `{"service_name":"a","severity":"INFO","message":"m","attributes":{"x":null}}`, "attribute \"x\""},
		{"attribute name starting with $", `{"service_name":"a","severity":"INFO","message":"m","attributes":{"$where":"1"}}`, "not allowed"},
		{"empty attribute name", `{"service_name":"a","severity":"INFO","message":"m","attributes":{"":"1"}}`, "not allowed"},
		{"attribute value too long", `{"service_name":"a","severity":"INFO","message":"m","attributes":{"x":"` + strings.Repeat("v", maxAttrValueBytes+1) + `"}}`, "at most"},
		{"too many attributes", `{"service_name":"a","severity":"INFO","message":"m","attributes":{` + manyAttrs(maxAttributes+1) + `}}`, "attributes allowed"},
		{"sender tries to set project_id", `{"service_name":"a","severity":"INFO","message":"m","project_id":"someone-else"}`, `unknown field "project_id"`},
		{"wrong type", `{"service_name":"a","severity":"INFO","message":123}`, "wrong type"},
		{"a number instead of a log", `42`, "log object or an array"},
		{"a string instead of a log", `"hello"`, "log object or an array"},
		{"empty array", `[]`, "must not be empty"},
		{"too many logs", logArray(maxLogsPerRequest + 1), "at most 500 logs"},
		{"array of non-objects", `[1,2]`, "logs[0]"},
	}
	for _, tt := range tests {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			rec := e.sendLogs(t, key, tt.body)
			wantError(t, rec, http.StatusBadRequest, codeValidationFailed)
			var resp errorResponse
			decodeBody(t, rec, &resp)
			if !strings.Contains(resp.Error.Message, tt.wantMsg) {
				t.Errorf("message %q does not mention %q", resp.Error.Message, tt.wantMsg)
			}
		})
	}
	if n := len(e.queue.queued()); n != 0 {
		t.Errorf("%d invalid logs were queued", n)
	}
}

func manyAttrs(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf(`"k%d":%d`, i, i)
	}
	return strings.Join(parts, ",")
}

func TestLogsBatchIsAllOrNothing(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	key := e.newProjectKey(t)
	rec := e.sendLogs(t, key, `[
		{"service_name":"a","severity":"INFO","message":"ok"},
		{"service_name":"b","severity":"INFO","message":"ok"},
		{"service_name":"c","severity":"LOUD","message":"bad"}
	]`)
	wantError(t, rec, http.StatusBadRequest, codeValidationFailed)
	var resp errorResponse
	decodeBody(t, rec, &resp)
	if !strings.HasPrefix(resp.Error.Message, "logs[2]: ") {
		t.Errorf("message %q does not name the bad item", resp.Error.Message)
	}
	if n := len(e.queue.queued()); n != 0 {
		t.Errorf("%d logs queued from a rejected batch", n)
	}
}

func TestLogsQueueErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		err       error
		status    int
		code      string
		retryHint string
	}{
		{"returns 503 when the queue is full", ingest.ErrQueueFull, http.StatusServiceUnavailable, codeQueueFull, "1"},
		{"returns 503 while shutting down", ingest.ErrClosed, http.StatusServiceUnavailable, codeShuttingDown, "5"},
		{"returns 500 for an unexpected error", errDB, http.StatusInternalServerError, codeInternalError, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEnv(t)
			key := e.newProjectKey(t)
			e.queue.err = tt.err
			rec := e.sendLogs(t, key, validLog)
			wantError(t, rec, tt.status, tt.code)
			if got := rec.Header().Get("Retry-After"); got != tt.retryHint {
				t.Errorf("Retry-After = %q, want %q", got, tt.retryHint)
			}
		})
	}
}

func TestLogsBodyLimit(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	key := e.newProjectKey(t)
	body := `{"service_name":"a","severity":"INFO","message":"` + strings.Repeat("m", telemetryMaxBody) + `"}`
	wantError(t, e.sendLogs(t, key, body), http.StatusRequestEntityTooLarge, codeInvalidJSON)
}

func TestIsErrorSeverityRouting(t *testing.T) {
	for sev, want := range map[string]bool{
		model.SeverityInfo: false, model.SeverityWarn: false,
		model.SeverityError: true, model.SeverityCritical: true,
	} {
		if got := model.IsErrorSeverity(sev); got != want {
			t.Errorf("IsErrorSeverity(%s) = %v, want %v", sev, got, want)
		}
	}
}
