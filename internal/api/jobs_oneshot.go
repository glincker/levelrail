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

// registerOneShotJobRoutes serves one-shot jobs. Starting one runs a command
// right now, so it needs AbilityDeploy on the app, like "run now".
func (rt *Router) registerOneShotJobRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/jobs/runs", rt.requireAbilityForResource(AbilityDeploy, oneShotAppFromBody, rt.handleStartOneShotJob))
	mux.HandleFunc("GET /api/v1/jobs/runs/{id}", rt.requireAbilityForResource(AbilityRead, oneShotAppFromID, rt.handleGetOneShotJob))
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

func oneShotAppFromID(r *http.Request) string {
	oneShotInit()
	if e, ok := oneShotRegistry.Get(r.PathValue("id")); ok {
		return "app:" + e.App
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
}

func toOneShotResponse(e *scheduledtask.OneShotEntry) oneShotResponse {
	s := e.Job.Snapshot()
	return oneShotResponse{
		ID: e.ID, App: e.App, State: s.State, Completions: s.Completions, Parallelism: e.Spec.Parallelism,
		BackoffLimit: e.Spec.BackoffLimit, Succeeded: s.Succeeded, FailedAttempts: s.FailedAttempts,
		Active: s.Active, RunIDs: s.RunIDs, Message: s.Message, StartedAt: s.StartedAt, FinishedAt: s.FinishedAt,
	}
}

// handleStartOneShotJob handles POST /api/v1/jobs/runs. Attempts reuse the
// scheduled-task runner against a hidden, disabled task row so every attempt
// lands in the normal run history; the answer is 202 with the job to poll.
func (rt *Router) handleStartOneShotJob(w http.ResponseWriter, r *http.Request) {
	oneShotInit()
	attempter, ok := rt.scheduledTaskRunner.(oneShotAttempter)
	if !ok {
		writeError(w, http.StatusNotImplemented, "scheduled task execution is not configured on this control plane")
		return
	}
	var req oneShotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.App == "" || len(req.Command) == 0 {
		writeError(w, http.StatusBadRequest, "app and command are required")
		return
	}
	if !rt.requireJobApp(w, r, req.App, "start one-shot job") {
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
		ID: taskID, ServiceName: req.App, Command: req.Command, Schedule: "@yearly", Enabled: false,
		Name: "oneshot-" + jobID, Mode: store.ScheduledTaskModeFresh, TimeoutSeconds: timeout,
		Source: store.ScheduledTaskSourceAPI, CreatedAt: now, UpdatedAt: now,
	}
	if err := rt.scheduledTasks.SaveScheduledTask(r.Context(), task); err != nil {
		rt.internalError(w, "api: start one-shot job: save task failed", err, slog.String("app", req.App))
		return
	}

	entry := &scheduledtask.OneShotEntry{ID: jobID, App: req.App, TaskID: taskID, Spec: spec, Job: &scheduledtask.OneShot{}}
	entry.Job.Init(spec)
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
	entry, ok := oneShotRegistry.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "one-shot job not found (jobs are kept in memory and lost on restart)")
		return
	}
	writeJSON(w, http.StatusOK, toOneShotResponse(entry))
}

func randomOneShotID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", errors.Join(fmt.Errorf("api: generate one-shot job id"), err)
	}
	return "jb_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
