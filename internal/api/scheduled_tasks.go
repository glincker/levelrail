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

// ScheduledTaskStore is the store surface the scheduled task handlers
// need, mirroring BackupTargetStore's own shape for a different child
// resource.
type ScheduledTaskStore interface {
	SaveScheduledTask(ctx context.Context, t store.ScheduledTask) error
	GetScheduledTask(ctx context.Context, id string) (store.ScheduledTask, error)
	ListScheduledTasksForService(ctx context.Context, serviceName string) ([]store.ScheduledTask, error)
	UpdateScheduledTask(ctx context.Context, id string, command []string, schedule string, enabled bool, updatedAt time.Time) error
	DeleteScheduledTask(ctx context.Context, id string) error
}

// ScheduledTaskRunner is the surface POST .../scheduled-tasks/{id}/run
// needs to actually run a task on demand: identical in shape to
// internal/scheduledtask.TaskRunner, redeclared here rather than
// imported across the package boundary, the same reasoning
// backup.ScheduledBackupRunner's own doc comment gives for its own
// redeclared interfaces. *scheduledtask.Runner satisfies this
// structurally, and cmd/levelrail/main.go wires that exact same
// instance into both this option and scheduledtask.NewScheduler, so a
// scheduled tick and a manual "run now" are always the one
// exec-and-record implementation, never two copies of it.
type ScheduledTaskRunner interface {
	Run(ctx context.Context, task store.ScheduledTask) error
}

// scheduledTaskResource is the wire shape for a scheduled task.
// ServiceName is always the app name from the URL, never taken from the
// request body, the same "the URL is the only thing that gets to say
// which resource this belongs to" reasoning handleCreateAlertRule's own
// doc comment establishes for resource_id.
type scheduledTaskResource struct {
	ID          string   `json:"id,omitempty"`
	ServiceName string   `json:"service_name,omitempty"`
	Command     []string `json:"command"`
	Schedule    string   `json:"schedule"`
	Enabled     bool     `json:"enabled"`

	LastRunAt     *time.Time `json:"last_run_at,omitempty"`
	LastRunStatus string     `json:"last_run_status,omitempty"`
	LastRunOutput string     `json:"last_run_output,omitempty"`
	// ConsecutiveFailures mirrors store.ScheduledTask's own field: what a
	// kind=scheduled_task_failure alert rule (internal/alerting) watches.
	ConsecutiveFailures int `json:"consecutive_failures"`

	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

func toScheduledTaskResource(t store.ScheduledTask) scheduledTaskResource {
	return scheduledTaskResource{
		ID:                  t.ID,
		ServiceName:         t.ServiceName,
		Command:             t.Command,
		Schedule:            t.Schedule,
		Enabled:             t.Enabled,
		LastRunAt:           t.LastRunAt,
		LastRunStatus:       t.LastRunStatus,
		LastRunOutput:       t.LastRunOutput,
		ConsecutiveFailures: t.ConsecutiveFailures,
		CreatedAt:           t.CreatedAt,
		UpdatedAt:           t.UpdatedAt,
	}
}

// validateScheduledTaskInput checks the two fields a caller actually
// controls (Command, Schedule) regardless of whether this is a create or
// an update: a non-empty argv and a cron expression internal/cronexpr
// itself can parse, the same "validate against the real parser, not a
// hand-rolled regex" precedent handleCreateAlertRule's channel_id lookup
// establishes for a different field.
func validateScheduledTaskInput(command []string, schedule string) error {
	if len(command) == 0 {
		return errors.New("command is required")
	}
	for _, part := range command {
		if strings.TrimSpace(part) == "" {
			return errors.New("command must not contain empty arguments")
		}
	}
	if strings.TrimSpace(schedule) == "" {
		return errors.New("schedule is required")
	}
	if _, err := cronexpr.Parse(schedule); err != nil {
		return fmt.Errorf("schedule: %w", err)
	}
	return nil
}

// handleCreateScheduledTask handles POST
// /api/v1/apps/{name}/scheduled-tasks.
func (rt *Router) handleCreateScheduledTask(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: create scheduled task: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req scheduledTaskResource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateScheduledTaskInput(req.Command, req.Schedule); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	id, err := randomScheduledTaskID()
	if err != nil {
		rt.logger.Error("api: create scheduled task: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	now := time.Now().UTC()
	task := store.ScheduledTask{
		ID:          id,
		ServiceName: name,
		Command:     req.Command,
		Schedule:    req.Schedule,
		Enabled:     req.Enabled,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := rt.scheduledTasks.SaveScheduledTask(r.Context(), task); err != nil {
		rt.logger.Error("api: create scheduled task failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, toScheduledTaskResource(task))
}

// handleListScheduledTasks handles GET
// /api/v1/apps/{name}/scheduled-tasks.
func (rt *Router) handleListScheduledTasks(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: list scheduled tasks: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	tasks, err := rt.scheduledTasks.ListScheduledTasksForService(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: list scheduled tasks failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]scheduledTaskResource, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, toScheduledTaskResource(task))
	}
	writeJSON(w, http.StatusOK, out)
}

// loadOwnedScheduledTask loads the scheduled task with this id and
// verifies it actually belongs to appName, the same ownership check
// handleDeleteAlertRule's own doc comment explains: a task ID is
// globally unique and doesn't itself encode which app it belongs to, so
// this verifies ServiceName matches before letting a caller read,
// change, delete, or run it through this app's URL. Returns a 404
// written to w and ok=false either way (not found, or belongs to a
// different app) so a caller with access to app A learns nothing about
// whether some ID belongs to app B.
func (rt *Router) loadOwnedScheduledTask(w http.ResponseWriter, r *http.Request, appName, id, op string) (store.ScheduledTask, bool) {
	task, err := rt.scheduledTasks.GetScheduledTask(r.Context(), id)
	if errors.Is(err, store.ErrScheduledTaskNotFound) {
		writeError(w, http.StatusNotFound, "scheduled task not found")
		return store.ScheduledTask{}, false
	}
	if err != nil {
		rt.logger.Error("api: "+op+": load scheduled task failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return store.ScheduledTask{}, false
	}
	if task.ServiceName != appName {
		writeError(w, http.StatusNotFound, "scheduled task not found")
		return store.ScheduledTask{}, false
	}
	return task, true
}

// handleGetScheduledTask handles GET
// /api/v1/apps/{name}/scheduled-tasks/{id}.
func (rt *Router) handleGetScheduledTask(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	task, ok := rt.loadOwnedScheduledTask(w, r, name, r.PathValue("id"), "get scheduled task")
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, toScheduledTaskResource(task))
}

// handleUpdateScheduledTask handles PUT
// /api/v1/apps/{name}/scheduled-tasks/{id}: updates Command/Schedule/
// Enabled only, never ServiceName (UpdateScheduledTask's own doc
// comment).
func (rt *Router) handleUpdateScheduledTask(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")
	if _, ok := rt.loadOwnedScheduledTask(w, r, name, id, "update scheduled task"); !ok {
		return
	}

	var req scheduledTaskResource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateScheduledTaskInput(req.Command, req.Schedule); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := rt.scheduledTasks.UpdateScheduledTask(r.Context(), id, req.Command, req.Schedule, req.Enabled, time.Now().UTC()); err != nil {
		if errors.Is(err, store.ErrScheduledTaskNotFound) {
			writeError(w, http.StatusNotFound, "scheduled task not found")
			return
		}
		rt.logger.Error("api: update scheduled task failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	updated, err := rt.scheduledTasks.GetScheduledTask(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: update scheduled task: reload after update failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toScheduledTaskResource(updated))
}

// handleDeleteScheduledTask handles DELETE
// /api/v1/apps/{name}/scheduled-tasks/{id}.
func (rt *Router) handleDeleteScheduledTask(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")
	if _, ok := rt.loadOwnedScheduledTask(w, r, name, id, "delete scheduled task"); !ok {
		return
	}

	if err := rt.scheduledTasks.DeleteScheduledTask(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrScheduledTaskNotFound) {
			writeError(w, http.StatusNotFound, "scheduled task not found")
			return
		}
		rt.logger.Error("api: delete scheduled task failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleRunScheduledTaskNow handles POST
// /api/v1/apps/{name}/scheduled-tasks/{id}/run: an on-demand trigger
// reusing the exact same Runner.Run internal/scheduledtask.Scheduler
// itself calls on a due tick (rt.scheduledTaskRunner, wired from the one
// shared instance cmd/levelrail/main.go builds), so there is only ever
// one place that execs a scheduled task's command and records the
// outcome. Dispatched into its own goroutine and answered with 202
// Accepted immediately, the same "an HTTP response must return quickly,
// the run itself may legitimately take minutes" reasoning
// handleTriggerBackup's own doc comment gives for the identical
// shape: this handler's own request context is cancelled the moment it
// returns, which would abort the run within microseconds of starting it
// if the run used r.Context() instead of a fresh background one.
func (rt *Router) handleRunScheduledTaskNow(w http.ResponseWriter, r *http.Request) {
	if rt.scheduledTaskRunner == nil {
		writeError(w, http.StatusNotImplemented, "scheduled task execution is not configured on this control plane")
		return
	}

	name := r.PathValue("name")
	id := r.PathValue("id")
	task, ok := rt.loadOwnedScheduledTask(w, r, name, id, "run scheduled task")
	if !ok {
		return
	}

	go func() { //nolint:gosec // deliberately not r.Context(): see this handler's own doc comment
		if err := rt.scheduledTaskRunner.Run(context.Background(), task); err != nil {
			rt.logger.Error("api: run scheduled task failed", slog.String("error", err.Error()), slog.String("id", id), slog.String("name", name))
		}
	}()

	writeJSON(w, http.StatusAccepted, toScheduledTaskResource(task))
}

// randomScheduledTaskID mirrors randomBackupTargetID's exact shape (9
// random bytes, URL-safe base64, a short type prefix). Duplicated rather
// than shared, the same "different resource, different ID space"
// reasoning randomBackupTargetID's own doc comment gives.
func randomScheduledTaskID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate scheduled task id: %w", err)
	}
	return "sct_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
