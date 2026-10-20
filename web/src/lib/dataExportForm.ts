import type { DataExportKind } from '../types/dataExport'

export const KINDS: DataExportKind[] = [
  'logs_newrelic',
  'logs_datadog',
  'logs_axiom',
  'logs_betterstack',
  'logs_loki',
  'logs_otlp',
  'logs_http',
  'logs_syslog',
  'audit_webhook',
  'audit_s3',
  'audit_splunk',
  'audit_syslog',
  'audit_otlp',
  'metrics_otlp',
  'metrics_remote_write',
]

export const KIND_KEYS = {
  logs_http: 'kinds.logs_http',
  logs_loki: 'kinds.logs_loki',
  logs_otlp: 'kinds.logs_otlp',
  logs_syslog: 'kinds.logs_syslog',
  logs_newrelic: 'kinds.logs_newrelic',
  logs_datadog: 'kinds.logs_datadog',
  logs_axiom: 'kinds.logs_axiom',
  logs_betterstack: 'kinds.logs_betterstack',
  audit_webhook: 'kinds.audit_webhook',
  audit_s3: 'kinds.audit_s3',
  audit_splunk: 'kinds.audit_splunk',
  audit_syslog: 'kinds.audit_syslog',
  audit_otlp: 'kinds.audit_otlp',
  metrics_otlp: 'kinds.metrics_otlp',
  metrics_remote_write: 'kinds.metrics_remote_write',
} as const satisfies Record<DataExportKind, string>

export const LEVELS = ['any', 'debug', 'info', 'warn', 'error'] as const

export const LEVEL_KEYS = {
  any: 'levels.any',
  debug: 'levels.debug',
  info: 'levels.info',
  warn: 'levels.warn',
  error: 'levels.error',
} as const

export const isLogKind = (k: DataExportKind) => k.startsWith('logs_')
export const isVendorKind = (k: DataExportKind) =>
  k === 'logs_newrelic' ||
  k === 'logs_datadog' ||
  k === 'logs_axiom' ||
  k === 'logs_betterstack'
export const isMetricsKind = (k: DataExportKind) => k.startsWith('metrics_')
export const hasAppFilter = (k: DataExportKind) =>
  isLogKind(k) || isMetricsKind(k)

export function parseHeaders(raw: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const line of raw.split('\n')) {
    const idx = line.indexOf(':')
    if (idx <= 0) continue
    const name = line.slice(0, idx).trim()
    const value = line.slice(idx + 1).trim()
    if (name && value) out[name] = value
  }
  return out
}
