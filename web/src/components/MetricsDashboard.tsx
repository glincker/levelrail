import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PulseIcon } from '@phosphor-icons/react/dist/ssr'
import { RequestMetricsSection } from './RequestMetricsSection'
import { MetricChartCard } from './MetricChartCard'
import { TimeRangeControls } from './TimeRangeControls'
import { ViewInGrafanaLink } from './ViewInGrafanaLink'
import { CustomRangePicker } from './CustomRangePicker'
import { DeployMarkerDetails } from './DeployMarkerDetails'
import { InvestigationPanel } from './InvestigationPanel'
import { Button } from './ui/button'
import { useMetricSeries } from '../queries/metrics'
import type { MetricName } from '../types/metrics'
import type { DeployAttempt } from '../types/deployAttempt'
import {
  type ChartMarker,
  type ChartUnit,
  useMergedChartQuery,
} from '../lib/metricChart'
import { attachPrevious, capRows, MAX_CHART_POINTS } from '../lib/chartSeries'
import { chartPalette } from '../lib/chartPalette'
import {
  bucketRestarts,
  deployMarkers as buildDeployMarkers,
  restartMarkers as buildRestartMarkers,
} from '../lib/chartMarkers'
import {
  investigationWindow,
  resolveMetricsRange,
  type MetricsSearch,
} from '../lib/observabilitySearch'
import type { ResolvedTimeRange, TimeRangeKey } from '../lib/timeRange'

// Per-app metrics dashboard over GET /api/v1/apps/{name}/metrics and
// /requests. The URL search params (range, compare, selected time) define
// the view, so a copied link reproduces it.

interface SeriesConfig {
  metric: MetricName
  label:
    | 'CPU'
    | 'usage'
    | 'limit'
    | 'received'
    | 'sent'
    | 'read'
    | 'write'
    | 'duration'
  color: string
}

interface ChartGroupConfig {
  titleKey: 'cpu' | 'memory' | 'network' | 'disk' | 'build'
  unit: ChartUnit
  primary: SeriesConfig
  secondary?: SeriesConfig
}

const CHART_GROUPS: ChartGroupConfig[] = [
  {
    titleKey: 'cpu',
    unit: 'percent',
    primary: {
      metric: 'cpu_percent',
      label: 'CPU',
      color: chartPalette.primary,
    },
  },
  {
    titleKey: 'memory',
    unit: 'bytes',
    primary: {
      metric: 'memory_usage_bytes',
      label: 'usage',
      color: chartPalette.primary,
    },
    secondary: {
      metric: 'memory_limit_bytes',
      label: 'limit',
      color: chartPalette.warning,
    },
  },
  {
    titleKey: 'network',
    unit: 'bytes',
    primary: {
      metric: 'network_rx_bytes',
      label: 'received',
      color: chartPalette.success,
    },
    secondary: {
      metric: 'network_tx_bytes',
      label: 'sent',
      color: chartPalette.neutral,
    },
  },
  {
    titleKey: 'disk',
    unit: 'bytes',
    primary: {
      metric: 'disk_read_bytes',
      label: 'read',
      color: chartPalette.success,
    },
    secondary: {
      metric: 'disk_write_bytes',
      label: 'write',
      color: chartPalette.neutral,
    },
  },
  {
    titleKey: 'build',
    unit: 'seconds',
    primary: {
      metric: 'build_duration_seconds',
      label: 'duration',
      color: chartPalette.primary,
    },
  },
]

interface Interaction {
  compare: boolean
  selectedAt?: number
  onSelectTime: (t: number) => void
  onMarkerClick: (marker: ChartMarker) => void
}

function ChartCard({
  appName,
  group,
  range,
  markers,
  interaction,
}: {
  appName: string
  group: ChartGroupConfig
  range: ResolvedTimeRange
  markers: ChartMarker[]
  interaction: Interaction
}) {
  const { t } = useTranslation('observability')
  const params = {
    from: range.from,
    to: range.to,
    maxPoints: MAX_CHART_POINTS,
    compare: interaction.compare,
  }
  const primaryQuery = useMetricSeries(appName, group.primary.metric, params)
  // Called unconditionally (rules of hooks); `enabled` skips the request.
  const secondaryQuery = useMetricSeries(
    appName,
    group.secondary?.metric ?? group.primary.metric,
    params,
    { enabled: Boolean(group.secondary) },
  )
  const {
    rows: merged,
    isLoading,
    error,
  } = useMergedChartQuery(
    primaryQuery,
    secondaryQuery,
    Boolean(group.secondary),
  )
  const previous = primaryQuery.data?.previous_points
  const rows = useMemo(
    () => capRows(attachPrevious(merged, previous), MAX_CHART_POINTS),
    [merged, previous],
  )

  return (
    <MetricChartCard
      title={t(`charts.${group.titleKey}`)}
      unit={group.unit}
      primaryLabel={t(`series.${group.primary.label}`)}
      primaryColor={group.primary.color}
      secondaryLabel={
        group.secondary ? t(`series.${group.secondary.label}`) : undefined
      }
      secondaryColor={group.secondary?.color}
      rows={rows}
      range={range}
      markers={markers}
      isLoading={isLoading}
      error={error}
      showPrevious={interaction.compare && Boolean(previous?.length)}
      previousLabel={t('compare.previousPeriod')}
      selectedAt={interaction.selectedAt}
      onSelectTime={interaction.onSelectTime}
      onMarkerClick={interaction.onMarkerClick}
    />
  )
}

export function MetricsDashboard({
  appName,
  deployAttempts,
  search,
  onSearchChange,
}: {
  appName: string
  deployAttempts: DeployAttempt[]
  search: MetricsSearch
  onSearchChange: (next: MetricsSearch) => void
}) {
  const { t } = useTranslation('observability')
  // Bumped by Refresh to mint a new "now" without changing the range.
  const [refreshNonce, setRefreshNonce] = useState(0)
  const [detail, setDetail] = useState<DeployAttempt | null>(null)
  const range = useMemo(
    () => resolveMetricsRange(search),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [search.range, search.from, search.to, refreshNonce],
  )
  const compare = search.compare === true
  const selectedAt = search.at ? Date.parse(search.at) : undefined

  const restartQuery = useMetricSeries(appName, 'container_restart_count', {
    from: range.from,
    to: range.to,
  })
  const restartBuckets = useMemo(
    () => bucketRestarts(restartQuery.data?.points ?? [], range),
    [restartQuery.data, range],
  )
  const restartCount = restartQuery.data?.points.length ?? null

  const deployMarkerList = useMemo(
    () =>
      buildDeployMarkers(deployAttempts, range, (a, at) =>
        t('markers.deploy', {
          status: t(`deployStatus.${a.status}`),
          image: a.image,
          time: new Date(at).toLocaleString(),
        }),
      ),
    [deployAttempts, range, t],
  )
  const markers = useMemo(
    () =>
      [
        ...deployMarkerList,
        ...buildRestartMarkers(restartBuckets, (count, at) =>
          t('markers.restart', {
            count,
            time: new Date(at).toLocaleString(),
          }),
        ),
      ].sort((a, b) => a.t - b.t),
    [deployMarkerList, restartBuckets, t],
  )

  const select = (at: number) => {
    onSearchChange({ ...search, at: new Date(at).toISOString() })
  }
  const interaction: Interaction = {
    compare,
    selectedAt,
    onSelectTime: select,
    onMarkerClick: (marker) => {
      if (marker.kind === 'deploy') {
        setDetail(deployAttempts.find((a) => a.id === marker.key) ?? null)
      } else {
        select(marker.t)
      }
    },
  }
  const invWindow =
    selectedAt !== undefined
      ? investigationWindow(new Date(selectedAt), range)
      : null

  return (
    <section className="rounded-lg border border-border p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
            <PulseIcon
              className="size-4 text-muted-foreground"
              aria-hidden="true"
            />
            {t('dashboard.title')}
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            {t('dashboard.summary', {
              time: range.to.toLocaleTimeString(),
              deploys: deployMarkerList.length,
            })}{' '}
            {restartCount !== null
              ? t('dashboard.restarts', { count: restartCount })
              : ''}
          </p>
          <p className="mt-1 text-xs text-muted-foreground">
            {t('dashboard.hint')}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <ViewInGrafanaLink />
          <Button
            type="button"
            variant="outline"
            size="sm"
            aria-pressed={compare}
            onClick={() => {
              onSearchChange({ ...search, compare: compare ? undefined : true })
            }}
          >
            {t('compare.toggle')}
          </Button>
          <TimeRangeControls
            rangeKey={search.range ?? '1h'}
            onRangeChange={(key: TimeRangeKey) => {
              onSearchChange({
                ...search,
                range: key,
                from: undefined,
                to: undefined,
              })
            }}
            onRefresh={() => {
              setRefreshNonce((n) => n + 1)
            }}
          />
        </div>
      </div>
      <div className="mt-3">
        <CustomRangePicker
          from={range.from}
          to={range.to}
          onApply={(from, to) => {
            onSearchChange({
              ...search,
              range: 'custom',
              from: from.toISOString(),
              to: to.toISOString(),
            })
          }}
        />
      </div>

      {invWindow && selectedAt !== undefined ? (
        <InvestigationPanel
          appName={appName}
          from={invWindow.from}
          to={invWindow.to}
          at={new Date(selectedAt)}
          onClear={() => {
            onSearchChange({ ...search, at: undefined })
          }}
        />
      ) : null}

      <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
        {CHART_GROUPS.map((group) => (
          <ChartCard
            key={group.titleKey}
            appName={appName}
            group={group}
            range={range}
            markers={markers}
            interaction={interaction}
          />
        ))}
      </div>

      <RequestMetricsSection
        appName={appName}
        range={range}
        markers={markers}
        compare={compare}
        selectedAt={selectedAt}
        onSelectTime={interaction.onSelectTime}
        onMarkerClick={interaction.onMarkerClick}
      />

      <DeployMarkerDetails
        appName={appName}
        attempt={detail}
        onClose={() => {
          setDetail(null)
        }}
      />
    </section>
  )
}
