import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { MagnifyingGlassIcon, XIcon } from '@phosphor-icons/react/dist/ssr'
import { useInvestigation } from '../queries/investigate'
import {
  formatMs as ms,
  formatPercent as pct,
  summaryRows,
} from '../lib/observabilityFormat'
import { WhatChangedTimeline } from './WhatChangedTimeline'
import { buttonVariants, Button } from './ui/button'
import { Skeleton } from './ui/skeleton'

function Heading({ children }: { children: string }) {
  return (
    <h4 className="mb-2 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
      {children}
    </h4>
  )
}

// Opens when a point on a chart is clicked: what the traffic looked like in a
// window around it, versus the window before, and what changed nearby.
export function InvestigationPanel({
  appName,
  from,
  to,
  at,
  onClear,
}: {
  appName: string
  from: Date
  to: Date
  at: Date
  onClear: () => void
}) {
  const { t } = useTranslation('observability')
  const { data, isLoading, error } = useInvestigation(appName, from, to)
  const logFrom = data?.logs.from ?? from.toISOString()
  const logTo = data?.logs.to ?? to.toISOString()

  return (
    <section
      aria-label={t('investigation.title')}
      className="mt-4 rounded-lg border border-tone-accent-border bg-card p-4"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
            <MagnifyingGlassIcon className="size-4" aria-hidden="true" />
            {t('investigation.heading', { time: at.toLocaleString() })}
          </h3>
          <p className="mt-1 text-xs text-muted-foreground">
            {t('investigation.window', {
              from: from.toLocaleTimeString(),
              to: to.toLocaleTimeString(),
            })}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Link
            to="/apps/$name/logs"
            params={{ name: appName }}
            search={{ tab: 'search', from: logFrom, to: logTo }}
            className={buttonVariants({ variant: 'outline', size: 'sm' })}
          >
            {t('investigation.viewLogs')}
          </Link>
          <Button type="button" variant="ghost" size="sm" onClick={onClear}>
            <XIcon className="size-3.5" aria-hidden="true" />
            {t('investigation.clear')}
          </Button>
        </div>
      </div>

      {isLoading ? (
        <Skeleton className="mt-3 h-32 w-full" />
      ) : error ? (
        <p className="mt-3 text-sm text-destructive">{error.message}</p>
      ) : data ? (
        <div className="mt-4 grid grid-cols-1 gap-6 lg:grid-cols-2">
          <div className="space-y-5">
            <div>
              <Heading>{t('investigation.beforeAfter')}</Heading>
              {data.summary.has_traffic ? (
                <table className="w-full text-sm tabular-nums">
                  <thead>
                    <tr className="text-xs text-muted-foreground">
                      <th className="py-1 text-left font-medium" />
                      <th className="py-1 text-right font-medium">
                        {t('investigation.baseline')}
                      </th>
                      <th className="py-1 text-right font-medium">
                        {t('investigation.during')}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {summaryRows(data.summary, data.baseline).map((r) => (
                      <tr key={r.key} className="border-t border-border">
                        <td className="py-1">
                          {t(`investigation.metric.${r.key}`)}
                        </td>
                        <td className="py-1 text-right text-muted-foreground">
                          {r.before}
                        </td>
                        <td className="py-1 text-right font-medium">
                          {r.during}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              ) : (
                <p className="text-sm text-muted-foreground">
                  {t('investigation.noTraffic')}
                </p>
              )}
            </div>
            <div>
              <Heading>{t('investigation.topRoutes')}</Heading>
              {data.top_routes.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  {data.routes_available
                    ? t('investigation.noRoutes')
                    : t('investigation.routesUnavailable')}
                </p>
              ) : (
                <table className="w-full text-sm tabular-nums">
                  <thead>
                    <tr className="text-xs text-muted-foreground">
                      <th className="py-1 text-left font-medium">
                        {t('investigation.route')}
                      </th>
                      <th className="py-1 text-right font-medium">
                        {t('investigation.requests')}
                      </th>
                      <th className="py-1 text-right font-medium">
                        {t('investigation.metric.err5')}
                      </th>
                      <th className="py-1 text-right font-medium">
                        {t('investigation.avg')}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.top_routes.map((r) => (
                      <tr key={r.route} className="border-t border-border">
                        <td
                          className="max-w-48 truncate py-1 font-mono text-xs"
                          title={r.route}
                        >
                          {r.route}
                        </td>
                        <td className="py-1 text-right">{r.requests}</td>
                        <td className="py-1 text-right">
                          {pct(r.error_rate_5xx)}
                        </td>
                        <td className="py-1 text-right">{ms(r.avg_ms)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>
            <div>
              <Heading>{t('investigation.statusCodes')}</Heading>
              {data.status_codes.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  {t('investigation.noTraffic')}
                </p>
              ) : (
                <ul className="flex flex-wrap gap-2">
                  {data.status_codes.map((s) => (
                    <li
                      key={s.status}
                      className="rounded-md border border-border px-2 py-1 text-xs tabular-nums"
                    >
                      <span className="font-mono font-medium">{s.status}</span>{' '}
                      <span className="text-muted-foreground">
                        {s.count} ({pct(s.share)})
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </div>
          <div>
            <Heading>{t('investigation.whatChanged')}</Heading>
            <WhatChangedTimeline events={data.timeline} />
          </div>
        </div>
      ) : null}
    </section>
  )
}
