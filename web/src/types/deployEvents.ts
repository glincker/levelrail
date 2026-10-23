export type DeployEventKind =
  | 'trigger'
  | 'build'
  | 'log'
  | 'image_push'
  | 'hook'
  | 'step'
  | 'readiness'
  | 'cutover'
  | 'watch'
  | 'rollback'
  | 'outcome'
  | 'traffic_impact'

export type DeployEventStatus = 'running' | 'ok' | 'failed' | 'warning' | 'info'

export type DeployVerdict =
  | 'running'
  | 'watching'
  | 'succeeded'
  | 'failed'
  | 'rolled_back'
  | 'canceled'
  | 'queued'

export interface DeployEventTrigger {
  source: string
  actor?: string
  actor_kind?: string
  branch?: string
  commit_sha?: string
  commit_message?: string
  author?: string
  rerun_of?: string
}

export interface DeployEventWindow {
  has_traffic: boolean
  requests: number
  rate_per_sec: number
  error_rate_5xx: number
  p95_ms: number
}

export interface DeployEventImpact {
  cutover_at: string
  window_ms: number
  complete: boolean
  before: DeployEventWindow
  after: DeployEventWindow
}

export interface DeployEvent {
  id: string
  at: string
  kind: DeployEventKind
  phase: string
  status: DeployEventStatus
  code: string
  title: string
  detail?: string
  reason?: string
  service?: string
  stream?: string
  duration_ms?: number
  trigger?: DeployEventTrigger
  impact?: DeployEventImpact
}

export interface DeployEventPhase {
  phase: string
  started_at: string
  ended_at: string
  duration_ms: number
}

export interface DeployEventTimeline {
  deploy_id: string
  app: string
  verdict: DeployVerdict
  live: boolean
  frozen: boolean
  started_at: string
  finished_at?: string
  trigger: DeployEventTrigger
  events: DeployEvent[]
  phases: DeployEventPhase[]
  impact?: DeployEventImpact
  logs_dropped?: number
}

export interface DeployEventPhaseDelta {
  phase: string
  a_ms?: number
  b_ms?: number
  delta_ms?: number
}

export interface DeployEventComparison {
  a: DeployEventTimeline
  b: DeployEventTimeline
  phases: DeployEventPhaseDelta[]
  only_a: string[]
  only_b: string[]
}

export type DeployEventMessage =
  | { type: 'snapshot'; timeline: DeployEventTimeline; frozen?: boolean }
  | { type: 'event'; event: DeployEvent; live: boolean }
  | {
      type: 'state'
      verdict: DeployVerdict
      live: boolean
      phases?: DeployEventPhase[]
    }
  | { type: 'end'; verdict?: DeployVerdict; frozen?: boolean }
