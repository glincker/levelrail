import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useRef } from 'react'
import {
  DatabaseIcon,
  PlusIcon,
} from '@phosphor-icons/react/dist/ssr'
import { databaseListQueryOptions } from '../../queries/databases'
import {
  DATABASE_LIST_GRID,
  DatabaseRow,
  RowSkeleton,
} from '../../components/DatabaseRow'
import { CreateDatabaseDialog } from '../../components/CreateDatabaseDialog'
import { Button } from '../../components/ui/button'

// Typed loader primes the Query cache, the component only reads that
// cache via useSuspenseQuery against the same key, mirroring
// routes/apps/index.tsx exactly. The list is virtualized unconditionally
// (every list that can exceed 50 items must be, a project-wide rule)
// even though databases will rarely exceed a handful of rows and GET
// /api/v1/databases returns everything in one response with no
// server-side pagination.
export const Route = createFileRoute('/databases/')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(databaseListQueryOptions()),
  component: DatabaseListPage,
  pendingComponent: DatabaseListPending,
})

// Column labels for the sticky header, matching DatabaseRow's
// DATABASE_LIST_GRID exactly (icon column and trailing chevron column
// stay blank) so the header never drifts out of alignment with row
// content.
function ListHeader() {
  return (
    <div
      className={`${DATABASE_LIST_GRID} sticky top-0 z-10 border-b border-border bg-card px-4 py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase`}
    >
      <span aria-hidden="true" />
      <span>Name</span>
      <span>Engine</span>
      <span>Version</span>
      <span aria-hidden="true" />
    </div>
  )
}

function DatabaseListPage() {
  const { data: databases } = useSuspenseQuery(databaseListQueryOptions())
  const parentRef = useRef<HTMLDivElement>(null)

  const virtualizer = useVirtualizer({
    count: databases.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 60,
    overscan: 8,
  })

  return (
    <div>
      <div className="mb-4 flex items-baseline justify-between gap-3">
        <h1 className="text-lg font-semibold text-foreground">Databases</h1>
        <div className="flex items-baseline gap-3">
          {databases.length > 0 ? (
            <span className="text-sm text-muted-foreground">
              {databases.length}{' '}
              {databases.length === 1 ? 'database' : 'databases'}
            </span>
          ) : null}
          <CreateDatabaseDialog
            trigger={
              <Button size="sm">
                <PlusIcon />
                New database
              </Button>
            }
          />
        </div>
      </div>
      {databases.length === 0 ? (
        <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed border-border bg-card/50 px-4 py-16 text-center">
          <span className="flex size-10 items-center justify-center rounded-full bg-muted text-muted-foreground">
            <DatabaseIcon className="size-5" aria-hidden="true" />
          </span>
          <div className="space-y-1">
            <p className="text-sm font-medium text-foreground">
              No databases yet
            </p>
            <p className="mx-auto max-w-sm text-sm text-muted-foreground">
              Create a managed Postgres or Redis database and reference it from
              an app&apos;s environment.
            </p>
          </div>
          <CreateDatabaseDialog
            trigger={
              <Button size="sm">
                <PlusIcon />
                New database
              </Button>
            }
          />
        </div>
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
              const database = databases[virtualRow.index]
              if (!database) {
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
                  <DatabaseRow database={database} />
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
// and container chrome as the real list, mirroring AppListPending.
function DatabaseListPending() {
  return (
    <div>
      <div className="mb-4 flex items-baseline justify-between">
        <h1 className="text-lg font-semibold text-foreground">Databases</h1>
      </div>
      <div className="overflow-hidden rounded-lg border border-border bg-card">
        <ListHeader />
        {Array.from({ length: 6 }, (_, i) => (
          <RowSkeleton key={i} />
        ))}
      </div>
    </div>
  )
}
