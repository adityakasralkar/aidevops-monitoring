package api

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/auth"
)

func TestCreateProject(t *testing.T) {
	t.Parallel()

	setup := func(t *testing.T) (*testEnv, string) {
		e := newTestEnv(t)
		e.signup(t, testEmail, testPassword)
		return e, e.login(t, testEmail, testPassword)
	}
	create := func(e *testEnv, t *testing.T, token, body string) *httpRecorder {
		return e.do(t, request{method: "POST", path: "/api/v1/projects", token: token, body: body})
	}

	t.Run("returns the API key once and stores only its hash", func(t *testing.T) {
		e, token := setup(t)
		rec := create(e, t, token, `{"name":"  My Shop ","environment":" TEST "}`)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Error("missing Cache-Control: no-store")
		}
		var got projectResponse
		decodeBody(t, rec, &got)
		if got.Name != "My Shop" || got.Environment != "test" || got.ID == "" {
			t.Errorf("response = %+v", got)
		}
		if !regexp.MustCompile(`^aidv_test_[A-Za-z0-9_-]{43}$`).MatchString(got.APIKey) {
			t.Errorf("api_key %q has the wrong format", got.APIKey)
		}
		if !strings.HasPrefix(got.APIKey, got.KeyPrefix) {
			t.Errorf("key_prefix %q does not match the key", got.KeyPrefix)
		}

		stored := e.store.projects[0]
		if stored.APIKeyHash != auth.HashToken(got.APIKey) {
			t.Error("stored hash does not match the key")
		}
		if strings.Contains(stored.APIKeyHash, got.APIKey) || stored.KeyPrefix == got.APIKey {
			t.Error("plain API key stored")
		}
	})

	t.Run("allows the same name in a different environment", func(t *testing.T) {
		e, token := setup(t)
		if rec := create(e, t, token, `{"name":"shop","environment":"test"}`); rec.Code != http.StatusCreated {
			t.Fatalf("test: status %d", rec.Code)
		}
		if rec := create(e, t, token, `{"name":"shop","environment":"live"}`); rec.Code != http.StatusCreated {
			t.Errorf("live: status %d", rec.Code)
		}
	})

	t.Run("rejects the same name in the same environment", func(t *testing.T) {
		e, token := setup(t)
		create(e, t, token, `{"name":"shop","environment":"test"}`)
		wantError(t, create(e, t, token, `{"name":"shop","environment":"test"}`), http.StatusConflict, codeProjectExists)
	})

	t.Run("lets different users use the same name", func(t *testing.T) {
		e, token := setup(t)
		e.signup(t, "other@example.com", testPassword)
		other := e.login(t, "other@example.com", testPassword)
		create(e, t, token, `{"name":"shop","environment":"test"}`)
		if rec := create(e, t, other, `{"name":"shop","environment":"test"}`); rec.Code != http.StatusCreated {
			t.Errorf("status = %d", rec.Code)
		}
	})

	invalid := []struct{ name, body string }{
		{"empty name", `{"name":"","environment":"test"}`},
		{"spaces-only name", `{"name":"   ","environment":"test"}`},
		{"name starting with a symbol", `{"name":"-shop","environment":"test"}`},
		{"name with a slash", `{"name":"a/b","environment":"test"}`},
		{"name over 64 characters", `{"name":"` + strings.Repeat("a", 65) + `","environment":"test"}`},
		{"unknown environment", `{"name":"shop","environment":"production"}`},
		{"missing environment", `{"name":"shop"}`},
	}
	for _, tt := range invalid {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			e, token := setup(t)
			wantError(t, create(e, t, token, tt.body), http.StatusBadRequest, codeValidationFailed)
		})
	}

	t.Run("requires login", func(t *testing.T) {
		e := newTestEnv(t)
		wantError(t, create(e, t, "", `{"name":"shop","environment":"test"}`), http.StatusUnauthorized, codeMissingToken)
	})

	t.Run("returns 500 when the store fails", func(t *testing.T) {
		e, token := setup(t)
		// Fail only project writes, so authentication still succeeds.
		h := NewRouter(Deps{Logger: discardLogger(), DB: e.store, Users: e.store, Sessions: e.store, Projects: failingProjects{}, Now: e.clock.Now})
		rec := (&testEnv{h: h}).do(t, request{method: "POST", path: "/api/v1/projects", token: token, body: `{"name":"shop","environment":"test"}`})
		wantError(t, rec, http.StatusInternalServerError, codeInternalError)
	})
}

func TestListProjects(t *testing.T) {
	t.Parallel()

	t.Run("lists only the user's own projects, newest first, without keys", func(t *testing.T) {
		e := newTestEnv(t)
		e.signup(t, testEmail, testPassword)
		e.signup(t, "other@example.com", testPassword)
		mine := e.login(t, testEmail, testPassword)
		theirs := e.login(t, "other@example.com", testPassword)

		for _, body := range []string{`{"name":"first","environment":"test"}`, `{"name":"second","environment":"live"}`} {
			e.do(t, request{method: "POST", path: "/api/v1/projects", token: mine, body: body})
		}
		e.do(t, request{method: "POST", path: "/api/v1/projects", token: theirs, body: `{"name":"secret-project","environment":"test"}`})

		rec := e.do(t, request{method: "GET", path: "/api/v1/projects", token: mine})
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		var got listProjectsResponse
		decodeBody(t, rec, &got)
		if len(got.Projects) != 2 || got.Projects[0].Name != "second" || got.Projects[1].Name != "first" {
			t.Errorf("projects = %+v", got.Projects)
		}
		body := rec.Body.String()
		if strings.Contains(body, "secret-project") {
			t.Error("another user's project is visible")
		}
		if strings.Contains(body, "api_key\"") || strings.Contains(body, "api_key_hash") {
			t.Errorf("list exposes keys: %s", body)
		}
	})

	t.Run("returns an empty JSON array, not null, when there are no projects", func(t *testing.T) {
		e := newTestEnv(t)
		e.signup(t, testEmail, testPassword)
		token := e.login(t, testEmail, testPassword)
		rec := e.do(t, request{method: "GET", path: "/api/v1/projects", token: token})
		if strings.TrimSpace(rec.Body.String()) != `{"projects":[]}` {
			t.Errorf("body = %s", rec.Body)
		}
	})

	t.Run("requires login", func(t *testing.T) {
		e := newTestEnv(t)
		wantError(t, e.do(t, request{method: "GET", path: "/api/v1/projects"}), http.StatusUnauthorized, codeMissingToken)
	})

	t.Run("returns 500 when the store fails", func(t *testing.T) {
		e := newTestEnv(t)
		e.signup(t, testEmail, testPassword)
		token := e.login(t, testEmail, testPassword)
		h := NewRouter(Deps{Logger: discardLogger(), DB: e.store, Users: e.store, Sessions: e.store, Projects: failingProjects{}, Now: e.clock.Now})
		rec := (&testEnv{h: h}).do(t, request{method: "GET", path: "/api/v1/projects", token: token})
		wantError(t, rec, http.StatusInternalServerError, codeInternalError)
	})
}
