import type {
  DashboardWidget,
  TargetKind,
  WidgetSize,
  WidgetType,
} from '../types/dashboards'

export const WIDGET_TYPES: WidgetType[] = [
  'metric_chart',
  'request_percentiles',
  'error_rate',
  'log_tail',
  'deploy_timeline',
  'resource_counts',
  'cost',
  'alert_status',
  'node_capacity',
  'link',
  'fleet_health',
  'fleet_traffic',
  'top_apps',
  'deploy_activity',
  'fleet_capacity',
  'attention',
]

export const WIDGET_SIZES: WidgetSize[] = ['S', 'M', 'L']

// Widget types that read from one resource, and which kinds they accept.
export const WIDGET_TARGETS: Partial<Record<WidgetType, TargetKind[]>> = {
  metric_chart: ['app', 'database', 'node'],
  request_percentiles: ['app'],
  error_rate: ['app'],
  log_tail: ['app'],
  deploy_timeline: ['app'],
  alert_status: ['app'],
  node_capacity: ['node'],
}

export const APP_METRICS = [
  'cpu_percent',
  'memory_usage_bytes',
  'network_rx_bytes',
  'network_tx_bytes',
  'disk_read_bytes',
  'disk_write_bytes',
  'container_restart_count',
] as const

export const NODE_METRICS = [
  'cpu_percent',
  'memory_usage_bytes',
  'network_rx_bytes',
  'network_tx_bytes',
  'disk_used_bytes',
] as const

export function metricsFor(kind: TargetKind): readonly string[] {
  return kind === 'node' ? NODE_METRICS : APP_METRICS
}

// Static class names so Tailwind can see them; S is a quarter row, M a half, L a full row.
export const SIZE_SPAN: Record<WidgetSize, string> = {
  S: 'lg:col-span-1',
  M: 'lg:col-span-2',
  L: 'lg:col-span-4',
}

export function newWidgetId(
  widgets: readonly DashboardWidget[],
  type: WidgetType,
): string {
  const used = new Set(widgets.map((w) => w.id))
  const base = type.replace(/_/g, '-')
  for (let n = 1; ; n += 1) {
    const id = `${base}-${n}`
    if (!used.has(id)) return id
  }
}

export function addWidget(
  widgets: readonly DashboardWidget[],
  w: DashboardWidget,
): DashboardWidget[] {
  return [...widgets, w]
}

export function removeWidget(
  widgets: readonly DashboardWidget[],
  id: string,
): DashboardWidget[] {
  return widgets.filter((w) => w.id !== id)
}

export function resizeWidget(
  widgets: readonly DashboardWidget[],
  id: string,
  size: WidgetSize,
): DashboardWidget[] {
  return widgets.map((w) => (w.id === id ? { ...w, size } : w))
}

// Moves a widget one slot earlier (-1) or later (1); a no-op at the edges.
export function moveWidget(
  widgets: readonly DashboardWidget[],
  id: string,
  delta: -1 | 1,
): DashboardWidget[] {
  const from = widgets.findIndex((w) => w.id === id)
  const to = from + delta
  if (from < 0 || to < 0 || to >= widgets.length) return [...widgets]
  const next = [...widgets]
  const [item] = next.splice(from, 1)
  if (item) next.splice(to, 0, item)
  return next
}

// A starter template with its empty target names filled in with the picked resource.
export function fillTemplateTarget(
  widgets: readonly DashboardWidget[],
  name: string,
): DashboardWidget[] {
  return widgets.map((w) =>
    w.target ? { ...w, target: { ...w.target, name } } : { ...w },
  )
}

export type DashboardMetric =
  (typeof APP_METRICS)[number] | (typeof NODE_METRICS)[number]

export function isDashboardMetric(m: string): m is DashboardMetric {
  return (
    (APP_METRICS as readonly string[]).includes(m) ||
    (NODE_METRICS as readonly string[]).includes(m)
  )
}

export const TEMPLATE_KEYS = ['app-health', 'fleet-overview', 'cost'] as const
export type TemplateKey = (typeof TEMPLATE_KEYS)[number]

export function isTemplateKey(k: string): k is TemplateKey {
  return (TEMPLATE_KEYS as readonly string[]).includes(k)
}

export const REFRESH_CHOICES = [0, 15, 30, 60, 300] as const

export function widgetTitle(w: DashboardWidget, typeLabel: string): string {
  if (w.title) return w.title
  return w.target?.name ? `${typeLabel}: ${w.target.name}` : typeLabel
}

/** isHttpUrl accepts only http and https addresses, so a link widget can never carry a script or data URL. */
export function isHttpUrl(value: string): boolean {
  try {
    const u = new URL(value)
    return (u.protocol === 'http:' || u.protocol === 'https:') && u.host !== ''
  } catch {
    return false
  }
}
