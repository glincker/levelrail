import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Card, CardContent, CardHeader } from './card'
import { Skeleton } from './skeleton'

type CardSkeletonProps = {
  /** Tailwind width class for the title bar. */
  titleWidth?: string
  /** Body rows; each reserves roughly one form row of final height. */
  rows?: number
  className?: string
}

// Reserves a card's final height while its data loads, so siblings below
// do not shift when the real card swaps in. One status label for readers.
export function CardSkeleton({
  titleWidth = 'w-40',
  rows = 2,
  className,
}: CardSkeletonProps) {
  const { t } = useTranslation('common')
  return (
    <div role="status" data-slot="card-skeleton">
      <span className="sr-only">{t('cardSkeleton.loading')}</span>
      <Card aria-hidden="true" className={className}>
        <CardHeader>
          <Skeleton decorative className={cn('h-5', titleWidth)} />
        </CardHeader>
        <CardContent className="space-y-3">
          {Array.from({ length: rows }, (_, i) => (
            <Skeleton key={i} decorative className="h-9 w-full" />
          ))}
        </CardContent>
      </Card>
    </div>
  )
}
