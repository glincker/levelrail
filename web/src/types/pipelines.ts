// Wire types for the CI/CD pipeline resources, matching
// internal/api/pipelines.go's resource structs exactly (snake_case JSON).

export type PipelineStatus =
  | 'queued'
  | 'pending'
  | 'running'
  | 'waiting_approval'
  | 'succeeded'
  | 'failed'
  | 'cancelled'
  | 'skipped'

export type PipelineTrigger =
  | 'push'
  | 'pull_request'
  | 'merge_group'
  | 'tag'
  | 'manual'
  | 'schedule'
  | 'api'

export interface PipelineRunBrief {
  id: string
  number: number
  status: PipelineStatus
}

export interface Pipeline {
  id: string
  app: string
  name: string
  source: string
  enabled: boolean
  yaml?: string
  triggers?: PipelineTrigger[]
  jobs: number
  last_run?: PipelineRunBrief
  created_at: string
  updated_at: string
  source_sha?: string
  diverged?: boolean
}

export interface PipelineStep {
  index: number
  name: string
  kind: string
  status: PipelineStatus
  reason?: string
  exit_code?: number
  attempt: number
  started_at?: string
  finished_at?: string
}

export interface PipelineJob {
  key: string
  name: string
  stage?: string
  needs: string[]
  matrix?: Record<string, string>
  node_id?: string
  status: PipelineStatus
  reason?: string
  attempt: number
  outputs?: Record<string, string>
  started_at?: string
  finished_at?: string
  steps: PipelineStep[]
}

export interface PipelineApproval {
  id: number
  job: string
  step_index: number
  message: string
  required_ability: string
  decision?: 'approved' | 'rejected'
  decided_by?: string
  comment?: string
  expires_at?: string
  created_at: string
  decided_at?: string
}

export interface PipelineRun {
  id: string
  pipeline_id: string
  pipeline_name: string
  app: string
  number: number
  trigger: PipelineTrigger
  actor?: string
  ref?: string
  commit_sha?: string
  inputs?: Record<string, string>
  status: PipelineStatus
  reason?: string
  created_at: string
  started_at?: string
  finished_at?: string
  jobs?: PipelineJob[]
  approvals?: PipelineApproval[]
  hold?: PipelineHold
  report?: PipelineRunReport
}

// PipelineRunReport is the run's commit status as posted to the git forge.
// url is the forge's page for the commit; warning says why a post failed
// (a failed post never fails the run).
export interface PipelineRunReport {
  provider?: string
  state?: string
  url?: string
  warning?: string
}

export interface PipelineFilters {
  paths: string[]
  paths_ignore: string[]
  report_status: boolean
}

export interface PipelineFiltersRequest {
  yaml: string
  paths?: string[]
  paths_ignore?: string[]
  report_status?: boolean
}

export interface PipelineHold {
  state: 'pending' | 'approved' | 'rejected'
  reason: string
  by?: string
  at?: string
}

export interface PipelineSyncStatus {
  connected: boolean
  repo_is_truth: boolean
  last_sha?: string
  last_synced_at?: string
  last_error?: string
}

export type PipelineSyncOutcome =
  'created' | 'updated' | 'unchanged' | 'diverged' | 'invalid' | 'refused'

export interface PipelineSyncItem {
  file: string
  name: string
  outcome: PipelineSyncOutcome
  message?: string
}

export interface PipelineSyncResult {
  sha: string
  dir?: string
  items: PipelineSyncItem[]
}

export interface PipelineTriggerDecision {
  id: number
  pipeline?: string
  event: string
  ref?: string
  sha?: string
  decision: 'started' | 'held' | 'skipped' | 'failed'
  reason: string
  run_id?: string
  created_at: string
}

export interface PipelineIssue {
  path: string
  line: number
  message: string
}

export interface PipelineValidation {
  valid: boolean
  issues: PipelineIssue[]
  triggers?: PipelineTrigger[]
  jobs?: number
  filters?: PipelineFilters
}

export interface PipelineSaveRequest {
  name?: string
  yaml: string
  enabled?: boolean
}

export interface PipelineStartRequest {
  ref?: string
  sha?: string
  inputs?: Record<string, string>
}

export interface PipelineLogLine {
  id: number
  job: string
  step: number
  stream: 'stdout' | 'stderr'
  line: string
  time: string
}
