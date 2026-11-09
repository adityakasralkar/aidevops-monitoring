package api

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

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
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Environment  string     `json:"environment"`
	KeyPrefix    string     `json:"key_prefix"`
	CreatedAt    time.Time  `json:"created_at"`
	KeyCreatedAt time.Time  `json:"key_created_at"`
	LastUsedAt   *time.Time `json:"last_used_at"` // null until data arrives
	APIKey       string     `json:"api_key,omitempty"`
}

func toProjectResponse(p model.Project) projectResponse {
	keyCreated := p.CreatedAt
	if p.KeyCreatedAt != nil {
		keyCreated = *p.KeyCreatedAt
	}
	return projectResponse{
		ID:           p.ID.Hex(),
		Name:         p.Name,
		Environment:  p.Environment,
		KeyPrefix:    p.KeyPrefix,
		CreatedAt:    p.CreatedAt,
		KeyCreatedAt: keyCreated,
		LastUsedAt:   p.LastUsedAt,
	}
}

// validProjectName reports whether name (already trimmed) is allowed.
func validProjectName(name string) bool {
	return projectNamePattern.MatchString(name)
}

const projectNameRule = "name must be 1-64 characters: letters, digits, spaces, '_', '-' or '.', starting with a letter or digit"

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

	if !validProjectName(name) {
		writeError(w, http.StatusBadRequest, codeValidationFailed, projectNameRule)
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

	now := s.now()
	p := model.Project{
		OwnerID:      user.ID,
		Name:         name,
		Environment:  env,
		APIKeyHash:   key.Hash,
		KeyPrefix:    key.Prefix,
		CreatedAt:    now,
		KeyCreatedAt: &now,
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

// ownedProject loads the project named by the {id} path value, if it
// belongs to the logged-in user. Otherwise it writes 404 and returns false:
// another user's project is reported exactly like a missing one.
func (s *server) ownedProject(w http.ResponseWriter, r *http.Request) (model.Project, bool) {
	id, err := bson.ObjectIDFromHex(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, codeNotFound, "project not found")
		return model.Project{}, false
	}
	user := currentAuth(r).User
	p, err := s.Projects.ProjectByID(r.Context(), user.ID, id)
	if errors.Is(err, model.ErrNotFound) {
		writeError(w, http.StatusNotFound, codeNotFound, "project not found")
		return model.Project{}, false
	}
	if err != nil {
		s.Logger.ErrorContext(r.Context(), "find project", "error", err, "user_id", user.ID.Hex())
		writeInternalError(w)
		return model.Project{}, false
	}
	return p, true
}

// handleGetProject returns one project. 200 · 404.
func (s *server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	p, ok := s.ownedProject(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, toProjectResponse(p))
}

// renameProjectRequest is the body for renaming a project.
type renameProjectRequest struct {
	Name string `json:"name"`
}

// handleRenameProject changes a project's name. The environment cannot be
// changed, because it is written into the API key.
// 200 · 400 bad name · 404 · 409 name already used in this environment.
func (s *server) handleRenameProject(w http.ResponseWriter, r *http.Request) {
	p, ok := s.ownedProject(w, r)
	if !ok {
		return
	}
	var req renameProjectRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if !validProjectName(name) {
		writeError(w, http.StatusBadRequest, codeValidationFailed, projectNameRule)
		return
	}

	ctx := r.Context()
	err := s.Projects.RenameProject(ctx, p.OwnerID, p.ID, name)
	switch {
	case errors.Is(err, model.ErrProjectExists):
		writeError(w, http.StatusConflict, codeProjectExists, "you already have a project with this name in this environment")
		return
	case errors.Is(err, model.ErrNotFound):
		writeError(w, http.StatusNotFound, codeNotFound, "project not found")
		return
	case err != nil:
		s.Logger.ErrorContext(ctx, "rename project", "error", err, "project_id", p.ID.Hex())
		writeInternalError(w)
		return
	}
	p.Name = name
	writeJSON(w, http.StatusOK, toProjectResponse(p))
}

// handleRotateAPIKey replaces the project's API key and returns the new one
// once. The old key stops working at once, so apps must be updated.
// 200 · 404.
func (s *server) handleRotateAPIKey(w http.ResponseWriter, r *http.Request) {
	p, ok := s.ownedProject(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	key, err := auth.NewAPIKey(p.Environment)
	if err != nil {
		s.Logger.ErrorContext(ctx, "rotate key: new api key", "error", err)
		writeInternalError(w)
		return
	}
	now := s.now()
	err = s.Projects.RotateAPIKey(ctx, p.OwnerID, p.ID, key.Hash, key.Prefix, now)
	if errors.Is(err, model.ErrNotFound) {
		writeError(w, http.StatusNotFound, codeNotFound, "project not found")
		return
	}
	if err != nil {
		s.Logger.ErrorContext(ctx, "rotate key", "error", err, "project_id", p.ID.Hex())
		writeInternalError(w)
		return
	}

	s.Logger.InfoContext(ctx, "api key rotated", "project_id", p.ID.Hex())
	p.KeyPrefix, p.KeyCreatedAt, p.LastUsedAt = key.Prefix, &now, nil
	resp := toProjectResponse(p)
	resp.APIKey = key.Key
	w.Header().Set("Cache-Control", "no-store") // the response contains a secret
	writeJSON(w, http.StatusOK, resp)
}

// handleDeleteProject removes the project, its API key and all its data.
// 204 · 404.
func (s *server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	p, ok := s.ownedProject(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	err := s.Projects.DeleteProject(ctx, p.OwnerID, p.ID)
	if errors.Is(err, model.ErrNotFound) {
		writeError(w, http.StatusNotFound, codeNotFound, "project not found")
		return
	}
	if err != nil {
		s.Logger.ErrorContext(ctx, "delete project", "error", err, "project_id", p.ID.Hex())
		writeInternalError(w)
		return
	}
	s.Logger.InfoContext(ctx, "project deleted", "project_id", p.ID.Hex())
	w.WriteHeader(http.StatusNoContent)
}
