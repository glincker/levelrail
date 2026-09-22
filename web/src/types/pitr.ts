// Wire types for point-in-time restore, matching internal/api/pitr.go's
// resource shapes exactly: PITRStatus (GET .../pitr), BaseBackupHistoryRecord
// (GET/POST .../base-backups), and PITRRestoreHistoryRecord
// (GET/POST .../pitr-restore(s)). Status literals reuse BackupStatus/
// RestoreStatus's own three-value union (types/backupHistory.ts,
// types/restoreHistory.ts), the same "one attempt lifecycle, several
// resources" reasoning backupAttemptStatus.tsx's AttemptStatus already
// documents.

import type { BackupStatus } from './backupHistory'

export interface PITRStatus {
  enabled: boolean
  enabled_at?: string
  has_base_backup: boolean
  // window_start/window_end are both set only when has_base_backup is
  // true and the live window could be computed (window_error empty).
  window_start?: string
  window_end?: string
  // window_error is set whenever the currently recoverable window could
  // not be computed just now (most commonly: the database's own
  // container isn't running), independent of enabled/has_base_backup
  // still being real, useful information on their own.
  window_error?: string
}

export interface BaseBackupHistoryRecord {
  id: string
  database_name: string
  target_id: string
  lsn?: string
  size_bytes: number
  status: BackupStatus
  error?: string
  started_at: string
  finished_at?: string
}

export interface PITRRestoreHistoryRecord {
  id: string
  database_name: string
  base_backup_history_id: string
  target_timestamp: string
  status: BackupStatus
  error?: string
  started_at: string
  finished_at?: string
}

export interface TriggerPITRRestoreRequest {
  base_backup_id: string
  target_time: string
}
