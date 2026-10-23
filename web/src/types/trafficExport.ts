// Wire types for internal/api/traffic_export.go. Credentials are write-only:
// reads carry only the *_set booleans.

export type TrafficExportKind = 'posthog' | 'otlp' | 'webhook'

export const TRAFFIC_EXPORT_KINDS: readonly TrafficExportKind[] = [
  'posthog',
  'otlp',
  'webhook',
]

export type TrafficExportField =
  'referrer_host' | 'client_ip' | 'user_agent' | 'visitor_id'

export const TRAFFIC_EXPORT_FIELDS: readonly TrafficExportField[] = [
  'referrer_host',
  'client_ip',
  'user_agent',
  'visitor_id',
]

/** Fields that can single out a visitor; mirrors trafficexport.IsIdentityField. */
export const TRAFFIC_EXPORT_IDENTITY_FIELDS: readonly TrafficExportField[] = [
  'client_ip',
  'user_agent',
  'visitor_id',
]

export interface TrafficExportSink {
  id: string
  name: string
  kind: TrafficExportKind
  enabled: boolean
  host?: string
  target_host?: string
  fields: string[]
  ip_truncate: boolean
  sample: number
  url_set?: boolean
  token_set?: boolean
  auth_header_set?: boolean
  identity_enabled: boolean
  created_at?: string
  updated_at?: string
}

export interface TrafficExportSinkList {
  sinks: TrafficExportSink[]
}

export interface TrafficExportSinkStatus {
  id: string
  name: string
  kind: TrafficExportKind
  enabled: boolean
  active: boolean
  last_success: string
  last_error_kind?: string
  queued: number
  sent: number
  dropped: number
  failed: number
  sampled_out: number
}

export interface TrafficExportStatus {
  sinks: TrafficExportSinkStatus[]
}

export interface TrafficExportTestResult {
  ok: boolean
  error_kind?: string
  duration_ms: number
}

/** Create or update body. url, token and auth_header are sent only when typed. */
export interface TrafficExportSinkInput {
  name: string
  kind: TrafficExportKind
  enabled: boolean
  host?: string
  fields: string[]
  ip_truncate: boolean
  sample: number
  url?: string
  token?: string
  auth_header?: string
}
