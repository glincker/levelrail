package alerting

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// ScheduledTaskSource is the narrow store surface a scheduled-task-failure
// evaluation needs. *store.DB satisfies this structurally.
type ScheduledTaskSource interface {
	GetScheduledTask(ctx context.Context, id string) (store.ScheduledTask, error)
}

// EvaluateScheduledTaskFailure reads the task's persisted ConsecutiveFailures counter and fires at RestartCountThreshold.
// No ForDuration debounce: the consecutive-failure count is already the debounce.
func EvaluateScheduledTaskFailure(ctx context.Context, tasks ScheduledTaskSource, r Rule, now time.Time) (Rule, string, error) {
	next := r
	next.LastEvaluatedAt = &now

	task, err := tasks.GetScheduledTask(ctx, r.ScheduledTaskID)
	if errors.Is(err, store.ErrScheduledTaskNotFound) {
		// The task this rule watches was deleted: nothing left to alert
		// on, so the rule simply goes quiet rather than erroring on every
		// future tick.
		next.PendingSince = nil
		next.Firing = false
		next.FiringSince = nil
		return next, "", nil
	}
	if err != nil {
		return r, "", fmt.Errorf("alerting: evaluate rule %q: load scheduled task %q: %w", r.ID, r.ScheduledTaskID, err)
	}

	v := float64(task.ConsecutiveFailures)
	next.LastValue = &v

	var notice string
	if task.ConsecutiveFailures >= r.RestartCountThreshold {
		notice = scheduledTaskFailureNotice(task)
	}

	return advanceState(next, r, task.ConsecutiveFailures >= r.RestartCountThreshold, 0, now), notice, nil
}

func scheduledTaskFailureNotice(task store.ScheduledTask) string {
	what := strings.Join(task.Command, " ")
	if task.Mode == store.ScheduledTaskModeHTTP {
		what = jobLabel(task) + " (HTTP call)"
	}
	return fmt.Sprintf("%q failed %d runs in a row, last status %q", what, task.ConsecutiveFailures, task.LastRunStatus)
}
