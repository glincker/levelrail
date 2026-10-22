import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useEnvironmentScope } from '../../lib/environmentScope'
import {
  keepPreviousData,
  useInfiniteQuery,
  useQuery,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  DatabaseIcon,
  FunnelIcon,
  PackageIcon,
  PlusIcon,
} from '@phosphor-icons/react/dist/ssr'
import { EmptyState } from '@/components/ui/empty-state'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { PageBanner } from '../../components/PageBanner'
import { LazyAppsDiagram } from '../../components/banners/LazyDiagrams'
import {
  appPagesQueryOptions,
  appsSummaryQueryOptions,
} from '../../queries/appPages'
import { LoadMore } from '@/components/ui/load-more'
import { useDebouncedValue } from '../../hooks/useDebouncedValue'
import { staticSitesQueryOptions } from '../../queries/staticSites'
import { RowSkeleton } from '../../components/AppRow'
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
  loaderDeps: ({ search }) => ({ status: search.status ?? null }),
  loader: ({ context: { queryClient }, deps }) =>
    Promise.all([
      queryClient.ensureInfiniteQueryData(
        appPagesQueryOptions({ status: deps.status }),
      ),
      queryClient.ensureQueryData(appsSummaryQueryOptions()),
      queryClient.ensureQueryData(staticSitesQueryOptions()),
    ]),
  component: AppListPage,
  pendingComponent: AppListPending,
  errorComponent: AppListError,
})

function AppListPage() {
  useEnvironmentScope()
  const { t } = useTranslation('common')
  const { data: staticSites } = useSuspenseQuery(staticSitesQueryOptions())
  const status = Route.useSearch().status ?? null
  const navigate = useNavigate({ from: Route.fullPath })
  const [filters, setFilters] = useState<AppListFilters>(EMPTY_FILTERS)
  const debouncedQuery = useDebouncedValue(filters.query, 250)
  const pageParams = useMemo(
    () => ({
      status,
      q: debouncedQuery,
      environments: filters.environments,
      tags: filters.tags,
    }),
    [status, debouncedQuery, filters.environments, filters.tags],
  )
  const pages = useInfiniteQuery({
    ...appPagesQueryOptions(pageParams),
    refetchInterval: 15_000,
    placeholderData: keepPreviousData,
  })
  const { data: summary } = useQuery({
    ...appsSummaryQueryOptions(),
    refetchInterval: 15_000,
  })
  const apps = useMemo(
    () => pages.data?.pages.flatMap((p) => p.items) ?? [],
    [pages.data],
  )
  const filteredTotal = pages.data?.pages.at(-1)?.total ?? 0
  const totalApps = summary?.total ?? filteredTotal
  const [selected, setSelected] = useState<string[]>([])
  const [mode, setMode] = useState<ViewMode>(loadViewMode)

  const environments = useMemo(
    () => Object.keys(summary?.by_environment ?? {}).sort(),
    [summary],
  )
  const counts = useMemo(
    () => ({
      running: summary?.running ?? 0,
      deploying: summary?.deploying ?? 0,
      failing: summary?.failing ?? 0,
      stopped: summary?.stopped ?? 0,
    }),
    [summary],
  )
  const filteredApps = apps

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
    totalApps === 0
      ? undefined
      : t('appsList.count', {
          defaultValue: '{{count}} apps',
          count: filteredTotal,
          total: totalApps,
          context: narrowed ? 'filtered' : undefined,
        })

  const noApps = totalApps === 0 && staticSites.length === 0
  const bannerSize = noApps ? 'hero' : 'compact'
  const headerActions = (
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
      {totalApps > 0 ? (
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
      <Button size="sm" render={<Link to="/apps/new" />} nativeButton={false}>
        <PlusIcon />
        {t('appsList.newApp', { defaultValue: 'New app' })}
      </Button>
    </>
  )

  return (
    <PageTemplate
      title={t('appsList.title', { defaultValue: 'Apps' })}
      description={countLabel}
      actions={headerActions}
      banner={
        <PageBanner
          icon={<PackageIcon />}
          titleKey="common:appsList.title"
          descriptionKey={
            noApps
              ? 'common:appsList.banner.description'
              : 'common:appsList.banner.compactDescription'
          }
          action={headerActions}
          illustration={<LazyAppsDiagram size={bannerSize} />}
          size={bannerSize}
        />
      }
    >
      {totalApps > 0 ? (
        <div className="space-y-3">
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
      {noApps ? null : pages.isPending ? (
        <AppListRowsPending />
      ) : filteredApps.length === 0 ? (
        <EmptyState
          illustration="chart"
          icon={<FunnelIcon className="size-6" />}
          title={t('appsList.noMatchTitle', { defaultValue: 'No apps match' })}
          description={t('appsList.noMatchBody', {
            defaultValue:
              'No app fits the current status and search. Clear the filters to see all {{total}}.',
            total: totalApps,
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
        <>
          <AppsListBody
            apps={filteredApps}
            mode={mode}
            selected={selected}
            onSelect={toggleSelected}
          />
          <LoadMore
            loaded={filteredApps.length}
            total={filteredTotal}
            loading={pages.isFetchingNextPage}
            onLoadMore={() => {
              void pages.fetchNextPage()
            }}
          />
        </>
      )}
      <StaticSitesCard />
      <AppsBulkBar
        selected={selected}
        onClear={() => {
          setSelected([])
        }}
      />
    </PageTemplate>
  )
}

function AppListRowsPending() {
  return (
    <div className="overflow-hidden rounded-lg border border-border bg-card">
      <ListHeader />
      {Array.from({ length: 6 }, (_, i) => (
        <RowSkeleton key={i} />
      ))}
    </div>
  )
}

function AppListPending() {
  return (
    <PageTemplate title="Apps">
      <AppListRowsPending />
    </PageTemplate>
  )
}

function AppListError({ error }: { error: unknown }) {
  const { t } = useTranslation('common')
  return (
    <PageTemplate title={t('appsList.title', { defaultValue: 'Apps' })}>
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
    </PageTemplate>
  )
}
