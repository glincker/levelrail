package scheduledtask

import "context"

// historyKeep prefers the live runtime setting so a change applies to the next
// finished run, and falls back to the boot-time limit when no reader is wired.
func (r *Runner) historyKeep(ctx context.Context) int {
	if r.HistoryKeepFn != nil {
		if n := r.HistoryKeepFn(ctx); n > 0 {
			return n
		}
	}
	return r.limits().HistoryKeep
}
