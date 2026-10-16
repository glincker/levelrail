// Wire types for internal/api/data_export.go. Header values and the signing
// key are write-only: they appear on requests and never on responses.

export type DataExportKind =
  | 'logs_http'
  | 'logs_loki'
  | 'logs_otlp'
  | 'logs_syslog'
  | 'logs_newrelic'
  | 'logs_datadog'
  | 'logs_axiom'
  | 'logs_betterstack'
  | 'audit_webhook'
  | 'audit_s3'
  | 'metrics_otlp'
  | 'metrics_remote_write'

export interface DataExportOptions {
  storage_target_id?: string
  prefix?: string
  labels?: Record<string, string>
  interval_seconds?: number
  backfill?: boolean
  region?: string
  site?: string
  dataset?: string
  username?: string
}

export interface DataExportStatus {
  last_success_at: string
  last_error: string
  last_error_at: string
  sent_total: number
  dropped_total: number
  failures_total: number
  queue_depth: number
  circuit_open: boolean
  cursor_at?: string
}

export interface DataExportDestination {
  id: string
  name: string
  kind: DataExportKind
  endpoint: string
  enabled: boolean
  apps: string[]
  min_level: string
  options: DataExportOptions
  header_names: string[]
  has_hmac_key: boolean
  has_api_key?: boolean
  status: DataExportStatus
  created_at: string
  updated_at: string
}

export interface CreateDataExportDestinationRequest {
  name: string
  kind: DataExportKind
  endpoint?: string
  enabled?: boolean
  apps?: string[]
  min_level?: string
  options?: DataExportOptions
  headers?: Record<string, string>
  hmac_key?: string
  api_key?: string
}

export interface UpdateDataExportDestinationRequest {
  enabled?: boolean
  headers?: Record<string, string>
  hmac_key?: string
  api_key?: string
}

export interface DataExportTestResult {
  ok: boolean
  error?: string
}
