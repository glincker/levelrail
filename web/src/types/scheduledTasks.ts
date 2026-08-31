// Wire types for the scheduled-task resource, matching
// internal/api/scheduled_tasks.go's scheduledTaskResource exactly: same
// field names, same snake_case JSON tags.

export type ScheduledTaskRunStatus =
  | 'success'
  | 'failed'
  | 'timeout'
  | 'container_not_running'

// GET/POST/PUT /api/v1/apps/{name}/scheduled-tasks response shape.
// last_run_* fields are absent until the task has run at least once
// (scheduled or manual "run now"), matching store.ScheduledTask's own
// LastRunAt nil-until-first-run convention.
export interface ScheduledTask {
  id: string
  service_name: string
  command: string[]
  schedule: string
  enabled: boolean
  last_run_at?: string
  last_run_status?: ScheduledTaskRunStatus
  last_run_output?: string
  // consecutive_failures is what a kind=scheduled_task_failure alert
  // rule (see types/alerts.ts) watches: runs in a row that were not
  // "success", reset to 0 on the next success.
  consecutive_failures: number
  created_at: string
  updated_at: string
}

// POST/PUT /api/v1/apps/{name}/scheduled-tasks request body. No `id` or
// `service_name`: the server mints the former and derives the latter
// from the app name in the URL, discarding anything a caller puts in
// those fields on the wire type above.
export interface ScheduledTaskRequest {
  command: string[]
  schedule: string
  enabled: boolean
}
