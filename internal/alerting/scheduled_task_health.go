package alerting

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// EvaluateScheduledTaskHealth evaluates a KindScheduledTaskMissed or
// KindScheduledTaskRunningLong rule from the job's persisted counters, so
// firing does not depend on how often the engine ticks.
func EvaluateScheduledTaskHealth(ctx context.Context, tasks ScheduledTaskSource, r Rule, now time.Time) (Rule, string, error) {
	next := r
	next.LastEvaluatedAt = &now

	task, err := tasks.GetScheduledTask(ctx, r.ScheduledTaskID)
	if errors.Is(err, store.ErrScheduledTaskNotFound) {
		next.PendingSince, next.Firing, next.FiringSince = nil, false, nil
		return next, "", nil
	}
	if err != nil {
		return r, "", fmt.Errorf("alerting: evaluate rule %q: load scheduled task %q: %w", r.ID, r.ScheduledTaskID, err)
	}

	var (
		firing bool
		value  float64
		notice string
	)
	switch r.Kind {
	case KindScheduledTaskMissed:
		threshold := r.RestartCountThreshold
		if threshold <= 0 {
			threshold = 1
		}
		value = float64(task.ConsecutiveMissed)
		firing = task.ConsecutiveMissed >= threshold
		if firing {
			notice = fmt.Sprintf("job %q missed %d scheduled run(s) in a row (the control plane was not running them)", jobLabel(task), task.ConsecutiveMissed)
		}
	case KindScheduledTaskRunningLong:
		if task.RunningSince != nil && task.WarnAfterSeconds > 0 {
			elapsed := now.Sub(*task.RunningSince)
			value = elapsed.Seconds()
			firing = elapsed >= time.Duration(task.WarnAfterSeconds)*time.Second
			if firing {
				notice = fmt.Sprintf("job %q has been running for %s, past its %ds warning threshold", jobLabel(task), elapsed.Round(time.Second), task.WarnAfterSeconds)
			}
		}
	}
	next.LastValue = &value
	return advanceState(next, r, firing, 0, now), notice, nil
}

func jobLabel(task store.ScheduledTask) string {
	if task.Name != "" {
		return task.Name
	}
	return task.ID
}
