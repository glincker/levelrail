import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { Button } from '@/components/ui/button'
import { groupByDay, type DayGroup } from '../../lib/deliveryTimeline'
import type { TimelineItem } from '../../types/appDelivery'
import { DeliveryItemRow } from './DeliveryItemRow'

function dayLabel(day: string, t: TFunction<'delivery'>, now: Date): string {
  const [y, m, d] = day.split('-').map(Number)
  if (!y || !m || !d) return t('day.unknown')
  const date = new Date(y, m - 1, d)
  const startToday = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const diff = Math.round((startToday.getTime() - date.getTime()) / 86_400_000)
  if (diff === 0) return t('day.today')
  if (diff === 1) return t('day.yesterday')
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'full' }).format(date)
}

export function DeliveryTimeline({
  app,
  items,
  hasMore,
  loadingMore,
  onLoadMore,
}: {
  app: string
  items: readonly TimelineItem[]
  hasMore: boolean
  loadingMore: boolean
  onLoadMore: () => void
}) {
  const { t } = useTranslation('delivery')
  const groups: DayGroup[] = useMemo(() => groupByDay(items), [items])
  const now = new Date()
  return (
    <div className="space-y-5">
      {groups.map((g) => (
        <section key={g.day} aria-label={dayLabel(g.day, t, now)}>
          <h3 className="mb-2 text-xs font-semibold text-muted-foreground">
            {dayLabel(g.day, t, now)}
          </h3>
          <ol className="list-none">
            {g.items.map((item) => (
              <DeliveryItemRow key={item.id} item={item} app={app} />
            ))}
          </ol>
        </section>
      ))}
      {hasMore ? (
        <div className="flex justify-center">
          <Button
            variant="outline"
            size="sm"
            onClick={onLoadMore}
            disabled={loadingMore}
            aria-busy={loadingMore}
          >
            {t('loadMore')}
          </Button>
        </div>
      ) : null}
    </div>
  )
}
