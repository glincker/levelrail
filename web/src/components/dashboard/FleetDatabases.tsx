import { useMemo } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { DatabaseIcon } from '@phosphor-icons/react/dist/ssr'
import { EmptyState } from '@/components/ui/empty-state'
import { Badge } from '@/components/ui/badge'
import { SkeletonTile } from '@/components/kit'
import { TONE } from '../kit/tone'
import type { Tone } from '../kit/tone'
import { cn } from '@/lib/utils'
import { useDatabases } from '../../queries/databases'
import { useUsageSummary } from '../../queries/usageSummary'
import { formatSize } from '../../lib/format'
import {
  summarizeDatabases,
  type DatabaseHealth,
  type DatabaseRow,
} from '../../lib/fleetDatabases'
import { Readout } from './Readout'

const LIST_CAP = 6

const HEALTH_TONE: Record<DatabaseHealth, Tone> = {
  healthy: 'success',
  unhealthy: 'danger',
  starting: 'warning',
  stopped: 'neutral',
}

function Highlight({
  label,
  row,
  detail,
}: {
  label: string
  row: DatabaseRow
  detail: string
}) {
  return (
    <Link
      to="/databases/$name"
      params={{ name: row.name }}
      className="flex min-w-0 flex-col rounded-lg border border-border px-3 py-2 text-sm hover:bg-muted/40 focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:outline-none"
    >
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className="truncate font-medium">{row.name}</span>
      <span className="text-xs tabular-nums text-muted-foreground">
        {detail}
      </span>
    </Link>
  )
}

export function FleetDatabases() {
  const { t } = useTranslation('dashboard')
  const dbs = useDatabases()
  const usage = useUsageSummary()

  const summary = useMemo(
    () =>
      summarizeDatabases(
        dbs.data ?? [],
        usage.data?.databases ?? [],
        usage.data?.volumes.items ?? [],
      ),
    [dbs.data, usage.data],
  )

  if (dbs.isPending) {
    return (
      <section aria-label={t('fleet.databases.title')} aria-busy="true">
        <SkeletonTile className="h-40" />
      </section>
    )
  }
  if (dbs.isError) return null

  if (summary.total === 0) {
    return (
      <section aria-label={t('fleet.databases.title')} className="space-y-3">
        <h2 className="text-sm font-semibold text-foreground">
          {t('fleet.databases.title')}
        </h2>
        <EmptyState
          icon={<DatabaseIcon className="size-5" />}
          title={t('fleet.databases.emptyTitle')}
          description={t('fleet.databases.emptyBody')}
          action={
            <Link
              to="/databases"
              className="text-sm font-medium text-primary hover:underline"
            >
              {t('fleet.databases.emptyAction')}
            </Link>
          }
        />
      </section>
    )
  }

  const sizeKnown = summary.sizedCount > 0
  const unsizedNote =
    sizeKnown && summary.sizedCount < summary.total
      ? t('fleet.databases.sizedOf', {
          known: summary.sizedCount,
          total: summary.total,
        })
      : t('fleet.databases.sizeScope')

  return (
    <section aria-label={t('fleet.databases.title')} className="space-y-3">
      <div className="flex items-baseline justify-between">
        <h2 className="text-sm font-semibold text-foreground">
          {t('fleet.databases.title')}
        </h2>
        <Link
          to="/databases"
          className="text-xs text-muted-foreground hover:text-foreground"
        >
          {t('fleet.databases.viewAll', { count: summary.total })}
        </Link>
      </div>

      <dl className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Readout
          label={t('fleet.databases.total')}
          value={summary.total}
          hint={summary.engines.map((e) => `${e.engine} ${e.count}`).join(', ')}
        />
        <Readout
          label={t('fleet.databases.healthy')}
          value={summary.counts.healthy}
          tone={summary.counts.healthy > 0 ? 'success' : 'neutral'}
          hint={
            summary.counts.starting > 0
              ? t('fleet.databases.starting', {
                  count: summary.counts.starting,
                })
              : undefined
          }
        />
        <Readout
          label={t('fleet.databases.unhealthy')}
          value={summary.counts.unhealthy}
          tone={summary.counts.unhealthy > 0 ? 'danger' : 'neutral'}
          hint={t('fleet.databases.stoppedCount', {
            count: summary.counts.stopped,
          })}
        />
        <Readout
          label={t('fleet.databases.totalSize')}
          value={sizeKnown ? formatSize(summary.totalBytes) : '-'}
          hint={sizeKnown ? unsizedNote : t('fleet.databases.sizeUnknown')}
        />
      </dl>

      {summary.largest || summary.busiest ? (
        <div className="grid gap-3 sm:grid-cols-2">
          {summary.largest ? (
            <Highlight
              label={t('fleet.databases.largest')}
              row={summary.largest}
              detail={formatSize(summary.largest.sizeBytes ?? 0)}
            />
          ) : null}
          {summary.busiest ? (
            <Highlight
              label={t('fleet.databases.busiest')}
              row={summary.busiest}
              detail={t('fleet.databases.cpuValue', {
                value: (summary.busiest.cpuPercent ?? 0).toFixed(1),
              })}
            />
          ) : null}
        </div>
      ) : null}

      <ul className="divide-y divide-border rounded-xl border border-border bg-card">
        {summary.rows.slice(0, LIST_CAP).map((r) => (
          <li key={r.name}>
            <Link
              to="/databases/$name"
              params={{ name: r.name }}
              className="flex items-center gap-3 px-4 py-2.5 text-sm hover:bg-muted/40 focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:outline-none"
            >
              <span
                className={cn(
                  'size-2 shrink-0 rounded-full',
                  TONE[HEALTH_TONE[r.health]].solid,
                )}
                aria-hidden="true"
              />
              <span className="min-w-0 flex-1 truncate font-medium">
                {r.name}
              </span>
              <Badge variant="outline">
                {r.engine} {r.version}
              </Badge>
              <span className="w-20 shrink-0 text-right text-xs tabular-nums text-muted-foreground">
                {r.sizeBytes === undefined ? '-' : formatSize(r.sizeBytes)}
              </span>
              <span className="sr-only">
                {t(`fleet.databases.health.${r.health}`)}
              </span>
            </Link>
          </li>
        ))}
      </ul>
      {summary.rows.length > LIST_CAP ? (
        <p className="text-xs text-muted-foreground">
          {t('fleet.databases.more', { count: summary.rows.length - LIST_CAP })}
        </p>
      ) : null}
    </section>
  )
}
