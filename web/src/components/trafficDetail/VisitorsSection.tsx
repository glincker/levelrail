import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { UsersGlassIcon } from '@thesvg/ui-react'
import { Sparkline } from '@/components/kit'
import { Button } from '@/components/ui/button'
import { NativeSelect } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import { formatDate } from '../../lib/format'
import {
  STATUS_CLASSES,
  STATUS_TEXT_TONE,
  isPartialData,
  statusPercent,
  statusTotal,
} from '../../lib/visitorsView'
import {
  isVisitorsUnavailable,
  useAppVisitors,
  type AppVisitors,
  type VisitorCount,
  type VisitorDomain,
  type VisitorsWindow,
} from '../../queries/appVisitors'

const WINDOWS: VisitorsWindow[] = ['1h', '24h', '7d', '30d']
const TOP_ROWS = 10
const STATUS_FILL: Record<(typeof STATUS_CLASSES)[number], string> = {
  '2xx': 'fill-tone-success-solid',
  '3xx': 'fill-tone-neutral-solid',
  '4xx': 'fill-tone-warning-solid',
  '5xx': 'fill-tone-danger-solid',
}

function StatusBar({ status }: Readonly<{ status: AppVisitors['status'] }>) {
  const { t } = useTranslation('common')
  const total = statusTotal(status)
  const segments = STATUS_CLASSES.map((c, i) => ({
    c,
    x: STATUS_CLASSES.slice(0, i).reduce(
      (sum, p) => sum + (total > 0 ? ((status[p] ?? 0) / total) * 100 : 0),
      0,
    ),
    w: total > 0 ? ((status[c] ?? 0) / total) * 100 : 0,
  }))
  return (
    <div className="space-y-1">
      <p className="text-xs text-muted-foreground">
        {t('visitors.statusSplit')}
      </p>
      <svg
        role="img"
        aria-label={STATUS_CLASSES.map(
          (c) => `${c} ${statusPercent(status, c)}%`,
        ).join(', ')}
        viewBox="0 0 100 1"
        preserveAspectRatio="none"
        className="h-2 w-full overflow-hidden rounded-full bg-muted"
      >
        {segments.map((s) => (
          <rect
            key={s.c}
            x={s.x}
            y={0}
            width={s.w}
            height={1}
            className={STATUS_FILL[s.c]}
          />
        ))}
      </svg>
      <ul
        aria-label={t('visitors.statusLabel')}
        className="flex gap-3 text-xs tabular-nums"
      >
        {STATUS_CLASSES.map((c) => (
          <li key={c} className={STATUS_TEXT_TONE[c]}>
            {c} {statusPercent(status, c)}%
          </li>
        ))}
      </ul>
    </div>
  )
}

function TopTable({
  caption,
  head,
  rows,
}: Readonly<{ caption: string; head: string; rows: VisitorCount[] }>) {
  const { t } = useTranslation('common')
  return (
    <table className="w-full text-xs">
      <caption className="mb-1 text-left text-xs font-medium text-muted-foreground">
        {caption}
      </caption>
      <thead>
        <tr className="text-left text-muted-foreground">
          <th scope="col" className="pb-1 font-medium">
            {head}
          </th>
          <th scope="col" className="pb-1 text-right font-medium">
            {t('visitors.countHeader')}
          </th>
        </tr>
      </thead>
      <tbody>
        {rows.length === 0 ? (
          <tr>
            <td colSpan={2} className="py-1 text-muted-foreground">
              {t('visitors.none')}
            </td>
          </tr>
        ) : (
          rows.slice(0, TOP_ROWS).map((r) => (
            <tr key={r.label} className="border-t border-border">
              <td className="max-w-0 truncate py-1 font-mono" title={r.label}>
                {r.label}
              </td>
              <td className="py-1 text-right tabular-nums text-muted-foreground">
                {r.count}
              </td>
            </tr>
          ))
        )}
      </tbody>
    </table>
  )
}

export function VisitorsDetail({ data }: Readonly<{ data: AppVisitors }>) {
  const { t } = useTranslation('common')
  if (data.requests === 0) {
    return (
      <p className="text-sm text-muted-foreground">{t('visitors.empty')}</p>
    )
  }
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-end gap-x-8 gap-y-2">
        <div>
          <p className="text-xs text-muted-foreground">
            {t('visitors.requests')}
          </p>
          <p className="text-2xl font-semibold tabular-nums">{data.requests}</p>
        </div>
        <div>
          <p
            className="text-xs text-muted-foreground"
            title={t('visitors.visitorsHint', { period: data.salt_period })}
          >
            {t('visitors.visitorsLabel')}
          </p>
          <p className="text-2xl font-semibold tabular-nums">{data.visitors}</p>
        </div>
        <div className="min-w-48 flex-1">
          <Sparkline
            values={data.series.map((b) => b.requests)}
            tone="accent"
            width="fill"
            height={72}
            fill
            ariaLabel={t('visitors.chartHeading')}
          />
        </div>
      </div>
      <p className="text-xs text-muted-foreground">
        {t('visitors.visitorsHint', { period: data.salt_period })}
      </p>
      {isPartialData(data) ? (
        <p className="text-xs text-muted-foreground">
          {t('visitors.partial', { when: formatDate(data.oldest_record, '') })}
        </p>
      ) : null}
      <StatusBar status={data.status} />
      <div className="grid gap-6 md:grid-cols-2">
        <TopTable
          caption={t('visitors.topPathsCaption')}
          head={t('visitors.pathHeader')}
          rows={data.top_paths}
        />
        <TopTable
          caption={t('visitors.topReferrersCaption')}
          head={t('visitors.referrerHeader')}
          rows={data.top_referrers}
        />
      </div>
    </div>
  )
}

/** VisitorsSection is the edge access log summary at the top of the Traffic tab. */
export function VisitorsSection({ app }: Readonly<{ app: string }>) {
  const { t } = useTranslation('common')
  const [win, setWin] = useState<VisitorsWindow>('24h')
  const [domain, setDomain] = useState('')
  const { data, isPending, isError, error, refetch } = useAppVisitors(
    app,
    win,
    domain,
  )
  // The unfiltered query is the same cache entry when no domain is chosen,
  // and keeps the full domain list when one is.
  const all = useAppVisitors(app, win, '')
  const known: VisitorDomain[] = all.data?.domains ?? []

  if (isError && isVisitorsUnavailable(error)) {
    return (
      <section
        aria-label={t('visitors.section')}
        className="rounded-[var(--glinui-radius-md)] border border-border bg-card p-3"
      >
        <p className="text-sm text-muted-foreground">
          {t('visitors.unavailable')}
        </p>
      </section>
    )
  }

  const options =
    domain !== '' && !known.some((d) => d.domain === domain)
      ? [...known, { domain, requests: 0, visitors: 0 }]
      : known

  return (
    <section
      aria-label={t('visitors.section')}
      className="rounded-[var(--glinui-radius-md)] border border-border bg-card p-3"
    >
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <h3 className="flex items-center gap-1.5 text-sm font-medium">
          <UsersGlassIcon size="1.3em" aria-hidden="true" />
          {t('visitors.title')}
        </h3>
        <div className="flex flex-wrap items-center gap-2">
          {options.length > 1 || domain !== '' ? (
            <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
              {t('visitors.domainLabel')}
              <NativeSelect
                value={domain}
                onChange={(e) => {
                  setDomain(e.target.value)
                }}
              >
                <option value="">{t('visitors.allDomains')}</option>
                {options.map((d) => (
                  <option key={d.domain} value={d.domain}>
                    {d.domain}
                  </option>
                ))}
              </NativeSelect>
            </label>
          ) : null}
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
                onClick={() => {
                  setWin(w)
                }}
              >
                {t(`visitors.window.${w}`)}
              </Button>
            ))}
          </div>
        </div>
      </div>
      {isPending ? (
        <Skeleton className="h-24 w-full" />
      ) : isError || !data ? (
        <div className="flex items-center gap-3 text-sm text-muted-foreground">
          <span>{t('visitors.loadError')}</span>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => {
              void refetch()
            }}
          >
            {t('visitors.retry')}
          </Button>
        </div>
      ) : (
        <VisitorsDetail data={data} />
      )}
    </section>
  )
}
