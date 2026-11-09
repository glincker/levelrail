import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { topicPollMs } from '../../lib/pollTier'
import { useEnvironmentScope } from '../../lib/environmentScope'
import {
  keepPreviousData,
  useInfiniteQuery,
  useQuery,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { lazy, Suspense, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  DatabaseGlassIcon,
  LayersGlassIcon,
  PlusGlassIcon,
} from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { PageBanner } from '../../components/PageBanner'
import { LazyAppsDiagram } from '../../components/banners/LazyDiagrams'
import {
  appPagesQueryOptions,
  appsSummaryQueryOptions,
} from '../../queries/appPages'
import { LoadMore } from '@/components/ui/load-more'
import { staticSitesQueryOptions } from '../../queries/staticSites'
import { RowSkeleton } from '../../components/AppRow'
import { StaticSitesCard } from '../../components/StaticSitesCard'
import { FilterChips, type ActiveFilter } from '../../components/FilterChips'
import { ListKeyboardHint } from '../../components/ListKeyboardHint'
import { useListKeyboard } from '../../hooks/useListKeyboard'
import { useCanDeploy } from '../../hooks/useCanDeploy'

const BulkActionsBar = lazy(() =>
  import('../../components/apps/BulkActionsBar').then((m) => ({
    default: m.BulkActionsBar,
  })),
)
import { AppsFilterBar } from '../../components/AppsFilterBar'
const SavedViewsMenu = lazy(() => import('../../components/SavedViewsMenu'))
import {
  AppsStatusChips,
  ViewToggle,
} from '../../components/apps/AppsStatusChips'
import { AppsFirstRun } from '../../components/apps/AppsFirstRun'
import { AppsEmptyActions } from '../../components/apps/AppsEmptyActions'
import { AppsListBody, ListHeader } from '../../components/apps/AppsListBody'
import { filtersActive, type AppListFilters } from '../../lib/appViews'
import {
  appFiltersFromSearch,
  appsSearchFrom,
  loadViewMode,
  parseAppsSearch,
  storeViewMode,
  type AppsSearch,
  type StatusFilter,
  type ViewMode,
} from '../../lib/appsListView'
import { Button } from '../../components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { routeErrorMessage } from '../../lib/apiError'
import { LabelFilterField } from '../../components/LabelFilterInput'
import { useLabelMatches } from '../../queries/resourceLabels'

// The loader primes the Query cache; the component only reads it. The
// list is virtualized unconditionally (project rule for lists that can
// exceed 50 items). Every filter sits in the URL so the command palette,
// shared links and saved views deep-link into a filtered list.
export const Route = createFileRoute('/apps/')({
  validateSearch: (search: Record<string, unknown>): AppsSearch =>
    parseAppsSearch(search),
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
  const search = Route.useSearch()
  const status = search.status ?? null
  const labelSelector = search.labels ?? ''
  const navigate = useNavigate({ from: Route.fullPath })
  const filters = useMemo(() => appFiltersFromSearch(search), [search])
  const writeSearch = (next: AppsSearch) => {
    void navigate({ search: next, replace: true })
  }
  const setFilters = (next: AppListFilters) => {
    writeSearch(appsSearchFrom(next, status, labelSelector))
  }
  const pageParams = useMemo(
    () => ({
      status,
      q: filters.query,
      environments: filters.environments,
      tags: filters.tags,
    }),
    [status, filters.query, filters.environments, filters.tags],
  )
  const pages = useInfiniteQuery({
    ...appPagesQueryOptions(pageParams),
    refetchInterval: () => topicPollMs('active'),
    placeholderData: keepPreviousData,
  })
  const { data: summary } = useQuery({
    ...appsSummaryQueryOptions(),
    refetchInterval: () => topicPollMs('active'),
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
  const setLabelSelector = (next: string) => {
    writeSearch(appsSearchFrom(filters, status, next))
  }
  const labelMatches = useLabelMatches('apps', labelSelector)
  const filteredApps = useMemo(
    () =>
      labelMatches.data
        ? apps.filter((app) => labelMatches.data.has(app.name))
        : apps,
    [apps, labelMatches.data],
  )

  const setStatus = (next: StatusFilter) => {
    writeSearch(appsSearchFrom(filters, next, labelSelector))
  }
  const currentView = {
    q: search.q ?? '',
    status: search.status ?? '',
    env: search.env ?? '',
    tags: search.tags ?? '',
    labels: labelSelector,
  }
  const applyView = (f: Record<string, string>) => {
    writeSearch(parseAppsSearch(f))
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
  const { t: tf } = useTranslation('uiShared')
  const {
    setContainer: setListContainer,
    hintOpen,
    closeHint,
  } = useListKeyboard<HTMLDivElement>()
  const canDeploy = useCanDeploy()
  const clearAll = () => {
    writeSearch({})
  }
  const activeFilters: ActiveFilter[] = [
    ...(status
      ? [
          {
            id: 'status',
            label: tf('filterChips.status', { value: status }),
            onRemove: () => {
              setStatus(null)
            },
          },
        ]
      : []),
    ...(filters.query
      ? [
          {
            id: 'q',
            label: tf('filterChips.search', { value: filters.query }),
            onRemove: () => {
              setFilters({ ...filters, query: '' })
            },
          },
        ]
      : []),
    ...filters.tags.map((tag) => ({
      id: `tag:${tag}`,
      label: tf('filterChips.tag', { value: tag }),
      onRemove: () => {
        setFilters({ ...filters, tags: filters.tags.filter((x) => x !== tag) })
      },
    })),
    ...filters.environments.map((env) => ({
      id: `env:${env}`,
      label: tf('filterChips.environment', { value: env }),
      onRemove: () => {
        setFilters({
          ...filters,
          environments: filters.environments.filter((x) => x !== env),
        })
      },
    })),
    ...(labelSelector
      ? [
          {
            id: 'labels',
            label: tf('filterChips.labels', { value: labelSelector }),
            onRemove: () => {
              setLabelSelector('')
            },
          },
        ]
      : []),
  ]

  // Hide the count until the first page lands so it never reads "0 apps" mid-load.
  const countLabel =
    totalApps === 0 || pages.isPending
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
        <span>
          <ViewToggle mode={mode} onChange={changeMode} />
        </span>
      ) : null}
      {noApps ? <AppsEmptyActions /> : null}
      <Button
        size="sm"
        variant="outline"
        render={<Link to="/databases" />}
        nativeButton={false}
      >
        <DatabaseGlassIcon size="1.4em" />
        {t('appsList.newDatabase', { defaultValue: 'New database' })}
      </Button>
      <Button size="sm" render={<Link to="/apps/new" />} nativeButton={false}>
        <PlusGlassIcon size="1.4em" />
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
          icon={<LayersGlassIcon size="1.4em" />}
          titleKey="common:appsList.title"
          descriptionKey={
            noApps
              ? 'common:appsList.banner.description'
              : 'common:appsList.banner.compactDescription'
          }
          descriptionText={noApps ? undefined : countLabel}
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
          <div className="flex flex-wrap items-center gap-2">
            <div className="min-w-0 flex-1 basis-56">
              <AppsFilterBar
                filters={filters}
                environments={environments}
                onChange={setFilters}
              />
            </div>
            <LabelFilterField
              value={labelSelector}
              onChange={setLabelSelector}
              invalid={labelMatches.isError}
            />
            <Suspense fallback={null}>
              <SavedViewsMenu
                list="apps"
                current={currentView}
                onApply={applyView}
              />
            </Suspense>
          </div>
          <FilterChips
            filters={activeFilters}
            shown={filteredApps.length}
            total={totalApps}
            onClearAll={clearAll}
          />
          <ListKeyboardHint open={hintOpen} onClose={closeHint} />
        </div>
      ) : null}
      {noApps ? (
        <AppsFirstRun />
      ) : pages.isPending ? (
        <AppListRowsPending />
      ) : filteredApps.length === 0 ? null : (
        <div ref={setListContainer} className="contents">
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
        </div>
      )}
      <StaticSitesCard />
      {selected.length > 0 ? (
        <Suspense fallback={null}>
          <BulkActionsBar
            selected={selected}
            canAct={canDeploy}
            onClear={() => {
              setSelected([])
            }}
          />
        </Suspense>
      ) : null}
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
