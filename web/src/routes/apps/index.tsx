import { createFileRoute, Link } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useMemo, useRef, useState } from 'react'
import {
  PackageIcon,
  DatabaseIcon,
  GitBranchIcon,
  PlusIcon,
} from '@phosphor-icons/react/dist/ssr'
import { appListQueryOptions } from '../../queries/apps'
import { staticSitesQueryOptions } from '../../queries/staticSites'
import { APP_LIST_GRID, AppRow, RowSkeleton } from '../../components/AppRow'
import { CreateResourceWizard } from '../../components/CreateResourceWizard'
import { StaticSitesCard } from '../../components/StaticSitesCard'
import { AppsBulkBar } from '../../components/AppsBulkBar'
import { AppsFilterBar } from '../../components/AppsFilterBar'
import { AppsStatusStrip } from '../../components/AppsStatusStrip'
import {
  EMPTY_FILTERS,
  filtersActive,
  type AppListFilters,
} from '../../lib/appViews'
import { Button } from '../../components/ui/button'
import { EmptyState } from '../../components/ui/empty-state'

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
    Promise.all([
      queryClient.ensureQueryData(appListQueryOptions()),
      queryClient.ensureQueryData(staticSitesQueryOptions()),
    ]),
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
  const [filters, setFilters] = useState<AppListFilters>(EMPTY_FILTERS)
  const [selected, setSelected] = useState<string[]>([])

  const environments = useMemo(
    () =>
      [
        ...new Set(
          apps.flatMap((app) =>
            app.environment_name ? [app.environment_name] : [],
          ),
        ),
      ].sort(),
    [apps],
  )

  // Client-side over the already-loaded list, which is fine to a few
  // hundred apps and keeps typing instant. The same filters exist
  // server-side (GET /api/v1/apps?tag=&environment=&q=) for the CLI and MCP.
  const filteredApps = useMemo(() => {
    const q = filters.query.trim().toLowerCase()
    return apps.filter(
      (app) =>
        (q === '' ||
          app.name.toLowerCase().includes(q) ||
          app.image.toLowerCase().includes(q)) &&
        (filters.environments.length === 0 ||
          filters.environments.includes(app.environment_name ?? '')) &&
        (filters.tags.length === 0 ||
          (app.tags ?? []).some((tag) => filters.tags.includes(tag))),
    )
  }, [apps, filters])

  const toggleSelected = (name: string, checked: boolean) => {
    setSelected((prev) =>
      checked ? [...new Set([...prev, name])] : prev.filter((n) => n !== name),
    )
  }
  const allVisibleSelected =
    filteredApps.length > 0 &&
    filteredApps.every((app) => selected.includes(app.name))

  const virtualizer = useVirtualizer({
    count: filteredApps.length,
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
              {filteredApps.length}
              {filtersActive(filters) ? ` of ${apps.length}` : ''}{' '}
              {apps.length === 1 ? 'app' : 'apps'}
            </span>
          ) : null}
          {filteredApps.length > 0 ? (
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                setSelected(
                  allVisibleSelected ? [] : filteredApps.map((a) => a.name),
                )
              }}
            >
              {allVisibleSelected ? 'Deselect all' : 'Select all'}
            </Button>
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
          <CreateResourceWizard
            scope="applications"
            trigger={
              <Button size="sm">
                <PlusIcon />
                New app
              </Button>
            }
          />
        </div>
      </div>
      {apps.length > 0 ? (
        <>
          <AppsStatusStrip />
          <AppsFilterBar
            filters={filters}
            environments={environments}
            onChange={setFilters}
          />
        </>
      ) : null}
      {apps.length === 0 ? (
        <EmptyState
          icon={<PackageIcon className="size-5" />}
          title="No apps yet"
          description="Deploy your first app to get started: push to a connected git repo, or create one directly if you already have a built image."
          action={
            <CreateResourceWizard
              scope="applications"
              trigger={
                <Button size="sm">
                  <PlusIcon />
                  New app
                </Button>
              }
            />
          }
          secondaryAction={
            <div className="flex items-center gap-2">
              <CreateResourceWizard
                scope="applications"
                initialSelected="browse-templates"
                trigger={
                  <Button size="sm" variant="outline">
                    Browse templates
                  </Button>
                }
              />
              <Button
                size="sm"
                variant="outline"
                render={<Link to="/settings/github-app" />}
                nativeButton={false}
              >
                <GitBranchIcon />
                Connect Git
              </Button>
            </div>
          }
        />
      ) : filteredApps.length === 0 ? (
        <EmptyState
          icon={<PackageIcon className="size-5" />}
          title="No apps match these filters"
          description="Clear the filters to see every app again."
          action={
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                setFilters(EMPTY_FILTERS)
              }}
            >
              Clear filter
            </Button>
          }
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
              const app = filteredApps[virtualRow.index]
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
                  <AppRow
                    app={app}
                    selected={selected.includes(app.name)}
                    onSelect={toggleSelected}
                  />
                </div>
              )
            })}
          </div>
        </div>
      )}
      <StaticSitesCard />
      <AppsBulkBar
        selected={selected}
        onClear={() => {
          setSelected([])
        }}
      />
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
