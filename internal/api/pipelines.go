package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/pipeline"
	"github.com/GLINCKER/levelrail/internal/store"
)

// PipelineStore is the store surface the pipeline handlers need.
type PipelineStore interface {
	SavePipeline(ctx context.Context, p store.Pipeline) (store.Pipeline, error)
	GetPipelineByName(ctx context.Context, app, name string) (store.Pipeline, error)
	ListPipelines(ctx context.Context, app string) ([]store.Pipeline, error)
	DeletePipeline(ctx context.Context, id string) error
	ListPipelineRuns(ctx context.Context, app, pipelineID string, limit int) ([]store.PipelineRun, error)
	GetPipelineRun(ctx context.Context, id string) (store.PipelineRun, error)
	ListPipelineJobs(ctx context.Context, runID string) ([]store.PipelineJob, error)
	ListPipelineLogs(ctx context.Context, runID, jobKey string, afterID int64, limit int) ([]store.PipelineLogLine, error)
	ListPipelineApprovals(ctx context.Context, runID string) ([]store.PipelineApproval, error)
	DecidePipelineApproval(ctx context.Context, id int64, decision, by, comment string, now time.Time) (bool, error)
	ListPipelineTriggerLog(ctx context.Context, app string, limit int) ([]store.PipelineTriggerLog, error)
}

// PipelineRunner starts, cancels, and re-runs pipeline runs. *pipeline.Engine
// satisfies it.
type PipelineRunner interface {
	Start(ctx context.Context, p store.Pipeline, opt pipeline.StartOptions) (store.PipelineRun, error)
	Cancel(ctx context.Context, runID string) error
	Rerun(ctx context.Context, runID, actor string) (store.PipelineRun, error)
	// DecideHold releases or rejects a run held for approval.
	DecideHold(ctx context.Context, runID string, approved bool, by string) (bool, error)
	Nudge()
}

const pipelineBodyLimit = 256 * 1024

type pipelineResource struct {
	ID        string    `json:"id"`
	App       string    `json:"app"`
	Name      string    `json:"name"`
	Source    string    `json:"source"`
	Enabled   bool      `json:"enabled"`
	YAML      string    `json:"yaml,omitempty"`
	Triggers  []string  `json:"triggers,omitempty"`
	Jobs      int       `json:"jobs"`
	LastRun   *runBrief `json:"last_run,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// SourceSHA is the commit a repo-sourced definition was synced from, and
	// Diverged is true when it was edited after that sync.
	SourceSHA string `json:"source_sha,omitempty"`
	Diverged  bool   `json:"diverged,omitempty"`
}

type runBrief struct {
	ID     string `json:"id"`
	Number int    `json:"number"`
	Status string `json:"status"`
}

type pipelineRunResource struct {
	ID           string             `json:"id"`
	PipelineID   string             `json:"pipeline_id"`
	PipelineName string             `json:"pipeline_name"`
	App          string             `json:"app"`
	Number       int                `json:"number"`
	Trigger      string             `json:"trigger"`
	Actor        string             `json:"actor,omitempty"`
	Ref          string             `json:"ref,omitempty"`
	CommitSHA    string             `json:"commit_sha,omitempty"`
	Inputs       map[string]string  `json:"inputs,omitempty"`
	Status       string             `json:"status"`
	Reason       string             `json:"reason,omitempty"`
	CreatedAt    time.Time          `json:"created_at"`
	StartedAt    *time.Time         `json:"started_at,omitempty"`
	FinishedAt   *time.Time         `json:"finished_at,omitempty"`
	Jobs         []pipelineJobView  `json:"jobs,omitempty"`
	Approvals    []pipelineApproval `json:"approvals,omitempty"`
	Issues       []pipeline.Issue   `json:"issues,omitempty"`
	Hold         *pipelineHold      `json:"hold,omitempty"`
}

// pipelineHold is a run held for approval, such as a pull request from a fork.
type pipelineHold struct {
	State  string     `json:"state"`
	Reason string     `json:"reason"`
	By     string     `json:"by,omitempty"`
	At     *time.Time `json:"at,omitempty"`
}

type pipelineJobView struct {
	Key        string             `json:"key"`
	Name       string             `json:"name"`
	Stage      string             `json:"stage,omitempty"`
	Needs      []string           `json:"needs"`
	Matrix     map[string]string  `json:"matrix,omitempty"`
	NodeID     string             `json:"node_id,omitempty"`
	Status     string             `json:"status"`
	Reason     string             `json:"reason,omitempty"`
	Attempt    int                `json:"attempt"`
	Outputs    map[string]string  `json:"outputs,omitempty"`
	StartedAt  *time.Time         `json:"started_at,omitempty"`
	FinishedAt *time.Time         `json:"finished_at,omitempty"`
	Steps      []pipelineStepView `json:"steps"`
}

type pipelineStepView struct {
	Index      int        `json:"index"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind"`
	Status     string     `json:"status"`
	Reason     string     `json:"reason,omitempty"`
	ExitCode   *int       `json:"exit_code,omitempty"`
	Attempt    int        `json:"attempt"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type pipelineApproval struct {
	ID              int64      `json:"id"`
	Job             string     `json:"job"`
	StepIndex       int        `json:"step_index"`
	Message         string     `json:"message"`
	RequiredAbility string     `json:"required_ability"`
	Decision        string     `json:"decision,omitempty"`
	DecidedBy       string     `json:"decided_by,omitempty"`
	Comment         string     `json:"comment,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	DecidedAt       *time.Time `json:"decided_at,omitempty"`
}

type pipelineLogView struct {
	ID     int64     `json:"id"`
	Job    string    `json:"job"`
	Step   int       `json:"step"`
	Stream string    `json:"stream"`
	Line   string    `json:"line"`
	Time   time.Time `json:"time"`
}

func (rt *Router) pipelinesReady(w http.ResponseWriter) bool {
	if rt.pipelineStore == nil {
		writeError(w, http.StatusNotImplemented, "pipelines are not configured on this control plane")
		return false
	}
	return true
}

func (rt *Router) runnerReady(w http.ResponseWriter) bool {
	if !rt.pipelinesReady(w) {
		return false
	}
	if rt.pipelineRunner == nil {
		writeError(w, http.StatusNotImplemented, "the pipeline engine is not running on this control plane")
		return false
	}
	return true
}

var pipelineNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

func newPipelineID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (rt *Router) writePipelineIssues(w http.ResponseWriter, issues []pipeline.Issue) {
	writeJSON(w, http.StatusBadRequest, map[string]any{"error": "pipeline definition is invalid", "issues": issues})
}

func (rt *Router) pipelineActor(r *http.Request) string {
	if id, ok := rt.currentSessionUserID(r); ok {
		if u, err := rt.auth.GetUserByID(r.Context(), id); err == nil && u.DisplayName != "" {
			return u.DisplayName
		}
		return "user:" + id
	}
	if tok, ok := bearerToken(r); ok {
		if rec, err := rt.tokens.GetAPITokenByHash(r.Context(), hashToken(tok)); err == nil {
			return "token:" + rec.Name
		}
	}
	return "unknown"
}

func toPipelineResource(p store.Pipeline, withYAML bool) pipelineResource {
	res := pipelineResource{ID: p.ID, App: p.AppName, Name: p.Name, Source: p.Source, Enabled: p.Enabled, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
		SourceSHA: p.SourceSHA, Diverged: pipeline.Diverged(p)}
	if withYAML {
		res.YAML = p.YAML
	}
	if def, issues := pipeline.Validate([]byte(p.YAML)); len(issues) == 0 {
		res.Jobs = len(def.Jobs)
		res.Triggers = triggerNames(def.On)
	}
	return res
}

func triggerNames(t pipeline.Triggers) []string {
	var out []string
	if t.Push != nil {
		out = append(out, pipeline.TriggerPush)
	}
	if t.PullRequest != nil {
		out = append(out, pipeline.TriggerPullRequest)
	}
	if t.Tag != nil {
		out = append(out, pipeline.TriggerTag)
	}
	if t.Manual != nil {
		out = append(out, pipeline.TriggerManual)
	}
	if len(t.Schedule) > 0 {
		out = append(out, pipeline.TriggerSchedule)
	}
	if t.API {
		out = append(out, pipeline.TriggerAPI)
	}
	return out
}

func toRunResource(r store.PipelineRun, pipelineName string) pipelineRunResource {
	res := pipelineRunResource{
		ID: r.ID, PipelineID: r.PipelineID, PipelineName: pipelineName, App: r.AppName, Number: r.Number, Trigger: r.TriggerKind,
		Actor: r.TriggerActor, Ref: r.Ref, CommitSHA: r.CommitSHA, Status: r.Status, Reason: r.Reason,
		CreatedAt: r.CreatedAt, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt,
	}
	_ = json.Unmarshal([]byte(r.InputsJSON), &res.Inputs)
	if r.HoldState != "" {
		res.Hold = &pipelineHold{State: r.HoldState, Reason: r.HoldReason, By: r.HoldBy, At: r.HoldAt}
	}
	return res
}

func toJobView(j store.PipelineJob) pipelineJobView {
	v := pipelineJobView{Key: j.Key, Name: j.DisplayName, Stage: j.Stage, NodeID: j.NodeID, Status: j.Status, Reason: j.Reason,
		Attempt: j.Attempt, StartedAt: j.StartedAt, FinishedAt: j.FinishedAt, Steps: make([]pipelineStepView, 0, len(j.Steps))}
	_ = json.Unmarshal([]byte(j.NeedsJSON), &v.Needs)
	_ = json.Unmarshal([]byte(j.MatrixJSON), &v.Matrix)
	_ = json.Unmarshal([]byte(j.OutputsJSON), &v.Outputs)
	if v.Needs == nil {
		v.Needs = []string{}
	}
	for _, s := range j.Steps {
		v.Steps = append(v.Steps, pipelineStepView{Index: s.Index, Name: s.Name, Kind: s.Kind, Status: s.Status, Reason: s.Reason,
			ExitCode: s.ExitCode, Attempt: s.Attempt, StartedAt: s.StartedAt, FinishedAt: s.FinishedAt})
	}
	return v
}

func (rt *Router) loadPipeline(w http.ResponseWriter, r *http.Request) (store.Pipeline, bool) {
	p, err := rt.pipelineStore.GetPipelineByName(r.Context(), r.PathValue("name"), r.PathValue("pname"))
	if errors.Is(err, store.ErrPipelineNotFound) {
		writeError(w, http.StatusNotFound, "pipeline not found")
		return store.Pipeline{}, false
	}
	if err != nil {
		rt.internalError(w, "api: load pipeline failed", err)
		return store.Pipeline{}, false
	}
	return p, true
}

// loadOwnedPipelineRun loads a run and verifies it belongs to the app in the
// URL, so an ID from another app never leaks through this app's routes.
func (rt *Router) loadOwnedPipelineRun(w http.ResponseWriter, r *http.Request) (store.PipelineRun, bool) {
	run, err := rt.pipelineStore.GetPipelineRun(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrPipelineRunNotFound) || (err == nil && run.AppName != r.PathValue("name")) {
		writeError(w, http.StatusNotFound, "pipeline run not found")
		return store.PipelineRun{}, false
	}
	if err != nil {
		rt.internalError(w, "api: load pipeline run failed", err)
		return store.PipelineRun{}, false
	}
	return run, true
}

func (rt *Router) requireApp(w http.ResponseWriter, r *http.Request) bool {
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return false
	} else if err != nil {
		rt.internalError(w, "api: pipelines: load app failed", err, slog.String("name", name))
		return false
	}
	return true
}

type pipelineSaveRequest struct {
	Name    string `json:"name"`
	YAML    string `json:"yaml"`
	Enabled *bool  `json:"enabled"`
}

func (rt *Router) decodePipelineSave(w http.ResponseWriter, r *http.Request) (pipelineSaveRequest, *pipeline.Definition, bool) {
	var req pipelineSaveRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, pipelineBodyLimit)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return req, nil, false
	}
	def, issues := pipeline.Validate([]byte(req.YAML))
	if len(issues) > 0 {
		rt.writePipelineIssues(w, issues)
		return req, nil, false
	}
	if targets := pipeline.TargetApps(def, r.PathValue("name")); len(targets) > 0 && !rt.callerHasAbility(r, AbilityRoot) {
		writeError(w, http.StatusForbidden, fmt.Sprintf("steps that act on other apps (%v) require the root ability to save", targets))
		return req, nil, false
	}
	return req, def, true
}

// handleCreatePipeline handles POST /api/v1/apps/{name}/pipelines.
func (rt *Router) handleCreatePipeline(w http.ResponseWriter, r *http.Request) {
	if !rt.pipelinesReady(w) || !rt.requireApp(w, r) {
		return
	}
	req, def, ok := rt.decodePipelineSave(w, r)
	if !ok {
		return
	}
	name := req.Name
	if name == "" {
		name = def.Name
	}
	if !pipelineNameRe.MatchString(name) {
		writeError(w, http.StatusBadRequest, "name must be 1-64 characters: letters, digits, dash, underscore")
		return
	}
	app := r.PathValue("name")
	if _, err := rt.pipelineStore.GetPipelineByName(r.Context(), app, name); err == nil {
		writeError(w, http.StatusConflict, "a pipeline with this name already exists")
		return
	} else if !errors.Is(err, store.ErrPipelineNotFound) {
		rt.internalError(w, "api: create pipeline: lookup failed", err)
		return
	}
	enabled := req.Enabled == nil || *req.Enabled
	now := time.Now().UTC()
	saved, err := rt.pipelineStore.SavePipeline(r.Context(), store.Pipeline{ID: newPipelineID(), AppName: app, Name: name, YAML: req.YAML, Enabled: enabled, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		rt.internalError(w, "api: create pipeline failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, toPipelineResource(saved, true))
}

// handleUpdatePipeline handles PUT /api/v1/apps/{name}/pipelines/{pname}.
func (rt *Router) handleUpdatePipeline(w http.ResponseWriter, r *http.Request) {
	if !rt.pipelinesReady(w) {
		return
	}
	existing, ok := rt.loadPipeline(w, r)
	if !ok {
		return
	}
	req, _, ok := rt.decodePipelineSave(w, r)
	if !ok {
		return
	}
	existing.YAML = req.YAML
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	existing.UpdatedAt = time.Now().UTC()
	saved, err := rt.pipelineStore.SavePipeline(r.Context(), existing)
	if err != nil {
		rt.internalError(w, "api: update pipeline failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toPipelineResource(saved, true))
}

// handleListPipelines handles GET /api/v1/apps/{name}/pipelines.
func (rt *Router) handleListPipelines(w http.ResponseWriter, r *http.Request) {
	if !rt.pipelinesReady(w) || !rt.requireApp(w, r) {
		return
	}
	app := r.PathValue("name")
	pipes, err := rt.pipelineStore.ListPipelines(r.Context(), app)
	if err != nil {
		rt.internalError(w, "api: list pipelines failed", err)
		return
	}
	runs, err := rt.pipelineStore.ListPipelineRuns(r.Context(), app, "", 200)
	if err != nil {
		rt.internalError(w, "api: list pipeline runs failed", err)
		return
	}
	last := map[string]*runBrief{}
	for _, run := range runs {
		if _, seen := last[run.PipelineID]; !seen {
			last[run.PipelineID] = &runBrief{ID: run.ID, Number: run.Number, Status: run.Status}
		}
	}
	out := make([]pipelineResource, 0, len(pipes))
	for _, p := range pipes {
		res := toPipelineResource(p, false)
		res.LastRun = last[p.ID]
		out = append(out, res)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetPipeline handles GET /api/v1/apps/{name}/pipelines/{pname}.
func (rt *Router) handleGetPipeline(w http.ResponseWriter, r *http.Request) {
	if !rt.pipelinesReady(w) {
		return
	}
	if p, ok := rt.loadPipeline(w, r); ok {
		writeJSON(w, http.StatusOK, toPipelineResource(p, true))
	}
}

// handleDeletePipeline handles DELETE /api/v1/apps/{name}/pipelines/{pname}.
func (rt *Router) handleDeletePipeline(w http.ResponseWriter, r *http.Request) {
	if !rt.pipelinesReady(w) {
		return
	}
	p, ok := rt.loadPipeline(w, r)
	if !ok {
		return
	}
	if err := rt.pipelineStore.DeletePipeline(r.Context(), p.ID); err != nil {
		rt.internalError(w, "api: delete pipeline failed", err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// handleValidatePipeline handles POST /api/v1/pipelines/validate.
func (rt *Router) handleValidatePipeline(w http.ResponseWriter, r *http.Request) {
	var req pipelineSaveRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, pipelineBodyLimit)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	def, issues := pipeline.Validate([]byte(req.YAML))
	resp := map[string]any{"valid": len(issues) == 0, "issues": issues}
	if issues == nil {
		resp["issues"] = []pipeline.Issue{}
	}
	if def != nil {
		resp["triggers"] = triggerNames(def.On)
		resp["jobs"] = len(def.Jobs)
	}
	writeJSON(w, http.StatusOK, resp)
}

// handlePipelineSchema handles GET /api/v1/pipelines/schema.
func (rt *Router) handlePipelineSchema(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/schema+json")
	_, _ = w.Write(pipeline.SchemaJSON())
}

type startPipelineRequest struct {
	Ref    string            `json:"ref"`
	SHA    string            `json:"sha"`
	Inputs map[string]string `json:"inputs"`
}

// handleStartPipelineRun handles POST /api/v1/apps/{name}/pipelines/{pname}/runs.
func (rt *Router) handleStartPipelineRun(w http.ResponseWriter, r *http.Request) {
	if !rt.runnerReady(w) {
		return
	}
	p, ok := rt.loadPipeline(w, r)
	if !ok {
		return
	}
	var req startPipelineRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, pipelineBodyLimit)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !p.Enabled {
		writeError(w, http.StatusConflict, "pipeline is disabled")
		return
	}
	run, err := rt.pipelineRunner.Start(r.Context(), p, pipeline.StartOptions{Trigger: pipelineTrigger(r), Actor: rt.pipelineActor(r), Ref: req.Ref, SHA: req.SHA, Inputs: req.Inputs})
	if errors.Is(err, pipeline.ErrInvalidInput) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, toRunResource(run, p.Name))
}

func pipelineTrigger(r *http.Request) string {
	if _, ok := bearerToken(r); ok {
		return pipeline.TriggerAPI
	}
	return pipeline.TriggerManual
}

// handleListPipelineRuns handles GET /api/v1/apps/{name}/pipeline-runs.
func (rt *Router) handleListPipelineRuns(w http.ResponseWriter, r *http.Request) {
	if !rt.pipelinesReady(w) || !rt.requireApp(w, r) {
		return
	}
	app := r.PathValue("name")
	pipes, err := rt.pipelineStore.ListPipelines(r.Context(), app)
	if err != nil {
		rt.internalError(w, "api: list pipelines failed", err)
		return
	}
	names := map[string]string{}
	filter := ""
	for _, p := range pipes {
		names[p.ID] = p.Name
		if p.Name == r.URL.Query().Get("pipeline") {
			filter = p.ID
		}
	}
	if q := r.URL.Query().Get("pipeline"); q != "" && filter == "" {
		writeJSON(w, http.StatusOK, []pipelineRunResource{})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	runs, err := rt.pipelineStore.ListPipelineRuns(r.Context(), app, filter, limit)
	if err != nil {
		rt.internalError(w, "api: list pipeline runs failed", err)
		return
	}
	out := make([]pipelineRunResource, 0, len(runs))
	for _, run := range runs {
		out = append(out, toRunResource(run, names[run.PipelineID]))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetPipelineRun handles GET /api/v1/apps/{name}/pipeline-runs/{id}.
func (rt *Router) handleGetPipelineRun(w http.ResponseWriter, r *http.Request) {
	if !rt.pipelinesReady(w) {
		return
	}
	run, ok := rt.loadOwnedPipelineRun(w, r)
	if !ok {
		return
	}
	jobs, err := rt.pipelineStore.ListPipelineJobs(r.Context(), run.ID)
	if err != nil {
		rt.internalError(w, "api: list pipeline jobs failed", err)
		return
	}
	approvals, err := rt.pipelineStore.ListPipelineApprovals(r.Context(), run.ID)
	if err != nil {
		rt.internalError(w, "api: list pipeline approvals failed", err)
		return
	}
	res := toRunResource(run, "")
	if def, perr := pipeline.Parse([]byte(run.Definition)); perr == nil {
		res.PipelineName = def.Name
	}
	keyByID := map[string]string{}
	for _, j := range jobs {
		keyByID[j.ID] = j.Key
		res.Jobs = append(res.Jobs, toJobView(j))
	}
	for _, a := range approvals {
		res.Approvals = append(res.Approvals, pipelineApproval{ID: a.ID, Job: keyByID[a.JobID], StepIndex: a.StepIndex, Message: a.Message,
			RequiredAbility: a.RequiredAbility, Decision: a.Decision, DecidedBy: a.DecidedBy, Comment: a.Comment,
			ExpiresAt: a.ExpiresAt, CreatedAt: a.CreatedAt, DecidedAt: a.DecidedAt})
	}
	writeJSON(w, http.StatusOK, res)
}

// handleListPipelineRunLogs handles GET .../pipeline-runs/{id}/logs.
func (rt *Router) handleListPipelineRunLogs(w http.ResponseWriter, r *http.Request) {
	if !rt.pipelinesReady(w) {
		return
	}
	run, ok := rt.loadOwnedPipelineRun(w, r)
	if !ok {
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	lines, err := rt.pipelineStore.ListPipelineLogs(r.Context(), run.ID, r.URL.Query().Get("job"), after, limit)
	if err != nil {
		rt.internalError(w, "api: list pipeline logs failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toLogViews(lines))
}

func toLogViews(lines []store.PipelineLogLine) []pipelineLogView {
	out := make([]pipelineLogView, 0, len(lines))
	for _, l := range lines {
		out = append(out, pipelineLogView{ID: l.ID, Job: l.JobKey, Step: l.StepIndex, Stream: l.Stream, Line: l.Line, Time: l.CreatedAt})
	}
	return out
}

const pipelineStreamPoll = 500 * time.Millisecond

// handleStreamPipelineRunLogs handles GET .../pipeline-runs/{id}/logs/stream:
// SSE that replays stored lines after Last-Event-ID (or ?after) and then
// follows new ones until the run finishes. Polling the store keeps the
// stream restart-safe and needs no in-memory fan-out.
func (rt *Router) handleStreamPipelineRunLogs(w http.ResponseWriter, r *http.Request) {
	if !rt.pipelinesReady(w) {
		return
	}
	run, ok := rt.loadOwnedPipelineRun(w, r)
	if !ok {
		return
	}
	after, _ := strconv.ParseInt(firstNonEmptyStr(r.Header.Get("Last-Event-ID"), r.URL.Query().Get("after")), 10, 64)
	job := r.URL.Query().Get("job")
	flusher, ok := startSSE(w)
	if !ok {
		return
	}
	tick := time.NewTicker(pipelineStreamPoll)
	defer tick.Stop()
	for {
		lines, err := rt.pipelineStore.ListPipelineLogs(r.Context(), run.ID, job, after, 500)
		if err != nil {
			rt.logger.Warn("api: stream pipeline logs failed", slog.String("error", err.Error()), slog.String("run_id", run.ID))
			return
		}
		for _, v := range toLogViews(lines) {
			data, _ := json.Marshal(v)
			_, _ = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", v.ID, data)
			after = v.ID
		}
		if len(lines) > 0 {
			flusher.Flush()
			continue
		}
		cur, err := rt.pipelineStore.GetPipelineRun(r.Context(), run.ID)
		if err != nil {
			return
		}
		if store.IsPipelineTerminal(cur.Status) {
			_, _ = fmt.Fprintf(w, "event: end\ndata: {\"status\":%q}\n\n", cur.Status)
			flusher.Flush()
			return
		}
		_, _ = fmt.Fprint(w, ": keepalive\n\n")
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
		}
	}
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// handleCancelPipelineRun handles POST .../pipeline-runs/{id}/cancel.
func (rt *Router) handleCancelPipelineRun(w http.ResponseWriter, r *http.Request) {
	if !rt.runnerReady(w) {
		return
	}
	run, ok := rt.loadOwnedPipelineRun(w, r)
	if !ok {
		return
	}
	if store.IsPipelineTerminal(run.Status) {
		writeError(w, http.StatusConflict, "run already finished")
		return
	}
	if err := rt.pipelineRunner.Cancel(r.Context(), run.ID); err != nil {
		rt.internalError(w, "api: cancel pipeline run failed", err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "cancelling"})
}

// handleRerunPipelineRun handles POST .../pipeline-runs/{id}/rerun.
func (rt *Router) handleRerunPipelineRun(w http.ResponseWriter, r *http.Request) {
	if !rt.runnerReady(w) {
		return
	}
	old, ok := rt.loadOwnedPipelineRun(w, r)
	if !ok {
		return
	}
	run, err := rt.pipelineRunner.Rerun(r.Context(), old.ID, rt.pipelineActor(r))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, toRunResource(run, ""))
}

type approvalDecisionRequest struct {
	Decision string `json:"decision"`
	Comment  string `json:"comment"`
}

// handleDecidePipelineApproval handles POST
// .../pipeline-runs/{id}/approvals/{approval}. The route is gated at deploy;
// the gate's own required ability is enforced here.
func (rt *Router) handleDecidePipelineApproval(w http.ResponseWriter, r *http.Request) {
	if !rt.pipelinesReady(w) {
		return
	}
	run, ok := rt.loadOwnedPipelineRun(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("approval"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid approval id")
		return
	}
	var req approvalDecisionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, pipelineBodyLimit)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Decision != "approved" && req.Decision != "rejected" {
		writeError(w, http.StatusBadRequest, `decision must be "approved" or "rejected"`)
		return
	}
	approvals, err := rt.pipelineStore.ListPipelineApprovals(r.Context(), run.ID)
	if err != nil {
		rt.internalError(w, "api: list pipeline approvals failed", err)
		return
	}
	for _, a := range approvals {
		if a.ID != id {
			continue
		}
		if !rt.callerHasAbility(r, a.RequiredAbility) {
			writeError(w, http.StatusForbidden, "this gate requires the "+a.RequiredAbility+" ability")
			return
		}
		applied, err := rt.pipelineStore.DecidePipelineApproval(r.Context(), id, req.Decision, rt.pipelineActor(r), req.Comment, time.Now().UTC())
		if err != nil {
			rt.internalError(w, "api: decide pipeline approval failed", err)
			return
		}
		if !applied {
			writeError(w, http.StatusConflict, "approval already decided")
			return
		}
		if rt.pipelineRunner != nil {
			rt.pipelineRunner.Nudge()
		}
		writeJSON(w, http.StatusOK, map[string]string{"decision": req.Decision})
		return
	}
	writeError(w, http.StatusNotFound, "approval not found")
}
