// Wire types for internal/api/predeploy_snapshot.go.

export type PredeployMode = 'off' | 'on' | 'require'

export const PREDEPLOY_MODES: PredeployMode[] = ['off', 'on', 'require']

export interface PredeploySnapshotSettings {
  mode: PredeployMode
  target_id: string
}

export interface PredeploySnapshot {
  id: string
  deploy_id?: string
  prior_deploy_id?: string
  database_name: string
  engine?: string
  backup_id?: string
  status: string
  error?: string
  size_bytes: number
  created_at: string
  restored_database?: string
  restore_status?: string
  restore_error?: string
}

export interface RollbackWithDataResult {
  deploy_id: string
  rollback_to: string
  snapshots: PredeploySnapshot[]
}
