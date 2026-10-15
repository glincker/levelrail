import type { BrandIconName } from '../components/BrandIcon'
import type { DataExportKind } from '../types/dataExport'

/** KIND_MARKS is the brand mark a destination kind shows, where one exists. */
export const KIND_MARKS: Partial<Record<DataExportKind, BrandIconName>> = {
  logs_newrelic: 'newrelic',
  logs_datadog: 'datadog',
  logs_axiom: 'axiom',
  logs_betterstack: 'betterstack',
  logs_loki: 'grafana',
  logs_otlp: 'opentelemetry',
  metrics_otlp: 'opentelemetry',
  metrics_remote_write: 'grafana',
}
