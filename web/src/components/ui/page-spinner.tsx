import { useTranslation } from 'react-i18next'
import { Skeleton } from './skeleton'

// Body placeholder for a page: header, toolbar and a list panel. Spacing
// matches the real page container so content does not jump when it arrives.
export function PageBody() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-2">
          <Skeleton className="h-7 w-56" decorative />
          <Skeleton className="h-4 w-80 max-w-full" decorative />
        </div>
        <Skeleton className="h-9 w-28 rounded-full" decorative />
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Skeleton className="h-9 w-64 max-w-full rounded-full" decorative />
        <Skeleton className="h-9 w-24 rounded-full" decorative />
        <Skeleton className="h-9 w-24 rounded-full" decorative />
      </div>
      <div className="divide-y divide-border rounded-xl border border-border">
        {Array.from({ length: 6 }, (_, i) => (
          <div key={i} className="flex items-center gap-3 px-4 py-3">
            <Skeleton className="size-8 rounded-lg" decorative />
            <div className="flex-1 space-y-1.5">
              <Skeleton className="h-4 w-1/3" decorative />
              <Skeleton className="h-3 w-1/2" decorative />
            </div>
            <Skeleton className="h-6 w-16 rounded-full" decorative />
          </div>
        ))}
      </div>
    </div>
  )
}

// Route pendingComponent fallback inside the app shell: only the body, since
// the sidebar and top bar are already on screen.
export function PageSpinner() {
  const { t } = useTranslation('common')
  return (
    <div role="status" aria-busy="true" aria-label={t('pageLoading.label')}>
      <PageBody />
    </div>
  )
}
