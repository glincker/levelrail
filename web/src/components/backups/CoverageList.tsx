import { useRef } from 'react'
import { Link } from '@tanstack/react-router'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { formatBytes, formatDate } from '../../lib/format'
import type { CoverageRow, CoverageTarget } from '../../types/backupCoverage'
import { CoverageRowActions } from './CoverageRowActions'
import {
  coverageBadgeVariant,
  rowKey,
  VIRTUALIZE_AFTER,
} from './coverageHelpers'

const ROW_ESTIMATE_PX = 96

function ResourceName({ row }: { row: CoverageRow }) {
  const { t } = useTranslation('backups')
  const link =
    row.kind === 'volume' ? (
      <Link
        to="/apps/$name/volumes"
        params={{ name: row.app_name ?? '' }}
        className="underline-offset-2 hover:underline"
      >
        {row.app_name}/{row.name}
      </Link>
    ) : (
      <Link
        to="/databases/$name/overview"
        params={{ name: row.name }}
        className="underline-offset-2 hover:underline"
      >
        {row.name}
      </Link>
    )
  return (
    <div className="min-w-0">
      <div className="flex flex-wrap items-center gap-2">
        <span className="truncate font-medium">{link}</span>
        <Badge variant="outline">{t(`row.kind.${row.kind}`)}</Badge>
        {row.scratch ? <Badge variant="muted">{t('row.scratch')}</Badge> : null}
      </div>
      {row.reason ? (
        <p className="mt-0.5 text-xs text-muted-foreground">{row.reason}</p>
      ) : null}
    </div>
  )
}

function Row({
  row,
  targets,
  onNeedTarget,
}: {
  row: CoverageRow
  targets: CoverageTarget[]
  onNeedTarget: () => void
}) {
  const { t } = useTranslation('backups')
  return (
    <li className="grid gap-2 border-b border-border px-3 py-3 last:border-b-0 md:grid-cols-[minmax(0,1.6fr)_auto_minmax(0,1.4fr)_auto] md:items-center md:gap-4">
      <ResourceName row={row} />
      <div>
        <Badge variant={coverageBadgeVariant(row.state)}>
          {t(`coverage.state.${row.state}`)}
        </Badge>
      </div>
      <dl className="grid grid-cols-2 gap-x-3 gap-y-0.5 text-xs text-muted-foreground">
        <dt>{t('coverage.lastBackup')}</dt>
        <dd className="text-foreground">
          {row.last_backup_at
            ? formatDate(row.last_backup_at, '-')
            : t('health.none')}
        </dd>
        <dt>{t('coverage.lastRestore')}</dt>
        <dd className="text-foreground">
          {row.last_verified_at
            ? formatDate(row.last_verified_at, '-')
            : t('health.never')}
        </dd>
        <dt>{t('coverage.nextRun')}</dt>
        <dd className="text-foreground">
          {row.next_run
            ? formatDate(row.next_run, '-')
            : t('health.notScheduled')}
        </dd>
        <dt>{t('coverage.size')}</dt>
        <dd className="text-foreground">{formatBytes(row.total_bytes)}</dd>
      </dl>
      <CoverageRowActions
        row={row}
        targets={targets}
        onNeedTarget={onNeedTarget}
      />
    </li>
  )
}

function VirtualRows({
  rows,
  targets,
  onNeedTarget,
}: {
  rows: CoverageRow[]
  targets: CoverageTarget[]
  onNeedTarget: () => void
}) {
  const parentRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_ESTIMATE_PX,
    overscan: 6,
  })
  return (
    <div ref={parentRef} className="max-h-[70dvh] overflow-y-auto">
      <ul className="relative" style={{ height: virtualizer.getTotalSize() }}>
        {virtualizer.getVirtualItems().map((item) => {
          const row = rows[item.index]
          if (!row) return null
          return (
            <div
              key={rowKey(row)}
              ref={virtualizer.measureElement}
              data-index={item.index}
              className="absolute inset-x-0 top-0"
              style={{ transform: `translateY(${String(item.start)}px)` }}
            >
              <Row row={row} targets={targets} onNeedTarget={onNeedTarget} />
            </div>
          )
        })}
      </ul>
    </div>
  )
}

/** CoverageList shows every database and app volume with its protection state and one primary action. */
export function CoverageList({
  rows,
  targets,
  onNeedTarget,
}: {
  rows: CoverageRow[]
  targets: CoverageTarget[]
  onNeedTarget: () => void
}) {
  return (
    <div className="overflow-hidden rounded-lg border border-border">
      {rows.length > VIRTUALIZE_AFTER ? (
        <VirtualRows
          rows={rows}
          targets={targets}
          onNeedTarget={onNeedTarget}
        />
      ) : (
        <ul>
          {rows.map((row) => (
            <Row
              key={rowKey(row)}
              row={row}
              targets={targets}
              onNeedTarget={onNeedTarget}
            />
          ))}
        </ul>
      )}
    </div>
  )
}
