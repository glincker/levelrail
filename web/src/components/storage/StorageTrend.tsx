import { useEffect, useState } from 'react'
import type { KeyboardEvent, PointerEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { ChartLineGlassIcon } from '@thesvg/ui-react'
import { ApiError } from '../../lib/apiError'
import { formatBytes } from '../../lib/format'
import { useDiskHistory } from '../../queries/diskHistory'
import {
  MIN_FORECAST_DAYS,
  areaPath,
  buildScale,
  defaultRange,
  distinctDays,
  linePath,
  nearestIndex,
  projection,
  rangeOptions,
  toPoints,
  xAt,
  yAt,
} from './trendModel'
import type { TrendPoint } from './trendModel'

const CHART_HEIGHT = 168
const FALLBACK_WIDTH = 640

// Callback ref so the observer attaches when the chart mounts, not on first render.
function useWidth() {
  const [el, setEl] = useState<HTMLDivElement | null>(null)
  const [width, setWidth] = useState(FALLBACK_WIDTH)
  useEffect(() => {
    if (!el || typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver((entries) => {
      const w = Math.floor(entries[0]?.contentRect.width ?? 0)
      if (w > 0) setWidth(w)
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [el])
  return { ref: setEl, width }
}

function Panel({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-2 rounded-lg border border-border bg-card/50 px-3 py-4 text-sm text-muted-foreground">
      <ChartLineGlassIcon className="size-5 shrink-0" aria-hidden="true" />
      <p>{children}</p>
    </div>
  )
}

export interface StorageTrendProps {
  className?: string
}

// Used disk over the chosen range with a dashed projection to the fill date.
export function StorageTrend({ className = '' }: StorageTrendProps) {
  const { t, i18n } = useTranslation('storageTrend')
  const [days, setDays] = useState<number | null>(null)
  const query = useDiskHistory(days ?? 30)
  const { ref, width } = useWidth()
  const [active, setActive] = useState<number | null>(null)

  const data = query.data
  const options = rangeOptions(data?.retention_days ?? 90)
  const selected = days ?? defaultRange(options)
  const points = data ? toPoints(data.samples) : []
  const fmtDate = (ms: number) =>
    new Date(ms).toLocaleDateString(i18n.language, {
      month: 'short',
      day: 'numeric',
    })

  let body: React.ReactNode
  if (query.isPending) {
    body = <Panel>{t('states.loading')}</Panel>
  } else if (query.error) {
    const unavailable =
      query.error instanceof ApiError && query.error.status === 501
    body = (
      <Panel>
        {unavailable ? t('states.unavailable') : t('states.error')}
        {unavailable ? null : (
          <button
            type="button"
            onClick={() => void query.refetch()}
            className="ml-2 rounded text-foreground underline underline-offset-2 focus-visible:outline-2 focus-visible:outline-ring"
          >
            {t('states.retry')}
          </button>
        )}
      </Panel>
    )
  } else if (points.length < 2) {
    body = <Panel>{t('states.collecting')}</Panel>
  } else {
    const haveDays = distinctDays(points)
    const daysUntilFull = data?.forecast.days_until_full ?? null
    const proj =
      haveDays >= MIN_FORECAST_DAYS ? projection(points, daysUntilFull) : null
    const scale = buildScale(points, proj)
    const h = CHART_HEIGHT
    const cur: TrendPoint | null =
      active !== null ? (points[active] ?? null) : null
    const curX = cur ? xAt(scale, cur.t, width) : 0
    const note =
      proj !== null
        ? null
        : haveDays < MIN_FORECAST_DAYS
          ? t('forecast.needsDays', { count: MIN_FORECAST_DAYS })
          : t('forecast.flat')

    const move = (e: PointerEvent<HTMLDivElement>) => {
      const rect = e.currentTarget.getBoundingClientRect()
      if (rect.width === 0) return
      const px = ((e.clientX - rect.left) / rect.width) * width
      setActive(nearestIndex(points, scale, px, width))
    }
    const key = (e: KeyboardEvent<HTMLDivElement>) => {
      const i = active ?? points.length - 1
      const last = points.length - 1
      const steps: Record<string, number> = {
        ArrowLeft: Math.max(0, i - 1),
        ArrowRight: Math.min(last, i + 1),
        Home: 0,
        End: last,
      }
      const next = steps[e.key]
      if (next === undefined) return
      e.preventDefault()
      setActive(next)
    }

    const split: Array<[string, number]> = cur
      ? [
          [t('tooltip.images'), cur.images],
          [t('tooltip.containers'), cur.containers],
          [t('tooltip.volumes'), cur.volumes],
          [t('tooltip.buildCache'), cur.buildCache],
        ]
      : []

    body = (
      <div className="space-y-1.5">
        <div
          ref={ref}
          role="group"
          tabIndex={0}
          aria-label={t('chart.label', { days: selected })}
          onPointerMove={move}
          onPointerLeave={() => setActive(null)}
          onKeyDown={key}
          onFocus={() => setActive((a) => a ?? points.length - 1)}
          onBlur={() => setActive(null)}
          className="relative rounded-md focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
        >
          <svg
            width={width}
            height={h}
            viewBox={`0 0 ${width} ${h}`}
            aria-hidden="true"
            className="block"
          >
            <line
              x1={0}
              x2={width}
              y1={h - 0.5}
              y2={h - 0.5}
              className="stroke-border"
            />
            <path
              d={areaPath(points, scale, width, h)}
              fillOpacity={0.1}
              className="fill-[var(--chart-series-1)]"
            />
            <path
              d={linePath(points, scale, width, h)}
              fill="none"
              strokeWidth={2}
              strokeLinejoin="round"
              className="stroke-[var(--chart-series-1)]"
            />
            {proj ? (
              <path
                d={linePath([proj.from, proj.to], scale, width, h)}
                fill="none"
                strokeWidth={2}
                strokeDasharray="5 4"
                className="stroke-[var(--chart-series-2)]"
              />
            ) : null}
            {cur ? (
              <>
                <line
                  x1={curX}
                  x2={curX}
                  y1={0}
                  y2={h}
                  className="stroke-muted-foreground"
                />
                <circle
                  cx={curX}
                  cy={yAt(scale, cur.used, h)}
                  r={4}
                  strokeWidth={2}
                  className="fill-card stroke-[var(--chart-series-1)]"
                />
              </>
            ) : null}
          </svg>
          {cur ? (
            <div
              role="status"
              className={`pointer-events-none absolute top-1 w-48 rounded-lg border border-border bg-popover/95 p-2.5 text-xs text-popover-foreground shadow-md ${
                curX > width / 2 ? 'left-2' : 'right-2'
              }`}
            >
              <p className="font-medium">{fmtDate(cur.t)}</p>
              <dl className="mt-1 space-y-0.5">
                <div className="flex justify-between gap-2">
                  <dt>{t('tooltip.used')}</dt>
                  <dd className="tabular-nums">{formatBytes(cur.used)}</dd>
                </div>
                <div className="flex justify-between gap-2">
                  <dt>{t('tooltip.free')}</dt>
                  <dd className="tabular-nums">{formatBytes(cur.free)}</dd>
                </div>
                {split.map(([label, v]) => (
                  <div
                    key={label}
                    className="flex justify-between gap-2 text-muted-foreground"
                  >
                    <dt>{label}</dt>
                    <dd className="tabular-nums">{formatBytes(v)}</dd>
                  </div>
                ))}
              </dl>
            </div>
          ) : null}
        </div>
        <div className="flex justify-between text-xs text-muted-foreground">
          <span>{fmtDate(scale.x0)}</span>
          <span>{fmtDate(scale.x1)}</span>
        </div>
        <p className="text-xs text-muted-foreground">
          {proj && daysUntilFull !== null ? (
            <span className="text-foreground">
              {t('forecast.full', {
                count: Math.max(1, Math.round(daysUntilFull)),
              })}
            </span>
          ) : (
            note
          )}
        </p>
      </div>
    )
  }

  return (
    <section
      aria-label={t('title')}
      className={`space-y-2 rounded-lg border border-border bg-card/50 p-3 ${className}`}
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-sm font-medium">{t('title')}</h3>
        <div
          role="group"
          aria-label={t('range.label')}
          className="flex gap-0.5 rounded-md border border-border p-0.5"
        >
          {options.map((d) => (
            <button
              key={d}
              type="button"
              aria-pressed={d === selected}
              onClick={() => {
                setDays(d)
                setActive(null)
              }}
              className={`rounded px-2 py-0.5 text-xs focus-visible:outline-2 focus-visible:outline-ring ${
                d === selected
                  ? 'bg-muted text-foreground'
                  : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              {t('range.days', { count: d })}
            </button>
          ))}
        </div>
      </div>
      {body}
    </section>
  )
}
