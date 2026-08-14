import { createFileRoute, Link } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useRef } from 'react'
import {
  PackageIcon,
  DatabaseIcon,
  PlusIcon,
} from '@phosphor-icons/react/dist/ssr'
import { appListQueryOptions } from '../../queries/apps'
import { APP_LIST_GRID, AppRow, RowSkeleton } from '../../components/AppRow'
import { CreateAppDialog } from '../../components/CreateAppDialog'
import { Button } from '../../components/ui/button'

// Typed loader primes the Query cache, the component only reads that
// cache via useSuspenseQuery against the same key (no data fetching in
// the component body is a project-wide rule). The list is virtualized
// unconditionally (every list that can exceed 50 items must be, per the
// same rule) rather than branching on row count, even though GET
// /api/v1/apps returns everything in one response with no server-side
// pagination to page through.
//
// pendingComponent is purely a rendering fallback for the loader's own
// pending phase (router-level, shown only past TanStack Router's default
// pendingMs), it does not add or change a data fetch: the loader above
// is still the one and only ensureQueryData call.
export const Route = createFileRoute('/apps/')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(appListQueryOptions()),
  component: AppListPage,
  pendingComponent: AppListPending,
})

// Column labels for the sticky header, matching AppRow's APP_LIST_GRID
// exactly (icon column and trailing chevron column stay blank) so the
// header never drifts out of alignment with row content.
function ListHeader() {
  return (
    <div
      className={`${APP_LIST_GRID} sticky top-0 z-10 border-b border-border bg-card px-4 py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase`}
    >
      <span aria-hidden="true" />
      <span>Name</span>
      <span>Image</span>
      <span>Domain</span>
      <span>Port</span>
      <span aria-hidden="true" />
    </div>
  )
}

function AppListPage() {
  const { data: apps } = useSuspenseQuery(appListQueryOptions())
  const parentRef = useRef<HTMLDivElement>(null)

  const virtualizer = useVirtualizer({
    count: apps.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 60,
    overscan: 8,
  })

  return (
    <div>
      <div className="mb-4 flex items-baseline justify-between gap-3">
        <h1 className="text-lg font-semibold text-foreground">Apps</h1>
        <div className="flex items-baseline gap-3">
          {apps.length > 0 ? (
            <span className="text-sm text-muted-foreground">
              {apps.length} {apps.length === 1 ? 'app' : 'apps'}
            </span>
          ) : null}
          {/* Secondary entry point for the databases resource kind,
              lower-emphasis (outline) than "New app" since the sidebar's
              Databases nav item is the primary way in: this is here
              purely so the gap Coolify closes (creating a database at
              all) is discoverable from the page most operators land on
              first, not a competing CTA. */}
          <Button
            size="sm"
            variant="outline"
            render={<Link to="/databases" />}
            nativeButton={false}
          >
            <DatabaseIcon />
            New database
          </Button>
          <CreateAppDialog
            trigger={
              <Button size="sm">
                <PlusIcon />
                New app
              </Button>
            }
          />
        </div>
      </div>
      {apps.length === 0 ? (
        <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed border-border bg-card/50 px-4 py-16 text-center">
          <span className="flex size-10 items-center justify-center rounded-full bg-muted text-muted-foreground">
            <PackageIcon className="size-5" aria-hidden="true" />
          </span>
          <div className="space-y-1">
            <p className="text-sm font-medium text-foreground">No apps yet</p>
            <p className="mx-auto max-w-sm text-sm text-muted-foreground">
              Apps deployed from an app.yaml spec in a connected repo will show
              up here, or create one directly if you already have a built image.
            </p>
          </div>
          <CreateAppDialog
            trigger={
              <Button size="sm">
                <PlusIcon />
                New app
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
              const app = apps[virtualRow.index]
              if (!app) {
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
                  <AppRow app={app} />
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
// and container chrome as the real list, so a slow /api/v1/apps request
// shows a layout-accurate loading state instead of a blank page.
function AppListPending() {
  return (
    <div>
      <div className="mb-4 flex items-baseline justify-between">
        <h1 className="text-lg font-semibold text-foreground">Apps</h1>
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
