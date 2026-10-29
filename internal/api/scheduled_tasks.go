package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/cronexpr"
)

// ScheduledTaskStore is the store surface the scheduled job handlers need.
type ScheduledTaskStore interface {
	SaveScheduledTask(ctx context.Context, t store.ScheduledTask) error
	GetScheduledTask(ctx context.Context, id string) (store.ScheduledTask, error)
	ListScheduledTasksForService(ctx context.Context, serviceName string) ([]store.ScheduledTask, error)
	UpdateScheduledTask(ctx context.Context, id string, command []string, schedule string, enabled bool, concurrencyPolicy string, updatedAt time.Time) error
	UpdateScheduledTaskConfig(ctx context.Context, t store.ScheduledTask, now time.Time) error
	CountScheduledTasksForService(ctx context.Context, serviceName string) (int, error)
	DeleteScheduledTask(ctx context.Context, id string) error
	ListScheduledTaskRuns(ctx context.Context, f store.ScheduledTaskRunFilter) ([]store.ScheduledTaskRun, error)
	GetScheduledTaskRun(ctx context.Context, id string) (store.ScheduledTaskRun, error)
}

// ScheduledTaskRunner runs a job on demand. *scheduledtask.Runner satisfies
// it structurally, and cmd/levelrail/main.go wires that same instance into
// the scheduler, so a scheduled tick and "run now" share one execution path.
type ScheduledTaskRunner interface {
	Run(ctx context.Context, task store.ScheduledTask) error
}

// ScheduledTaskStarter is the richer on-demand surface: it records the
// run's history row synchronously and returns its ID while the run itself
// proceeds in the background. Preferred over ScheduledTaskRunner when the
// wired runner provides it.
type ScheduledTaskStarter interface {
	Start(ctx context.Context, task store.ScheduledTask, trigger string) (runID string, err error)
}

// defaultJobsMaxPerApp is the per-app job cap when no option sets one.
const defaultJobsMaxPerApp = 20

// jobNamePattern keeps names URL-safe and distinct from generated IDs.
var jobNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

// scheduledTaskResource is the wire shape for a scheduled job (also served
// as /jobs). ServiceName is always the app name from the URL.
type scheduledTaskResource struct {
	ID          string   `json:"id,omitempty"`
	ServiceName string   `json:"service_name,omitempty"`
	Name        string   `json:"name,omitempty"`
	Mode        string   `json:"mode,omitempty"`
	Command     []string `json:"command"`
	// HTTP is the request an http-mode job sends instead of running Command.
	HTTP     *jobHTTPResource `json:"http,omitempty"`
	Schedule string           `json:"schedule"`
	Timezone string           `json:"timezone,omitempty"`
	Enabled  bool             `json:"enabled"`
	// ConcurrencyPolicy is allow, forbid or replace.
	ConcurrencyPolicy      string `json:"concurrency_policy"`
	TimeoutSeconds         int    `json:"timeout_seconds,omitempty"`
	MaxRetries             int    `json:"max_retries,omitempty"`
	RetryBackoffSeconds    int    `json:"retry_backoff_seconds,omitempty"`
	CatchUp                string `json:"catch_up,omitempty"`
	CatchUpLookbackSeconds int    `json:"catch_up_lookback_seconds,omitempty"`
	CPULimitMillis         int    `json:"cpu_limit_millis,omitempty"`
	MemoryLimitMB          int    `json:"memory_limit_mb,omitempty"`
	WarnAfterSeconds       int    `json:"warn_after_seconds,omitempty"`
	Source                 string `json:"source,omitempty"`

	LastRunAt           *time.Time `json:"last_run_at,omitempty"`
	LastRunStatus       string     `json:"last_run_status,omitempty"`
	LastRunOutput       string     `json:"last_run_output,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	ConsecutiveMissed   int        `json:"consecutive_missed"`
	RunningSince        *time.Time `json:"running_since,omitempty"`
	NextRunAt           *time.Time `json:"next_run_at,omitempty"`

	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

func toScheduledTaskResource(t store.ScheduledTask) scheduledTaskResource {
	res := scheduledTaskResource{
		ID: t.ID, ServiceName: t.ServiceName, Name: t.Name, Mode: t.Mode, Command: t.Command, Schedule: t.Schedule,
		Timezone: t.Timezone, Enabled: t.Enabled, ConcurrencyPolicy: t.ConcurrencyPolicy,
		TimeoutSeconds: t.TimeoutSeconds, MaxRetries: t.MaxRetries, RetryBackoffSeconds: t.RetryBackoffSeconds,
		CatchUp: t.CatchUp, CatchUpLookbackSeconds: t.CatchUpLookbackSeconds, CPULimitMillis: t.CPULimitMillis,
		MemoryLimitMB: t.MemoryLimitMB, WarnAfterSeconds: t.WarnAfterSeconds, Source: t.Source,
		LastRunAt: t.LastRunAt, LastRunStatus: t.LastRunStatus, LastRunOutput: t.LastRunOutput,
		ConsecutiveFailures: t.ConsecutiveFailures, ConsecutiveMissed: t.ConsecutiveMissed, RunningSince: t.RunningSince,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
	if t.Enabled {
		res.NextRunAt = nextJobRun(t, time.Now())
	}
	return res
}

func nextJobRun(t store.ScheduledTask, after time.Time) *time.Time {
	sched, err := cronexpr.Parse(t.Schedule)
	if err != nil {
		return nil
	}
	loc, err := time.LoadLocation(orUTC(t.Timezone))
	if err != nil {
		return nil
	}
	next := cronexpr.NextInLocation(sched, after, loc)
	if next.IsZero() {
		return nil
	}
	next = next.UTC()
	return &next
}

func orUTC(tz string) string {
	if tz == "" {
		return "UTC"
	}
	return tz
}

// toTask applies a validated resource onto the editable fields of base.
func (r scheduledTaskResource) toTask(base store.ScheduledTask) store.ScheduledTask {
	base.Name, base.Mode, base.Command, base.Schedule, base.Timezone = r.Name, r.Mode, r.Command, r.Schedule, orUTC(r.Timezone)
	base.Enabled, base.ConcurrencyPolicy = r.Enabled, r.ConcurrencyPolicy
	base.TimeoutSeconds, base.MaxRetries, base.RetryBackoffSeconds = r.TimeoutSeconds, r.MaxRetries, r.RetryBackoffSeconds
	base.CatchUp, base.CatchUpLookbackSeconds = r.CatchUp, r.CatchUpLookbackSeconds
	base.CPULimitMillis, base.MemoryLimitMB, base.WarnAfterSeconds = r.CPULimitMillis, r.MemoryLimitMB, r.WarnAfterSeconds
	return base
}

// validateScheduledTaskInput checks the fields a caller controls against
// the real cron parser, the real timezone database and the allowed enums.
func validateScheduledTaskInput(r *scheduledTaskResource) error {
	if r.Mode == store.ScheduledTaskModeHTTP {
		if err := validateJobHTTP(r.HTTP); err != nil {
			return err
		}
		r.Command = []string{}
	} else if len(r.Command) == 0 {
		return errors.New("command is required")
	}
	for _, part := range r.Command {
		if strings.TrimSpace(part) == "" {
			return errors.New("command must not contain empty arguments")
		}
	}
	if strings.TrimSpace(r.Schedule) == "" {
		return errors.New("schedule is required")
	}
	if _, err := cronexpr.Parse(r.Schedule); err != nil {
		return fmt.Errorf("schedule: %w", err)
	}
	if r.Timezone == "" {
		r.Timezone = "UTC"
	}
	if _, err := time.LoadLocation(r.Timezone); err != nil {
		return fmt.Errorf("timezone: %q is not a known IANA time zone", r.Timezone)
	}
	if r.Name != "" && !jobNamePattern.MatchString(r.Name) {
		return errors.New("name must be lowercase letters, digits, hyphens or underscores, starting with a letter or digit, at most 63 characters")
	}
	switch r.Mode {
	case "":
	case store.ScheduledTaskModeExec, store.ScheduledTaskModeFresh, store.ScheduledTaskModeHTTP:
	default:
		return fmt.Errorf("mode: must be %q, %q or %q", store.ScheduledTaskModeFresh, store.ScheduledTaskModeExec, store.ScheduledTaskModeHTTP)
	}
	switch r.CatchUp {
	case "":
		r.CatchUp = store.ScheduledTaskCatchUpSkip
	case store.ScheduledTaskCatchUpSkip, store.ScheduledTaskCatchUpRunOnce:
	default:
		return fmt.Errorf("catch_up: must be %q or %q", store.ScheduledTaskCatchUpSkip, store.ScheduledTaskCatchUpRunOnce)
	}
	for field, v := range map[string]int{
		"timeout_seconds": r.TimeoutSeconds, "max_retries": r.MaxRetries, "retry_backoff_seconds": r.RetryBackoffSeconds,
		"catch_up_lookback_seconds": r.CatchUpLookbackSeconds, "cpu_limit_millis": r.CPULimitMillis,
		"memory_limit_mb": r.MemoryLimitMB, "warn_after_seconds": r.WarnAfterSeconds,
	} {
		if v < 0 {
			return fmt.Errorf("%s must not be negative", field)
		}
	}
	return nil
}

// normalizeScheduledTaskConcurrencyPolicy validates policy, defaulting an
// empty one to allow so an older client keeps today's behavior.
func normalizeScheduledTaskConcurrencyPolicy(policy string) (string, error) {
	switch policy {
	case "":
		return store.ScheduledTaskConcurrencyAllow, nil
	case store.ScheduledTaskConcurrencyAllow, store.ScheduledTaskConcurrencyForbid, store.ScheduledTaskConcurrencyReplace, store.ScheduledTaskConcurrencyQueue:
		return policy, nil
	default:
		return "", fmt.Errorf("concurrency_policy: must be one of %q, %q, %q, %q (or empty for %q)",
			store.ScheduledTaskConcurrencyAllow, store.ScheduledTaskConcurrencyForbid, store.ScheduledTaskConcurrencyReplace, store.ScheduledTaskConcurrencyQueue, store.ScheduledTaskConcurrencyAllow)
	}
}

// defaultJobMode is fresh on the /jobs routes and exec on the legacy
// /scheduled-tasks routes, whose clients were written against exec.
func defaultJobMode(r *http.Request) string {
	if strings.Contains(r.URL.Path, "/jobs") {
		return store.ScheduledTaskModeFresh
	}
	return store.ScheduledTaskModeExec
}

func (rt *Router) maxJobsPerApp() int {
	if rt.jobsMaxPerApp > 0 {
		return rt.jobsMaxPerApp
	}
	return defaultJobsMaxPerApp
}

// requireJobApp writes a 404 or 500 and returns false unless the app exists.
func (rt *Router) requireJobApp(w http.ResponseWriter, r *http.Request, name, op string) bool {
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, rt.jobAppNotFoundMessage(r.Context(), name))
		return false
	} else if err != nil {
		rt.internalError(w, "api: "+op+": load app failed", err, slog.String("name", name))
		return false
	}
	return true
}

// jobAppNotFoundMessage names the member services when name is an app group,
// since jobs belong to one service and a group id alone is a dead end.
func (rt *Router) jobAppNotFoundMessage(ctx context.Context, name string) string {
	if rt.appGroups == nil {
		return "app not found"
	}
	group, err := rt.appGroups.GetAppByName(ctx, name)
	if err != nil {
		return "app not found"
	}
	members, err := rt.appGroups.ListServicesByApp(ctx, group.ID)
	if err != nil || len(members) == 0 {
		return "app not found"
	}
	names := make([]string, 0, len(members))
	for i := range members {
		names = append(names, members[i].Name)
	}
	return fmt.Sprintf("%q is an app group, not a service; jobs run on one of its services: %s", name, strings.Join(names, ", "))
}

// handleCreateScheduledTask handles POST /api/v1/apps/{name}/jobs and the
// legacy /scheduled-tasks alias.
func (rt *Router) handleCreateScheduledTask(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !rt.requireJobApp(w, r, name, "create scheduled task") {
		return
	}

	mode := defaultJobMode(r)
	req := scheduledTaskResource{Enabled: mode == store.ScheduledTaskModeFresh, Mode: mode}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateScheduledTaskInput(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var err error
	if req.ConcurrencyPolicy, err = normalizeScheduledTaskConcurrencyPolicy(req.ConcurrencyPolicy); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	count, err := rt.scheduledTasks.CountScheduledTasksForService(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: create scheduled task: count failed", err, slog.String("name", name))
		return
	}
	if count >= rt.maxJobsPerApp() {
		writeError(w, http.StatusConflict, fmt.Sprintf("app already has %d jobs, the limit (APP_JOBS_MAX_PER_APP)", count))
		return
	}
	if conflict := rt.jobNameTaken(r.Context(), name, req.Name, ""); conflict {
		writeError(w, http.StatusConflict, fmt.Sprintf("a job named %q already exists on this app", req.Name))
		return
	}

	id, err := randomScheduledTaskID()
	if err != nil {
		rt.internalError(w, "api: create scheduled task: generate id failed", err)
		return
	}
	now := time.Now().UTC()
	task := req.toTask(store.ScheduledTask{ID: id, ServiceName: name, Source: store.ScheduledTaskSourceAPI, CreatedAt: now, UpdatedAt: now})
	if err := rt.scheduledTasks.SaveScheduledTask(r.Context(), task); err != nil {
		rt.internalError(w, "api: create scheduled task failed", err, slog.String("name", name))
		return
	}
	if err := rt.persistJobHTTP(r.Context(), task, req.HTTP); err != nil {
		_ = rt.scheduledTasks.DeleteScheduledTask(r.Context(), id)
		rt.internalError(w, "api: create scheduled task: save http target failed", err, slog.String("name", name))
		return
	}
	saved, err := rt.scheduledTasks.GetScheduledTask(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusCreated, rt.jobResource(r.Context(), task))
		return
	}
	writeJSON(w, http.StatusCreated, rt.jobResource(r.Context(), saved))
}

func (rt *Router) jobNameTaken(ctx context.Context, app, jobName, exceptID string) bool {
	if jobName == "" {
		return false
	}
	tasks, err := rt.scheduledTasks.ListScheduledTasksForService(ctx, app)
	if err != nil {
		return false
	}
	for _, t := range tasks {
		if t.Name == jobName && t.ID != exceptID {
			return true
		}
	}
	return false
}

// handleListScheduledTasks handles GET /api/v1/apps/{name}/jobs.
func (rt *Router) handleListScheduledTasks(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !rt.requireJobApp(w, r, name, "list scheduled tasks") {
		return
	}
	tasks, err := rt.scheduledTasks.ListScheduledTasksForService(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: list scheduled tasks failed", err, slog.String("name", name))
		return
	}
	out := make([]scheduledTaskResource, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, rt.jobResource(r.Context(), task))
	}
	writeJSON(w, http.StatusOK, out)
}

// loadOwnedScheduledTask loads the job addressed by its ID or, failing
// that, its name within appName. A job of another app is a 404 either way,
// so a caller with access to app A learns nothing about app B.
func (rt *Router) loadOwnedScheduledTask(w http.ResponseWriter, r *http.Request, appName, id, op string) (store.ScheduledTask, bool) {
	task, err := rt.scheduledTasks.GetScheduledTask(r.Context(), id)
	if errors.Is(err, store.ErrScheduledTaskNotFound) {
		if byName, ok := rt.findJobByName(r.Context(), appName, id); ok {
			return byName, true
		}
		writeError(w, http.StatusNotFound, "scheduled task not found")
		return store.ScheduledTask{}, false
	}
	if err != nil {
		rt.internalError(w, "api: "+op+": load scheduled task failed", err, slog.String("id", id))
		return store.ScheduledTask{}, false
	}
	if task.ServiceName != appName {
		writeError(w, http.StatusNotFound, "scheduled task not found")
		return store.ScheduledTask{}, false
	}
	return task, true
}

func (rt *Router) findJobByName(ctx context.Context, appName, name string) (store.ScheduledTask, bool) {
	tasks, err := rt.scheduledTasks.ListScheduledTasksForService(ctx, appName)
	if err != nil {
		return store.ScheduledTask{}, false
	}
	for _, t := range tasks {
		if t.Name != "" && t.Name == name {
			return t, true
		}
	}
	return store.ScheduledTask{}, false
}

// handleGetScheduledTask handles GET /api/v1/apps/{name}/jobs/{id}.
func (rt *Router) handleGetScheduledTask(w http.ResponseWriter, r *http.Request) {
	task, ok := rt.loadOwnedScheduledTask(w, r, r.PathValue("name"), r.PathValue("id"), "get scheduled task")
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, rt.jobResource(r.Context(), task))
}

// handleUpdateScheduledTask handles PUT /api/v1/apps/{name}/jobs/{id}. The
// body is merged over the stored job, so a field a client omits keeps its
// value. A job declared in app.yaml may only be enabled or disabled here.
func (rt *Router) handleUpdateScheduledTask(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	existing, ok := rt.loadOwnedScheduledTask(w, r, name, r.PathValue("id"), "update scheduled task")
	if !ok {
		return
	}

	req := rt.jobResource(r.Context(), existing)
	if defaultJobMode(r) == store.ScheduledTaskModeExec {
		// The legacy route keeps its original full-replace contract for
		// the four fields it always had.
		req.Command, req.Schedule, req.Enabled, req.ConcurrencyPolicy = nil, "", false, ""
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateScheduledTaskInput(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var err error
	if req.ConcurrencyPolicy, err = normalizeScheduledTaskConcurrencyPolicy(req.ConcurrencyPolicy); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if existing.Source == store.ScheduledTaskSourceSpec && specManagedFieldsChanged(existing, req) {
		writeError(w, http.StatusConflict, "this job is declared in app.yaml; change it there (only enabled can be toggled here)")
		return
	}
	if rt.jobNameTaken(r.Context(), name, req.Name, existing.ID) {
		writeError(w, http.StatusConflict, fmt.Sprintf("a job named %q already exists on this app", req.Name))
		return
	}

	updated := req.toTask(existing)
	if err := rt.scheduledTasks.UpdateScheduledTaskConfig(r.Context(), updated, time.Now().UTC()); err != nil {
		if errors.Is(err, store.ErrScheduledTaskNotFound) {
			writeError(w, http.StatusNotFound, "scheduled task not found")
			return
		}
		rt.internalError(w, "api: update scheduled task failed", err, slog.String("id", existing.ID))
		return
	}
	if err := rt.persistJobHTTP(r.Context(), updated, req.HTTP); err != nil {
		rt.internalError(w, "api: update scheduled task: save http target failed", err, slog.String("id", existing.ID))
		return
	}
	reloaded, err := rt.scheduledTasks.GetScheduledTask(r.Context(), existing.ID)
	if err != nil {
		rt.internalError(w, "api: update scheduled task: reload failed", err, slog.String("id", existing.ID))
		return
	}
	writeJSON(w, http.StatusOK, rt.jobResource(r.Context(), reloaded))
}

func specManagedFieldsChanged(cur store.ScheduledTask, req scheduledTaskResource) bool {
	want := req.toTask(cur)
	want.Enabled = cur.Enabled
	return !sameJobConfig(cur, want)
}

func sameJobConfig(a, b store.ScheduledTask) bool {
	if len(a.Command) != len(b.Command) {
		return false
	}
	for i := range a.Command {
		if a.Command[i] != b.Command[i] {
			return false
		}
	}
	return a.Name == b.Name && a.Mode == b.Mode && a.Schedule == b.Schedule && orUTC(a.Timezone) == orUTC(b.Timezone) &&
		a.Enabled == b.Enabled && a.ConcurrencyPolicy == b.ConcurrencyPolicy && a.TimeoutSeconds == b.TimeoutSeconds &&
		a.MaxRetries == b.MaxRetries && a.RetryBackoffSeconds == b.RetryBackoffSeconds && a.CatchUp == b.CatchUp &&
		a.CatchUpLookbackSeconds == b.CatchUpLookbackSeconds && a.CPULimitMillis == b.CPULimitMillis &&
		a.MemoryLimitMB == b.MemoryLimitMB && a.WarnAfterSeconds == b.WarnAfterSeconds
}

// handleSetScheduledTaskEnabled serves POST .../jobs/{id}/enable and
// .../disable, the one-click form of PUT with just enabled set.
func (rt *Router) handleSetScheduledTaskEnabled(enabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		existing, ok := rt.loadOwnedScheduledTask(w, r, r.PathValue("name"), r.PathValue("id"), "toggle scheduled task")
		if !ok {
			return
		}
		updated := existing
		updated.Enabled = enabled
		if err := rt.scheduledTasks.UpdateScheduledTaskConfig(r.Context(), updated, time.Now().UTC()); err != nil {
			rt.internalError(w, "api: toggle scheduled task failed", err, slog.String("id", existing.ID))
			return
		}
		reloaded, err := rt.scheduledTasks.GetScheduledTask(r.Context(), existing.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, rt.jobResource(r.Context(), reloaded))
	}
}

// handleDeleteScheduledTask handles DELETE /api/v1/apps/{name}/jobs/{id}.
func (rt *Router) handleDeleteScheduledTask(w http.ResponseWriter, r *http.Request) {
	existing, ok := rt.loadOwnedScheduledTask(w, r, r.PathValue("name"), r.PathValue("id"), "delete scheduled task")
	if !ok {
		return
	}
	if existing.Source == store.ScheduledTaskSourceSpec {
		writeError(w, http.StatusConflict, "this job is declared in app.yaml; remove it there")
		return
	}
	if err := rt.scheduledTasks.DeleteScheduledTask(r.Context(), existing.ID); err != nil {
		if errors.Is(err, store.ErrScheduledTaskNotFound) {
			writeError(w, http.StatusNotFound, "scheduled task not found")
			return
		}
		rt.internalError(w, "api: delete scheduled task failed", err, slog.String("id", existing.ID))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// scheduledTaskRunResource is the wire shape of one run. Output is only
// populated by the single-run endpoint.
type scheduledTaskRunResource struct {
	ID           string     `json:"id"`
	TaskID       string     `json:"task_id"`
	JobName      string     `json:"job_name,omitempty"`
	ServiceName  string     `json:"service_name"`
	Trigger      string     `json:"trigger"`
	Status       string     `json:"status"`
	ScheduledFor *time.Time `json:"scheduled_for,omitempty"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	ExitCode     *int       `json:"exit_code,omitempty"`
	DurationMS   int64      `json:"duration_ms"`
	Attempts     int        `json:"attempts"`
	NodeID       string     `json:"node_id,omitempty"`
	Mode         string     `json:"mode,omitempty"`
	Message      string     `json:"message,omitempty"`
	Output       string     `json:"output,omitempty"`
}

func toJobRunResource(run store.ScheduledTaskRun, withOutput bool) scheduledTaskRunResource {
	res := scheduledTaskRunResource{
		ID: run.ID, TaskID: run.TaskID, ServiceName: run.ServiceName, Trigger: run.Trigger, Status: run.Status,
		ScheduledFor: run.ScheduledFor, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt, ExitCode: run.ExitCode,
		DurationMS: run.DurationMS, Attempts: run.Attempts, NodeID: run.NodeID, Mode: run.Mode, Message: run.Message,
	}
	if withOutput {
		res.Output = run.Output
	}
	return res
}

// handleListScheduledTaskRuns serves GET .../jobs/{id}/runs and the
// app-wide GET /api/v1/apps/{name}/job-runs, newest first, with optional
// ?status= and ?limit= filters.
func (rt *Router) handleListScheduledTaskRuns(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !rt.requireJobApp(w, r, name, "list job runs") {
		return
	}
	filter := store.ScheduledTaskRunFilter{ServiceName: name, Status: r.URL.Query().Get("status")}
	jobNames := map[string]string{}
	if id := r.PathValue("id"); id != "" {
		task, ok := rt.loadOwnedScheduledTask(w, r, name, id, "list job runs")
		if !ok {
			return
		}
		filter.TaskID = task.ID
	}
	if tasks, err := rt.scheduledTasks.ListScheduledTasksForService(r.Context(), name); err == nil {
		for _, t := range tasks {
			jobNames[t.ID] = t.Name
		}
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		var n int
		if _, err := fmt.Sscanf(raw, "%d", &n); err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		filter.Limit = n
	}
	runs, err := rt.scheduledTasks.ListScheduledTaskRuns(r.Context(), filter)
	if err != nil {
		rt.internalError(w, "api: list job runs failed", err, slog.String("name", name))
		return
	}
	out := make([]scheduledTaskRunResource, 0, len(runs))
	for _, run := range runs {
		res := toJobRunResource(run, false)
		res.JobName = jobNames[run.TaskID]
		out = append(out, res)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetScheduledTaskRun serves GET .../jobs/{id}/runs/{run}, including
// the captured output (secrets already redacted when the run was recorded).
func (rt *Router) handleGetScheduledTaskRun(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	task, ok := rt.loadOwnedScheduledTask(w, r, name, r.PathValue("id"), "get job run")
	if !ok {
		return
	}
	run, err := rt.scheduledTasks.GetScheduledTaskRun(r.Context(), r.PathValue("run"))
	if errors.Is(err, store.ErrScheduledTaskRunNotFound) || (err == nil && run.TaskID != task.ID) {
		writeError(w, http.StatusNotFound, "job run not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get job run failed", err, slog.String("run_id", r.PathValue("run")))
		return
	}
	res := toJobRunResource(run, true)
	res.JobName = task.Name
	writeJSON(w, http.StatusOK, res)
}

// runNowResponse is the 202 body of a run-now call: the job plus the ID of
// the run that was started, when the runner can report one.
type runNowResponse struct {
	scheduledTaskResource
	RunID string `json:"run_id,omitempty"`
}

// handleRunScheduledTaskNow handles POST .../jobs/{id}/run. The run goes
// through the same runner the scheduler uses and is answered with 202
// immediately: it may legitimately take minutes, and this request's own
// context ends the moment the handler returns.
func (rt *Router) handleRunScheduledTaskNow(w http.ResponseWriter, r *http.Request) {
	if rt.scheduledTaskRunner == nil {
		writeError(w, http.StatusNotImplemented, "scheduled task execution is not configured on this control plane")
		return
	}
	name := r.PathValue("name")
	task, ok := rt.loadOwnedScheduledTask(w, r, name, r.PathValue("id"), "run scheduled task")
	if !ok {
		return
	}

	trigger := store.ScheduledTaskTriggerManual
	if ptype, _, _, err := rt.callerPrincipal(r); err == nil && ptype == store.PrincipalTypeToken {
		trigger = store.ScheduledTaskTriggerAPI
	}

	resp := runNowResponse{scheduledTaskResource: toScheduledTaskResource(task)}
	if starter, ok := rt.scheduledTaskRunner.(ScheduledTaskStarter); ok {
		runID, err := starter.Start(context.WithoutCancel(r.Context()), task, trigger)
		if err != nil {
			rt.internalError(w, "api: run scheduled task failed", err, slog.String("id", task.ID), slog.String("name", name))
			return
		}
		resp.RunID = runID
		writeJSON(w, http.StatusAccepted, resp)
		return
	}

	go func() { //nolint:gosec // deliberately not r.Context(): see this handler's own doc comment
		if err := rt.scheduledTaskRunner.Run(context.Background(), task); err != nil {
			rt.logger.Error("api: run scheduled task failed", slog.String("error", err.Error()), slog.String("id", task.ID), slog.String("name", name))
		}
	}()
	writeJSON(w, http.StatusAccepted, resp)
}

// randomScheduledTaskID mints a job ID (9 random bytes, URL-safe base64,
// sct_ prefix).
func randomScheduledTaskID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate scheduled task id: %w", err)
	}
	return "sct_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
