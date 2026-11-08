package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/scheduledtask"
	"github.com/GLINCKER/levelrail/internal/store"
)

// oneShotAttempter is what *scheduledtask.Runner offers a one-shot job: one
// recorded attempt of a task. A runner without it answers 501.
type oneShotAttempter interface {
	RunAttempt(ctx context.Context, task store.ScheduledTask) (scheduledtask.AttemptOutcome, error)
}

// oneShotPersister is the optional durable side of one-shot jobs. A store
// without it keeps jobs in memory only, as before.
type oneShotPersister interface {
	SaveOneShotJob(ctx context.Context, j store.OneShotJob) error
	GetOneShotJob(ctx context.Context, id string) (store.OneShotJob, error)
	FailInterruptedOneShotJobs(ctx context.Context, reason string, now time.Time) (int, error)
	PruneOneShotJobs(ctx context.Context, keep int) ([]string, error)
}

const oneShotInterruptedReason = "control plane restarted while the job was running; interrupted attempts are not resumed so the command never runs twice"

// oneShotRecovered records which stores already had their stale Running
// jobs failed in this process.
var oneShotRecovered sync.Map

func (rt *Router) oneShotPersister(ctx context.Context) oneShotPersister {
	p, ok := rt.scheduledTasks.(oneShotPersister)
	if !ok {
		return nil
	}
	if _, done := oneShotRecovered.LoadOrStore(p, true); !done {
		if n, err := p.FailInterruptedOneShotJobs(ctx, oneShotInterruptedReason, time.Now()); err != nil {
			rt.logger.Error("api: fail interrupted one-shot jobs", slog.String("error", err.Error()))
		} else if n > 0 {
			rt.logger.Warn("api: marked one-shot jobs interrupted by a restart as failed", slog.Int("jobs", n))
		}
	}
	return p
}

var (
	oneShotOnce     sync.Once
	oneShotLimits   scheduledtask.OneShotLimits
	oneShotBackoff  scheduledtask.Limits
	oneShotRegistry *scheduledtask.OneShotRegistry
)

func oneShotInit() {
	oneShotOnce.Do(func() {
		oneShotLimits = scheduledtask.OneShotLimitsFromEnv(os.LookupEnv)
		oneShotBackoff = scheduledtask.LimitsFromEnv(os.LookupEnv)
		oneShotRegistry = scheduledtask.NewOneShotRegistry(oneShotLimits.Keep)
	})
}

// oneShotAppFromBody names the app resource from the JSON body without
// consuming it, so the policy check sees the app the job will run against.
func oneShotAppFromBody(r *http.Request) string {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return "app:"
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	var probe struct {
		App string `json:"app"`
	}
	_ = json.Unmarshal(raw, &probe)
	return "app:" + probe.App
}

func (rt *Router) oneShotAppFromID(r *http.Request) string {
	oneShotInit()
	if e, ok := oneShotRegistry.Get(r.PathValue("id")); ok {
		return "app:" + e.App
	}
	if p := rt.oneShotPersister(r.Context()); p != nil {
		if j, err := p.GetOneShotJob(r.Context(), r.PathValue("id")); err == nil {
			return "app:" + j.App
		}
	}
	return "app:"
}

type oneShotRequest struct {
	App            string   `json:"app"`
	Command        []string `json:"command"`
	Completions    int      `json:"completions"`
	Parallelism    int      `json:"parallelism"`
	BackoffLimit   *int     `json:"backoff_limit"`
	TimeoutSeconds int      `json:"timeout_seconds"`
}

type oneShotResponse struct {
	ID             string     `json:"id"`
	App            string     `json:"app"`
	State          string     `json:"state"`
	Completions    int        `json:"completions"`
	Parallelism    int        `json:"parallelism"`
	BackoffLimit   int        `json:"backoff_limit"`
	Succeeded      int        `json:"succeeded"`
	FailedAttempts int        `json:"failed_attempts"`
	Active         int        `json:"active"`
	RunIDs         []string   `json:"run_ids"`
	Message        string     `json:"message,omitempty"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	// Latest attempt's result, filled on GET so a caller needs no second lookup.
	ExitCode        *int   `json:"exit_code,omitempty"`
	RunStatus       string `json:"run_status,omitempty"`
	Output          string `json:"output,omitempty"`
	OutputTruncated bool   `json:"output_truncated,omitempty"`
	// OutputWithheld is set when the run printed output the caller may not read without read_sensitive.
	OutputWithheld bool `json:"output_withheld,omitempty"`
}

// oneShotTruncatedHeader is the prefix the runner puts on output it cut to the job output limit.
const oneShotTruncatedHeader = "[output truncated"

// oneShotOutputTail is how many trailing output bytes a job response carries: the job output limit,
// so the answer never holds more than a run keeps.
func oneShotOutputTail() int {
	return scheduledtask.LimitsFromEnv(os.LookupEnv).MaxOutputBytes
}

// attachOneShotOutput adds the newest attempt's exit code and output tail.
// Output was redacted when the run was recorded; a lookup failure leaves the status-only answer.
// Redaction only covers known secrets, so the output itself needs read_sensitive.
func (rt *Router) attachOneShotOutput(ctx context.Context, taskID string, withOutput bool, resp *oneShotResponse) {
	if taskID == "" || rt.scheduledTasks == nil {
		return
	}
	runs, err := rt.scheduledTasks.ListScheduledTaskRuns(ctx, store.ScheduledTaskRunFilter{ServiceName: resp.App, TaskID: taskID, Limit: 1})
	if err != nil || len(runs) == 0 {
		return
	}
	run, err := rt.scheduledTasks.GetScheduledTaskRun(ctx, runs[0].ID)
	if err != nil {
		return
	}
	resp.ExitCode, resp.RunStatus = run.ExitCode, run.Status
	if !withOutput {
		resp.OutputWithheld = run.Output != ""
		return
	}
	resp.Output = run.Output
	resp.OutputTruncated = strings.HasPrefix(resp.Output, oneShotTruncatedHeader)
	if limit := oneShotOutputTail(); limit > 0 && len(resp.Output) > limit {
		resp.Output, resp.OutputTruncated = resp.Output[len(resp.Output)-limit:], true
	}
}

func toOneShotResponse(e *scheduledtask.OneShotEntry) oneShotResponse {
	s := e.Job.Snapshot()
	return oneShotResponse{
		ID: e.ID, App: e.App, State: s.State, Completions: s.Completions, Parallelism: e.Spec.Parallelism,
		BackoffLimit: e.Spec.BackoffLimit, Succeeded: s.Succeeded, FailedAttempts: s.FailedAttempts,
		Active: s.Active, RunIDs: s.RunIDs, Message: s.Message, StartedAt: s.StartedAt, FinishedAt: s.FinishedAt,
	}
}

func storedOneShotJob(e *scheduledtask.OneShotEntry, s scheduledtask.OneShotSnapshot) store.OneShotJob {
	return store.OneShotJob{
		ID: e.ID, App: e.App, TaskID: e.TaskID, State: s.State, Completions: s.Completions,
		Parallelism: e.Spec.Parallelism, BackoffLimit: e.Spec.BackoffLimit, Succeeded: s.Succeeded,
		FailedAttempts: s.FailedAttempts, Active: s.Active, RunIDs: s.RunIDs, Message: s.Message,
		StartedAt: s.StartedAt, FinishedAt: s.FinishedAt,
	}
}

func fromStoredOneShot(j store.OneShotJob) oneShotResponse {
	return oneShotResponse{
		ID: j.ID, App: j.App, State: j.State, Completions: j.Completions, Parallelism: j.Parallelism,
		BackoffLimit: j.BackoffLimit, Succeeded: j.Succeeded, FailedAttempts: j.FailedAttempts, Active: j.Active,
		RunIDs: append([]string{}, j.RunIDs...), Message: j.Message, StartedAt: j.StartedAt, FinishedAt: j.FinishedAt,
	}
}

// handleStartOneShotJob handles POST /api/v1/jobs/runs. Attempts reuse the
// scheduled-task runner against a hidden, disabled task row so every attempt
// lands in the normal run history; the answer is 202 with the job to poll.
func (rt *Router) handleStartOneShotJob(w http.ResponseWriter, r *http.Request) {
	oneShotInit()
	var req oneShotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.App == "" || len(req.Command) == 0 {
		writeError(w, http.StatusBadRequest, "app and command are required")
		return
	}
	if !rt.authorizeRefs(w, r, AbilityWrite, notFoundAs(http.StatusNotFound, "app not found"), appRef(req.App)) {
		return
	}
	if !rt.requireJobApp(w, r, req.App, "start one-shot job") || !rt.requireJobExecAccess(w, r, req.App) {
		return
	}
	attempter, ok := rt.scheduledTaskRunner.(oneShotAttempter)
	if !ok {
		writeError(w, http.StatusNotImplemented, "scheduled task execution is not configured on this control plane")
		return
	}
	spec := scheduledtask.OneShotSpec{
		Completions: req.Completions, Parallelism: req.Parallelism, BackoffLimit: oneShotLimits.BackoffLimit,
		BackoffBase: oneShotBackoff.RetryBackoff, BackoffMax: oneShotBackoff.MaxRetryBackoff,
	}
	if req.BackoffLimit != nil {
		spec.BackoffLimit = *req.BackoffLimit
	}
	if err := spec.Validate(oneShotLimits); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	timeout := int(oneShotLimits.Timeout.Seconds())
	if req.TimeoutSeconds > 0 {
		timeout = req.TimeoutSeconds
	}

	jobID, err := randomOneShotID()
	if err != nil {
		rt.internalError(w, "api: start one-shot job: generate id failed", err)
		return
	}
	taskID, err := randomScheduledTaskID()
	if err != nil {
		rt.internalError(w, "api: start one-shot job: generate task id failed", err)
		return
	}
	now := time.Now().UTC()
	task := store.ScheduledTask{
		ID: taskID, ServiceName: req.App, Command: req.Command, Schedule: oneShotTaskSchedule, Enabled: false,
		Name: oneShotTaskNamePrefix + jobID, Mode: store.ScheduledTaskModeFresh, TimeoutSeconds: timeout,
		Source: store.ScheduledTaskSourceAPI, CreatedAt: now, UpdatedAt: now,
	}
	if err := rt.scheduledTasks.SaveScheduledTask(r.Context(), task); err != nil {
		rt.internalError(w, "api: start one-shot job: save task failed", err, slog.String("app", req.App))
		return
	}

	entry := &scheduledtask.OneShotEntry{ID: jobID, App: req.App, TaskID: taskID, Spec: spec, Job: &scheduledtask.OneShot{}}
	entry.Job.Init(spec)
	if p := rt.oneShotPersister(r.Context()); p != nil {
		bg := context.WithoutCancel(r.Context())
		if err := p.SaveOneShotJob(bg, storedOneShotJob(entry, entry.Job.Snapshot())); err != nil {
			_ = rt.scheduledTasks.DeleteScheduledTask(bg, taskID)
			rt.internalError(w, "api: start one-shot job: persist job failed", err, slog.String("app", req.App))
			return
		}
		entry.Job.OnUpdate = func(s scheduledtask.OneShotSnapshot) {
			if err := p.SaveOneShotJob(bg, storedOneShotJob(entry, s)); err != nil {
				rt.logger.Error("api: persist one-shot job failed", slog.String("error", err.Error()), slog.String("job", jobID))
			}
		}
		rt.pruneOneShotJobs(bg, p)
	}
	oneShotRegistry.Add(entry)
	go entry.Job.Run(context.WithoutCancel(r.Context()), spec, func(ctx context.Context, _, _ int) scheduledtask.AttemptOutcome {
		out, err := attempter.RunAttempt(ctx, task)
		if err != nil {
			rt.logger.Error("api: one-shot attempt failed", slog.String("error", err.Error()), slog.String("job", jobID))
			if out.Status == "" {
				out.Status = store.ScheduledTaskStatusFailed
			}
		}
		return out
	})
	writeJSON(w, http.StatusAccepted, toOneShotResponse(entry))
}

// handleGetOneShotJob handles GET /api/v1/jobs/runs/{id}.
func (rt *Router) handleGetOneShotJob(w http.ResponseWriter, r *http.Request) {
	oneShotInit()
	id := r.PathValue("id")
	p := rt.oneShotPersister(r.Context())
	if entry, ok := oneShotRegistry.Get(id); ok {
		resp := toOneShotResponse(entry)
		rt.attachOneShotOutput(r.Context(), entry.TaskID, rt.callerHasAbility(r, AbilityReadSensitive), &resp)
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if p != nil {
		j, err := p.GetOneShotJob(r.Context(), id)
		if err == nil {
			resp := fromStoredOneShot(j)
			rt.attachOneShotOutput(r.Context(), j.TaskID, rt.callerHasAbility(r, AbilityReadSensitive), &resp)
			writeJSON(w, http.StatusOK, resp)
			return
		}
		if !errors.Is(err, store.ErrOneShotJobNotFound) {
			rt.internalError(w, "api: get one-shot job failed", err, slog.String("job", id))
			return
		}
		writeError(w, http.StatusNotFound, "one-shot job not found (it may have been pruned)")
		return
	}
	writeError(w, http.StatusNotFound, "one-shot job not found (jobs are kept in memory and lost on restart)")
}

// pruneOneShotJobs applies the APP_JOB_ONESHOT_KEEP retention to stored jobs
// and drops the hidden task rows of the pruned ones.
func (rt *Router) pruneOneShotJobs(ctx context.Context, p oneShotPersister) {
	tasks, err := p.PruneOneShotJobs(ctx, oneShotLimits.Keep)
	if err != nil {
		rt.logger.Error("api: prune one-shot jobs failed", slog.String("error", err.Error()))
		return
	}
	for _, id := range tasks {
		if err := rt.scheduledTasks.DeleteScheduledTask(ctx, id); err != nil && !errors.Is(err, store.ErrScheduledTaskNotFound) {
			rt.logger.Error("api: drop pruned one-shot task failed", slog.String("error", err.Error()), slog.String("task", id))
		}
	}
}

func randomOneShotID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", errors.Join(fmt.Errorf("api: generate one-shot job id"), err)
	}
	return "jb_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
