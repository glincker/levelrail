// GET /api/v1/apps/{name}/deploys/{deployId}/gate, /deploy-safety and
// /rollback-targets (internal/api/deploy_gate.go).

export type GatePhase =
  'readiness' | 'stability' | 'cutover' | 'drain' | 'watch' | 'rollback' | 'pin'

export type GateEventStatus =
  'started' | 'passed' | 'failed' | 'info' | 'stopped'

export type WatchMode = 'off' | 'notify' | 'auto'

export type WatchState =
  | 'off'
  | 'waiting'
  | 'watching'
  | 'cleared'
  | 'tripped'
  | 'rolled_back'
  | 'stopped'

export type GateStage =
  | 'pending'
  | 'gating'
  | 'gate_failed'
  | 'watching'
  | 'cleared'
  | 'watch_failed'
  | 'rolled_back'
  | 'watch_stopped'

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
  watch_mode: WatchMode
  watch_latency_ratio: number
  watch_latency_min_delta: string
  watch_soft_ticks: number
  watch_soft_rollback: boolean
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
  watch_mode: WatchMode | null
  watch_latency_ratio: number | null
  watch_soft_rollback: boolean | null
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
  watch: WatchStatus
}

export interface WatchStatus {
  state: WatchState
  mode: WatchMode
  started_at?: string
  ends_at?: string
  remaining_seconds: number
  window_seconds: number
  reason?: string
  message?: string
  notice?: string
}
