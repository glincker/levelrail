// Wire types for scheduled jobs, matching internal/api/scheduled_tasks.go's
// scheduledTaskResource and scheduledTaskRunResource: same field names,
// same snake_case JSON tags.

export type ScheduledTaskRunStatus =
  | 'running'
  | 'success'
  | 'failed'
  | 'timeout'
  | 'container_not_running'
  | 'skipped_concurrency'
  | 'replaced'
  | 'missed'
  | 'interrupted'

// What happens when a job's next run finds the previous one still going.
export type ScheduledTaskConcurrencyPolicy =
  'allow' | 'forbid' | 'replace' | 'queue'

// fresh runs the command in a new container of the app's image with its
// env, volumes and network; exec runs it inside the running container.
export type ScheduledTaskMode = 'fresh' | 'exec' | 'http'

export type JobHttpMethod = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'

// The request an http job sends. A background job is awaited by the control
// plane under a longer timeout; the caller of run-now gets 202 either way.
export interface ScheduledTaskHttp {
  method?: JobHttpMethod
  path: string
  body?: string
  background?: boolean
}

// What to do about occurrences missed while the control plane was down.
export type ScheduledTaskCatchUp = 'skip' | 'run_once'

export type JobRunTrigger = 'schedule' | 'manual' | 'api'

// GET/POST/PUT /api/v1/apps/{name}/jobs response shape.
export interface ScheduledTask {
  id: string
  service_name: string
  name?: string
  mode?: ScheduledTaskMode
  command: string[]
  http?: ScheduledTaskHttp
  schedule: string
  timezone?: string
  enabled: boolean
  concurrency_policy: ScheduledTaskConcurrencyPolicy
  timeout_seconds?: number
  max_retries?: number
  retry_backoff_seconds?: number
  catch_up?: ScheduledTaskCatchUp
  catch_up_lookback_seconds?: number
  cpu_limit_millis?: number
  memory_limit_mb?: number
  warn_after_seconds?: number
  // 'spec' means declared in app.yaml and owned by it.
  source?: 'api' | 'spec'
  last_run_at?: string
  last_run_status?: ScheduledTaskRunStatus
  last_run_output?: string
  consecutive_failures: number
  consecutive_missed?: number
  running_since?: string
  next_run_at?: string
  // Set on the response to a run-now call.
  run_id?: string
  created_at: string
  updated_at: string
}

// POST/PUT /api/v1/apps/{name}/jobs request body. On PUT, a field left out
// keeps its stored value.
export interface ScheduledTaskRequest {
  name?: string
  mode?: ScheduledTaskMode
  command: string[]
  http?: ScheduledTaskHttp
  schedule: string
  timezone?: string
  enabled: boolean
  concurrency_policy?: ScheduledTaskConcurrencyPolicy
  timeout_seconds?: number
  max_retries?: number
  retry_backoff_seconds?: number
  catch_up?: ScheduledTaskCatchUp
  catch_up_lookback_seconds?: number
  cpu_limit_millis?: number
  memory_limit_mb?: number
  warn_after_seconds?: number
}

// One run of a job. output is only populated by the single-run endpoint.
export interface JobRun {
  id: string
  task_id: string
  job_name?: string
  service_name: string
  trigger: JobRunTrigger
  status: ScheduledTaskRunStatus
  scheduled_for?: string
  started_at: string
  finished_at?: string
  exit_code?: number
  duration_ms: number
  attempts: number
  node_id?: string
  mode?: ScheduledTaskMode
  message?: string
  output?: string
}
