import { Link } from '@tanstack/react-router'
import { CaretRightIcon, DatabaseIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import type { DatabaseListEntry } from '../types/databaseDetail'
import { StatusDot } from './AppRow'

// Shared column grid between the sticky header (routes/databases/index.tsx)
// and every row below (DatabaseRow, RowSkeleton), mirroring AppRow's own
// APP_LIST_GRID convention so header labels line up with row content
// pixel-for-pixel. Icon avatar, name, engine, version, trailing chevron,
// in that order. One column narrower than APP_LIST_GRID: databaseResource
// has no domain field, apps' one extra column has no equivalent here.
export const DATABASE_LIST_GRID =
  'grid grid-cols-[2rem_minmax(0,1.5fr)_minmax(0,1fr)_minmax(0,1fr)_1rem] items-center gap-3'

// Links by database.name, the only identifier databaseResource carries,
// same as AppRow keying off app.name. The status dot is backed by GET
// /api/v1/databases' own batched status field (databaseListResource,
// internal/api/databases.go), the database counterpart to AppRow's own
// GET /api/v1/apps status field, computed server-side from one query
// across every listed database's conditions, no per-row fetch.
export function DatabaseRow({ database }: { database: DatabaseListEntry }) {
  return (
    <Link
      to="/databases/$name"
      params={{ name: database.name }}
      className={`${DATABASE_LIST_GRID} h-full w-full border-b border-border px-4 py-3 transition-colors hover:bg-muted/60`}
    >
      <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
        <DatabaseIcon className="size-4" aria-hidden="true" />
      </span>

      <span className="flex min-w-0 items-center gap-2 text-sm font-medium text-foreground">
        <StatusDot status={database.status} />
        <span className="truncate">{database.name}</span>
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
