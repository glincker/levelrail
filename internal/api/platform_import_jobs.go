package api

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/store"
)

// createImportedJobs turns source cron entries into scheduled jobs. Staged apps are suspended,
// so their jobs are created disabled and the warning says to enable them at cutover.
func (a *importApplier) createImportedJobs(ctx context.Context, app string, crons []platformimport.CronPlan) []string {
	var warnings []string
	made := 0
	for _, c := range crons {
		req := scheduledTaskResource{
			Name: c.Name, Mode: store.ScheduledTaskModeExec, Schedule: c.Schedule,
			Command: []string{"sh", "-c", c.Command}, Enabled: a.sessionID == "",
		}
		if err := validateScheduledTaskInput(&req); err != nil {
			warnings = append(warnings, fmt.Sprintf("cron job %q was not created: %v", c.Name, err))
			continue
		}
		var err error
		if req.ConcurrencyPolicy, err = normalizeScheduledTaskConcurrencyPolicy(""); err != nil {
			warnings = append(warnings, fmt.Sprintf("cron job %q was not created: %v", c.Name, err))
			continue
		}
		id, err := randomScheduledTaskID()
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("cron job %q was not created: %v", c.Name, err))
			continue
		}
		now := time.Now().UTC()
		task := req.toTask(store.ScheduledTask{ID: id, ServiceName: app, Source: store.ScheduledTaskSourceAPI, CreatedAt: now, UpdatedAt: now})
		if err := a.rt.scheduledTasks.SaveScheduledTask(ctx, task); err != nil {
			warnings = append(warnings, fmt.Sprintf("cron job %q was not created: %v", c.Name, err))
			continue
		}
		made++
	}
	if made > 0 && a.sessionID != "" {
		warnings = append(warnings, fmt.Sprintf("%d scheduled job(s) were created disabled; enable them after cutover so they do not run twice", made))
	}
	return warnings
}
