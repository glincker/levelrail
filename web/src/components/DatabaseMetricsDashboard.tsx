import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { ActivityGlassIcon, PlayGlassIcon } from '@thesvg/ui-react'
import { Button, buttonVariants } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { engineHasBackups } from '@/lib/databaseCatalog'
import { useDatabase, useStartDatabase } from '../queries/databases'
import {
  metricsEmptyReason,
  panelsForEngine,
  type PanelId,
} from './databases/metrics/metricPanels'
import { MetricChartCard } from './MetricChartCard'
import { chartPalette } from '../lib/chartPalette'
import { TimeRangeControls } from './TimeRangeControls'
import { useDatabaseMetricSeries } from '../queries/databaseMetrics'
import type { MetricName } from '../types/metrics'
import { type ChartUnit, useMergedChartQuery } from '../lib/metricChart'
import {
  DEFAULT_TIME_RANGE_KEY,
  resolveTimeRange,
  type ResolvedTimeRange,
  type TimeRangeKey,
} from '../lib/timeRange'

// Per-database metrics dashboard: GET /api/v1/databases/{name}/metrics
// (internal/api/database_metrics.go), the database-kind counterpart to
// MetricsDashboard.tsx (per-app). Deliberately its own component rather
// than that one reused wholesale, the same reasoning
// NodeMetricsDashboard.tsx's own header comment gives for its own case:
// a database has no deploy-attempt history to overlay as markers
// (deploys are an app-scoped concept, internal/api/deploy_attempts.go),
// and none of MetricsDashboard's "not yet collected" list (request
// rate, response time percentiles, error rate) is a meaningful gap to
// call out for a database, which never terminates HTTP requests itself.
// The chart rendering itself (MetricChartCard, lib/metricChart.ts) is
// still fully shared, only the data-fetching and page chrome differ.

interface SeriesConfig {
  metric: MetricName
  label: string
  color: string
}

interface ChartGroupConfig {
  id: PanelId
  title: string
  unit: ChartUnit
  primary: SeriesConfig
  secondary?: SeriesConfig
}

const CHART_GROUPS: ChartGroupConfig[] = [
  {
    id: 'cpu',
    title: 'CPU',
    unit: 'percent',
    primary: {
      metric: 'cpu_percent',
      label: 'CPU',
      color: chartPalette.primary,
    },
  },
  {
    id: 'memory',
    title: 'Memory',
    unit: 'bytes',
    primary: {
      metric: 'memory_usage_bytes',
      label: 'Usage',
      color: chartPalette.primary,
    },
    secondary: {
      metric: 'memory_limit_bytes',
      label: 'Limit',
      color: chartPalette.warning,
    },
  },
  {
    id: 'network',
    title: 'Network I/O',
    unit: 'bytes',
    primary: {
      metric: 'network_rx_bytes',
      label: 'Received',
      color: chartPalette.success,
    },
    secondary: {
      metric: 'network_tx_bytes',
      label: 'Sent',
      color: chartPalette.neutral,
    },
  },
  {
    id: 'disk',
    title: 'Disk I/O',
    unit: 'bytes',
    primary: {
      metric: 'disk_read_bytes',
      label: 'Read',
      color: chartPalette.success,
    },
    secondary: {
      metric: 'disk_write_bytes',
      label: 'Write',
      color: chartPalette.neutral,
    },
  },
]

function ChartCard({
  databaseName,
  group,
  range,
}: {
  databaseName: string
  group: ChartGroupConfig
  range: ResolvedTimeRange
}) {
  const primaryQuery = useDatabaseMetricSeries(
    databaseName,
    group.primary.metric,
    range,
  )
  // Rules of hooks: called unconditionally regardless of `secondary`,
  // the same pattern MetricsDashboard.tsx's own ChartCard uses.
  const secondaryQuery = useDatabaseMetricSeries(
    databaseName,
    group.secondary?.metric ?? group.primary.metric,
    range,
    { enabled: Boolean(group.secondary) },
  )

  const { rows, isLoading, error } = useMergedChartQuery(
    primaryQuery,
    secondaryQuery,
    Boolean(group.secondary),
  )

  const { t } = useTranslation('databases')
  return (
    <MetricChartCard
      title={t(`metrics.panels.${group.id}`)}
      unit={group.unit}
      primaryLabel={group.primary.label}
      primaryColor={group.primary.color}
      secondaryLabel={group.secondary?.label}
      secondaryColor={group.secondary?.color}
      rows={rows}
      range={range}
      isLoading={isLoading}
      error={error}
    />
  )
}

export function DatabaseMetricsDashboard({
  databaseName,
}: {
  databaseName: string
}) {
  const { t } = useTranslation('databases')
  const { data: database } = useDatabase(databaseName)
  const start = useStartDatabase()
  const [rangeKey, setRangeKey] = useState<TimeRangeKey>(DEFAULT_TIME_RANGE_KEY)
  // Bumped by Refresh to recompute "now" without changing rangeKey, which
  // would otherwise refetch in a loop.
  const [refreshNonce, setRefreshNonce] = useState(0)
  const range = useMemo(
    () => resolveTimeRange(rangeKey),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [rangeKey, refreshNonce],
  )
  const hasDisk = engineHasBackups(database.engine)
  const panelIds = panelsForEngine(hasDisk)
  const emptyReason = metricsEmptyReason(database)

  return (
    <section className="rounded-lg border border-border p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="flex items-center gap-1.5 text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground">
            <ActivityGlassIcon
              className="size-5 text-muted-foreground"
              aria-hidden="true"
            />
            {t('metrics.title')}
          </h2>
          {emptyReason ? null : (
            <p className="mt-1 text-xs text-muted-foreground">
              {t('metrics.asOf', { time: range.to.toLocaleTimeString() })}
            </p>
          )}
        </div>
        {emptyReason ? null : (
          <TimeRangeControls
            rangeKey={rangeKey}
            onRangeChange={setRangeKey}
            onRefresh={() => {
              setRefreshNonce((n) => n + 1)
            }}
          />
        )}
      </div>

      {emptyReason ? (
        <EmptyState
          className="mt-4"
          icon={<ActivityGlassIcon className="size-6" />}
          title={t(`metrics.empty.${emptyReason}.title`)}
          description={t(`metrics.empty.${emptyReason}.body`)}
          action={
            emptyReason === 'stopped' ? (
              <Button
                size="sm"
                disabled={start.isPending}
                onClick={() => {
                  start.mutate(databaseName)
                }}
              >
                <PlayGlassIcon size="1.4em" aria-hidden="true" />
                {start.isPending
                  ? t('metrics.empty.starting')
                  : t('metrics.empty.start')}
              </Button>
            ) : (
              <Link
                to="/databases/$name/overview"
                params={{ name: databaseName }}
                className={buttonVariants({ size: 'sm' })}
              >
                {t('metrics.empty.viewProgress')}
              </Link>
            )
          }
        />
      ) : (
        <>
          <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
            {panelIds.map((id) => {
              const group = CHART_GROUPS.find((g) => g.id === id)
              return group ? (
                <ChartCard
                  key={id}
                  databaseName={databaseName}
                  group={group}
                  range={range}
                />
              ) : null
            })}
          </div>
          <p className="mt-4 text-xs text-muted-foreground">
            {hasDisk ? t('metrics.gapsSql') : t('metrics.gapsMemory')}{' '}
            {hasDisk ? (
              <Link
                to="/databases/$name/slow-queries"
                params={{ name: databaseName }}
                className="text-primary underline underline-offset-2"
              >
                {t('metrics.slowQueriesLink')}
              </Link>
            ) : null}
          </p>
        </>
      )}
    </section>
  )
}
