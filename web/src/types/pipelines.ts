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
  'push' | 'pull_request' | 'tag' | 'manual' | 'schedule' | 'api'

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
