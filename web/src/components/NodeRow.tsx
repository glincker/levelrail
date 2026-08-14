import { Link } from '@tanstack/react-router'
import { CaretRightIcon, HardDrivesIcon } from '@phosphor-icons/react/dist/ssr'
import type { NodeResource, NodeStatus } from '../types/nodeDetail'
import { DeleteNodeDialog } from './DeleteNodeDialog'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { VariantProps } from 'class-variance-authority'

// Shared column grid between the sticky header (routes/nodes/index.tsx)
// and every row below (NodeRow, RowSkeleton), the same convention
// DATABASE_LIST_GRID and APP_LIST_GRID already establish. The status
// column is wider than the original single-badge version to fit two
// badges side by side (connectivity + schedulable, see the comment on
// STATUS_BADGE_VARIANT below for why these are two separate signals).
// Unlike DatabaseRow/AppRow the row itself is not a whole-row Link:
// cordon/drain/health now genuinely justify a detail route
// (routes/nodes/$id.tsx), but the trailing column holds a "Manage" link
// and the delete action as sibling controls rather than nesting a Link
// around the whole row, which would otherwise nest DeleteNodeDialog's
// own interactive Dialog trigger inside another interactive element.
export const NODE_LIST_GRID =
  'grid grid-cols-[2rem_minmax(0,1.5fr)_minmax(9rem,auto)_minmax(0,1fr)_minmax(0,1fr)_auto] items-center gap-3'

// Connectivity (Status) and cordon (Schedulable) are two independent
// axes, not one merged state: internal/store/nodes.go's own doc comment
// on NodeStatusCordoned confirms nothing in this codebase actually sets
// status to "cordoned" (the only real writers of UpdateNodeStatus, the
// node-health controller and the agent's heartbeat handler, only ever
// write online/offline), and nodeResource's own doc comment in
// internal/api/nodes.go is explicit that Schedulable is a separate field
// from Status rather than folded into it. So this row renders two
// badges: STATUS_BADGE_VARIANT for connectivity, and a second
// schedulable-driven badge built inline below, both using the shared
// Badge component's variants (components/ui/badge.tsx) rather than
// hand-rolled color classes.
const STATUS_BADGE_VARIANT: Record<
  NodeStatus,
  VariantProps<typeof badgeVariants>['variant']
> = {
  pending: 'muted',
  online: 'success',
  offline: 'destructive',
  cordoned: 'warning',
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
        <HardDrivesIcon className="size-4" aria-hidden="true" />
      </span>

      <span className="min-w-0 truncate text-sm font-medium text-foreground">
        {node.name}
      </span>

      <span className="flex min-w-0 flex-wrap items-center gap-1">
        <Badge variant={STATUS_BADGE_VARIANT[node.status]}>
          {STATUS_LABEL[node.status]}
        </Badge>
        {node.schedulable ? null : <Badge variant="warning">Cordoned</Badge>}
      </span>

      <span className="min-w-0 truncate font-mono text-xs text-muted-foreground">
        {node.address ?? 'Not set'}
      </span>

      <span className="min-w-0 truncate text-xs text-muted-foreground">
        {formatNodeDate(node.last_seen_at)}
      </span>

      <span className="flex shrink-0 items-center gap-2 justify-self-end">
        <Link to="/nodes/$id" params={{ id: node.id }}>
          <Button type="button" variant="outline" size="sm">
            Manage
            <CaretRightIcon className="size-3.5" aria-hidden="true" />
          </Button>
        </Link>
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
      <div className="h-6 w-28 animate-pulse justify-self-end rounded bg-muted" />
    </div>
  )
}
