import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ApiError } from '@/lib/apiError'
import { formatBytes } from '@/lib/format'
import { cn } from '@/lib/utils'
import { useAppsMetrics } from '@/queries/appsMetrics'
import type { AppMetricsSummary } from '@/types/appsMetrics'
import { Muted, WidgetSkeleton } from './parts'

const TOP_ROWS = 5
const SORTS = ['cpu', 'memory', 'requests'] as const
type Sort = (typeof SORTS)[number]

const VALUE: Record<Sort, (r: AppMetricsSummary) => number> = {
  cpu: (r) => r.cpu_percent ?? 0,
  memory: (r) => r.memory_usage_bytes ?? 0,
  requests: (r) => r.rate_per_sec,
}

function show(sort: Sort, r: AppMetricsSummary): string {
  const v = VALUE[sort](r)
  if (sort === 'cpu') return `${v.toFixed(1)}%`
  if (sort === 'memory') return formatBytes(v)
  return `${v.toFixed(2)}/s`
}

/** Top 5 apps by CPU, memory or request rate. */
export function TopAppsWidget() {
  const { t } = useTranslation('dashboards')
  const [sort, setSort] = useState<Sort>('cpu')
  const q = useAppsMetrics()
  if (q.isLoading) return <WidgetSkeleton />
  if (q.error instanceof ApiError && q.error.status === 501) {
    return <Muted>{t('fleet.traffic.notConfigured')}</Muted>
  }
  if (q.error || !q.data) return <Muted>{t('fleet.loadFailed')}</Muted>
  const rows = q.data
    .filter((r) => VALUE[sort](r) > 0)
    .sort((a, b) => VALUE[sort](b) - VALUE[sort](a))
    .slice(0, TOP_ROWS)
  return (
    <div className="space-y-2">
      <div
        role="group"
        aria-label={t('fleet.top.sortBy')}
        className="flex gap-1"
      >
        {SORTS.map((s) => (
          <button
            key={s}
            type="button"
            aria-pressed={sort === s}
            onClick={() => setSort(s)}
            className={cn(
              'rounded-md px-2 py-0.5 text-xs transition-colors focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:outline-none',
              sort === s
                ? 'bg-muted font-medium text-foreground'
                : 'text-muted-foreground hover:text-foreground',
            )}
          >
            {t(`fleet.top.${s}`)}
          </button>
        ))}
      </div>
      {rows.length === 0 ? (
        <Muted>{t('fleet.top.empty')}</Muted>
      ) : (
        <ul className="space-y-1 text-sm">
          {rows.map((r) => (
            <li key={r.name}>
              <Link
                to="/apps/$name/overview"
                params={{ name: r.name }}
                className="flex justify-between gap-3 rounded-sm hover:underline focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:outline-none"
              >
                <span className="truncate">{r.name}</span>
                <span className="shrink-0 tabular-nums text-muted-foreground">
                  {show(sort, r)}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
