import { createFileRoute } from '@tanstack/react-router'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useRef } from 'react'
import { HardDrivesIcon } from '@phosphor-icons/react/dist/ssr'
import { nodeListQueryOptions, useNodes } from '../../queries/nodes'
import { NODE_LIST_GRID, NodeRow, RowSkeleton } from '../../components/NodeRow'
import { AddNodeDialog } from '../../components/AddNodeDialog'
import { EmptyState } from '../../components/ui/empty-state'

// Typed loader primes the Query cache, the component only reads that
// cache via useNodes() (suspense), mirroring routes/databases/index.tsx
// exactly. The list is virtualized unconditionally, per this project's
// standing rule that every list over 50 items is virtualized, even
// though nodes are a 1-10 machine list for this product's own target
// audience and GET /api/v1/nodes returns everything in one response
// with no server-side pagination: a follow-the-rule exercise here, not
// something that will ever visibly matter at this scale.
export const Route = createFileRoute('/nodes/')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(nodeListQueryOptions()),
  component: NodeListPage,
  pendingComponent: NodeListPending,
})

// Column labels for the sticky header, matching NodeRow's NODE_LIST_GRID
// exactly (icon column and trailing action column stay blank) so the
// header never drifts out of alignment with row content.
function ListHeader() {
  return (
    <div
      className={`${NODE_LIST_GRID} sticky top-0 z-10 border-b border-border bg-card px-4 py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase`}
    >
      <span aria-hidden="true" />
      <span>Name</span>
      <span>Status</span>
      <span>Address</span>
      <span>Last seen</span>
      <span aria-hidden="true" />
    </div>
  )
}

function NodeListPage() {
  const { data: nodes } = useNodes()
  const parentRef = useRef<HTMLDivElement>(null)

  const virtualizer = useVirtualizer({
    count: nodes.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 60,
    overscan: 8,
  })

  return (
    <div>
      <div className="mb-4 flex items-baseline justify-between gap-3">
        <h1 className="text-lg font-semibold text-foreground">Nodes</h1>
        <div className="flex items-baseline gap-3">
          {nodes.length > 0 ? (
            <span className="text-sm text-muted-foreground">
              {nodes.length} {nodes.length === 1 ? 'node' : 'nodes'}
            </span>
          ) : null}
          <AddNodeDialog />
        </div>
      </div>
      {nodes.length === 0 ? (
        <EmptyState
          icon={<HardDrivesIcon className="size-5" />}
          title="Running on this single control plane node"
          description="Join another machine as a node to scale out: spread apps and databases across more than one server, isolate builds from production workloads, or add capacity without resizing this box."
          action={<AddNodeDialog />}
        />
      ) : (
        <div
          ref={parentRef}
          className="h-[70vh] overflow-auto rounded-lg border border-border bg-card"
        >
          <ListHeader />
          <div
            style={{
              height: virtualizer.getTotalSize(),
              position: 'relative',
            }}
          >
            {virtualizer.getVirtualItems().map((virtualRow) => {
              const node = nodes[virtualRow.index]
              if (!node) {
                return null
              }
              return (
                <div
                  key={virtualRow.key}
                  data-index={virtualRow.index}
                  ref={virtualizer.measureElement}
                  style={{
                    position: 'absolute',
                    top: 0,
                    left: 0,
                    width: '100%',
                    transform: `translateY(${virtualRow.start}px)`,
                  }}
                >
                  <NodeRow node={node} />
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}

// Route-level fallback for the loader's pending phase (slow network,
// cold cache): a static stack of RowSkeleton rows under the same header
// and container chrome as the real list, mirroring DatabaseListPending.
function NodeListPending() {
  return (
    <div>
      <div className="mb-4 flex items-baseline justify-between">
        <h1 className="text-lg font-semibold text-foreground">Nodes</h1>
      </div>
      <div className="overflow-hidden rounded-lg border border-border bg-card">
        <ListHeader />
        {Array.from({ length: 3 }, (_, i) => (
          <RowSkeleton key={i} />
        ))}
      </div>
    </div>
  )
}
