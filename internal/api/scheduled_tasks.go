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
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/cronexpr"
	"github.com/GLINCKER/levelrail/internal/store"
)

// This file implements app-level scheduled tasks: CRUD, manual trigger,
// and run history (internal/scheduledtask.Scheduler runs the background
// side). Mutating/running/history endpoints sit at AbilityRoot, matching
// exec.go's exec endpoint, since a task's command has the same access to
// the container's secrets; plain List stays at AbilityRead.

// ScheduledTaskStore is the store surface the CRUD handlers need.
type ScheduledTaskStore interface {
	SaveScheduledTask(ctx context.Context, t store.ScheduledTask) error
	GetScheduledTask(ctx context.Context, id string) (store.ScheduledTask, error)
	ListScheduledTasksForService(ctx context.Context, serviceName string) ([]store.ScheduledTask, error)
	UpdateScheduledTask(ctx context.Context, t store.ScheduledTask) error
	DeleteScheduledTask(ctx context.Context, id string) error
}

// ScheduledTaskHistoryStore is the store surface the execution-history
// list handler needs.
type ScheduledTaskHistoryStore interface {
	ListScheduledTaskRuns(ctx context.Context, taskID string) ([]store.ScheduledTaskRun, error)
}

// ScheduledTaskRunner is the surface the manual "run now" handler needs.
// *scheduledtask.Runner satisfies this structurally; internal/api never
// imports internal/scheduledtask directly (see BackupRunner for the same
// boundary).
type ScheduledTaskRunner interface {
	Run(ctx context.Context, runID string, task store.ScheduledTask) error
}

// scheduledTaskResource is the wire shape for one scheduled task.
type scheduledTaskResource struct {
	ID             string `json:"id"`
	ServiceName    string `json:"service_name"`
	Name           string `json:"name"`
	Command        string `json:"command"`
	Schedule       string `json:"schedule"`
	Enabled        bool   `json:"enabled"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

func toScheduledTaskResource(t store.ScheduledTask) scheduledTaskResource {
	return scheduledTaskResource{
		ID:             t.ID,
		ServiceName:    t.ServiceName,
		Name:           t.Name,
		Command:        t.Command,
		Schedule:       t.Schedule,
		Enabled:        t.Enabled,
		TimeoutSeconds: t.TimeoutSeconds,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
	}
}

// scheduledTaskRunResource is the wire shape for one execution attempt.
type scheduledTaskRunResource struct {
	ID              string `json:"id"`
	ScheduledTaskID string `json:"scheduled_task_id"`
	Status          string `json:"status"`
	ExitCode        int    `json:"exit_code"`
	Output          string `json:"output,omitempty"`
	Error           string `json:"error,omitempty"`
	StartedAt       string `json:"started_at"`
	FinishedAt      string `json:"finished_at,omitempty"`
}

func toScheduledTaskRunResource(r store.ScheduledTaskRun) scheduledTaskRunResource {
	return scheduledTaskRunResource{
		ID:              r.ID,
		ScheduledTaskID: r.ScheduledTaskID,
		Status:          r.Status,
		ExitCode:        r.ExitCode,
		Output:          r.Output,
		Error:           r.Error,
		StartedAt:       r.StartedAt,
		FinishedAt:      r.FinishedAt,
	}
}

// scheduledTaskRequest is the request body both create and update share:
// the same "PUT is a full-replace of every editable field" convention
// setBackupScheduleRequest already establishes.
type scheduledTaskRequest struct {
	Name           string `json:"name"`
	Command        string `json:"command"`
	Schedule       string `json:"schedule"`
	Enabled        bool   `json:"enabled"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

func validateScheduledTaskRequest(req scheduledTaskRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(req.Command) == "" {
		return errors.New("command is required")
	}
	if req.Schedule == "" {
		return errors.New("schedule is required")
	}
	if _, err := cronexpr.Parse(req.Schedule); err != nil {
		return fmt.Errorf("invalid schedule: %s", err.Error())
	}
	if req.TimeoutSeconds < 0 {
		return errors.New("timeout_seconds must not be negative")
	}
	return nil
}

// loadOwnedScheduledTask resolves id and verifies it belongs to appName,
// returning store.ErrScheduledTaskNotFound either way (doesn't exist, or
// belongs to a different app) so a caller with access to one app learns
// nothing about whether some ID belongs to another, the same
// information-hiding reasoning handleDeleteAlertRule's own doc comment
// applies to ResourceID.
func (rt *Router) loadOwnedScheduledTask(ctx context.Context, appName, id string) (store.ScheduledTask, error) {
	task, err := rt.scheduledTasks.GetScheduledTask(ctx, id)
	if err != nil {
		return store.ScheduledTask{}, err
	}
	if task.ServiceName != appName {
		return store.ScheduledTask{}, store.ErrScheduledTaskNotFound
	}
	return task, nil
}

// loadOwnedScheduledTaskOrRespond wraps loadOwnedScheduledTask with the
// not-found/internal-error response writing every mutating and
// history-reading handler below needs, returning ok=false once it has
// already written a response so the caller can just return.
func (rt *Router) loadOwnedScheduledTaskOrRespond(w http.ResponseWriter, r *http.Request, appName, id, errContext string) (store.ScheduledTask, bool) {
	task, err := rt.loadOwnedScheduledTask(r.Context(), appName, id)
	if errors.Is(err, store.ErrScheduledTaskNotFound) {
		writeError(w, http.StatusNotFound, "scheduled task not found")
		return store.ScheduledTask{}, false
	}
	if err != nil {
		rt.internalError(w, errContext, err, slog.String("id", id))
		return store.ScheduledTask{}, false
	}
	return task, true
}

// handleCreateScheduledTask handles POST /api/v1/apps/{name}/scheduled-tasks.
func (rt *Router) handleCreateScheduledTask(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: create scheduled task: load app failed", err, slog.String("name", name))
		return
	}

	var req scheduledTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateScheduledTaskRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	id, err := randomScheduledTaskID()
	if err != nil {
		rt.internalError(w, "api: create scheduled task: generate id failed", err)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	task := store.ScheduledTask{
		ID:             id,
		ServiceName:    name,
		Name:           req.Name,
		Command:        req.Command,
		Schedule:       req.Schedule,
		Enabled:        req.Enabled,
		TimeoutSeconds: req.TimeoutSeconds,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := rt.scheduledTasks.SaveScheduledTask(r.Context(), task); err != nil {
		rt.internalError(w, "api: create scheduled task: save failed", err, slog.String("id", id))
		return
	}

	writeJSON(w, http.StatusCreated, toScheduledTaskResource(task))
}

// handleListScheduledTasks handles GET /api/v1/apps/{name}/scheduled-tasks.
func (rt *Router) handleListScheduledTasks(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: list scheduled tasks: load app failed", err, slog.String("name", name))
		return
	}

	tasks, err := rt.scheduledTasks.ListScheduledTasksForService(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: list scheduled tasks failed", err, slog.String("name", name))
		return
	}
	out := make([]scheduledTaskResource, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, toScheduledTaskResource(t))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleUpdateScheduledTask handles
// PUT /api/v1/apps/{name}/scheduled-tasks/{id}.
func (rt *Router) handleUpdateScheduledTask(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")

	existing, ok := rt.loadOwnedScheduledTaskOrRespond(w, r, name, id, "api: update scheduled task: load failed")
	if !ok {
		return
	}

	var req scheduledTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateScheduledTaskRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	existing.Name = req.Name
	existing.Command = req.Command
	existing.Schedule = req.Schedule
	existing.Enabled = req.Enabled
	existing.TimeoutSeconds = req.TimeoutSeconds
	existing.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := rt.scheduledTasks.UpdateScheduledTask(r.Context(), existing); errors.Is(err, store.ErrScheduledTaskNotFound) {
		writeError(w, http.StatusNotFound, "scheduled task not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: update scheduled task failed", err, slog.String("id", id))
		return
	}

	writeJSON(w, http.StatusOK, toScheduledTaskResource(existing))
}

// handleDeleteScheduledTask handles
// DELETE /api/v1/apps/{name}/scheduled-tasks/{id}.
func (rt *Router) handleDeleteScheduledTask(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")

	if _, ok := rt.loadOwnedScheduledTaskOrRespond(w, r, name, id, "api: delete scheduled task: load failed"); !ok {
		return
	}

	if err := rt.scheduledTasks.DeleteScheduledTask(r.Context(), id); errors.Is(err, store.ErrScheduledTaskNotFound) {
		writeError(w, http.StatusNotFound, "scheduled task not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: delete scheduled task failed", err, slog.String("id", id))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleRunScheduledTask handles
// POST /api/v1/apps/{name}/scheduled-tasks/{id}/run: the manual
// counterpart of Scheduler's own cron-triggered run, going through the
// identical ScheduledTaskRunner. Asynchronous like handleTriggerBackup:
// this returns once the attempt is recorded and under way, not once the
// command actually finishes; GET .../runs is how a caller learns the
// outcome. Runner.Run runs in a goroutine against context.Background(),
// not r.Context(), for the identical reason handleTriggerBackup's own
// doc comment gives: r.Context() is cancelled the moment this handler
// returns, which would abort the command within microseconds of
// starting it.
func (rt *Router) handleRunScheduledTask(w http.ResponseWriter, r *http.Request) {
	if rt.scheduledTaskRunner == nil {
		writeError(w, http.StatusNotImplemented, "scheduled task execution is not configured on this control plane")
		return
	}

	name := r.PathValue("name")
	id := r.PathValue("id")

	task, ok := rt.loadOwnedScheduledTaskOrRespond(w, r, name, id, "api: run scheduled task: load failed")
	if !ok {
		return
	}

	runID, err := randomScheduledTaskRunID()
	if err != nil {
		rt.internalError(w, "api: run scheduled task: generate id failed", err)
		return
	}

	runner := rt.scheduledTaskRunner
	go func() { //nolint:gosec // deliberately not r.Context(), see this handler's own doc comment
		if err := runner.Run(context.Background(), runID, task); err != nil {
			rt.logger.Error("api: scheduled task run failed", slog.String("error", err.Error()), slog.String("id", runID), slog.String("task_id", task.ID))
		}
	}()

	writeJSON(w, http.StatusAccepted, scheduledTaskRunResource{
		ID:              runID,
		ScheduledTaskID: task.ID,
		Status:          store.ScheduledTaskRunStatusRunning,
	})
}

// handleListScheduledTaskRuns handles
// GET /api/v1/apps/{name}/scheduled-tasks/{id}/runs.
func (rt *Router) handleListScheduledTaskRuns(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")

	if _, ok := rt.loadOwnedScheduledTaskOrRespond(w, r, name, id, "api: list scheduled task runs: load task failed"); !ok {
		return
	}

	runs, err := rt.scheduledTaskHistory.ListScheduledTaskRuns(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: list scheduled task runs failed", err, slog.String("id", id))
		return
	}
	out := make([]scheduledTaskRunResource, 0, len(runs))
	for _, run := range runs {
		out = append(out, toScheduledTaskRunResource(run))
	}
	writeJSON(w, http.StatusOK, out)
}

// randomScheduledTaskID mirrors randomBackupTargetID's exact shape
// (internal/api/backup_targets.go): 9 random bytes, URL-safe base64,
// with its own "sct_" prefix.
func randomScheduledTaskID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate scheduled task id: %w", err)
	}
	return "sct_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// randomScheduledTaskRunID mirrors randomBackupHistoryID's exact shape,
// with its own "sctr_" prefix, the same prefix
// internal/scheduledtask.Scheduler mints for its own cron-triggered runs
// (scheduler.go), so a manual and a scheduled run are visually
// indistinguishable as run history entries, matching how a scheduled and
// manual backup share randomBackupHistoryID's identical "bkh_" prefix.
func randomScheduledTaskRunID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate scheduled task run id: %w", err)
	}
	return "sctr_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
