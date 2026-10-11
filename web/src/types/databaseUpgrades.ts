export type UpgradeSupport = 'supported' | 'eol_soon' | 'eol' | 'unknown'
export type UpgradeKind = 'patch' | 'minor' | 'major'
export type AutoUpgradeLevel = 'off' | 'patch' | 'minor'
export type UpgradeRunState =
  | 'pending'
  | 'backing_up'
  | 'upgrading'
  | 'verifying'
  | 'succeeded'
  | 'reverted'
  | 'failed'
export type UpgradeRevertPath = 'image' | 'volume_snapshot' | 'backup_restore'

export interface UpgradeTarget {
  version: string
  kind: UpgradeKind
  security: boolean
  advisories?: string[]
  eol?: string
  automatic: boolean
}

export interface UpgradeAdvice {
  engine: string
  current: string
  comparable: boolean
  floating: boolean
  line?: string
  eol?: string
  support: UpgradeSupport
  advisories?: string[]
  targets: UpgradeTarget[]
  auto_max: 'patch' | 'minor' | 'none'
  image_revert: boolean
  manual_reason?: string
  notes?: string
  note?: string
  catalog_updated: string
}

export interface UpgradePolicy {
  auto_upgrade: AutoUpgradeLevel
  window_cron: string
  window_duration_seconds: number
  window_timezone: string
  backup_before: boolean
  verify_after: boolean
  revert_on_failure: boolean
  notify: string[]
  inherited: boolean
}

export interface UpgradePolicyInput {
  inherit?: boolean
  auto_upgrade: AutoUpgradeLevel
  window_cron: string
  window_duration_seconds: number
  window_timezone: string
  verify_after: boolean
  revert_on_failure: boolean
  notify: string[]
}

export interface UpgradeRun {
  id: string
  database_name: string
  engine: string
  from_version: string
  to_version: string
  kind: string
  source: 'auto' | 'manual'
  state: UpgradeRunState
  phase?: string
  verify_after: boolean
  revert_on_failure: boolean
  backup_id?: string
  verification_id?: string
  from_image_digest?: string
  snapshot_volume?: string
  revert_path?: UpgradeRevertPath
  reason?: string
  requested_by?: string
  timings: Record<string, string>
  created_at: string
  finished_at?: string
}

export interface DatabaseUpgrades {
  database: string
  engine: string
  version: string
  advice: UpgradeAdvice
  policy: UpgradePolicy
  window_open: boolean
  next_window?: string
  next_target?: UpgradeTarget
  blockers: string[]
  active?: UpgradeRun
  history: UpgradeRun[]
}

export interface UpgradeNowInput {
  version: string
  confirm: string
}
