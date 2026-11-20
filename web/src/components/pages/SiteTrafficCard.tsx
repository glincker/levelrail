import { lazy, Suspense, useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { ProgressBar } from '@/components/ui/progress-ring'
import { SegmentedControl } from '@/components/ui/segmented-control'
import { Skeleton } from '@/components/ui/skeleton'
import { formatSize } from '../../lib/format'
import {
  siteTrafficQueryOptions,
  type SiteTraffic,
  type TrafficCount,
  type TrafficWindow,
} from '../../queries/pagesSites'
import { SiteAnalyticsPanel } from './SiteAnalyticsPanel'
import { splitCells, STATUS_CELLS, STATUS_CLASSES } from './pageHelpers'

const TrafficSeriesChart = lazy(() => import('./TrafficSeriesChart'))
const WINDOWS: readonly TrafficWindow[] = ['24h', '7d', '30d']
const nf = new Intl.NumberFormat()

function Stat({ label, value }: Readonly<{ label: string; value: string }>) {
  return (
    <div className="rounded-md bg-muted/40 px-3 py-2">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="text-lg font-semibold tabular-nums">{value}</dd>
    </div>
  )
}

function StatusSplit({ status }: Readonly<{ status: SiteTraffic['status'] }>) {
  const { t } = useTranslation('pagesSites')
  const keys = ['2xx', '3xx', '4xx', '5xx'] as const
  const cells = splitCells(
    keys.map((k) => status[k]),
    STATUS_CELLS,
  )
  return (
    <div className="space-y-1">
      <div
        role="img"
        aria-label={keys.map((k) => `${k}: ${nf.format(status[k])}`).join(', ')}
        className="flex h-2 gap-px overflow-hidden rounded-full bg-muted"
      >
        {keys.flatMap((k, ki) =>
          Array.from({ length: cells[ki] ?? 0 }, (_, i) => (
            <span key={`${k}-${i}`} className={`flex-1 ${STATUS_CLASSES[k]}`} />
          )),
        )}
      </div>
      <p className="text-xs text-muted-foreground">
        {t('traffic.statusLine', {
          ok: nf.format(status['2xx']),
          redirects: nf.format(status['3xx']),
          clientErrors: nf.format(status['4xx']),
          serverErrors: nf.format(status['5xx']),
        })}
      </p>
    </div>
  )
}

function TopList({
  title,
  rows,
  action,
}: Readonly<{
  title: string
  rows: TrafficCount[]
  action?: (path: string) => void
}>) {
  const { t } = useTranslation('pagesSites')
  const max = Math.max(1, ...rows.map((r) => r.count))
  return (
    <div className="min-w-0 space-y-1.5">
      <h3 className="text-xs font-medium text-muted-foreground">{title}</h3>
      {rows.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t('traffic.none')}</p>
      ) : (
        <ul className="space-y-1.5">
          {rows.slice(0, 5).map((r) => (
            <li key={r.label} className="space-y-0.5">
              <div className="flex items-center justify-between gap-2 text-xs">
                <code className="truncate">{r.label}</code>
                <span className="flex shrink-0 items-center gap-1 tabular-nums">
                  {nf.format(r.count)}
                  {action ? (
                    <Button
                      size="xs"
                      variant="ghost"
                      onClick={() => action(r.label)}
                    >
                      {t('traffic.addRedirect')}
                    </Button>
                  ) : null}
                </span>
              </div>
              <ProgressBar
                value={(r.count / max) * 100}
                label={r.label}
                className="h-1"
              />
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

/** Quiet one-line summary that expands on demand; collapsed by default so noise stays out of view. */
function Fold({
  summary,
  children,
}: Readonly<{ summary: string; children: ReactNode }>) {
  const { t } = useTranslation('pagesSites')
  const [open, setOpen] = useState(false)
  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
        <span className="tabular-nums">{summary}</span>
        <CollapsibleTrigger
          render={<Button size="xs" variant="ghost" />}
          className="w-auto"
        >
          {open ? t('traffic.hide') : t('traffic.show')}
        </CollapsibleTrigger>
      </div>
      <CollapsiblePanel className="pt-1.5">{children}</CollapsiblePanel>
    </Collapsible>
  )
}

export function SiteTrafficCard({
  name,
  onAddRedirect,
}: Readonly<{ name: string; onAddRedirect?: (path: string) => void }>) {
  const { t } = useTranslation('pagesSites')
  const [win, setWin] = useState<TrafficWindow>('24h')
  const q = useQuery(siteTrafficQueryOptions(name, win))
  const d = q.data
  return (
    <section
      aria-labelledby="pages-traffic-title"
      className="space-y-3 rounded-lg border border-border bg-card p-3"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id="pages-traffic-title" className="text-sm font-semibold">
          {t('traffic.title')}
        </h2>
        <SegmentedControl
          aria-label={t('traffic.window')}
          value={win}
          onValueChange={setWin}
          options={WINDOWS.map((w) => ({
            value: w,
            label: t(`traffic.windows.${w}`),
          }))}
        />
      </div>
      {q.isPending ? <Skeleton className="h-24 w-full" /> : null}
      {q.isError ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription>{t('traffic.failed')}</AlertDescription>
        </Alert>
      ) : null}
      {d === null ? (
        <p className="text-sm text-muted-foreground">
          {t('traffic.unavailable')}
        </p>
      ) : null}
      {d ? (
        <>
          <dl className="grid grid-cols-3 gap-3">
            <Stat label={t('traffic.requests')} value={nf.format(d.requests)} />
            <Stat label={t('traffic.visitors')} value={nf.format(d.visitors)} />
            <Stat
              label={t('traffic.bandwidth')}
              value={formatSize(d.bytes_out)}
            />
          </dl>
          {d.series ? (
            <Suspense fallback={<Skeleton className="h-16 w-full" />}>
              <TrafficSeriesChart series={d.series} />
            </Suspense>
          ) : null}
          <StatusSplit status={d.status} />
          <div className="grid gap-4 sm:grid-cols-2">
            <TopList
              title={t('traffic.topPages')}
              rows={d.top_pages ?? d.top_paths}
            />
            <TopList
              title={t('traffic.brokenLinks')}
              rows={d.broken_links ?? d.top_404_paths}
              action={onAddRedirect}
            />
          </div>
          <SiteAnalyticsPanel name={name} range={win} />
          {d.assets ? (
            <Fold
              summary={t('traffic.assets', {
                count: d.assets.requests,
                size: formatSize(d.assets.bytes),
              })}
            >
              <p className="text-xs text-muted-foreground">
                {t('traffic.assetsNote')}
              </p>
            </Fold>
          ) : null}
          {d.probe_requests && d.probe_requests.length > 0 ? (
            <Fold
              summary={t('traffic.probes', {
                count: d.probe_requests.reduce((n, r) => n + r.count, 0),
              })}
            >
              <TopList
                title={t('traffic.probesList')}
                rows={d.probe_requests}
              />
            </Fold>
          ) : null}
          {d.partial ? (
            <p className="text-xs text-muted-foreground">
              {t('traffic.partial')}
              {d.partial_notes?.length ? ` ${d.partial_notes.join(' ')}` : ''}
            </p>
          ) : null}
        </>
      ) : null}
    </section>
  )
}
