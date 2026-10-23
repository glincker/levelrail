import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { Skeleton } from '@/components/ui/skeleton'
import { MetricChartCard } from '@/components/MetricChartCard'
import { ArrowSquareOutIcon } from '@phosphor-icons/react/dist/ssr'
import { isDashboardMetric, isHttpUrl } from '@/lib/dashboardGrid'
import { chartPalette } from '@/lib/chartPalette'
import { formatMoney } from '@/lib/usageFormat'
import { mergeMetricSeries, type ChartUnit } from '@/lib/metricChart'
import type { ResolvedTimeRange } from '@/lib/timeRange'
import { getProjectScope } from '@/lib/projectScope'
import { useEnvironmentScope } from '@/lib/environmentScope'
import { useAlertRules } from '@/queries/alerts'
import { appsSummaryQueryOptions } from '@/queries/appPages'
import { fetchTimeline } from '@/queries/appTimeline'
import { useAppTraffic } from '@/queries/appTraffic'
import { useDatabaseMetricSeries } from '@/queries/databaseMetrics'
import { useDatabases } from '@/queries/databases'
import { InstanceWideLabel } from '@/components/dashboard/ScopeIndicator'
import { useLogSearch } from '@/queries/logs'
import { useMetricSeries } from '@/queries/metrics'
import { useNodeMetricSeries } from '@/queries/nodeMetrics'
import { useNodeCapacityForecast, useNodeListOptional } from '@/queries/nodes'
import { useResourceList } from '@/queries/resources'
import { useUsageReport } from '@/queries/usage'
import type { DashboardWidget } from '@/types/dashboards'
import type { MetricName } from '@/types/metrics'
import type { NodeMetricName } from '@/types/nodeMetrics'

const LOG_LINES = 50
const TIMELINE_ROWS = 8
const COST_ROWS = 5
const NO_FILTER = { q: '', kind: '', tags: [] } as const

function unitFor(metric: string): ChartUnit {
  if (metric.endsWith('_percent')) return 'percent'
  if (metric.endsWith('_bytes')) return 'bytes'
  if (metric.endsWith('_seconds')) return 'seconds'
  return 'count'
}

function Muted({ children }: { children: string }) {
  return <p className="text-sm text-muted-foreground">{children}</p>
}

function Stat({
  value,
  label,
  note,
}: {
  value: string
  label: string
  note?: ReactNode
}) {
  return (
    <div>
      <p className="text-2xl font-semibold tabular-nums">{value}</p>
      <p className="text-sm text-muted-foreground">{label}</p>
      {note}
    </div>
  )
}

function MetricWidget({
  w,
  range,
}: {
  w: DashboardWidget
  range: ResolvedTimeRange
}) {
  const { t } = useTranslation('dashboards')
  const kind = w.target?.kind
  const name = w.target?.name ?? ''
  const metric = w.metric ?? 'cpu_percent'
  const nodes = useNodeListOptional()
  const nodeId =
    nodes.data?.find((n) => n.name === name || n.id === name)?.id ?? name
  const params = { from: range.from, to: range.to, step: range.step }
  const app = useMetricSeries(name, metric as MetricName, params, {
    enabled: kind === 'app',
  })
  const db = useDatabaseMetricSeries(name, metric as MetricName, params, {
    enabled: kind === 'database',
  })
  const node = useNodeMetricSeries(nodeId, metric as NodeMetricName, params, {
    enabled: kind === 'node' && nodes.isFetched,
    retry: false,
  })
  const q = kind === 'node' ? node : kind === 'database' ? db : app
  const rows = useMemo(() => mergeMetricSeries(q.data, undefined), [q.data])
  const label = isDashboardMetric(metric) ? t(`metric.${metric}`) : metric
  return (
    <MetricChartCard
      title={label}
      unit={unitFor(metric)}
      primaryLabel={label}
      primaryColor={chartPalette.primary}
      rows={rows}
      range={range}
      isLoading={q.isLoading}
      error={q.error}
    />
  )
}

function TrafficWidget({
  app,
  field,
}: {
  app: string
  field: 'p95' | 'error'
}) {
  const { t } = useTranslation('dashboards')
  const q = useAppTraffic(app)
  if (q.isLoading) return <Skeleton className="h-12 w-24" />
  if (!q.data || !q.data.hasTraffic)
    return <Muted>{t('widget.noTraffic')}</Muted>
  const cur = q.data.current
  return field === 'p95' ? (
    <div className="flex gap-6">
      <Stat value={`${Math.round(cur.p95Ms)} ms`} label={t('widget.p95')} />
      <Stat value={cur.ratePerSec.toFixed(2)} label={t('widget.reqPerSec')} />
    </div>
  ) : (
    <Stat
      value={`${(cur.errorRate * 100).toFixed(2)}%`}
      label={t('widget.errorShare')}
    />
  )
}

function LogWidget({
  w,
  range,
}: {
  w: DashboardWidget
  range: ResolvedTimeRange
}) {
  const { t } = useTranslation('dashboards')
  const q = useLogSearch(w.target?.name ?? '', {
    from: range.from,
    to: range.to,
    q: w.query,
    limit: LOG_LINES,
  })
  if (q.isLoading) return <Skeleton className="h-24 w-full" />
  if (q.error) return <Muted>{q.error.message}</Muted>
  const lines = q.data?.entries ?? []
  if (lines.length === 0) return <Muted>{t('widget.noLogs')}</Muted>
  return (
    <ol className="max-h-64 overflow-auto font-mono text-xs" tabIndex={0}>
      {lines.map((l, i) => (
        <li
          key={`${l.timestamp}-${i}`}
          className="whitespace-pre-wrap break-all"
        >
          <span className="text-muted-foreground">
            {new Date(l.timestamp).toLocaleTimeString()}{' '}
          </span>
          {l.message}
        </li>
      ))}
    </ol>
  )
}

function DeployWidget({ app }: { app: string }) {
  const { t } = useTranslation('dashboards')
  const q = useQuery({
    queryKey: ['custom-dashboards', 'timeline', app],
    queryFn: () => fetchTimeline(app),
    retry: false,
  })
  if (q.isLoading) return <Skeleton className="h-24 w-full" />
  const rows = (q.data ?? []).slice(0, TIMELINE_ROWS)
  if (rows.length === 0) return <Muted>{t('widget.noDeploys')}</Muted>
  return (
    <ul className="space-y-1.5 text-sm">
      {rows.map((r) => (
        <li key={r.id} className="flex items-baseline justify-between gap-3">
          <span className="truncate">{r.title}</span>
          <span className="shrink-0 text-xs text-muted-foreground">
            {r.status} {new Date(r.at).toLocaleString()}
          </span>
        </li>
      ))}
    </ul>
  )
}

function CountsWidget() {
  const { t } = useTranslation('dashboards')
  const environment = useEnvironmentScope()
  const project = getProjectScope()
  const scoped = environment !== '' || project !== ''
  const q = useResourceList({ ...NO_FILTER, tags: [] })
  // Apps and databases honour the picker through the same filters their lists use; nodes and domains are instance-wide.
  const apps = useQuery({ ...appsSummaryQueryOptions(), enabled: scoped })
  const dbs = useDatabases(scoped)
  if (q.isLoading || (scoped && (apps.isLoading || dbs.isLoading))) {
    return <Skeleton className="h-12 w-full" />
  }
  const counts = { ...(q.data?.counts ?? {}) }
  if (scoped) {
    counts.app = apps.data?.total ?? 0
    counts.database = dbs.data?.length ?? 0
  }
  return (
    <div className="flex flex-wrap gap-6">
      {(['app', 'database', 'node', 'domain'] as const).map((k) => (
        <Stat
          key={k}
          value={String(counts[k] ?? 0)}
          label={t(`widget.count.${k}`)}
          note={k === 'node' || k === 'domain' ? <InstanceWideLabel /> : null}
        />
      ))}
    </div>
  )
}

function CostWidget() {
  const { t } = useTranslation('dashboards')
  const environment = useEnvironmentScope()
  const project = getProjectScope()
  const q = useUsageReport('mtd')
  if (q.isLoading) return <Skeleton className="h-24 w-full" />
  if (!q.data) return <Muted>{t('widget.noCost')}</Muted>
  if (environment !== '' || project !== '') {
    const groups = environment ? q.data.environments : q.data.projects
    const group = groups.find((g) => g.key === (environment || project))
    return (
      <Stat
        value={formatMoney(group?.total ?? 0, q.data.currency)}
        label={t('widget.scopedCost')}
      />
    )
  }
  const rows = (q.data.tags ?? []).slice(0, COST_ROWS)
  if (rows.length === 0) return <Muted>{t('widget.noCost')}</Muted>
  return (
    <ul className="space-y-1.5 text-sm">
      {rows.map((r) => (
        <li key={r.key} className="flex justify-between gap-3">
          <span className="truncate">
            {r.key === 'untagged' ? t('widget.untagged') : r.key}
          </span>
          <span className="tabular-nums">
            {formatMoney(r.total, q.data.currency)}
          </span>
        </li>
      ))}
    </ul>
  )
}

function AlertWidget({ app }: { app: string }) {
  const { t } = useTranslation('dashboards')
  const q = useAlertRules(app)
  if (q.isLoading) return <Skeleton className="h-12 w-24" />
  if (q.error) return <Muted>{q.error.message}</Muted>
  const rules = q.data ?? []
  const on = rules.filter((r) => r.enabled).length
  return (
    <div className="space-y-2">
      <Stat value={`${on}/${rules.length}`} label={t('widget.rulesEnabled')} />
      {rules.length === 0 ? <Muted>{t('widget.noRules')}</Muted> : null}
    </div>
  )
}

function CapacityWidget({ node }: { node: string }) {
  const { t } = useTranslation('dashboards')
  const nodes = useNodeListOptional()
  const id =
    nodes.data?.find((n) => n.name === node || n.id === node)?.id ?? node
  const q = useNodeCapacityForecast(id)
  if (q.isLoading) return <Skeleton className="h-12 w-full" />
  if (q.error) return <Muted>{q.error.message}</Muted>
  const rows = [
    { key: 'disk' as const, m: q.data?.disk },
    { key: 'memory' as const, m: q.data?.memory },
  ]
  return (
    <div className="flex flex-wrap gap-6">
      {rows.map(({ key, m }) => (
        <Stat
          key={key}
          value={
            m && m.total_bytes > 0
              ? `${Math.round((m.current_used_bytes / m.total_bytes) * 100)}%`
              : t('widget.unknown')
          }
          label={t(`widget.capacity.${key}`)}
          note={<InstanceWideLabel />}
        />
      ))}
    </div>
  )
}

function LinkWidget({ w }: { w: DashboardWidget }) {
  const url = w.url ?? ''
  if (!isHttpUrl(url)) return null
  return (
    <a
      href={url}
      target="_blank"
      rel="noopener noreferrer"
      className="flex items-center justify-between gap-3 rounded-lg border border-border p-3 text-sm transition-colors hover:bg-muted/50"
    >
      <span className="min-w-0">
        <span className="block truncate font-medium text-foreground">
          {w.title}
        </span>
        <span className="block truncate text-xs text-muted-foreground">
          {new URL(url).host}
        </span>
      </span>
      <ArrowSquareOutIcon aria-hidden="true" className="size-4 shrink-0" />
    </a>
  )
}

/** Renders the live body of one dashboard widget from the existing telemetry and resource endpoints. */
export function WidgetBody({
  w,
  range,
}: {
  w: DashboardWidget
  range: ResolvedTimeRange
}) {
  const target = w.target?.name ?? ''
  switch (w.type) {
    case 'metric_chart':
      return <MetricWidget w={w} range={range} />
    case 'request_percentiles':
      return <TrafficWidget app={target} field="p95" />
    case 'error_rate':
      return <TrafficWidget app={target} field="error" />
    case 'log_tail':
      return <LogWidget w={w} range={range} />
    case 'deploy_timeline':
      return <DeployWidget app={target} />
    case 'resource_counts':
      return <CountsWidget />
    case 'cost':
      return <CostWidget />
    case 'alert_status':
      return <AlertWidget app={target} />
    case 'node_capacity':
      return <CapacityWidget node={target} />
    case 'link':
      return <LinkWidget w={w} />
  }
}
