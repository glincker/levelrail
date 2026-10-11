// Wire types for backup health, restore drills and volume backup policy,
// matching internal/api/backup_protection.go.

export type BackupHealthState =
  'healthy' | 'warning' | 'failing' | 'unverified' | 'unprotected'

export interface BackupSummary {
  id: string
  at: string
  status: string
  size_bytes: number
  codec?: string
  error?: string
}

export interface DrillSummary {
  id: string
  at: string
  status: string
  stage?: string
  error?: string
  trigger?: string
  files?: number
}

export interface ResourceHealth {
  kind: 'database' | 'volume'
  app_name?: string
  resource_name: string
  target_id?: string
  schedule?: string
  next_run?: string
  backup_count: number
  total_bytes: number
  last_attempt?: BackupSummary
  last_backup?: BackupSummary
  last_verified_restore?: DrillSummary
  last_drill?: DrillSummary
  encrypted: boolean
  state: BackupHealthState
  state_reason?: string
  protection_level?: 'locked' | 'versioned' | 'open'
  warning?: string
}

export interface TargetProtection {
  target_id: string
  level: 'locked' | 'versioned' | 'open'
  object_lock: boolean
  lock_mode?: string
  versioning?: string
  can_delete: boolean
  warning?: string
  probe_error?: string
  checked_at: string
}

export interface BackupHealth {
  resources: ResourceHealth[]
  targets: TargetProtection[]
  encryption: { enabled: boolean; recipient?: string; codec: string }
}

export interface BackupDrill {
  id: string
  backup_id: string
  resource_kind: 'database' | 'volume'
  database_name?: string
  service_name?: string
  volume_name?: string
  trigger: 'manual' | 'scheduled'
  status: 'running' | 'passed' | 'failed'
  stage: string
  object_ok: boolean
  checksum_ok: boolean
  restore_ok: boolean
  content_ok: boolean
  files: number
  bytes: number
  duration_ms: number
  error?: string
  started_at: string
  finished_at?: string
}

export interface VolumeBackupPolicy {
  service_name: string
  volume_name: string
  retain_daily: number
  retain_weekly: number
  retain_monthly: number
  pre_hook: string
  post_hook: string
  quiesce: '' | 'pause'
}

export interface VolumeRestoreToRequest {
  backup_id: string
  new_volume_name?: string
  target_app?: string
  node_id?: string
}

export interface VolumeRestoreToResponse {
  id: string
  new_volume_name: string
  node_id: string
}
