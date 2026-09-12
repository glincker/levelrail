// Shared display labels for MetricName (types/metrics.ts), mirroring
// notificationChannelKind.ts's own label-map shape. Wording follows
// MetricsDashboard.tsx's chart group titles/series labels so a metric
// reads the same way in the alert rule picker as it does on the chart.

import type { MetricName } from '../types/metrics'

export const METRIC_NAME_LABEL: Record<MetricName, string> = {
  cpu_percent: 'CPU usage (%)',
  memory_usage_bytes: 'Memory usage',
  memory_limit_bytes: 'Memory limit',
  network_rx_bytes: 'Network received',
  network_tx_bytes: 'Network sent',
  disk_read_bytes: 'Disk read',
  disk_write_bytes: 'Disk write',
}

export const METRIC_NAME_OPTIONS: MetricName[] = [
  'cpu_percent',
  'memory_usage_bytes',
  'memory_limit_bytes',
  'network_rx_bytes',
  'network_tx_bytes',
  'disk_read_bytes',
  'disk_write_bytes',
]
