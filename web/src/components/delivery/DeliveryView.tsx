import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlugGlassIcon, SearchGlassIcon } from '@thesvg/ui-react'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { ApiErrorAlert } from '@/components/ApiErrorAlert'
import { useConnections } from '../../queries/connections'
import { useAppBindings, useAppDelivery } from '../../queries/appDelivery'
import {
  EMPTY_FILTERS,
  filterItems,
  isFiltered,
  type DeliveryFilters as Filters,
} from '../../lib/deliveryTimeline'
import { DeliveryFilters } from './DeliveryFilters'
import { DeliveryHeader } from './DeliveryHeader'
import { DeliveryTimeline } from './DeliveryTimeline'

export function DeliveryView({ app }: { app: string }) {
  const { t } = useTranslation('delivery')
  const [filters, setFilters] = useState<Filters>(EMPTY_FILTERS)
  const timeline = useAppDelivery(app)
  const bindings = useAppBindings(app)
  const connections = useConnections()

  // Pages are concatenated; a null or missing page contributes nothing.
  const all = useMemo(
    () => (timeline.data?.pages ?? []).flatMap((p) => p?.items ?? []),
    [timeline.data],
  )
  const visible = useMemo(() => filterItems(all, filters), [all, filters])

  let body: React.ReactNode
  if (timeline.isLoading) {
    body = (
      <div className="space-y-3" aria-busy="true">
        <Skeleton className="h-12 w-full" />
        <Skeleton className="h-12 w-full" />
        <Skeleton className="h-12 w-full" />
      </div>
    )
  } else if (timeline.isError) {
    body = (
      <div className="space-y-2">
        <ApiErrorAlert error={timeline.error} />
        <Button
          variant="outline"
          size="sm"
          onClick={() => void timeline.refetch()}
        >
          {t('retry')}
        </Button>
      </div>
    )
  } else if (all.length === 0) {
    body = (
      <EmptyState
        icon={<PlugGlassIcon className="size-6" />}
        title={t('empty.title')}
        description={t('empty.description')}
      />
    )
  } else if (visible.length === 0 && !timeline.hasNextPage) {
    body = (
      <EmptyState
        kind="no-results"
        icon={<SearchGlassIcon className="size-6" />}
        title={t('noMatch.title')}
        description={t('noMatch.description')}
        action={
          isFiltered(filters) ? (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setFilters(EMPTY_FILTERS)}
            >
              {t('noMatch.clear')}
            </Button>
          ) : undefined
        }
      />
    )
  } else {
    body = (
      <DeliveryTimeline
        app={app}
        items={visible}
        hasMore={Boolean(timeline.hasNextPage)}
        loadingMore={timeline.isFetchingNextPage}
        onLoadMore={() => void timeline.fetchNextPage()}
      />
    )
  }

  return (
    <div className="space-y-4">
      <DeliveryHeader
        items={all}
        app={app}
        bindings={bindings.data?.resolved ?? []}
        connections={connections.data ?? []}
      />
      <DeliveryFilters value={filters} onChange={setFilters} />
      {body}
    </div>
  )
}
