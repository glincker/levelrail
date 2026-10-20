// Wire types for the backup verification sub-resource of one backup
// attempt, matching internal/api/backup_verify.go's
// backupVerificationResource. One row per re-download-and-check attempt
// against a given backup_history row: the "is this backup still
// trustworthy" counterpart to types/restoreHistory.ts's
// RestoreHistoryRecord ("does this backup still work as a real restore
// source"), deliberately never a live restore itself (see
// internal/backup.VerifyRunner's own doc comment).

export type BackupVerificationStatus = 'running' | 'passed' | 'failed'

export interface BackupVerificationRecord {
  id: string
  backup_history_id: string
  status: BackupVerificationStatus
  checksum_match: boolean
  size_match: boolean
  format_valid: boolean
  downloaded_bytes: number
  error?: string
  checked_by?: string
  started_at: string
  finished_at?: string
}
