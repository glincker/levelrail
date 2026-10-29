// Wire types for internal/api/backup_copies.go.

export type BackupCopyStatus = 'pending' | 'copying' | 'succeeded' | 'failed'

export interface BackupCopySettings {
  enabled: boolean
  target_id: string
  target_name?: string
}

export interface BackupCopy {
  backup_id: string
  resource: string
  resource_kind: string
  source_target_id: string
  target_id: string
  status: BackupCopyStatus
  attempts: number
  error?: string
  size_bytes: number
  backup_finished_at: string
  copied_at?: string
  lag_seconds: number
}
