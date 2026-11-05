import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ChevronRightGlassIcon, UsersGlassIcon } from '@thesvg/ui-react'
import { Sparkline } from '@/components/kit'
import { Button } from '@/components/ui/button'
import { formatDate } from '../../lib/format'
import { VisitorsExportButton } from '../trafficDetail/VisitorsSection'
import {
  isVisitorsUnavailable,
  useAppVisitors,
  type AppVisitors,
  type VisitorCount,
  type VisitorsWindow,
} from '../../queries/appVisitors'
import {
  STATUS_CLASSES,
  STATUS_TEXT_TONE as STATUS_TONE,
  isPartialData,
  statusPercent,
} from '../../lib/visitorsView'

const WINDOWS: VisitorsWindow[] = ['1h', '24h', '7d', '30d']
const TOP_LIMIT = 5

function TopList({ title, rows }: { title: string; rows: VisitorCount[] }) {
  const { t } = useTranslation('common')
  return (
    <div className="min-w-0">
      <h4 className="mb-1 text-xs font-medium text-muted-foreground">
        {title}
      </h4>
      {rows.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t('visitors.none')}</p>
      ) : (
        <ul className="space-y-0.5 text-xs">
          {rows.slice(0, TOP_LIMIT).map((r) => (
            <li key={r.label} className="flex justify-between gap-2">
              <span className="truncate font-mono" title={r.label}>
                {r.label}
              </span>
              <span className="tabular-nums text-muted-foreground">
                {r.count}
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

export function VisitorsBody({ data }: { data: AppVisitors }) {
  const { t } = useTranslation('common')
  const partial = isPartialData(data)

  if (data.requests === 0) {
    return (
      <p className="text-sm text-muted-foreground">{t('visitors.empty')}</p>
    )
  }
  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-x-6 gap-y-2">
        <div>
          <p className="text-xs text-muted-foreground">
            {t('visitors.requests')}
          </p>
          <p className="text-xl font-semibold tabular-nums">{data.requests}</p>
        </div>
        <div>
          <p
            className="text-xs text-muted-foreground"
            title={t('visitors.visitorsHint', { period: data.salt_period })}
          >
            {t('visitors.visitorsLabel')}
          </p>
          <p className="text-xl font-semibold tabular-nums">{data.visitors}</p>
        </div>
        <div className="min-w-24 flex-1">
          <Sparkline
            values={data.series.map((b) => b.requests)}
            tone="accent"
            width="fill"
            height={28}
            ariaLabel={t('visitors.trend')}
          />
        </div>
        <ul
          aria-label={t('visitors.statusLabel')}
          className="flex gap-3 text-xs tabular-nums"
        >
          {STATUS_CLASSES.map((c) => (
            <li key={c} className={STATUS_TONE[c]}>
              {c} {statusPercent(data.status, c)}%
            </li>
          ))}
        </ul>
      </div>
      <p className="text-xs text-muted-foreground">
        {t('visitors.visitorsHint', { period: data.salt_period })}
      </p>
      {partial ? (
        <p className="text-xs text-muted-foreground">
          {t('visitors.partial', {
            when: formatDate(data.oldest_record, ''),
          })}
        </p>
      ) : null}
      {data.domains.length > 1 ? (
        <div>
          <h4 className="mb-1 text-xs font-medium text-muted-foreground">
            {t('visitors.byDomain')}
          </h4>
          <ul className="flex flex-wrap gap-x-4 gap-y-1 text-xs">
            {data.domains.map((d) => (
              <li
                key={d.domain}
                title={t('visitors.domainRow', {
                  requests: d.requests,
                  visitors: d.visitors,
                })}
              >
                <span className="font-mono">{d.domain}</span>{' '}
                <span className="tabular-nums text-muted-foreground">
                  {d.requests} / {d.visitors}
                </span>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      <details className="group">
        <summary className="flex cursor-pointer list-none items-center gap-1 text-xs font-medium text-muted-foreground hover:text-foreground focus-visible:outline-2 focus-visible:outline-ring">
          <ChevronRightGlassIcon
            size="1.2em"
            aria-hidden="true"
            className="transition-transform group-open:rotate-90 motion-reduce:transition-none"
          />
          {t('visitors.details')}
        </summary>
        <div className="mt-2 grid gap-4 sm:grid-cols-2">
          <TopList title={t('visitors.topPaths')} rows={data.top_paths} />
          <TopList
            title={t('visitors.topReferrers')}
            rows={data.top_referrers}
          />
        </div>
      </details>
    </div>
  )
}

export function VisitorsCard({ appName }: { appName: string }) {
  const { t } = useTranslation('common')
  const [win, setWin] = useState<VisitorsWindow>('24h')
  const { data, isPending, isError, error, refetch } = useAppVisitors(
    appName,
    win,
  )

  if (isError && isVisitorsUnavailable(error)) return null

  return (
    <section
      aria-label={t('visitors.title')}
      className="rounded-[var(--glinui-radius-md)] border border-border bg-card p-3"
    >
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
        <h3 className="flex items-center gap-1.5 text-sm font-medium">
          <UsersGlassIcon size="1.3em" aria-hidden="true" />
          {t('visitors.title')}
        </h3>
        <div className="flex items-center gap-2">
          <div
            role="radiogroup"
            aria-label={t('visitors.windowLabel')}
            className="flex gap-1"
          >
            {WINDOWS.map((w) => (
              <Button
                key={w}
                size="sm"
                variant={w === win ? 'secondary' : 'ghost'}
                role="radio"
                aria-checked={w === win}
                onClick={() => setWin(w)}
              >
                {t(`visitors.window.${w}`)}
              </Button>
            ))}
          </div>
          <VisitorsExportButton
            app={appName}
            win={win}
            requests={data?.requests}
          />
          <Link
            to="/apps/$name/traffic"
            params={{ name: appName }}
            className="text-xs text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
          >
            {t('visitors.viewTraffic')}
          </Link>
        </div>
      </div>
      {isPending ? (
        <div
          aria-hidden="true"
          className="h-10 animate-pulse rounded bg-muted motion-reduce:animate-none"
        />
      ) : isError || !data ? (
        <div className="flex items-center gap-3 text-sm text-muted-foreground">
          <span>{t('visitors.loadError')}</span>
          <Button size="sm" variant="ghost" onClick={() => void refetch()}>
            {t('visitors.retry')}
          </Button>
        </div>
      ) : (
        <VisitorsBody data={data} />
      )}
    </section>
  )
}
