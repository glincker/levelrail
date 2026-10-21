package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ScheduledTaskModeHTTP makes a job call a path on its app instead of
// running a command.
const ScheduledTaskModeHTTP = "http"

// ErrScheduledTaskHTTPNotFound is returned when a task has no HTTP target.
var ErrScheduledTaskHTTPNotFound = errors.New("store: scheduled task http target not found")

// ScheduledTaskHTTP is the request an http-mode job sends. Background jobs
// are awaited by the control plane under a longer timeout, never by a caller.
type ScheduledTaskHTTP struct {
	TaskID     string
	Method     string
	Path       string
	Body       string
	Background bool
}

// SetScheduledTaskHTTP upserts the HTTP target of a task.
func (db *DB) SetScheduledTaskHTTP(ctx context.Context, h ScheduledTaskHTTP) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO scheduled_task_http (task_id, method, path, body, background)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET method = excluded.method, path = excluded.path,
			body = excluded.body, background = excluded.background
	`, h.TaskID, orDefault(h.Method, "POST"), h.Path, h.Body, h.Background)
	if err != nil {
		return fmt.Errorf("store: set scheduled task http %q: %w", h.TaskID, err)
	}
	return nil
}

// GetScheduledTaskHTTP returns a task's HTTP target or ErrScheduledTaskHTTPNotFound.
func (db *DB) GetScheduledTaskHTTP(ctx context.Context, taskID string) (ScheduledTaskHTTP, error) {
	h := ScheduledTaskHTTP{TaskID: taskID}
	var bg int
	err := db.QueryRowContext(ctx, `SELECT method, path, body, background FROM scheduled_task_http WHERE task_id = ?`, taskID).
		Scan(&h.Method, &h.Path, &h.Body, &bg)
	if errors.Is(err, sql.ErrNoRows) {
		return ScheduledTaskHTTP{}, ErrScheduledTaskHTTPNotFound
	}
	if err != nil {
		return ScheduledTaskHTTP{}, fmt.Errorf("store: get scheduled task http %q: %w", taskID, err)
	}
	h.Background = bg != 0
	return h, nil
}

// DeleteScheduledTaskHTTP removes a task's HTTP target; missing is not an error.
func (db *DB) DeleteScheduledTaskHTTP(ctx context.Context, taskID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM scheduled_task_http WHERE task_id = ?`, taskID); err != nil {
		return fmt.Errorf("store: delete scheduled task http %q: %w", taskID, err)
	}
	return nil
}
