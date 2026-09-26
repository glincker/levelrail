import { useRef } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { BroomIcon, HardDrivesIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { useIsRoot } from '../hooks/useIsRoot'
import { formatBytes } from '../lib/modelPreflight'
import { useModelCache } from '../queries/modelPreflight'
import type { CacheEntry, CacheNode } from '../types/modelPreflight'
import {
  EmptyState,
  InfoTip,
  RelativeTime,
  SkeletonList,
  StatusPill,
} from './kit'
import { PruneModelCacheDialog } from './PruneModelCacheDialog'

const ROW_HEIGHT = 60
const VIRTUALIZE_ABOVE = 50

function entryState(e: CacheEntry): {
  tone: 'success' | 'warning' | 'neutral'
  label: string
} {
  if (e.prunable) return { tone: 'warning', label: 'Prunable' }
  if (e.in_use) return { tone: 'success', label: 'In use' }
  if (e.configured) return { tone: 'success', label: 'Configured' }
  return { tone: 'neutral', label: 'Kept' }
}

function CacheRow({
  entry,
  unusedDays,
}: {
  entry: CacheEntry
  unusedDays: number
}) {
  const state = entryState(entry)
  return (
    <div className="grid grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_auto] items-center gap-3 border-b border-border px-4 py-2 text-sm last:border-b-0">
      <div className="min-w-0">
        <p
          className="truncate font-mono text-xs text-foreground"
          title={entry.volume}
        >
          {entry.volume}
        </p>
        <p className="truncate text-xs text-muted-foreground">
          {entry.model ? `Model ${entry.model}` : 'No model uses this'}
          {entry.duplicate_of
            ? `, same weights as ${entry.duplicate_of}, counted once`
            : ''}
        </p>
      </div>
      <span className="text-foreground">
        {entry.size_bytes === null
          ? 'size unknown'
          : formatBytes(entry.size_bytes)}
      </span>
      <span className="text-xs text-muted-foreground">
        {entry.last_used_at ? (
          <>
            Used <RelativeTime at={entry.last_used_at} />
          </>
        ) : (
          'last use unknown'
        )}
        {entry.unused ? ` (over ${unusedDays} days)` : ''}
      </span>
      <StatusPill
        tone={state.tone}
        label={state.label}
        size="sm"
        title={entry.keep_reason}
      />
    </div>
  )
}

function VirtualCacheEntries({
  node,
  unusedDays,
}: {
  node: CacheNode
  unusedDays: number
}) {
  const parentRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({
    count: node.entries.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 8,
  })
  return (
    <div
      ref={parentRef}
      className="max-h-80 overflow-auto rounded-lg border border-border bg-card"
    >
      <div className="relative" style={{ height: virtualizer.getTotalSize() }}>
        {virtualizer.getVirtualItems().map((row) => {
          const entry = node.entries[row.index]
          if (!entry) return null
          return (
            <div
              key={row.key}
              className="absolute top-0 left-0 w-full"
              style={{ transform: `translateY(${row.start}px)` }}
            >
              <CacheRow entry={entry} unusedDays={unusedDays} />
            </div>
          )
        })}
      </div>
    </div>
  )
}

function CacheEntries({
  node,
  unusedDays,
}: {
  node: CacheNode
  unusedDays: number
}) {
  if (node.entries.length > VIRTUALIZE_ABOVE) {
    return <VirtualCacheEntries node={node} unusedDays={unusedDays} />
  }
  return (
    <div className="max-h-80 overflow-auto rounded-lg border border-border bg-card">
      {node.entries.map((entry) => (
        <CacheRow key={entry.volume} entry={entry} unusedDays={unusedDays} />
      ))}
    </div>
  )
}

function NodeCache({
  node,
  unusedDays,
}: {
  node: CacheNode
  unusedDays: number
}) {
  const isRoot = useIsRoot()
  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-foreground">
          <span className="font-medium">{node.name}</span>
          <span className="text-muted-foreground">
            {' '}
            {formatBytes(node.total_bytes)} cached,{' '}
            {formatBytes(node.unique_bytes)} unique
            {node.disk_free_bytes !== null
              ? `, ${formatBytes(node.disk_free_bytes)} disk free`
              : ''}
          </span>
        </p>
        {node.supported && isRoot ? (
          <PruneModelCacheDialog
            reclaimableBytes={node.reclaimable_bytes}
            unusedDays={unusedDays}
          />
        ) : null}
      </div>
      {node.message ? (
        <p className="text-xs text-muted-foreground">{node.message}</p>
      ) : null}
      {node.entries.length > 0 ? (
        <CacheEntries node={node} unusedDays={unusedDays} />
      ) : node.supported ? (
        <p className="text-xs text-muted-foreground">
          No model weights cached on this node.
        </p>
      ) : null}
    </div>
  )
}

// Cached model weights per node, with a safe prune: volumes a model or
// container still uses are never offered for removal.
export function ModelCacheCard() {
  const { data, isPending, isError, error, refetch } = useModelCache()
  if (isPending) return <SkeletonList rows={2} />
  if (isError) {
    return (
      <div
        role="alert"
        className="flex items-center gap-3 text-sm text-destructive"
      >
        <span>{error.message}</span>
        <Button size="sm" variant="outline" onClick={() => void refetch()}>
          Retry
        </Button>
      </div>
    )
  }
  const hasAny = data.nodes.some((n) => n.entries.length > 0)
  if (!hasAny && data.nodes.every((n) => !n.message)) {
    return (
      <EmptyState
        icon={<HardDrivesIcon className="size-5" />}
        title="No cached model weights"
        description="Downloaded weights appear here once a model has been deployed on this host."
      />
    )
  }
  return (
    <div className="space-y-4">
      <p className="flex items-center gap-1 text-xs text-muted-foreground">
        <BroomIcon className="size-3.5" aria-hidden="true" />
        Unused means no use for {data.unused_days} days.
        <InfoTip label="How the cache is measured">{data.note}</InfoTip>
      </p>
      {data.nodes.map((n) => (
        <NodeCache
          key={n.node_id || n.name}
          node={n}
          unusedDays={data.unused_days}
        />
      ))}
    </div>
  )
}
