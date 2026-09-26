import { useEffect, useMemo, useRef, useState } from 'react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import {
  ArrowUpIcon,
  RocketLaunchIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { EmptyState, StatusPill } from '@/components/kit'
import { useNowTick } from '../../hooks/useNowTick'
import { useDeploymentActions } from '../../hooks/useDeploymentActions'
import { useDeploymentsKeyboard } from '../../hooks/useDeploymentsKeyboard'
import { useDeploymentsStream } from '../../hooks/useDeploymentsStream'
import {
  activeFilterCount,
  EMPTY_FILTERS,
  facetsFrom,
  matchesFilters,
  removeFilter,
  type DeploymentFilters,
  type DeploymentsSearch,
  type FilterKey,
  type UrlSearch,
  toUrlSearch,
} from '../../lib/deploymentFilters'
import { flattenPages } from '../../lib/deploymentsCache'
import {
  nextFocusId,
  type DeploymentKeyAction,
} from '../../lib/deploymentsKeyboard'
import {
  deploymentKeys,
  deploymentsInfiniteOptions,
  deploymentsLaneOptions,
  deploymentsSummaryOptions,
} from '../../queries/deployments'
import { BuildingNowLane } from './BuildingNowLane'
import { ConfirmActionDialog } from './ConfirmActionDialog'
import { DeploymentDrawer } from './DeploymentDrawer'
import { DeploymentList, DeploymentsSkeleton, LoadMore } from './DeploymentList'
import { DeploymentsFilterBar } from './DeploymentsFilterBar'
import { DeploymentsSummary } from './DeploymentsSummary'
import { ShortcutsHelp } from './ShortcutsHelp'

const AT_TOP_PX = 32
const MIN_AUTHOR_ROWS = 20

export interface DeploymentsPageProps {
  search: DeploymentsSearch
  onSearchChange: (next: UrlSearch) => void
}

function filtersOf(s: DeploymentsSearch): DeploymentFilters {
  return {
    status: s.status,
    trigger: s.trigger,
    app: s.app,
    environment: s.environment,
    branch: s.branch,
    author: s.author,
    q: s.q,
  }
}

export function DeploymentsPage({
  search,
  onSearchChange,
}: DeploymentsPageProps) {
  const filters = filtersOf(search)
  const openId = search.d
  const listQuery = useInfiniteQuery(deploymentsInfiniteOptions(filters))
  const lane = useQuery(deploymentsLaneOptions())
  const summary = useQuery(deploymentsSummaryOptions())
  const actions = useDeploymentActions()
  const [focusId, setFocusId] = useState('')
  const [addOpen, setAddOpen] = useState(false)
  const scrollRef = useRef<HTMLDivElement | null>(null)
  const topRef = useRef<HTMLDivElement | null>(null)

  const all = useMemo(() => flattenPages(listQuery.data), [listQuery.data])
  const rows = useMemo(
    () =>
      all.filter((d) =>
        matchesFilters(d, { ...EMPTY_FILTERS, author: filters.author }),
      ),
    [all, filters.author],
  )
  const laneRows = lane.data ?? []
  const anyBuilding =
    laneRows.some((d) => d.status === 'building') ||
    rows.some((d) => d.status === 'building')
  const now = useNowTick(anyBuilding)

  const isAtTop = () => {
    const main = topRef.current?.closest('main')
    return (
      (main?.scrollTop ?? 0) < AT_TOP_PX &&
      (scrollRef.current?.scrollTop ?? 0) < AT_TOP_PX
    )
  }
  const stream = useDeploymentsStream(
    filters,
    deploymentKeys.list(filters),
    isAtTop,
  )

  const setFilters = (next: DeploymentFilters, d = openId) => {
    onSearchChange(toUrlSearch(next, d))
  }
  const setOpen = (id: string) => {
    onSearchChange(toUrlSearch(filters, id))
  }
  const toggle = (key: FilterKey, value: string) => {
    if (key === 'status' || key === 'trigger') {
      const has = filters[key].includes(value)
      setFilters({
        ...filters,
        [key]: has
          ? filters[key].filter((v) => v !== value)
          : [...filters[key], value],
      })
      return
    }
    setFilters({ ...filters, [key]: filters[key] === value ? '' : value })
  }

  const drawerRow = openId
    ? (rows.find((d) => d.id === openId) ??
      laneRows.find((d) => d.id === openId))
    : undefined
  const searching =
    openId !== '' &&
    !drawerRow &&
    (listQuery.hasNextPage || listQuery.isFetching)

  useEffect(() => {
    if (
      openId &&
      !drawerRow &&
      listQuery.hasNextPage &&
      !listQuery.isFetchingNextPage
    ) {
      void listQuery.fetchNextPage()
    }
  }, [openId, drawerRow, listQuery])

  useEffect(() => {
    if (
      filters.author &&
      rows.length < MIN_AUTHOR_ROWS &&
      listQuery.hasNextPage &&
      !listQuery.isFetchingNextPage &&
      !listQuery.isFetchNextPageError
    ) {
      void listQuery.fetchNextPage()
    }
  }, [filters.author, rows.length, listQuery])

  const target = drawerRow ?? rows.find((d) => d.id === focusId)

  const onKey = (a: DeploymentKeyAction, e: KeyboardEvent) => {
    switch (a.type) {
      case 'move': {
        e.preventDefault()
        const id = nextFocusId(
          rows.map((d) => d.id),
          focusId || openId,
          a.delta,
        )
        setFocusId(id)
        if (openId && id) setOpen(id)
        return
      }
      case 'open':
        if (focusId) setOpen(focusId)
        return
      case 'close':
        setOpen('')
        return
      case 'add-filter':
        e.preventDefault()
        setAddOpen(true)
        return
      default:
        if (target) actions.request(a.type, target)
    }
  }
  useDeploymentsKeyboard(openId !== '', onKey)

  const showNew = () => {
    stream.flushPending()
    const main = topRef.current?.closest('main')
    main?.scrollTo({ top: 0 })
    scrollRef.current?.scrollTo({ top: 0 })
  }

  const filtered = activeFilterCount(filters) > 0
  const streamView =
    stream.state === 'live'
      ? { tone: 'success' as const, label: 'Live' }
      : stream.state === 'reconnecting'
        ? { tone: 'warning' as const, label: 'Reconnecting' }
        : { tone: 'neutral' as const, label: 'Connecting' }

  return (
    <div ref={topRef} className="flex flex-col gap-5">
      <header className="flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-lg font-semibold text-foreground">Deployments</h1>
        <div className="flex items-center gap-2">
          <StatusPill
            size="sm"
            tone={streamView.tone}
            label={streamView.label}
            live={stream.state === 'live'}
          />
          <ShortcutsHelp />
        </div>
      </header>

      <DeploymentsSummary
        summary={summary.data}
        loading={summary.isPending}
        failed={summary.isError}
        onNeedsAttention={() => {
          setFilters({ ...filters, status: ['held'] })
        }}
      />

      <BuildingNowLane
        rows={laneRows}
        now={now}
        cancelSupported={actions.cancelSupported}
        onOpen={setOpen}
        onCancel={(d) => {
          actions.request('cancel', d)
        }}
      />

      <DeploymentsFilterBar
        filters={filters}
        facets={facetsFrom(all)}
        addOpen={addOpen}
        onAddOpenChange={setAddOpen}
        onSearch={(q) => {
          setFilters({ ...filters, q })
        }}
        onToggle={toggle}
        onRemove={(key, value) => {
          setFilters(removeFilter(filters, key, value))
        }}
        onClear={() => {
          setFilters(EMPTY_FILTERS)
        }}
      />

      <div aria-live="polite" className="min-h-0">
        {stream.pending.length > 0 && (
          <div className="sticky top-2 z-10 flex justify-center pb-2">
            <Button size="sm" onClick={showNew}>
              <ArrowUpIcon aria-hidden="true" />
              {stream.pending.length} new
            </Button>
          </div>
        )}
      </div>

      {listQuery.isPending ? (
        <DeploymentsSkeleton />
      ) : listQuery.isError && all.length === 0 ? (
        <EmptyState
          icon={<WarningCircleIcon />}
          title="Deployments could not be loaded"
          description={listQuery.error.message}
          action={
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                void listQuery.refetch()
              }}
            >
              Retry
            </Button>
          }
        />
      ) : rows.length === 0 && !listQuery.hasNextPage ? (
        filtered ? (
          <EmptyState
            icon={<RocketLaunchIcon />}
            title="No deployments match"
            description="Try removing a filter or widening your search."
            action={
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  setFilters(EMPTY_FILTERS)
                }}
              >
                Clear filters
              </Button>
            }
          />
        ) : (
          <EmptyState
            icon={<RocketLaunchIcon />}
            title="No deployments yet"
            description="Push to a connected repository or deploy an app and it will show up here."
          />
        )
      ) : (
        <div>
          <DeploymentList
            rows={rows}
            now={now}
            openId={openId}
            focusId={focusId}
            onOpen={(id) => {
              setFocusId(id)
              setOpen(id)
            }}
            scrollRef={scrollRef}
          />
          <LoadMore
            hasNext={Boolean(listQuery.hasNextPage)}
            fetching={listQuery.isFetchingNextPage}
            failed={listQuery.isFetchNextPageError}
            onLoad={() => {
              void listQuery.fetchNextPage()
            }}
          />
        </div>
      )}

      <DeploymentDrawer
        id={openId}
        deployment={drawerRow}
        searching={searching}
        now={now}
        cancelSupported={actions.cancelSupported}
        onClose={() => {
          setOpen('')
        }}
        onAction={actions.request}
      />
      <ConfirmActionDialog
        action={actions.pending}
        busy={actions.busy}
        onConfirm={actions.confirm}
        onDismiss={actions.dismiss}
      />
    </div>
  )
}
