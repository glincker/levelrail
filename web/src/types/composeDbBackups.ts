// Wire types for internal/api/compose_db_backups.go.

export interface ComposeDbService {
  service: string
  engine: string
  image: string
  scheduled: boolean
  schedule?: string
  target_id?: string
  retain?: number
  retain_days?: number
  enabled: boolean
}

export interface SetComposeDbScheduleRequest {
  target_id: string
  schedule: string
  retain?: number
  retain_days?: number
  enabled?: boolean
}

export type ComposeDbRunStatus = string

export interface ComposeDbBackupRun {
  id: string
  service: string
  target_id: string
  status: ComposeDbRunStatus
  object_key?: string
  size_bytes: number
  checksum?: string
  error?: string
  started_at: string
  finished_at?: string
}
