import type { ChartUnit } from '../../../lib/metricChart'
import { chartPalette } from '../../../lib/chartPalette'
import type { MetricName, MetricSeries } from '../../../types/metrics'

// Chart definitions for the database monitoring grid. A new series (for
// example the Postgres db_* collector) plugs in by adding a def here; a def
// with no series is rendered as an honest "not collected" card.

export type SeriesLabelKey =
  'cpu' | 'usage' | 'limit' | 'received' | 'sent' | 'read' | 'write'

export type ChartTitleKey =
  | 'cpu'
  | 'memory'
  | 'network'
  | 'disk'
  | 'connections'
  | 'dbSize'
  | 'cacheHit'
  | 'deadlocks'
  | 'rows'

export interface ChartSeriesDef {
  metric: MetricName
  /** Suffix under dbMonitoring `series.` */
  labelKey: SeriesLabelKey
  color: string
  /** Hidden until the viewer turns it on (e.g. the memory limit line). */
  defaultHidden?: boolean
}

export interface ChartDef {
  id: string
  /** Suffix under dbMonitoring `charts.` */
  titleKey: ChartTitleKey
  unit: ChartUnit
  series: readonly ChartSeriesDef[]
  /** Engine ids the card applies to, or 'all'. */
  engines: readonly string[] | 'all'
  /** Needs a persistent volume, so in-memory engines skip it. */
  needsDisk?: boolean
}

const SQL_ENGINES = ['postgres', 'mysql'] as const

export const CHART_DEFS: readonly ChartDef[] = [
  {
    id: 'cpu',
    titleKey: 'cpu',
    unit: 'percent',
    engines: 'all',
    series: [
      { metric: 'cpu_percent', labelKey: 'cpu', color: chartPalette.primary },
    ],
  },
  {
    id: 'memory',
    titleKey: 'memory',
    unit: 'bytes',
    engines: 'all',
    series: [
      {
        metric: 'memory_usage_bytes',
        labelKey: 'usage',
        color: chartPalette.primary,
      },
      {
        metric: 'memory_limit_bytes',
        labelKey: 'limit',
        color: chartPalette.warning,
        defaultHidden: true,
      },
    ],
  },
  {
    id: 'network',
    titleKey: 'network',
    unit: 'bytes',
    engines: 'all',
    series: [
      {
        metric: 'network_rx_bytes',
        labelKey: 'received',
        color: chartPalette.success,
      },
      {
        metric: 'network_tx_bytes',
        labelKey: 'sent',
        color: chartPalette.neutral,
      },
    ],
  },
  {
    id: 'disk',
    titleKey: 'disk',
    unit: 'bytes',
    engines: 'all',
    needsDisk: true,
    series: [
      {
        metric: 'disk_read_bytes',
        labelKey: 'read',
        color: chartPalette.success,
      },
      {
        metric: 'disk_write_bytes',
        labelKey: 'write',
        color: chartPalette.neutral,
      },
    ],
  },
  {
    id: 'connections',
    titleKey: 'connections',
    unit: 'count',
    engines: SQL_ENGINES,
    series: [],
  },
  {
    id: 'dbSize',
    titleKey: 'dbSize',
    unit: 'bytes',
    engines: SQL_ENGINES,
    series: [],
  },
  {
    id: 'cacheHit',
    titleKey: 'cacheHit',
    unit: 'percent',
    engines: ['postgres'],
    series: [],
  },
  {
    id: 'deadlocks',
    titleKey: 'deadlocks',
    unit: 'count',
    engines: ['postgres'],
    series: [],
  },
  {
    id: 'rows',
    titleKey: 'rows',
    unit: 'count',
    engines: ['postgres'],
    series: [],
  },
]

export function isCollected(def: ChartDef): boolean {
  return def.series.length > 0
}

export function chartsForEngine(
  engine: string,
  hasDisk: boolean,
  defs: readonly ChartDef[] = CHART_DEFS,
): ChartDef[] {
  return defs.filter(
    (d) =>
      (d.engines === 'all' || d.engines.includes(engine)) &&
      (hasDisk || !d.needsDisk),
  )
}

export function metricsForCharts(defs: readonly ChartDef[]): MetricName[] {
  const seen = new Set<MetricName>()
  for (const d of defs) for (const s of d.series) seen.add(s.metric)
  return [...seen]
}

export type SeriesMap = Partial<Record<MetricName, MetricSeries>>

export interface MonitoringRow {
  t: number
  [seriesKey: string]: number
}

// One row per timestamp with a `s0..sN` column per series, so any number of
// series share one dataset for a chart.
export function buildRows(def: ChartDef, data: SeriesMap): MonitoringRow[] {
  const rows = new Map<number, MonitoringRow>()
  def.series.forEach((s, i) => {
    for (const p of data[s.metric]?.points ?? []) {
      const t = Date.parse(p.timestamp)
      if (Number.isNaN(t)) continue
      const row = rows.get(t) ?? { t }
      row[`s${i}`] = p.value
      rows.set(t, row)
    }
  })
  return [...rows.values()].sort((a, b) => a.t - b.t)
}

// Engines whose SQL telemetry and query tabs exist (Active queries, Query performance).
export function engineHasQueryTabs(engine: string): boolean {
  return (SQL_ENGINES as readonly string[]).includes(engine)
}
