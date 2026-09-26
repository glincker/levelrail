export const DEPLOYMENT_STATUSES = [
  'building',
  'queued',
  'ready',
  'failed',
  'canceled',
  'held',
  'awaiting_approval',
  'rolled_back',
  'superseded',
] as const

export type DeploymentStatus = (typeof DEPLOYMENT_STATUSES)[number]

export const DEPLOYMENT_TRIGGERS = [
  'git push',
  'manual',
  'rollback',
  'api',
  'preview',
  'schedule',
  'pipeline',
] as const

export interface DeploymentSteps {
  done: number
  running: number
  failed: number
  failing_step?: string
}

/** Wire shape of one row from GET /api/v1/deployments. */
export interface Deployment {
  id: string
  app: string
  status: DeploymentStatus
  trigger: string
  environment: string
  image: string
  image_ref: string
  image_digest: string
  digest_reason: string
  rollout_state: string
  commit_sha: string
  branch: string
  commit_message: string
  author: string
  pr_number: number | null
  started_at: string
  finished_at: string | null
  duration_ms: number | null
  steps: DeploymentSteps | null
  error_summary: string | null
  reason_code: string
  reason: string
  rollback_of: string | null
  rolled_back_by: string | null
  superseded_by: string | null
  is_live: boolean
  approval_id: string | null
  preview_image_url: string | null
}

export interface DeploymentListPage {
  items: Deployment[]
  next_cursor: string
}

export interface DeploymentDay {
  date: string
  total: number
  failed: number
}

export interface DeploymentsSummary {
  window: string
  counts: Record<string, number>
  in_progress: number
  needs_attention: number
  failure_rate_24h: number | null
  duration: {
    median_ms: number | null
    p95_ms: number | null
    samples: number
  }
  per_day: DeploymentDay[]
}

export type DeploymentEventType = 'created' | 'step' | 'finished'

export interface DeploymentEvent {
  type: DeploymentEventType
  step?: { name: string; status: string }
  deployment: Deployment
}
