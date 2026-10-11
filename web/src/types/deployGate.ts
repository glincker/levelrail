// GET /api/v1/apps/{name}/deploys/{deployId}/gate, /deploy-safety and
// /rollback-targets (internal/api/deploy_gate.go).

export type GatePhase =
  'readiness' | 'stability' | 'cutover' | 'drain' | 'watch' | 'rollback' | 'pin'

export type GateEventStatus = 'started' | 'passed' | 'failed' | 'info'

export type GateStage =
  | 'pending'
  | 'gating'
  | 'gate_failed'
  | 'watching'
  | 'cleared'
  | 'watch_failed'
  | 'rolled_back'

export interface GateEvent {
  phase: GatePhase
  status: GateEventStatus
  reason?: string
  message: string
  container?: string
  at: string
}

export interface DeploySafetySettings {
  success_threshold: number
  stable_window: string
  drain_timeout: string
  watch_window: string
  watch_max_restarts: number
  watch_error_rate: number
  watch_min_requests: number
  auto_rollback: boolean
  rollback_pins: number
}

export interface DeploySafetyOverrides {
  success_threshold: number | null
  stable_window: string | null
  drain_timeout: string | null
  watch_window: string | null
  watch_max_restarts: number | null
  watch_error_rate: number | null
  watch_min_requests: number | null
  auto_rollback: boolean | null
}

export interface DeploySafety {
  overrides: DeploySafetyOverrides
  defaults: DeploySafetySettings
  effective: DeploySafetySettings
}

export interface RollbackTarget {
  deploy_id: string
  image: string
  image_id?: string
  pin_ref: string
  pinned_at: string
  live: boolean
}

export interface DeployGate {
  deploy_id: string
  image: string
  status: string
  reason?: string
  error?: string
  stage: GateStage
  terminal: boolean
  failed: boolean
  superseded: boolean
  events: GateEvent[]
  settings: DeploySafetySettings
  rollback_pin: RollbackTarget | null
}
