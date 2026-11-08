import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SearchGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { EmptyState } from '@/components/ui/empty-state'
import { useVirtualizer } from '@tanstack/react-virtual'
import { SplitGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { SegmentedControl } from '@/components/ui/segmented-control'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { HelpLink } from './HelpLink'
import { PageBanner } from './PageBanner'
import { LazyLoadBalancersDiagram } from './banners/LazyDiagrams'
import { ConfigureLoadBalancerDialog } from './ConfigureLoadBalancerDialog'
import {
  LoadBalancerOverviewHeader,
  LoadBalancerOverviewRow,
} from './LoadBalancerOverviewRow'
import { useDebouncedValue } from '../hooks/useDebouncedValue'
import {
  OVERVIEW_PAGE_LIMIT,
  useLoadBalancerList,
  type LoadBalancerOverviewState,
} from '../queries/loadBalancers'

type StateFilter = LoadBalancerOverviewState | 'all'

const STATE_FILTERS: readonly StateFilter[] = [
  'all',
  'balancing',
  'degraded',
  'none',
]

const FILTER_KEY = {
  all: 'lb.overview.all',
  balancing: 'lb.overview.stateBalancing',
  degraded: 'lb.overview.stateDegraded',
  none: 'lb.overview.stateNone',
} as const satisfies Record<StateFilter, string>

function LoadBalancerTable({
  items,
}: {
  items: NonNullable<ReturnType<typeof useLoadBalancerList>['data']>['items']
}) {
  const { t } = useTranslation('statePlaceholders')
  const parentRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 56,
    overscan: 8,
  })
  return (
    <div
      ref={parentRef}
      className="h-[60vh] overflow-auto rounded-lg border border-border bg-card"
    >
      <LoadBalancerOverviewHeader />
      <div
        role="list"
        aria-label={t('lb.overview.listLabel')}
        style={{ height: virtualizer.getTotalSize(), position: 'relative' }}
      >
        {virtualizer.getVirtualItems().map((row) => {
          const item = items[row.index]
          if (!item) return null
          return (
            <div
              key={item.app}
              role="listitem"
              style={{
                position: 'absolute',
                top: 0,
                left: 0,
                width: '100%',
                height: row.size,
                transform: `translateY(${row.start}px)`,
              }}
            >
              <LoadBalancerOverviewRow item={item} />
            </div>
          )
        })}
      </div>
    </div>
  )
}

export function LoadBalancerOverview() {
  const { t } = useTranslation('statePlaceholders')
  const [search, setSearch] = useState('')
  const [filter, setFilter] = useState<StateFilter>('all')
  const state = filter === 'all' ? '' : filter
  const [pickerOpen, setPickerOpen] = useState(false)
  const debouncedSearch = useDebouncedValue(search.trim(), 250)
  const query = useLoadBalancerList({ search: debouncedSearch, state })

  const filtered = debouncedSearch !== '' || state !== ''
  const items = query.data?.items ?? []
  const total = query.data?.total ?? 0

  let content
  if (query.isPending) {
    content = <TableSkeleton columnCount={5} rowCount={4} />
  } else if (query.isError) {
    content = (
      <EmptyState
        kind="error"
        icon={<TriangleAlertGlassIcon className="size-5" aria-hidden="true" />}
        title={t('loadBalancers.loadError')}
        description={query.error.message}
      />
    )
  } else if (items.length === 0 && !filtered) {
    content = null
  } else if (items.length === 0) {
    content = (
      <EmptyState
        kind="no-results"
        icon={<SearchGlassIcon className="size-5" aria-hidden="true" />}
        title={t('lb.overview.noMatch')}
        description={t('lb.overview.noMatchHint')}
      />
    )
  } else {
    content = <LoadBalancerTable items={items} />
  }

  const isEmpty = query.isSuccess && items.length === 0 && !filtered
  const size = isEmpty ? 'hero' : 'compact'
  const showControls = !query.isPending && (total > 0 || filtered)

  return (
    <div className="mx-auto flex w-full min-w-0 max-w-(--glinr-content-max) flex-col gap-(--glinr-page-gap)">
      <PageBanner
        icon={<SplitGlassIcon size="1.4em" />}
        titleKey="common:banners.loadBalancers.title"
        descriptionKey={
          isEmpty
            ? 'common:banners.loadBalancers.description'
            : 'common:banners.loadBalancers.compactDescription'
        }
        action={
          <>
            <HelpLink path="/load-balancing" label={t('lb.page.guide')} />
            {query.isPending ? null : (
              <Button
                onClick={() => {
                  setPickerOpen(true)
                }}
              >
                {t('lb.overview.configure')}
              </Button>
            )}
          </>
        }
        illustration={<LazyLoadBalancersDiagram size={size} />}
        size={size}
      />

      {showControls ? (
        <div className="flex flex-wrap items-center gap-2">
          <Input
            type="search"
            value={search}
            onChange={(e) => {
              setSearch(e.target.value)
            }}
            placeholder={t('lb.overview.search')}
            aria-label={t('lb.overview.searchLabel')}
            className="max-w-xs"
          />
          <SegmentedControl
            aria-label={t('lb.overview.state')}
            value={filter}
            onValueChange={setFilter}
            options={STATE_FILTERS.map((s) => ({
              value: s,
              label: t(FILTER_KEY[s]),
            }))}
          />
          {total > items.length ? (
            <span className="text-xs text-muted-foreground">
              {t('lb.overview.showing', {
                shown: items.length,
                total,
                limit: OVERVIEW_PAGE_LIMIT,
              })}
            </span>
          ) : null}
        </div>
      ) : null}

      {content}

      <ConfigureLoadBalancerDialog
        open={pickerOpen}
        onOpenChange={setPickerOpen}
      />
    </div>
  )
}
