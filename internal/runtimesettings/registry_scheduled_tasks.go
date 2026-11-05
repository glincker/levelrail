package runtimesettings

// KeyScheduledTasksHistoryKeep caps the run history rows kept per scheduled task.
const KeyScheduledTasksHistoryKeep = "scheduled_tasks.history_keep"

const scheduledTasksHistoryCeiling = 10000

func init() {
	Register(Definition{
		Key: KeyScheduledTasksHistoryKeep, Category: CategoryRetention, Kind: KindInt, Default: "200",
		Env: "APP_JOBS_HISTORY_KEEP", Apply: ApplyLive, CheckValue: intRangeCheck(1, scheduledTasksHistoryCeiling),
		Title:       "Scheduled task runs kept",
		Description: "How many recent runs of each scheduled task keep their history (status, exit code, duration, output), at least 1. Older runs are removed after the next run finishes.",
	})
}
