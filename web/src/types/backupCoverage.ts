export type CoverageState =
  'verified' | 'unverified' | 'never_ran' | 'failing' | 'unprotected'

export type CoverageAction = 'protect' | 'backup_now' | 'restore'

export interface CoverageRow {
  kind: 'database' | 'volume'
  app_name?: string
  name: string
  scratch?: boolean
  state: CoverageState
  reason?: string
  primary_action: CoverageAction
  schedule?: string
  target_id?: string
  next_run?: string
  last_backup_at?: string
  last_backup_id?: string
  last_verified_at?: string
  total_bytes: number
  backup_count: number
  encrypted: boolean
  protection_level?: string
}

export interface CoverageSummary {
  total: number
  protected: number
  verified: number
  unverified: number
  never_ran: number
  failing: number
  unprotected: number
  scratch: number
  storage_bytes: number
  last_verified_restore?: string
  next_run?: string
}

export interface CoverageTarget {
  id: string
  name: string
  provider: string
}

export interface BackupDefaults {
  schedule: string
  retain: number
  retain_days: number
}

export interface CoverageEncryption {
  enabled: boolean
  recipient?: string
  codec?: string
  key_stored_off_box: boolean
  acknowledged_at?: string
}

export interface BackupCoverage {
  summary: CoverageSummary
  resources: CoverageRow[]
  targets: CoverageTarget[]
  defaults: BackupDefaults
  encryption: CoverageEncryption
}

export interface ProtectPlanItem {
  kind: 'database' | 'volume'
  app_name?: string
  name: string
  scratch?: boolean
}

export interface ProtectAllRequest {
  target_id: string
  schedule?: string
  retain?: number
  retain_days?: number
  include_scratch?: boolean
  /** Narrows the plan to these resources, for one row's Protect. */
  only?: ProtectPlanItem[]
  apply: boolean
}

export interface ProtectAllResponse {
  applied: boolean
  target_id: string
  schedule: string
  retain: number
  retain_days: number
  plan: ProtectPlanItem[]
  done?: ProtectPlanItem[]
  failed?: (ProtectPlanItem & { error: string })[]
}

export interface StorageOrphan {
  id: string
  name: string
  size_bytes?: number
  created_at?: string
  age_days?: number
  belonged_to: {
    kind: string
    app_name?: string
    name?: string
    role: string
  }
  backup_count: number
  last_backup?: { id: string; at: string; size_bytes: number }
}

export interface StorageOrphans {
  orphans: StorageOrphan[]
  total_bytes: number
  threshold_bytes: number
}
