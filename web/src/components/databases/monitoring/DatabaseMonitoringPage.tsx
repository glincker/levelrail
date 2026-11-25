import { lazy, Suspense, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import {
  DownloadGlassIcon,
  PlayGlassIcon,
  RefreshCwGlassIcon,
} from '@thesvg/ui-react'
import { Button, buttonVariants } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { SegmentedControl } from '@/components/ui/segmented-control'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { engineHasBackups } from '@/lib/databaseCatalog'
import { pollMs } from '../../../lib/pollTier'
import { useDatabase, useStartDatabase } from '../../../queries/databases'
import { databaseMetricKeys } from '../../../queries/databaseMetrics'
import { chartsForEngine, isCollected, metricsForCharts } from './chartRegistry'
import { buildExportTable, downloadText, toCsv, toJson } from './exportMetrics'
import {
  maxCustomSpanMs,
  parseCustomRange,
  PRESET_KEYS,
  resolvePresetRange,
  toLocalInputValue,
  type MonitoringRangeKey,
} from './monitoringRange'
import {
  isInactive,
  resolveMonitoringState,
  restartedRecently,
  restartWindowMs,
} from './monitoringState'
import { StatCards } from './StatCards'
import { useMonitoringSeries } from './useMonitoringSeries'
import type { ResolvedTimeRange } from '../../../lib/timeRange'

const MonitoringCharts = lazy(() => import('./MonitoringCharts'))

export function DatabaseMonitoringPage({
  databaseName,
}: {
  databaseName: string
}) {
  const { t } = useTranslation('dbMonitoring')
  const { data: db } = useDatabase(databaseName)
  const start = useStartDatabase()
  const queryClient = useQueryClient()
  const [rangeKey, setRangeKey] = useState<MonitoringRangeKey>('1h')
  const [nonce, setNonce] = useState(0)
  const [auto, setAuto] = useState(false)
  const [custom, setCustom] = useState<ResolvedTimeRange | null>(null)
  const [fromValue, setFromValue] = useState(() =>
    toLocalInputValue(new Date(Date.now() - 3_600_000)),
  )
  const [toValue, setToValue] = useState(() => toLocalInputValue(new Date()))
  const [customError, setCustomError] = useState<string | null>(null)

  const range = useMemo(
    () =>
      rangeKey === 'custom' && custom
        ? custom
        : resolvePresetRange(rangeKey === 'custom' ? '1h' : rangeKey),
    // nonce recomputes "now" for presets without changing the key.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [rangeKey, custom, nonce],
  )

  // Auto refresh follows the repo's active poll tier; react-query already
  // pauses on hidden tabs, and a custom window is fixed so it never ticks.
  useEffect(() => {
    if (!auto || rangeKey === 'custom') return
    const id = window.setInterval(() => {
      setNonce((n) => n + 1)
    }, pollMs('active'))
    return () => {
      window.clearInterval(id)
    }
  }, [auto, rangeKey])

  const hasDisk = engineHasBackups(db.engine)
  const defs = useMemo(
    () => chartsForEngine(db.engine, hasDisk),
    [db.engine, hasDisk],
  )
  const collected = useMemo(() => defs.filter(isCollected), [defs])
  const metrics = useMemo(
    () => [...metricsForCharts(defs), 'container_restart_count' as const],
    [defs],
  )
  const blocked = Boolean(db.suspended || db.provision_state)
  const series = useMonitoringSeries(databaseName, metrics, range, !blocked)
  const hasData = Object.values(series.data).some((s) => s.points.length > 0)
  const state = resolveMonitoringState({
    suspended: db.suspended,
    provisionState: db.provision_state,
    restarting: restartedRecently(
      series.data.container_restart_count?.points,
      range.to.getTime(),
      restartWindowMs(),
    ),
    hasData,
    isLoading: series.isLoading,
  })

  const refresh = () => {
    if (rangeKey === 'custom') {
      void queryClient.invalidateQueries({
        queryKey: databaseMetricKeys.all(databaseName),
      })
    } else {
      setNonce((n) => n + 1)
    }
  }

  const applyCustom = () => {
    const res = parseCustomRange(
      fromValue,
      toValue,
      new Date(),
      maxCustomSpanMs(),
    )
    if (res.ok) {
      setCustom(res.range)
      setCustomError(null)
    } else {
      setCustomError(t(`range.errors.${res.reason}`))
    }
  }

  const exportAs = (kind: 'csv' | 'json') => {
    const table = buildExportTable(defs, series.data, (def, i) =>
      def.series.length === 1
        ? t(`charts.${def.titleKey}`)
        : `${t(`charts.${def.titleKey}`)} ${t(`series.${def.series[i]?.labelKey ?? 'cpu'}`)}`,
    )
    const stamp = new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')
    const base = `${databaseName}-metrics-${stamp}`
    if (kind === 'csv') downloadText(`${base}.csv`, 'text/csv', toCsv(table))
    else downloadText(`${base}.json`, 'application/json', toJson(table))
  }

  const canExport = hasData && !isInactive(state)
  const rangeOptions = [
    ...PRESET_KEYS.map((k) => ({ value: k, label: k })),
    { value: 'custom' as MonitoringRangeKey, label: t('range.custom') },
  ]

  return (
    <section className="flex flex-col gap-3" aria-label={t('title')}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-xs text-muted-foreground">
          {t('asOf', { time: range.to.toLocaleTimeString() })}
        </p>
        <div className="flex flex-wrap items-center gap-2">
          <SegmentedControl<MonitoringRangeKey>
            aria-label={t('range.label')}
            value={rangeKey}
            options={rangeOptions}
            onValueChange={setRangeKey}
          />
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={refresh}
            disabled={blocked}
          >
            <RefreshCwGlassIcon className="size-5" aria-hidden="true" />
            {t('refresh')}
          </Button>
          <label className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
            <Switch
              size="sm"
              checked={auto && rangeKey !== 'custom'}
              disabled={blocked || rangeKey === 'custom'}
              onCheckedChange={setAuto}
              aria-label={t('autoRefresh')}
            />
            {t('autoRefresh')}
          </label>
          <div
            role="group"
            aria-label={t('export.label')}
            className="flex gap-1"
          >
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={!canExport}
              onClick={() => {
                exportAs('csv')
              }}
            >
              <DownloadGlassIcon className="size-5" aria-hidden="true" />
              {t('export.csv')}
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={!canExport}
              onClick={() => {
                exportAs('json')
              }}
            >
              {t('export.json')}
            </Button>
          </div>
        </div>
      </div>

      {rangeKey === 'custom' ? (
        <div className="flex flex-wrap items-end gap-2">
          <label className="flex flex-col gap-1 text-xs text-muted-foreground">
            {t('range.from')}
            <Input
              type="datetime-local"
              value={fromValue}
              onChange={(e) => {
                setFromValue(e.target.value)
              }}
              className="w-52"
            />
          </label>
          <label className="flex flex-col gap-1 text-xs text-muted-foreground">
            {t('range.to')}
            <Input
              type="datetime-local"
              value={toValue}
              onChange={(e) => {
                setToValue(e.target.value)
              }}
              className="w-52"
            />
          </label>
          <Button type="button" size="sm" onClick={applyCustom}>
            {t('range.apply')}
          </Button>
          {customError ? (
            <p role="alert" className="text-xs text-destructive">
              {customError}
            </p>
          ) : null}
        </div>
      ) : null}

      {isInactive(state) || state === 'restarting' ? (
        <div
          role="status"
          className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-border bg-muted/40 px-3 py-2"
        >
          <div>
            <p className="text-sm font-medium text-foreground">
              {t(`states.${state}.title`)}
            </p>
            <p className="text-xs text-muted-foreground">
              {t(`states.${state}.body`)}
            </p>
          </div>
          {state === 'stopped' ? (
            <Button
              size="sm"
              disabled={start.isPending}
              onClick={() => {
                start.mutate(databaseName)
              }}
            >
              <PlayGlassIcon size="1.4em" aria-hidden="true" />
              {start.isPending ? t('actions.starting') : t('actions.start')}
            </Button>
          ) : state === 'provisioning' || state === 'failed' ? (
            <Link
              to="/databases/$name/overview"
              params={{ name: databaseName }}
              className={buttonVariants({ size: 'sm' })}
            >
              {t('actions.viewProgress')}
            </Link>
          ) : null}
        </div>
      ) : null}

      {series.error && !blocked ? (
        <p role="alert" className="text-sm text-destructive">
          {series.error.message}
        </p>
      ) : null}

      <StatCards
        defs={collected}
        data={series.data}
        loading={series.isLoading}
      />

      <Suspense fallback={<Skeleton className="h-64 w-full" />}>
        <MonitoringCharts
          defs={defs}
          data={series.data}
          range={range}
          state={state}
          isLoading={series.isLoading}
          syncId={`db-monitoring-${databaseName}`}
        />
      </Suspense>
    </section>
  )
}
