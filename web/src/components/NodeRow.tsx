import { ServerIcon } from 'lucide-react'
import type { NodeResource, NodeStatus } from '../types/nodeDetail'
import { DeleteNodeDialog } from './DeleteNodeDialog'

// Shared column grid between the sticky header (routes/nodes/index.tsx)
// and every row below (NodeRow, RowSkeleton), the same convention
// DATABASE_LIST_GRID and APP_LIST_GRID already establish. Nodes have no
// detail route (this pass only builds list + add + delete, per the gap
// this closes), so unlike DatabaseRow/AppRow the row itself is not a
// Link: the trailing column holds an inline delete action instead of a
// chevron.
export const NODE_LIST_GRID =
  'grid grid-cols-[2rem_minmax(0,1.5fr)_7rem_minmax(0,1fr)_minmax(0,1fr)_auto] items-center gap-3'

// No "success"/"warning" badge variant exists in badgeVariants
// (components/ui/badge.tsx), so these override background/text classes
// directly, the same precedent ConditionsPanel's STATUS_BADGE_CLASS and
// AlertRulesPanel's STATE_BADGE_CLASS already set. No dot/pulse
// treatment here (unlike AlertRulesPanel's StateDot): there is no live
// status endpoint backing this page, status is exactly as fresh as the
// last list fetch, and a pulsing indicator would imply a liveness this
// page does not have.
const STATUS_BADGE_CLASS: Record<NodeStatus, string> = {
  pending: 'bg-muted text-muted-foreground',
  online:
    'bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-300',
  offline: 'bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-300',
  cordoned:
    'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300',
}

const STATUS_LABEL: Record<NodeStatus, string> = {
  pending: 'Pending',
  online: 'Online',
  offline: 'Offline',
  cordoned: 'Cordoned',
}

// Local formatter rather than a new export in lib/format.ts: matches
// routes/settings/security.tsx's own local formatDate precedent
// (toLocaleString(), no shared helper introduced for one-line date
// display) instead of centralizing a one-liner.
function formatNodeDate(iso?: string): string {
  if (!iso) {
    return 'Never'
  }
  return new Date(iso).toLocaleString()
}

export function NodeRow({ node }: { node: NodeResource }) {
  return (
    <div
      className={`${NODE_LIST_GRID} h-full w-full border-b border-border px-4 py-3`}
    >
      <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
        <ServerIcon className="size-4" aria-hidden="true" />
      </span>

      <span className="min-w-0 truncate text-sm font-medium text-foreground">
        {node.name}
      </span>

      <span className="min-w-0">
        <span
          className={`inline-flex w-fit shrink-0 items-center rounded-md px-2 py-0.5 text-xs font-medium whitespace-nowrap ${STATUS_BADGE_CLASS[node.status]}`}
        >
          {STATUS_LABEL[node.status]}
        </span>
      </span>

      <span className="min-w-0 truncate font-mono text-xs text-muted-foreground">
        {node.address ?? 'Not set'}
      </span>

      <span className="min-w-0 truncate text-xs text-muted-foreground">
        {formatNodeDate(node.last_seen_at)}
      </span>

      <span className="shrink-0 justify-self-end">
        <DeleteNodeDialog id={node.id} name={node.name} />
      </span>
    </div>
  )
}

// Backs the list route's pendingComponent, matching NODE_LIST_GRID
// exactly so the skeleton doesn't jump when real rows swap in, same
// reasoning DatabaseRow's own RowSkeleton comment gives.
export function RowSkeleton() {
  return (
    <div
      className={`${NODE_LIST_GRID} border-b border-border px-4 py-3`}
      aria-hidden="true"
    >
      <div className="size-8 animate-pulse rounded-md bg-muted" />
      <div className="h-4 w-32 animate-pulse rounded bg-muted" />
      <div className="h-4 w-16 animate-pulse rounded bg-muted" />
      <div className="h-4 w-24 animate-pulse rounded bg-muted" />
      <div className="h-4 w-24 animate-pulse rounded bg-muted" />
      <div className="h-6 w-16 animate-pulse justify-self-end rounded bg-muted" />
    </div>
  )
}
