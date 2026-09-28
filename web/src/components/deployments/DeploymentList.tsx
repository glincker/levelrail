import { useEffect, useRef, type RefObject } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { Button } from '@/components/ui/button'
import { SkeletonLine } from '@/components/kit'
import type { Deployment } from '../../types/deployment'
import { DeploymentRow } from './DeploymentRow'

export const VIRTUALIZE_AFTER = 50
const ESTIMATED_ROW_PX = 56

export function DeploymentsSkeleton({ rows = 8 }: { rows?: number }) {
  return (
    <div
      role="status"
      aria-label="Loading deployments"
      className="overflow-hidden rounded-xl border border-border bg-card"
    >
      {Array.from({ length: rows }, (_, i) => (
        <div
          key={i}
          data-testid="deployment-skeleton-row"
          className="flex items-center gap-3 border-b border-border px-3 py-3.5 last:border-b-0"
        >
          <SkeletonLine width="38%" />
          <div className="ml-auto flex items-center gap-4">
            <SkeletonLine width={64} />
            <SkeletonLine width={80} className="hidden md:block" />
            <SkeletonLine width={56} className="hidden md:block" />
          </div>
        </div>
      ))}
    </div>
  )
}

export interface DeploymentListProps {
  rows: Deployment[]
  now: number
  openId: string
  focusId: string
  onOpen: (id: string) => void
  scrollRef: RefObject<HTMLDivElement | null>
}

function PlainList({
  rows,
  now,
  openId,
  focusId,
  onOpen,
}: DeploymentListProps) {
  useEffect(() => {
    if (!focusId) return
    const el = document.querySelector(
      `[data-deployment-id="${CSS.escape(focusId)}"]`,
    )
    if (el && typeof el.scrollIntoView === 'function') {
      el.scrollIntoView({ block: 'nearest' })
    }
  }, [focusId])
  return (
    <div className="overflow-hidden rounded-xl border border-border bg-card">
      {rows.map((d) => (
        <DeploymentRow
          key={d.id}
          d={d}
          now={now}
          open={d.id === openId}
          focused={d.id === focusId}
          onOpen={onOpen}
        />
      ))}
    </div>
  )
}

function VirtualList({
  rows,
  now,
  openId,
  focusId,
  onOpen,
  scrollRef,
}: DeploymentListProps) {
  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ESTIMATED_ROW_PX,
    overscan: 8,
    getItemKey: (i) => rows[i]?.id ?? i,
  })
  const lastFocus = useRef('')
  useEffect(() => {
    if (!focusId || focusId === lastFocus.current) return
    lastFocus.current = focusId
    const idx = rows.findIndex((r) => r.id === focusId)
    if (idx >= 0) virtualizer.scrollToIndex(idx, { align: 'auto' })
  }, [focusId, rows, virtualizer])

  return (
    <div
      ref={scrollRef}
      className="h-[70vh] overflow-auto rounded-xl border border-border bg-card"
    >
      {/* Virtual row offsets are computed at runtime, so these two styles cannot be Tailwind classes. */}
      <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
        {virtualizer.getVirtualItems().map((v) => {
          const d = rows[v.index]
          if (!d) return null
          return (
            <div
              key={v.key}
              data-index={v.index}
              ref={virtualizer.measureElement}
              className="absolute top-0 left-0 w-full"
              style={{ transform: `translateY(${String(v.start)}px)` }}
            >
              <DeploymentRow
                d={d}
                now={now}
                open={d.id === openId}
                focused={d.id === focusId}
                onOpen={onOpen}
              />
            </div>
          )
        })}
      </div>
    </div>
  )
}

export function DeploymentList(props: DeploymentListProps) {
  return props.rows.length > VIRTUALIZE_AFTER ? (
    <VirtualList {...props} />
  ) : (
    <PlainList {...props} />
  )
}

export function LoadMore({
  hasNext,
  fetching,
  failed,
  onLoad,
}: {
  hasNext: boolean
  fetching: boolean
  failed: boolean
  onLoad: () => void
}) {
  if (!hasNext) return null
  return (
    <div className="flex flex-col items-center gap-1 pt-3">
      {failed && (
        <p className="text-xs text-tone-danger">Could not load more.</p>
      )}
      <Button variant="outline" size="sm" disabled={fetching} onClick={onLoad}>
        {fetching ? 'Loading' : failed ? 'Retry' : 'Load More'}
      </Button>
    </div>
  )
}
