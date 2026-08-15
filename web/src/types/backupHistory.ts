// Wire types for the backup history sub-resource of a database, matching
// internal/api/backups.go's backupHistoryResource. One row per dump-and-
// upload attempt against a given target, not the destination itself
// (that's types/backupTarget.ts's BackupTarget).
//
// object_key/size_bytes are typed optional even though a finished record
// always carries both: the POST trigger response (handleTriggerBackup's
// own 202 body) is a placeholder row with status "running", written
// before the dump or upload has happened, so neither field exists on the
// wire yet at that point, the same "not filled in until later" shape
// finished_at already has for that same placeholder.

export type BackupStatus = 'running' | 'succeeded' | 'failed'

export interface BackupHistoryRecord {
  id: string
  database_name: string
  target_id: string
  object_key?: string
  size_bytes?: number
  status: BackupStatus
  error?: string
  started_at: string
  finished_at?: string
}

export interface TriggerBackupRequest {
  target_id: string
}
