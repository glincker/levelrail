import { useRef, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { ArrowsSplitIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
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
  LB_STATE_LABEL,
  OVERVIEW_PAGE_LIMIT,
  useLoadBalancerList,
  type LoadBalancerOverviewState,
} from '../queries/loadBalancers'

const STATE_FILTERS: (LoadBalancerOverviewState | '')[] = [
  '',
  'balancing',
  'degraded',
  'none',
]

function LoadBalancerTable({
  items,
}: {
  items: NonNullable<ReturnType<typeof useLoadBalancerList>['data']>['items']
}) {
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
  const [search, setSearch] = useState('')
  const [state, setState] = useState<LoadBalancerOverviewState | ''>('')
  const [pickerOpen, setPickerOpen] = useState(false)
  const debouncedSearch = useDebouncedValue(search.trim(), 250)
  const query = useLoadBalancerList({ search: debouncedSearch, state })

  const filtered = debouncedSearch !== '' || state !== ''
  const items = query.data?.items ?? []
  const total = query.data?.total ?? 0

  let content
  if (query.isPending) {
    content = (
      <div className="h-40 animate-pulse rounded-lg border border-border bg-card" />
    )
  } else if (query.isError) {
    content = (
      <p className="text-sm text-destructive">
        Could not load load balancers: {query.error.message}
      </p>
    )
  } else if (items.length === 0 && !filtered) {
    content = null
  } else if (items.length === 0) {
    content = (
      <p className="rounded-lg border border-border bg-muted/30 px-4 py-4 text-center text-sm text-muted-foreground">
        No load balancers match this filter.
      </p>
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
        icon={<ArrowsSplitIcon />}
        titleKey="common:banners.loadBalancers.title"
        descriptionKey={
          isEmpty
            ? 'common:banners.loadBalancers.description'
            : 'common:banners.loadBalancers.compactDescription'
        }
        action={
          <>
            <HelpLink path="/load-balancing" label="Load balancing guide" />
            {query.isPending ? null : (
              <Button
                size="sm"
                onClick={() => {
                  setPickerOpen(true)
                }}
              >
                Configure a load balancer
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
            placeholder="Search by app name"
            aria-label="Search load balancers"
            className="max-w-xs"
          />
          <div
            className="flex items-center gap-1"
            role="group"
            aria-label="State"
          >
            {STATE_FILTERS.map((s) => (
              <Button
                key={s || 'all'}
                size="sm"
                variant={state === s ? 'secondary' : 'ghost'}
                aria-pressed={state === s}
                onClick={() => {
                  setState(s)
                }}
              >
                {s === '' ? 'All' : LB_STATE_LABEL[s]}
              </Button>
            ))}
          </div>
          {total > items.length ? (
            <span className="text-xs text-muted-foreground">
              Showing {items.length} of {total} (limit {OVERVIEW_PAGE_LIMIT})
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
