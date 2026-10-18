package api

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/auth"
	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

// projectNamePattern allows 1–64 characters: letters, digits, space, "_",
// "-" and ".", starting with a letter or digit.
var projectNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 _.-]{0,63}$`)

// createProjectRequest is the request body for creating a project.
type createProjectRequest struct {
	Name        string `json:"name"`
	Environment string `json:"environment"`
}

// projectResponse is how a project is shown to clients. APIKey is only
// filled in once, in the response to creating the project.
type projectResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Environment string    `json:"environment"`
	KeyPrefix   string    `json:"key_prefix"`
	CreatedAt   time.Time `json:"created_at"`
	APIKey      string    `json:"api_key,omitempty"`
}

func toProjectResponse(p model.Project) projectResponse {
	return projectResponse{
		ID:          p.ID.Hex(),
		Name:        p.Name,
		Environment: p.Environment,
		KeyPrefix:   p.KeyPrefix,
		CreatedAt:   p.CreatedAt,
	}
}

// handleCreateProject registers a project for the logged-in user and
// returns its API key. The key is shown only in this response; only its
// hash is stored, so it cannot be shown again.
// 201 on success · 400 bad input · 409 same name and environment exists.
func (s *server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := currentAuth(r).User

	var req createProjectRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	env := strings.ToLower(strings.TrimSpace(req.Environment))

	if !projectNamePattern.MatchString(name) {
		writeError(w, http.StatusBadRequest, codeValidationFailed,
			"name must be 1-64 characters: letters, digits, spaces, '_', '-' or '.', starting with a letter or digit")
		return
	}
	if !auth.ValidEnvironment(env) {
		writeError(w, http.StatusBadRequest, codeValidationFailed, `environment must be "live" or "test"`)
		return
	}

	key, err := auth.NewAPIKey(env)
	if err != nil {
		s.Logger.ErrorContext(ctx, "create project: new api key", "error", err)
		writeInternalError(w)
		return
	}

	p := model.Project{
		OwnerID:     user.ID,
		Name:        name,
		Environment: env,
		APIKeyHash:  key.Hash,
		KeyPrefix:   key.Prefix,
		CreatedAt:   s.now(),
	}
	err = s.Projects.CreateProject(ctx, &p)
	if errors.Is(err, model.ErrProjectExists) {
		writeError(w, http.StatusConflict, codeProjectExists, "you already have a project with this name in this environment")
		return
	}
	if err != nil {
		s.Logger.ErrorContext(ctx, "create project: insert", "error", err, "user_id", user.ID.Hex())
		writeInternalError(w)
		return
	}

	s.Logger.InfoContext(ctx, "project created", "project_id", p.ID.Hex(), "user_id", user.ID.Hex(), "environment", env)
	resp := toProjectResponse(p)
	resp.APIKey = key.Key
	w.Header().Set("Cache-Control", "no-store") // the response contains a secret
	writeJSON(w, http.StatusCreated, resp)
}

// listProjectsResponse is the body returned when listing projects.
type listProjectsResponse struct {
	Projects []projectResponse `json:"projects"`
}

// handleListProjects returns the logged-in user's projects, newest first.
// API keys are never included.
func (s *server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := currentAuth(r).User

	projects, err := s.Projects.ProjectsByOwner(ctx, user.ID)
	if err != nil {
		s.Logger.ErrorContext(ctx, "list projects", "error", err, "user_id", user.ID.Hex())
		writeInternalError(w)
		return
	}

	resp := listProjectsResponse{Projects: make([]projectResponse, 0, len(projects))}
	for _, p := range projects {
		resp.Projects = append(resp.Projects, toProjectResponse(p))
	}
	writeJSON(w, http.StatusOK, resp)
}
