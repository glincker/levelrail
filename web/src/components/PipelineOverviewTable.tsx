import { useRef } from 'react'
import { Link } from '@tanstack/react-router'
import { useVirtualizer } from '@tanstack/react-virtual'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { formatSeconds } from '../lib/pipelineOverview'
import { STATUS_LABEL, STATUS_VARIANT } from '../lib/pipelineStatus'
import type { PipelineRunRow } from '../types/pipelineOverview'
import { PipelineStatusIcon } from './PipelineStatusBadge'

const GRID =
  'grid grid-cols-[minmax(7rem,9rem)_minmax(0,2fr)_minmax(0,1fr)_minmax(0,1.5fr)_minmax(0,1.5fr)_5rem] items-center gap-3 px-4'

function StatusCell({ row }: { row: PipelineRunRow }) {
  const label = row.hold_pending
    ? 'Held'
    : row.approval_pending
      ? 'Needs approval'
      : STATUS_LABEL[row.status]
  const variant =
    row.hold_pending || row.approval_pending
      ? 'warning'
      : STATUS_VARIANT[row.status]
  return (
    <Badge variant={variant}>
      <PipelineStatusIcon status={row.status} />
      {label}
    </Badge>
  )
}

function RunRow({ row }: { row: PipelineRunRow }) {
  return (
    <Link
      to="/apps/$name/pipelines/runs/$runId"
      params={{ name: row.app, runId: row.id }}
      className={`${GRID} border-b border-border py-2 text-sm hover:bg-muted/50`}
    >
      <StatusCell row={row} />
      <span className="truncate text-foreground">
        <span className="font-medium">{row.app}</span> / {row.pipeline} #
        {row.number}
      </span>
      <span className="truncate text-xs text-muted-foreground">
        {row.trigger}
      </span>
      <span className="truncate font-mono text-xs text-muted-foreground">
        {[row.ref, row.short_sha].filter(Boolean).join(' @ ')}
      </span>
      <span className="truncate text-xs text-muted-foreground">
        {new Date(row.created_at).toLocaleString()}
      </span>
      <span className="text-xs tabular-nums text-muted-foreground">
        {formatSeconds(row.duration_seconds)}
      </span>
    </Link>
  )
}

export function PipelineOverviewTable({
  rows,
  hasMore,
  loadingMore,
  onLoadMore,
}: {
  rows: PipelineRunRow[]
  hasMore: boolean
  loadingMore: boolean
  onLoadMore: () => void
}) {
  const parentRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 44,
    overscan: 8,
  })
  return (
    <div className="space-y-2">
      <div
        ref={parentRef}
        className="h-[60vh] overflow-auto rounded-lg border border-border bg-card"
      >
        <div
          className={`${GRID} sticky top-0 z-10 border-b border-border bg-card py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase`}
        >
          <span>Status</span>
          <span>Run</span>
          <span>Trigger</span>
          <span>Ref</span>
          <span>Started</span>
          <span>Duration</span>
        </div>
        <div
          style={{ height: virtualizer.getTotalSize(), position: 'relative' }}
        >
          {virtualizer.getVirtualItems().map((v) => {
            const row = rows[v.index]
            if (!row) {
              return null
            }
            return (
              <div
                key={v.key}
                data-index={v.index}
                ref={virtualizer.measureElement}
                style={{
                  position: 'absolute',
                  top: 0,
                  left: 0,
                  width: '100%',
                  transform: `translateY(${v.start}px)`,
                }}
              >
                <RunRow row={row} />
              </div>
            )
          })}
        </div>
      </div>
      {hasMore ? (
        <div className="flex justify-center">
          <Button
            variant="outline"
            size="sm"
            disabled={loadingMore}
            onClick={onLoadMore}
          >
            {loadingMore ? 'Loading...' : 'Load more'}
          </Button>
        </div>
      ) : null}
    </div>
  )
}
