// Wire types for the "restore as new database" sub-resource of a
// database, matching internal/api/database_clone_restore.go's
// cloneRestoreResource. The non-destructive counterpart to
// types/restoreHistory.ts's RestoreHistoryRecord: this restores a backup
// into a brand-new database rather than overwriting the one it came
// from, so each row names both a source and a new database instead of
// just one.

export type CloneRestoreStatus = 'running' | 'succeeded' | 'failed'

export interface CloneRestoreRecord {
  id: string
  source_database_name: string
  new_database_name: string
  backup_history_id: string
  status: CloneRestoreStatus
  error?: string
  started_at: string
  finished_at?: string
}

export interface TriggerCloneRestoreRequest {
  backup_id: string
  new_name: string
  version?: string
  project_id?: string
}
