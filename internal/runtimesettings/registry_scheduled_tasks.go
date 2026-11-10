package runtimesettings

// KeyScheduledTasksHistoryKeep caps the run history rows kept per scheduled task.
const KeyScheduledTasksHistoryKeep = "scheduled_tasks.history_keep"

// KeyScheduledTasksMaxOutputBytes bounds the output tail kept per run.
const KeyScheduledTasksMaxOutputBytes = "scheduled_tasks.max_output_bytes"

const scheduledTasksHistoryCeiling = 10000

const scheduledTasksOutputFloor, scheduledTasksOutputCeiling = 1024, 1 << 20

func init() {
	Register(Definition{
		Key: KeyScheduledTasksHistoryKeep, Category: CategoryRetention, Kind: KindInt, Default: "200",
		Env: "APP_JOBS_HISTORY_KEEP", Apply: ApplyLive, CheckValue: intRangeCheck(1, scheduledTasksHistoryCeiling),
		Title:       "Scheduled task runs kept",
		Description: "How many recent runs of each scheduled task keep their history (status, exit code, duration, output), at least 1. Older runs are removed after the next run finishes.",
	})
	Register(Definition{
		Key: KeyScheduledTasksMaxOutputBytes, Category: CategoryRetention, Kind: KindInt, Default: "65536",
		Env: "APP_JOBS_MAX_OUTPUT_BYTES", Apply: ApplyLive, CheckValue: intRangeCheck(scheduledTasksOutputFloor, scheduledTasksOutputCeiling),
		Title:       "Scheduled task output kept per run",
		Description: "How many bytes of output each scheduled task or compose schedule run keeps (the tail, where errors land), between 1024 and 1048576.",
	})
}
