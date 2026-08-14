import { Link } from '@tanstack/react-router'
import {
  CaretRightIcon,
  DatabaseIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import type { DatabaseResource } from '../types/databaseDetail'

// Shared column grid between the sticky header (routes/databases/index.tsx)
// and every row below (DatabaseRow, RowSkeleton), mirroring AppRow's own
// APP_LIST_GRID convention so header labels line up with row content
// pixel-for-pixel. Icon avatar, name, engine, version, trailing chevron,
// in that order. One column narrower than APP_LIST_GRID: databaseResource
// has no domain field, apps' one extra column has no equivalent here.
export const DATABASE_LIST_GRID =
  'grid grid-cols-[2rem_minmax(0,1.5fr)_minmax(0,1fr)_minmax(0,1fr)_1rem] items-center gap-3'

// Links by database.name, the only identifier databaseResource carries,
// same as AppRow keying off app.name. No live status dot here for the
// same N+1 reasoning AppRow's own comment gives: rendering one would
// mean an extra GET /api/v1/databases/{name}/status per row, exactly
// the kind of per-row fetch the project's virtualization requirement
// exists to avoid (lists over 50 items must stay cheap to render).
// Current reconcile status lives on the detail route's
// ConditionsPanel instead.
export function DatabaseRow({ database }: { database: DatabaseResource }) {
  return (
    <Link
      to="/databases/$name"
      params={{ name: database.name }}
      className={`${DATABASE_LIST_GRID} h-full w-full border-b border-border px-4 py-3 transition-colors hover:bg-muted/60`}
    >
      <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
        <DatabaseIcon className="size-4" aria-hidden="true" />
      </span>

      <span className="min-w-0 truncate text-sm font-medium text-foreground">
        {database.name}
      </span>

      <span className="min-w-0">
        <Badge variant="outline" className="font-mono text-[11px] capitalize">
          {database.engine}
        </Badge>
      </span>

      <span className="min-w-0 truncate font-mono text-xs text-muted-foreground">
        {database.version}
      </span>

      <CaretRightIcon
        className="size-4 shrink-0 justify-self-end text-muted-foreground/50"
        aria-hidden="true"
      />
    </Link>
  )
}

// Backs the list route's pendingComponent, matching DATABASE_LIST_GRID
// exactly so the skeleton doesn't jump when real rows swap in, same
// reasoning AppRow's own RowSkeleton comment gives.
export function RowSkeleton() {
  return (
    <div
      className={`${DATABASE_LIST_GRID} border-b border-border px-4 py-3`}
      aria-hidden="true"
    >
      <div className="size-8 animate-pulse rounded-md bg-muted" />
      <div className="h-4 w-32 animate-pulse rounded bg-muted" />
      <div className="h-4 w-16 animate-pulse rounded bg-muted" />
      <div className="h-4 w-10 animate-pulse rounded bg-muted" />
      <div className="h-4 w-4 animate-pulse justify-self-end rounded bg-muted" />
    </div>
  )
}
