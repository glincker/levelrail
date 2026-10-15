// Wire shapes of /api/v1/resource-advisor (internal/resourceadvisor types).

export type AdvisorSeverity = 'info' | 'suggest' | 'warn'
export type AdvisorResourceKind = 'app' | 'database' | 'node'
export type AdvisorActionType =
  | 'set_memory_limit'
  | 'set_cpu_limit'
  | 'enable_sleep'
  | 'prune_docker'
  | 'manual'

export interface AdvisorRef {
  kind: AdvisorResourceKind
  name: string
}

export interface AdvisorPoint {
  t: string
  v: number
}

export interface AdvisorEvidence {
  window: string
  window_seconds: number
  samples: number
  unit?: 'bytes' | 'nano_cpus' | 'requests'
  p50?: number
  p95?: number
  p99?: number
  max?: number
  limit?: number
  numbers?: Record<string, number>
  series?: AdvisorPoint[]
}

export interface AdvisorImpact {
  saving_memory_bytes?: number
  saving_nano_cpus?: number
  saving_disk_bytes?: number
  exhaustion_at?: string
  risk?: string
}

export interface AdvisorAction {
  type: AdvisorActionType
  value?: number
  current?: number
  settings?: Record<string, string>
  instructions?: string
}

export interface AdvisorFinding {
  id: string
  kind: string
  severity: AdvisorSeverity
  resource: AdvisorRef
  title: string
  why: string
  evidence: AdvisorEvidence
  impact: AdvisorImpact
  action?: AdvisorAction
  params?: Record<string, string>
  state?: 'dismissed' | 'snoozed'
  snoozed_until?: string
  appliable: boolean
  can_apply: boolean
}

export interface AdvisorFindings {
  generated_at: string
  findings: AdvisorFinding[]
}

export interface AdvisorSummary {
  generated_at: string
  total: number
  by_severity: Partial<Record<AdvisorSeverity, number>>
  by_kind: Record<string, number>
  actionable: number
  pending_data: number
  saving_memory_bytes: number
  saving_disk_bytes: number
  top_savings: AdvisorFinding[]
}

export interface AdvisorSettings {
  memory_bytes: number
  nano_cpus: number
  sleep_idle_minutes?: number
}

export interface AdvisorApplyResult {
  finding_id: string
  resource: AdvisorRef
  action: AdvisorAction
  before: AdvisorSettings
  after: AdvisorSettings
  confirm_required: boolean
  applied: boolean
  applied_live: boolean
}

export type BetaMaturity = 'beta' | 'experimental'

export interface BetaFeature {
  id: string
  name: string
  summary: string
  enabled: boolean
  enabled_by: 'env' | 'setting' | 'none'
  toggleable: boolean
  requires_restart: boolean
  env_hint: string
  docs_path: string
  maturity: BetaMaturity
  locked: boolean
}
