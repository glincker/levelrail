// Wire types for GET /api/v1/apps/{name}/investigate and
// GET /api/v1/apps/{name}/failure-context.

export interface InvestigateSummary {
  has_traffic: boolean
  requests: number
  rate_per_sec: number
  error_rate_4xx: number
  error_rate_5xx: number
  p50_ms: number
  p95_ms: number
  p99_ms: number
}

export interface TopRoute {
  route: string
  host: string
  requests: number
  share: number
  error_rate_4xx: number
  error_rate_5xx: number
  avg_ms: number
}

export interface StatusCodeCount {
  status: number
  count: number
  share: number
}

export type TimelineKind =
  | 'deploy'
  | 'rollback'
  | 'config'
  | 'env'
  | 'secret'
  | 'domain'
  | 'scale'
  | 'loadbalancer'
  | 'freeze'
  | 'maintenance'
  | 'lifecycle'
  | 'restart'
  | 'saturation'
  | 'alert'

export type TimelineSeverity = 'info' | 'warning' | 'critical'

export interface TimelineEvent {
  at: string
  kind: TimelineKind
  severity: TimelineSeverity
  title: string
  detail?: string
  ref?: string
  likely_cause?: boolean
}

export interface InvestigateResponse {
  app: string
  from: string
  to: string
  step_seconds: number
  summary: InvestigateSummary
  baseline: InvestigateSummary
  routes_available: boolean
  top_routes: TopRoute[]
  status_codes: StatusCodeCount[]
  timeline: TimelineEvent[]
  logs: { from: string; to: string }
}

export type FailureState = 'healthy' | 'crashlooping' | 'deploy_failed'

export interface FailureLogLine {
  timestamp: string
  stream: string
  message: string
  level?: string
}

export interface FailureContext {
  state: FailureState
  since?: string
  restarts_in_window: number
  window_seconds: number
  container_id?: string
  deploy?: {
    id: string
    status: string
    image: string
    commit?: string
    started_at: string
    finished_at?: string
    error?: string
  }
  lines: FailureLogLine[]
  total_lines: number
}
