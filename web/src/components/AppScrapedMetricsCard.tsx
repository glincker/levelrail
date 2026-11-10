import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import {
  useSaveScrapeTarget,
  useScrapeTarget,
  useScrapedNames,
  useScrapedSeries,
  type ScrapedPoint,
} from '../queries/appScrapedMetrics'

const RANGES = [1, 6, 24] as const
type RangeHours = (typeof RANGES)[number]

// Scraped text is shown only as React text nodes and SVG coordinates, never as HTML.
export function AppScrapedMetricsCard({ appName }: { appName: string }) {
  const { t } = useTranslation('observability')
  const target = useScrapeTarget(appName)
  const configured = target.data?.configured ?? false
  const names = useScrapedNames(appName, configured)
  const [metric, setMetric] = useState('')
  const [range, setRange] = useState<RangeHours>(1)
  const series = useScrapedSeries(appName, metric, range)
  const list = names.data?.names ?? []

  return (
    <section className="rounded-2xl border border-border bg-card p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold">{t('appScrape.title')}</h2>
          <p className="text-xs text-muted-foreground">
            {t('appScrape.subtitle')}
          </p>
        </div>
        {configured && (
          <div className="flex flex-wrap items-center gap-2">
            <label className="sr-only" htmlFor="scraped-metric">
              {t('appScrape.metric')}
            </label>
            <select
              id="scraped-metric"
              className="h-8 rounded-lg border border-border bg-background px-2 text-sm"
              value={metric}
              onChange={(e) => setMetric(e.target.value)}
            >
              <option value="">{t('appScrape.pickMetric')}</option>
              {list.map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
            </select>
            <label className="sr-only" htmlFor="scraped-range">
              {t('appScrape.range')}
            </label>
            <select
              id="scraped-range"
              className="h-8 rounded-lg border border-border bg-background px-2 text-sm"
              value={range}
              onChange={(e) => setRange(Number(e.target.value) as RangeHours)}
            >
              {RANGES.map((h) => (
                <option key={h} value={h}>
                  {t('appScrape.hours', { count: h })}
                </option>
              ))}
            </select>
          </div>
        )}
      </div>

      <TargetForm
        key={target.data?.updated_at ?? (target.isSuccess ? 'new' : 'loading')}
        appName={appName}
        port={target.data?.port}
        path={target.data?.path}
        lastError={target.data?.last_error}
      />

      {configured && list.length === 0 && !names.isLoading && (
        <p className="mt-3 text-sm text-muted-foreground">
          {t('appScrape.empty')}
        </p>
      )}
      {series.error && (
        <p role="alert" className="mt-3 text-sm text-destructive">
          {series.error.message}
        </p>
      )}
      {metric && series.data && (
        <ul className="mt-3 space-y-2">
          {(series.data.series ?? []).map((s) => (
            <li
              key={s.labels}
              className="flex items-center justify-between gap-3 rounded-lg border border-border px-3 py-2"
            >
              <div className="min-w-0">
                <p className="truncate font-mono text-xs">
                  {metric}
                  {s.labels}
                </p>
                <p className="text-sm font-semibold tabular-nums">
                  {s.points[s.points.length - 1]?.value}
                </p>
              </div>
              <Sparkline points={s.points} />
            </li>
          ))}
          {(series.data.series ?? []).length === 0 && (
            <li className="text-sm text-muted-foreground">
              {t('chart.noData')}
            </li>
          )}
        </ul>
      )}
    </section>
  )
}

function TargetForm({
  appName,
  port,
  path,
  lastError,
}: {
  appName: string
  port?: number
  path?: string
  lastError?: string
}) {
  const { t } = useTranslation('observability')
  const save = useSaveScrapeTarget(appName)
  const [portText, setPortText] = useState(port ? String(port) : '')
  const [pathText, setPathText] = useState(path || '/metrics')
  const [invalid, setInvalid] = useState('')

  // Mirrors the server rules so obvious mistakes never leave the browser.
  function onSubmit(e: FormEvent) {
    e.preventDefault()
    const n = Number(portText)
    if (!Number.isInteger(n) || n < 1 || n > 65535) {
      setInvalid(t('appScrape.invalidPort'))
      return
    }
    if (
      !/^\/[A-Za-z0-9._~/-]*$/.test(pathText) ||
      pathText.startsWith('//') ||
      pathText.includes('..')
    ) {
      setInvalid(t('appScrape.invalidPath'))
      return
    }
    setInvalid('')
    save.mutate({ port: n, path: pathText })
  }

  return (
    <form onSubmit={onSubmit} className="mt-3 flex flex-wrap items-end gap-2">
      <label className="flex flex-col gap-1 text-xs text-muted-foreground">
        {t('appScrape.port')}
        <input
          inputMode="numeric"
          className="h-8 w-24 rounded-lg border border-border bg-background px-2 text-sm text-foreground"
          value={portText}
          placeholder={port ? String(port) : '9100'}
          onChange={(e) => setPortText(e.target.value)}
        />
      </label>
      <label className="flex flex-col gap-1 text-xs text-muted-foreground">
        {t('appScrape.path')}
        <input
          className="h-8 w-40 rounded-lg border border-border bg-background px-2 text-sm text-foreground"
          value={pathText}
          onChange={(e) => setPathText(e.target.value)}
        />
      </label>
      <button
        type="submit"
        disabled={save.isPending || portText === ''}
        className="h-8 rounded-lg bg-primary px-3 text-sm font-medium text-primary-foreground disabled:opacity-50"
      >
        {t('appScrape.save')}
      </button>
      {invalid && (
        <p role="alert" className="w-full text-xs text-destructive">
          {invalid}
        </p>
      )}
      {!invalid && (save.error || lastError) && (
        <p role="alert" className="w-full text-xs text-destructive">
          {save.error?.message ??
            t('appScrape.lastError', { error: lastError })}
        </p>
      )}
    </form>
  )
}

function Sparkline({ points }: { points: ScrapedPoint[] }) {
  if (points.length < 2) return null
  const values = points.map((p) => p.value)
  const lo = Math.min(...values)
  const span = Math.max(...values) - lo || 1
  const d = values
    .map(
      (v, i) =>
        `${(i / (values.length - 1)) * 100},${30 - ((v - lo) / span) * 28}`,
    )
    .join(' ')
  return (
    <svg viewBox="0 0 100 32" className="h-8 w-28 shrink-0" aria-hidden="true">
      <polyline points={d} className="fill-none stroke-primary stroke-2" />
    </svg>
  )
}
