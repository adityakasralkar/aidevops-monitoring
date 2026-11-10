package api

import (
	"cmp"
	"net/http"
	"slices"
	"time"

	"github.com/adityakasralkar/aidevops-monitoring/server/internal/model"
)

// maxPipelineRuns is how many runs one response lists, newest first.
const maxPipelineRuns = 100

// pipelineStep is one failed step of a run.
type pipelineStep struct {
	Name     string `json:"name"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Output   string `json:"output,omitempty"` // the end of the step's output
	LogID    string `json:"log_id"`
}

// pipelineRun is one CI run, rebuilt from the logs the pipeline sent.
type pipelineRun struct {
	RunID         string         `json:"run_id"`
	Pipeline      string         `json:"pipeline"`
	URL           string         `json:"url,omitempty"`
	Repository    string         `json:"repository,omitempty"`
	Branch        string         `json:"branch,omitempty"`
	CommitSHA     string         `json:"commit_sha,omitempty"`
	CommitMessage string         `json:"commit_message,omitempty"`
	Author        string         `json:"author,omitempty"`
	Service       string         `json:"service"`
	At            time.Time      `json:"at"`
	Result        string         `json:"result"` // "passed" or "failed"
	FailedSteps   []pipelineStep `json:"failed_steps"`
}

// attrString reads a string attribute; other types count as missing.
func attrString(attrs map[string]any, key string) string {
	v, _ := attrs[key].(string)
	return v
}

// groupRuns turns CI logs into runs. A run reports one INFO log when it
// passes, or one ERROR / CRITICAL log per failed step. Runs are newest first.
func groupRuns(logs []model.LogRecord) []pipelineRun {
	byID := map[string]*pipelineRun{}
	var order []*pipelineRun
	for _, l := range logs {
		id := attrString(l.Attributes, "ci.run.id")
		if id == "" {
			continue
		}
		run, ok := byID[id]
		if !ok {
			run = &pipelineRun{
				RunID:         id,
				Pipeline:      attrString(l.Attributes, "ci.pipeline"),
				URL:           attrString(l.Attributes, "ci.run.url"),
				Repository:    attrString(l.Attributes, "vcs.repository"),
				Branch:        attrString(l.Attributes, "vcs.branch"),
				CommitSHA:     attrString(l.Attributes, "vcs.commit.sha"),
				CommitMessage: attrString(l.Attributes, "vcs.commit.message"),
				Author:        attrString(l.Attributes, "vcs.commit.author"),
				Service:       l.ServiceName,
				Result:        "passed",
				FailedSteps:   []pipelineStep{},
			}
			byID[id] = run
			order = append(order, run)
		}
		if l.Timestamp.After(run.At) {
			run.At = l.Timestamp
		}
		if model.IsErrorSeverity(l.Severity) {
			run.Result = "failed"
			run.FailedSteps = append(run.FailedSteps, pipelineStep{
				Name:     cmp.Or(attrString(l.Attributes, "ci.failed_step"), "Unknown step"),
				Severity: l.Severity,
				Message:  l.Message,
				Output:   l.StackTrace,
				LogID:    l.ID.Hex(),
			})
		}
	}

	runs := make([]pipelineRun, 0, len(order))
	for _, r := range order {
		// Steps in the order they ran isn't known; keep the most serious first.
		slices.SortStableFunc(r.FailedSteps, func(a, b pipelineStep) int {
			return cmp.Compare(severityRank(b.Severity), severityRank(a.Severity))
		})
		runs = append(runs, *r)
	}
	slices.SortStableFunc(runs, func(a, b pipelineRun) int { return b.At.Compare(a.At) })
	if len(runs) > maxPipelineRuns {
		runs = runs[:maxPipelineRuns]
	}
	return runs
}

func severityRank(s string) int {
	if s == model.SeverityCritical {
		return 2
	}
	if s == model.SeverityError {
		return 1
	}
	return 0
}

// handlePipelines lists the project's CI runs (default range 7d): which
// pushes passed, and for failed ones the step, test and output that failed.
func (s *server) handlePipelines(w http.ResponseWriter, r *http.Request) {
	p, ok := s.ownedProject(w, r)
	if !ok {
		return
	}
	win, ok := s.parseRange(w, r, "7d")
	if !ok {
		return
	}
	logs, err := s.Reader.CILogs(r.Context(), p.ID, win.since)
	if err != nil {
		s.readFailed(w, r, "list pipeline runs", err, p)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"range": win.name, "runs": groupRuns(logs)})
}
