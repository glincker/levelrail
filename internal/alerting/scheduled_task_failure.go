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

// EvaluateScheduledTaskFailure runs one KindScheduledTaskFailure rule
// against r.ScheduledTaskID's current ConsecutiveFailures count and
// returns its updated evaluation state plus, when firing, a
// human-readable notice line for Engine to attach to the outgoing Event.
//
// Polls the task's own persisted counter rather than tracking anything
// itself, the same "read current state, no local history" shape
// EvaluateCertExpiry already uses: the counter is maintained by
// store.RecordScheduledTaskRun at the moment a run actually happens,
// decoupled from how often Engine.Tick itself runs.
//
// Fires the instant ConsecutiveFailures crosses RestartCountThreshold,
// with no ForDuration debounce: requiring N consecutive failures is
// already the debounce, the same reasoning EvaluateCrashloop's own doc
// comment gives for RestartWindow.
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
	return fmt.Sprintf("%q failed %d runs in a row, last status %q", strings.Join(task.Command, " "), task.ConsecutiveFailures, task.LastRunStatus)
}
