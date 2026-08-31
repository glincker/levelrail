// Wire types for the "restore as new volume" sub-resource of an app
// service volume, matching internal/api/app_volume_clone_restore.go's
// volumeCloneRestoreResource. The non-destructive counterpart to
// types/restoreHistory.ts's RestoreHistoryRecord for a volume: this
// restores a backup into a brand-new, standalone Docker volume rather than
// overwriting the one it came from, so each row names both a source
// service/volume and a new volume name instead of just one.

export type VolumeCloneRestoreStatus = 'running' | 'succeeded' | 'failed'

export interface VolumeCloneRestoreRecord {
  id: string
  source_service_name: string
  source_volume_name: string
  new_volume_name: string
  backup_history_id: string
  status: VolumeCloneRestoreStatus
  error?: string
  started_at: string
  finished_at?: string
}

export interface TriggerVolumeCloneRestoreRequest {
  backup_id: string
  new_volume_name?: string
}
