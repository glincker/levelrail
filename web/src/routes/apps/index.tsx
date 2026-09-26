import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import {
  DatabaseIcon,
  DownloadSimpleIcon,
  GitBranchIcon,
  FunnelIcon,
  PackageIcon,
  PlusIcon,
} from '@phosphor-icons/react/dist/ssr'
import { EmptyState } from '@/components/kit'
import { appListQueryOptions } from '../../queries/apps'
import { staticSitesQueryOptions } from '../../queries/staticSites'
import { RowSkeleton } from '../../components/AppRow'
import { CreateResourceWizard } from '../../components/CreateResourceWizard'
import { StaticSitesCard } from '../../components/StaticSitesCard'
import { AppsBulkBar } from '../../components/AppsBulkBar'
import { AppsFilterBar } from '../../components/AppsFilterBar'
import {
  AppsStatusChips,
  ViewToggle,
} from '../../components/apps/AppsStatusChips'
import { AppsListBody, ListHeader } from '../../components/apps/AppsListBody'
import {
  EMPTY_FILTERS,
  filtersActive,
  type AppListFilters,
} from '../../lib/appViews'
import {
  countBuckets,
  filterApps,
  loadViewMode,
  parseStatusSearch,
  storeViewMode,
  type StatusFilter,
  type ViewMode,
} from '../../lib/appsListView'
import { Button } from '../../components/ui/button'

// The loader primes the Query cache; the component only reads it. The
// list is virtualized unconditionally (project rule for lists that can
// exceed 50 items). `status` in the URL lets the command palette and
// dashboard deep-link into a filtered list.
export const Route = createFileRoute('/apps/')({
  validateSearch: (
    search: Record<string, unknown>,
  ): { status?: StatusFilter } => {
    const status = parseStatusSearch(search.status)
    return status ? { status } : {}
  },
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(appListQueryOptions()),
      queryClient.ensureQueryData(staticSitesQueryOptions()),
    ]),
  component: AppListPage,
  pendingComponent: AppListPending,
})

function AppListPage() {
  const { data: apps } = useSuspenseQuery({
    ...appListQueryOptions(),
    refetchInterval: 15_000,
  })
  const status = Route.useSearch().status ?? null
  const navigate = useNavigate({ from: Route.fullPath })
  const [filters, setFilters] = useState<AppListFilters>(EMPTY_FILTERS)
  const [selected, setSelected] = useState<string[]>([])
  const [mode, setMode] = useState<ViewMode>(loadViewMode)

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
  const counts = useMemo(() => countBuckets(apps), [apps])
  const filteredApps = useMemo(
    () => filterApps(apps, filters, status),
    [apps, filters, status],
  )

  const setStatus = (next: StatusFilter) => {
    void navigate({ search: next ? { status: next } : {}, replace: true })
  }
  const changeMode = (next: ViewMode) => {
    storeViewMode(next)
    setMode(next)
  }
  const toggleSelected = (name: string, checked: boolean) => {
    setSelected((prev) =>
      checked ? [...new Set([...prev, name])] : prev.filter((n) => n !== name),
    )
  }
  const allVisibleSelected =
    filteredApps.length > 0 &&
    filteredApps.every((app) => selected.includes(app.name))
  const narrowed = filtersActive(filters) || status !== null

  return (
    <div>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-lg font-semibold text-foreground">Apps</h1>
        <div className="flex flex-wrap items-center gap-3">
          {apps.length > 0 ? (
            <span className="text-sm text-muted-foreground tabular-nums">
              {filteredApps.length}
              {narrowed ? ` of ${apps.length}` : ''}{' '}
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
          {apps.length > 0 ? (
            <ViewToggle mode={mode} onChange={changeMode} />
          ) : null}
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
        <div className="mb-3 space-y-3">
          <AppsStatusChips
            counts={counts}
            active={status}
            onChange={setStatus}
          />
          <AppsFilterBar
            filters={filters}
            environments={environments}
            onChange={setFilters}
          />
        </div>
      ) : null}
      {apps.length === 0 ? (
        <EmptyState
          illustration="rocket"
          icon={<PackageIcon className="size-6" />}
          title="No apps yet"
          description="Deploy your first app in under a minute."
          action={
            <div className="flex flex-wrap items-center justify-center gap-2">
              <CreateResourceWizard
                scope="applications"
                trigger={
                  <Button>
                    <PlusIcon />
                    New app
                  </Button>
                }
              />
              <Button
                variant="outline"
                render={<Link to="/settings/import-platform" />}
                nativeButton={false}
              >
                <DownloadSimpleIcon />
                Import
              </Button>
              <Button
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
          illustration="chart"
          icon={<FunnelIcon className="size-6" />}
          title="No apps match"
          description="Try a different status or search."
          action={
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                setFilters(EMPTY_FILTERS)
                setStatus(null)
              }}
            >
              Clear filters
            </Button>
          }
        />
      ) : (
        <AppsListBody
          apps={filteredApps}
          mode={mode}
          selected={selected}
          onSelect={toggleSelected}
        />
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

function AppListPending() {
  return (
    <div>
      <div className="mb-4 flex items-baseline justify-between">
        <h1 className="text-lg font-semibold text-foreground">Apps</h1>
      </div>
      <div className="overflow-hidden rounded-2xl border border-border bg-card">
        <ListHeader />
        {Array.from({ length: 6 }, (_, i) => (
          <RowSkeleton key={i} />
        ))}
      </div>
    </div>
  )
}
