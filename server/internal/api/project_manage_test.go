package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/auth"
)

// newProject creates a project through the API and returns its response.
func (e *testEnv) newProject(t *testing.T, token, name, env string) projectResponse {
	t.Helper()
	rec := e.do(t, request{method: "POST", path: "/api/v1/projects", token: token, body: `{"name":"` + name + `","environment":"` + env + `"}`})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create project: %d %s", rec.Code, rec.Body)
	}
	var p projectResponse
	decodeBody(t, rec, &p)
	return p
}

func (e *testEnv) loggedIn(t *testing.T, email string) string {
	t.Helper()
	e.signup(t, email, testPassword)
	return e.login(t, email, testPassword)
}

func TestGetProject(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	mine := e.loggedIn(t, testEmail)
	theirs := e.loggedIn(t, "other@example.com")
	p := e.newProject(t, mine, "shop", "live")

	t.Run("returns my project with key dates and no key", func(t *testing.T) {
		rec := e.do(t, request{method: "GET", path: "/api/v1/projects/" + p.ID, token: mine})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		var got projectResponse
		decodeBody(t, rec, &got)
		if got.ID != p.ID || got.Name != "shop" || got.APIKey != "" || !got.KeyCreatedAt.Equal(e.clock.Now()) || got.LastUsedAt != nil {
			t.Errorf("project = %+v", got)
		}
		if strings.Contains(rec.Body.String(), "api_key\"") {
			t.Error("response contains the key")
		}
	})
	t.Run("another user's project is not found", func(t *testing.T) {
		wantError(t, e.do(t, request{method: "GET", path: "/api/v1/projects/" + p.ID, token: theirs}), http.StatusNotFound, codeNotFound)
	})
	t.Run("an invalid id is not found", func(t *testing.T) {
		wantError(t, e.do(t, request{method: "GET", path: "/api/v1/projects/not-an-id", token: mine}), http.StatusNotFound, codeNotFound)
	})
	t.Run("requires login", func(t *testing.T) {
		wantError(t, e.do(t, request{method: "GET", path: "/api/v1/projects/" + p.ID}), http.StatusUnauthorized, codeMissingToken)
	})
}

func TestRenameProject(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	token := e.loggedIn(t, testEmail)
	p := e.newProject(t, token, "shop", "live")
	e.newProject(t, token, "billing", "live")
	e.newProject(t, token, "shop-test", "test")

	rename := func(name string) *httpRecorder {
		return e.do(t, request{method: "PATCH", path: "/api/v1/projects/" + p.ID, token: token, body: `{"name":"` + name + `"}`})
	}

	t.Run("renames and returns the project", func(t *testing.T) {
		rec := rename("  shop-backend ")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d %s", rec.Code, rec.Body)
		}
		var got projectResponse
		decodeBody(t, rec, &got)
		if got.Name != "shop-backend" || got.Environment != "live" {
			t.Errorf("project = %+v", got)
		}
	})
	t.Run("rejects a name already used in the same environment", func(t *testing.T) {
		wantError(t, rename("billing"), http.StatusConflict, codeProjectExists)
	})
	t.Run("allows a name used in another environment", func(t *testing.T) {
		if rec := rename("shop-test"); rec.Code != http.StatusOK {
			t.Errorf("status = %d", rec.Code)
		}
	})
	t.Run("rejects an invalid name", func(t *testing.T) {
		wantError(t, rename("-bad"), http.StatusBadRequest, codeValidationFailed)
	})
	t.Run("cannot change the environment", func(t *testing.T) {
		rec := e.do(t, request{method: "PATCH", path: "/api/v1/projects/" + p.ID, token: token, body: `{"name":"x","environment":"test"}`})
		wantError(t, rec, http.StatusBadRequest, codeInvalidJSON)
	})
}

func TestRotateAPIKey(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	token := e.loggedIn(t, testEmail)
	p := e.newProject(t, token, "shop", "live")
	oldKey := p.APIKey

	e.clock.Advance(time.Hour)
	rec := e.do(t, request{method: "POST", path: "/api/v1/projects/" + p.ID + "/api-key", token: token})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("missing Cache-Control: no-store")
	}
	var got projectResponse
	decodeBody(t, rec, &got)
	if !strings.HasPrefix(got.APIKey, "aidv_live_") || got.APIKey == oldKey || !strings.HasPrefix(got.APIKey, got.KeyPrefix) {
		t.Fatalf("new key = %q prefix %q", got.APIKey, got.KeyPrefix)
	}
	if !got.KeyCreatedAt.Equal(e.clock.Now()) {
		t.Errorf("key_created_at = %s", got.KeyCreatedAt)
	}
	if e.store.projects[0].APIKeyHash != auth.HashToken(got.APIKey) {
		t.Error("stored hash is not the new key's")
	}

	t.Run("the old key no longer works and the new one does", func(t *testing.T) {
		wantError(t, e.sendLogs(t, oldKey, validLog), http.StatusUnauthorized, codeInvalidAPIKey)
		if rec := e.sendLogs(t, got.APIKey, validLog); rec.Code != http.StatusAccepted {
			t.Errorf("new key: status %d", rec.Code)
		}
	})
	t.Run("another user cannot rotate it", func(t *testing.T) {
		other := e.loggedIn(t, "other@example.com")
		wantError(t, e.do(t, request{method: "POST", path: "/api/v1/projects/" + p.ID + "/api-key", token: other}), http.StatusNotFound, codeNotFound)
	})
}

func TestDeleteProject(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	token := e.loggedIn(t, testEmail)
	p := e.newProject(t, token, "shop", "live")

	other := e.loggedIn(t, "other@example.com")
	wantError(t, e.do(t, request{method: "DELETE", path: "/api/v1/projects/" + p.ID, token: other}), http.StatusNotFound, codeNotFound)

	rec := e.do(t, request{method: "DELETE", path: "/api/v1/projects/" + p.ID, token: token})
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body)
	}
	wantError(t, e.do(t, request{method: "GET", path: "/api/v1/projects/" + p.ID, token: token}), http.StatusNotFound, codeNotFound)
	wantError(t, e.sendLogs(t, p.APIKey, validLog), http.StatusUnauthorized, codeInvalidAPIKey)
}

func TestLastUsedIsUpdatedAtMostOncePerMinute(t *testing.T) {
	t.Parallel()
	e := newTestEnv(t)
	token := e.loggedIn(t, testEmail)
	p := e.newProject(t, token, "shop", "live")

	for range 5 {
		e.sendLogs(t, p.APIKey, validLog)
	}
	if e.store.touches != 1 {
		t.Fatalf("touches = %d after 5 requests in the same minute, want 1", e.store.touches)
	}
	e.clock.Advance(time.Minute)
	e.sendLogs(t, p.APIKey, validLog)
	if e.store.touches != 2 {
		t.Errorf("touches = %d after a minute, want 2", e.store.touches)
	}

	rec := e.do(t, request{method: "GET", path: "/api/v1/projects/" + p.ID, token: token})
	var got projectResponse
	decodeBody(t, rec, &got)
	if got.LastUsedAt == nil || !got.LastUsedAt.Equal(e.clock.Now()) {
		t.Errorf("last_used_at = %v, want %s", got.LastUsedAt, e.clock.Now())
	}
}
