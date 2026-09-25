// Wire types for storage destinations and log archive, matching
// internal/api/storage_destinations.go and internal/api/log_archive.go.
// Credentials are write-only: they appear on the create request and never
// in a response.

export type StoragePreset = 'aws' | 'r2' | 'b2' | 'minio' | 'wasabi' | 'custom'

export interface StorageProvider {
  id: StoragePreset
  label: string
  endpoint_template?: string
  default_region?: string
  path_style: boolean
  needs_account_id: boolean
  needs_region: boolean
  needs_endpoint: boolean
}

export interface StorageDestination {
  id: string
  name: string
  preset: StoragePreset
  provider: string
  endpoint?: string
  region?: string
  bucket: string
  path_style: boolean
  account_id?: string
  created_at: string
  archive_policies: number
}

export interface CreateStorageDestinationRequest {
  name: string
  preset: StoragePreset
  endpoint?: string
  region?: string
  bucket: string
  account_id?: string
  path_style?: boolean
  access_key_id: string
  secret_access_key: string
  verify: boolean
}

export interface StorageProbeStep {
  name: string
  ok: boolean
  error?: string
}

export interface StorageProbeResult {
  ok: boolean
  reason?: string
  message?: string
  steps: StorageProbeStep[]
}

export interface LogArchivePolicy {
  id: string
  app_name: string
  target_id: string
  enabled: boolean
  interval: string
  interval_seconds: number
  retention_days: number
  last_run_at?: string
  last_success_at?: string
  last_error?: string
  created_at: string
}

export interface SetLogArchivePolicyRequest {
  app_name: string
  target_id: string
  interval: string
  retention_days: number
  enabled: boolean
}

export type LogArchiveRunStatus = 'running' | 'succeeded' | 'failed'

export interface LogArchiveRun {
  id: string
  app_name: string
  target_id: string
  kind: 'scheduled' | 'manual'
  from: string
  to: string
  status: LogArchiveRunStatus
  objects: number
  lines: number
  bytes: number
  error?: string
  started_at: string
  finished_at?: string
}

export interface LogArchiveDumpRequest {
  app_name: string
  target_id: string
  from: string
  to: string
}

export interface LogArchiveObject {
  key: string
  size: number
  last_modified: string
}

export interface LogArchiveObjectsPage {
  objects: LogArchiveObject[]
  next?: string
}
