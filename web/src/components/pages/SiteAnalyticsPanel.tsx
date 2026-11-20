import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  pagesAnalyticsQueryOptions,
  type AnalyticsCount,
  type AnalyticsRange,
} from '../../queries/pagesAnalytics'

const nf = new Intl.NumberFormat()

function Column({
  title,
  rows,
}: Readonly<{ title: string; rows: AnalyticsCount[] }>) {
  const { t } = useTranslation('pagesSites')
  return (
    <div className="min-w-0 space-y-1">
      <h3 className="text-xs font-medium text-muted-foreground">{title}</h3>
      {rows.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t('analytics.none')}</p>
      ) : (
        <ul className="space-y-0.5">
          {rows.slice(0, 5).map((r) => (
            <li
              key={r.label}
              className="flex items-center justify-between gap-2 text-xs"
            >
              <code className="truncate">{r.label}</code>
              <span className="shrink-0 tabular-nums">
                {nf.format(r.count)}
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

/** Cookie-less summary from the edge access log: pages, referrer hosts and exact status codes. */
export function SiteAnalyticsPanel({
  name,
  range,
}: Readonly<{ name: string; range: AnalyticsRange }>) {
  const { t } = useTranslation('pagesSites')
  const q = useQuery(pagesAnalyticsQueryOptions(name, range))
  const d = q.data
  if (!d) return null
  const codes = d.status.slice(0, 5).map((s) => ({
    label: String(s.code),
    count: s.count,
  }))
  return (
    <div
      aria-label={t('analytics.title')}
      className="grid gap-4 border-t border-border pt-3 sm:grid-cols-3"
    >
      <Column title={t('analytics.pages')} rows={d.top_pages} />
      <Column title={t('analytics.referrers')} rows={d.top_referrers} />
      <Column title={t('analytics.codes')} rows={codes} />
      <p className="text-xs text-muted-foreground sm:col-span-3">
        {t('analytics.privacy', { days: d.retention_days })}
      </p>
    </div>
  )
}
