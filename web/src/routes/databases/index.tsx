import { lazy, Suspense, useMemo, useState, type KeyboardEvent } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Checkbox } from '@/components/ui/checkbox'
import { FilterChips, type ActiveFilter } from '../../components/FilterChips'
import { ListKeyboardHint } from '../../components/ListKeyboardHint'
import { useListKeyboard } from '../../hooks/useListKeyboard'
import { useEnvironmentScope } from '../../lib/environmentScope'
import { routeErrorMessage } from '../../lib/apiError'
import { databaseListQueryOptions } from '../../queries/databases'
import { LabelFilterInput } from '../../components/LabelFilterInput'
import { useLabelMatches } from '../../queries/resourceLabels'
import {
  DatabasesError,
  DatabasesPage,
  DatabasesPending,
  DatabasesShell,
} from '../../components/DatabasesPage'
import { DatabasesTable } from '../../components/DatabasesTable'
import {
  DatabasesFilterBar,
  DatabasesStatusStrip,
} from '../../components/DatabasesFilterBar'
import { DatabasesBulkBar } from '../../components/DatabasesBulkBar'
const SavedViewsMenu = lazy(() => import('../../components/SavedViewsMenu'))
import {
  EMPTY_DB_FILTERS,
  countByStatus,
  databaseViewFilter,
  engineOptions,
  filterDatabases,
  filtersFromSearch,
  tagOptions,
  parseDatabasesSearch,
  searchFromFilters,
  type DatabaseListFilters,
} from '../../lib/databasesFilter'

// Loader primes the cache; the component reads it via useSuspenseQuery.
// Filters live in the URL so a filtered list can be linked and survives reloads.
export const Route = createFileRoute('/databases/')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(databaseListQueryOptions()),
  validateSearch: parseDatabasesSearch,
  component: DatabaseListPage,
  pendingComponent: DatabasesPending,
  errorComponent: DatabaseListError,
})

function isTyping(target: EventTarget): boolean {
  return (
    target instanceof HTMLElement &&
    (target.isContentEditable ||
      ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName))
  )
}

function DatabaseListPage() {
  useEnvironmentScope()
  const { t } = useTranslation('databases')
  const { data: databases } = useSuspenseQuery(databaseListQueryOptions())
  const search = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const filters = useMemo(() => filtersFromSearch(search), [search])
  const [labelSelector, setLabelSelector] = useState('')
  const [selected, setSelected] = useState<string[]>([])
  const labelMatches = useLabelMatches('databases', labelSelector)
  const { t: tf } = useTranslation('uiShared')
  const {
    setContainer: setListContainer,
    hintOpen,
    closeHint,
  } = useListKeyboard<HTMLDivElement>()

  const labelled = useMemo(
    () =>
      labelMatches.data
        ? databases.filter((db) => labelMatches.data.has(db.name))
        : databases,
    [databases, labelMatches.data],
  )
  const engines = useMemo(() => engineOptions(databases), [databases])
  const tags = useMemo(() => tagOptions(databases), [databases])
  const counts = useMemo(() => countByStatus(labelled), [labelled])
  const shown = useMemo(
    () => filterDatabases(labelled, filters),
    [labelled, filters],
  )
  const selectedRows = useMemo(
    () => shown.filter((db) => selected.includes(db.name)),
    [shown, selected],
  )

  if (databases.length === 0) {
    return <DatabasesPage databases={databases} />
  }

  const setFilters = (next: DatabaseListFilters) => {
    void navigate({
      search: (prev) => ({
        ...(prev.create ? { create: prev.create } : {}),
        ...searchFromFilters(next),
      }),
      replace: true,
    })
  }
  const clearAll = () => {
    setFilters(EMPTY_DB_FILTERS)
    setLabelSelector('')
  }
  const activeFilters: ActiveFilter[] = [
    ...(filters.status
      ? [
          {
            id: 'status',
            label: tf('filterChips.status', { value: filters.status }),
            onRemove: () => {
              setFilters({ ...filters, status: null })
            },
          },
        ]
      : []),
    ...(filters.q
      ? [
          {
            id: 'q',
            label: tf('filterChips.search', { value: filters.q }),
            onRemove: () => {
              setFilters({ ...filters, q: '' })
            },
          },
        ]
      : []),
    ...filters.engines.map((engine) => ({
      id: `engine:${engine}`,
      label: tf('filterChips.engine', { value: engine }),
      onRemove: () => {
        setFilters({
          ...filters,
          engines: filters.engines.filter((x) => x !== engine),
        })
      },
    })),
    ...filters.tags.map((tag) => ({
      id: `tag:${tag}`,
      label: tf('filterChips.tag', { value: tag }),
      onRemove: () => {
        setFilters({
          ...filters,
          tags: filters.tags.filter((x) => x !== tag),
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
  const allSelected = shown.length > 0 && selectedRows.length === shown.length
  const toggleAll = (on: boolean) => {
    setSelected(on ? shown.map((db) => db.name) : [])
  }
  // Mod+A inside the list selects every shown row, like a file manager.
  const onListKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'a' && !isTyping(e.target)) {
      e.preventDefault()
      toggleAll(!allSelected)
    }
  }

  return (
    <DatabasesShell size="compact" empty={false}>
      <div className="flex flex-col gap-3">
        <DatabasesStatusStrip
          counts={counts}
          active={filters.status}
          onChange={(status) => {
            setFilters({ ...filters, status })
          }}
        />
        <div className="flex flex-wrap items-center gap-2">
          <DatabasesFilterBar
            filters={filters}
            engines={engines}
            tags={tags}
            onChange={setFilters}
          />
          <LabelFilterInput
            value={labelSelector}
            onChange={setLabelSelector}
            invalid={labelMatches.isError}
          />
          <Suspense fallback={null}>
            <SavedViewsMenu
              list="databases"
              current={databaseViewFilter(filters, labelSelector)}
              onApply={(f) => {
                setFilters(filtersFromSearch(parseDatabasesSearch(f)))
                setLabelSelector(f.labels ?? '')
              }}
            />
          </Suspense>
        </div>
        <FilterChips
          filters={activeFilters}
          shown={shown.length}
          total={databases.length}
          onClearAll={clearAll}
        />
        <ListKeyboardHint open={hintOpen} onClose={closeHint} />
        {shown.length === 0 ? null : (
          <div
            ref={setListContainer}
            onKeyDown={onListKeyDown}
            className="flex flex-col gap-2"
          >
            <label className="flex w-fit items-center gap-2 text-sm text-muted-foreground">
              <Checkbox
                checked={allSelected}
                indeterminate={selectedRows.length > 0 && !allSelected}
                onCheckedChange={toggleAll}
              />
              {t('bulk.selectAll', { count: shown.length })}
            </label>
            <DatabasesTable
              databases={shown}
              selected={selected}
              onSelect={(name, checked) => {
                setSelected((cur) =>
                  checked ? [...cur, name] : cur.filter((n) => n !== name),
                )
              }}
            />
          </div>
        )}
      </div>
      <DatabasesBulkBar
        selected={selectedRows}
        onClear={() => {
          setSelected([])
        }}
      />
    </DatabasesShell>
  )
}

function DatabaseListError({ error }: { error: unknown }) {
  return <DatabasesError message={routeErrorMessage(error)} />
}
