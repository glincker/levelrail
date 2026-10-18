import { useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { useVirtualizer } from '@tanstack/react-virtual'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { RelativeTime, StatusPill } from '@/components/kit'
import { deployLogsPath } from '../../queries/deployments'
import { useBuildAction, type BuildRow } from '../../queries/buildCenter'
import {
  cacheLabel,
  deltaLabel,
  durationLabel,
  isStuck,
  isWaiting,
  sizeLabel,
  statusTone,
} from './buildFormat'

const VIRTUALIZE_ABOVE = 50
const ROW_HEIGHT_PX = 76
const GRID =
  'grid grid-cols-[minmax(0,2fr)_minmax(0,1.1fr)_5rem_4rem_minmax(0,1.1fr)_minmax(0,1fr)_minmax(0,1.6fr)] items-center gap-3 px-4'

interface RowProps {
  row: BuildRow
  running: number
  nowMs: number
  onFixCache: () => void
}

function BuildRowItem({ row, running, nowMs, onFixCache }: RowProps) {
  const { t } = useTranslation('buildCenter')
  const cancel = useBuildAction('cancel')
  const retry = useBuildAction('retry')
  const stuck = isStuck(row, running, nowMs)
  const delta = deltaLabel(row.image_delta_bytes)
  const status = isWaiting(row)
    ? t('status.queuedAt', { position: row.queue_position })
    : t(`status.${row.status}`, { defaultValue: row.status })

  const act = (kind: 'cancel' | 'retry') => {
    const m = kind === 'cancel' ? cancel : retry
    m.mutate(row, {
      onSuccess: () => {
        toast.add({ title: t(`actions.${kind}Done`), type: 'success' })
      },
      onError: (e) => {
        toast.add({
          title: t(`actions.${kind}Failed`),
          description: e.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <div
      className={`${GRID} h-[76px] border-b border-border text-sm`}
      data-testid={`build-row-${row.id}`}
    >
      <span className="min-w-0">
        <span className="block truncate font-medium">{row.app}</span>
        <span className="block truncate font-mono text-xs text-muted-foreground">
          {row.commit_sha ? row.commit_sha.slice(0, 8) : row.id}
          {row.branch ? ` ${row.branch}` : ''}
        </span>
      </span>
      <span className="flex min-w-0 flex-col items-start gap-1">
        <StatusPill
          tone={stuck ? 'warning' : statusTone(row.status)}
          label={stuck ? t('status.stuck') : status}
          live={row.status === 'running' && !stuck}
          size="sm"
        />
        <span className="text-xs text-muted-foreground">
          <RelativeTime at={row.started_at} />
        </span>
      </span>
      <span className="tabular-nums">
        {durationLabel(row.duration_seconds)}
      </span>
      <span className="tabular-nums">{cacheLabel(row.cache_hit_ratio)}</span>
      <span className="min-w-0">
        <span className="block tabular-nums">{sizeLabel(row.image_bytes)}</span>
        {delta ? (
          <span className="block text-xs text-muted-foreground tabular-nums">
            {delta}
          </span>
        ) : null}
      </span>
      <span className="min-w-0 text-muted-foreground">
        <span className="block truncate">{row.node || t('node.local')}</span>
        <span className="block truncate text-xs">{row.platform ?? '-'}</span>
      </span>
      <span className="flex min-w-0 flex-col items-end gap-1">
        {row.hint ? (
          <span
            className="block max-w-full truncate text-xs text-tone-warning"
            title={`${row.hint.explanation} ${row.hint.suggestion ?? ''}`}
          >
            {row.hint.title}
          </span>
        ) : null}
        <span className="flex flex-wrap justify-end gap-1.5">
          <Button
            variant="ghost"
            size="sm"
            nativeButton={false}
            render={<Link {...deployLogsPath(row)} />}
          >
            {t('actions.logs')}
          </Button>
          {row.can_cancel ? (
            <Button
              variant={stuck ? 'default' : 'outline'}
              size="sm"
              disabled={cancel.isPending}
              onClick={() => {
                act('cancel')
              }}
            >
              {stuck ? t('actions.cancelStuck') : t('actions.cancel')}
            </Button>
          ) : null}
          {row.can_retry ? (
            <Button
              variant="outline"
              size="sm"
              disabled={retry.isPending}
              onClick={() => {
                act('retry')
              }}
            >
              {t('actions.retry')}
            </Button>
          ) : null}
          {row.hint?.fix === 'prune_cache' ? (
            <Button size="sm" onClick={onFixCache}>
              {t('actions.freeCache')}
            </Button>
          ) : null}
        </span>
      </span>
    </div>
  )
}

interface TableProps {
  rows: BuildRow[]
  running: number
  onFixCache: () => void
}

export function BuildsTable({ rows, running, onFixCache }: TableProps) {
  const { t } = useTranslation('buildCenter')
  const parentRef = useRef<HTMLDivElement>(null)
  const virtualize = rows.length > VIRTUALIZE_ABOVE
  const nowMs = Date.now()
  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_HEIGHT_PX,
    overscan: 8,
  })

  return (
    <div
      ref={parentRef}
      className={`overflow-auto rounded-lg border border-border bg-card ${virtualize ? 'h-[60vh]' : ''}`}
    >
      <div
        className={`${GRID} sticky top-0 z-10 border-b border-border bg-card py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase`}
      >
        <span>{t('columns.build')}</span>
        <span>{t('columns.status')}</span>
        <span>{t('columns.duration')}</span>
        <span>{t('columns.cache')}</span>
        <span>{t('columns.image')}</span>
        <span>{t('columns.node')}</span>
        <span className="text-right">{t('columns.actions')}</span>
      </div>
      {virtualize ? (
        <div
          className="relative"
          style={{ height: virtualizer.getTotalSize() }}
        >
          {virtualizer.getVirtualItems().map((v) => {
            const row = rows[v.index]
            if (!row) return null
            return (
              <div
                key={row.id}
                className="absolute top-0 left-0 w-full"
                style={{ transform: `translateY(${v.start}px)` }}
              >
                <BuildRowItem
                  row={row}
                  running={running}
                  nowMs={nowMs}
                  onFixCache={onFixCache}
                />
              </div>
            )
          })}
        </div>
      ) : (
        rows.map((row) => (
          <BuildRowItem
            key={row.id}
            row={row}
            running={running}
            nowMs={nowMs}
            onFixCache={onFixCache}
          />
        ))
      )}
    </div>
  )
}
