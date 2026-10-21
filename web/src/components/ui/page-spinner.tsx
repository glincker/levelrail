import { useTranslation } from 'react-i18next'
import { Skeleton } from './skeleton'

// Route pendingComponent fallback: a header, stat row and two cards so the
// page does not jump when the loader resolves.
export function PageSpinner() {
  const { t } = useTranslation('common')
  return (
    <div
      className="space-y-6"
      role="status"
      aria-busy="true"
      aria-label={t('pageLoading.label')}
    >
      <div className="space-y-2">
        <Skeleton className="h-7 w-56" decorative />
        <Skeleton className="h-4 w-80 max-w-full" decorative />
      </div>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {Array.from({ length: 4 }, (_, i) => (
          <Skeleton key={i} className="h-20 w-full" decorative />
        ))}
      </div>
      <Skeleton className="h-48 w-full" decorative />
      <Skeleton className="h-32 w-full" decorative />
    </div>
  )
}
