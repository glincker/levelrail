import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useEnvironmentScope } from '../../lib/environmentScope'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  DatabaseIcon,
  DownloadSimpleIcon,
  GitBranchIcon,
  FunnelIcon,
  PackageIcon,
  PlusIcon,
} from '@phosphor-icons/react/dist/ssr'
import { EmptyState } from '@/components/ui/empty-state'
import { PageHeader } from '@/components/shell/PageHeader'
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
import { Alert, AlertDescription } from '@/components/ui/alert'
import { routeErrorMessage } from '../../lib/apiError'

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
  errorComponent: AppListError,
})

function AppListPage() {
  useEnvironmentScope()
  const { t } = useTranslation('common')
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

  const countLabel =
    apps.length === 0
      ? undefined
      : t('appsList.count', {
          defaultValue: '{{count}} apps',
          count: filteredApps.length,
          total: apps.length,
          context: narrowed ? 'filtered' : undefined,
        })

  return (
    <div>
      <div className="mb-4">
        <PageHeader
          title={t('appsList.title', { defaultValue: 'Apps' })}
          description={countLabel}
          actions={
            <>
              {selected.length > 0 ? (
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    setSelected(
                      allVisibleSelected ? [] : filteredApps.map((a) => a.name),
                    )
                  }}
                >
                  {allVisibleSelected
                    ? t('appsList.deselectAll', {
                        defaultValue: 'Deselect all',
                      })
                    : t('appsList.selectAll', { defaultValue: 'Select all' })}
                </Button>
              ) : null}
              {apps.length > 0 ? (
                <span className="hidden sm:block">
                  <ViewToggle mode={mode} onChange={changeMode} />
                </span>
              ) : null}
              <Button
                size="sm"
                variant="outline"
                className="max-sm:hidden"
                render={<Link to="/databases" />}
                nativeButton={false}
              >
                <DatabaseIcon />
                {t('appsList.newDatabase', { defaultValue: 'New database' })}
              </Button>
              <CreateResourceWizard
                scope="applications"
                trigger={
                  <Button size="sm">
                    <PlusIcon />
                    {t('appsList.newApp', { defaultValue: 'New app' })}
                  </Button>
                }
              />
            </>
          }
        />
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
          title={t('appsList.emptyTitle', { defaultValue: 'No apps yet' })}
          description={t('appsList.emptyBody', {
            defaultValue:
              'Deploy from a git repository or a container image, or import apps from another platform.',
          })}
          action={
            <div className="flex flex-wrap items-center justify-center gap-2">
              <CreateResourceWizard
                scope="applications"
                trigger={
                  <Button>
                    <PlusIcon />
                    {t('appsList.newApp', { defaultValue: 'New app' })}
                  </Button>
                }
              />
              <Button
                variant="outline"
                render={<Link to="/settings/import-platform" />}
                nativeButton={false}
              >
                <DownloadSimpleIcon />
                {t('appsList.import', { defaultValue: 'Import' })}
              </Button>
              <Button
                variant="outline"
                render={<Link to="/settings/github-app" />}
                nativeButton={false}
              >
                <GitBranchIcon />
                {t('appsList.connectGit', { defaultValue: 'Connect Git' })}
              </Button>
            </div>
          }
        />
      ) : filteredApps.length === 0 ? (
        <EmptyState
          illustration="chart"
          icon={<FunnelIcon className="size-6" />}
          title={t('appsList.noMatchTitle', { defaultValue: 'No apps match' })}
          description={t('appsList.noMatchBody', {
            defaultValue:
              'No app fits the current status and search. Clear the filters to see all {{total}}.',
            total: apps.length,
          })}
          action={
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                setFilters(EMPTY_FILTERS)
                setStatus(null)
              }}
            >
              {t('appsList.clearFilters', { defaultValue: 'Clear filters' })}
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
      <div className="mb-4">
        <PageHeader title="Apps" />
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

function AppListError({ error }: { error: unknown }) {
  const { t } = useTranslation('common')
  return (
    <div className="space-y-4">
      <PageHeader title={t('appsList.title', { defaultValue: 'Apps' })} />
      <Alert variant="destructive">
        <AlertDescription className="space-y-2">
          <p>
            {t('appsList.errorBody', {
              defaultValue: 'The app list could not be loaded. {{message}}',
              message: routeErrorMessage(error),
            })}
          </p>
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              globalThis.location.reload()
            }}
          >
            {t('appsList.retry', { defaultValue: 'Retry' })}
          </Button>
        </AlertDescription>
      </Alert>
    </div>
  )
}
