// Wire types for app-level scheduled tasks, matching
// internal/api/scheduled_tasks.go's scheduledTaskResource/
// scheduledTaskRequest/scheduledTaskRunResource exactly.

export type ScheduledTaskRunStatus = 'running' | 'succeeded' | 'failed'

export interface ScheduledTask {
  id: string
  service_name: string
  name: string
  command: string
  schedule: string
  enabled: boolean
  timeout_seconds?: number
  created_at: string
  updated_at: string
}

// POST/PUT request body: a full replace of every editable field (no
// `id`, no `service_name`, both server-assigned/derived from the URL).
export interface ScheduledTaskRequest {
  name: string
  command: string
  schedule: string
  enabled: boolean
  timeout_seconds?: number
}

export interface ScheduledTaskRun {
  id: string
  scheduled_task_id: string
  status: ScheduledTaskRunStatus
  exit_code: number
  output?: string
  error?: string
  started_at: string
  finished_at?: string
}
