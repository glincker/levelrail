// Wire types for the restore history sub-resource of a database, matching
// internal/api/restore.go's restoreHistoryResource. One row per
// download-and-apply attempt against a given backup, the restore
// direction's counterpart to types/backupHistory.ts's BackupHistoryRecord.
//
// No object_key/size_bytes here, unlike BackupHistoryRecord: nothing is
// uploaded on a restore, so there is no equivalent byte count or object
// key to report, only which backup_history_id it restored from.

export type RestoreStatus = 'running' | 'succeeded' | 'failed'

export interface RestoreHistoryRecord {
  id: string
  database_name: string
  backup_history_id: string
  status: RestoreStatus
  error?: string
  started_at: string
  finished_at?: string
}

export interface TriggerRestoreRequest {
  backup_id: string
}
