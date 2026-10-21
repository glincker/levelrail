import { useTranslation } from 'react-i18next'
import { getStoredUsername } from '../../lib/authStore'
import { PageBody } from './page-spinner'
import { Skeleton } from './skeleton'

const NAV_ROWS = 9

// Mirrors the signed-in shell (floating sidebar, top bar, centered body) with
// the same widths, heights and paddings, so the real UI replaces it in place.
export function ShellSkeleton() {
  const { t } = useTranslation('common', { useSuspense: false })
  return (
    <div
      role="status"
      aria-busy="true"
      aria-label={t('pageLoading.label', 'Loading')}
      className="flex min-h-svh w-full bg-background"
    >
      <div className="hidden w-64 shrink-0 p-2 md:block" aria-hidden="true">
        <div className="flex h-full flex-col gap-4 rounded-xl border border-sidebar-border bg-sidebar p-2">
          <div className="flex items-center gap-3 rounded-xl border border-sidebar-border p-1.5">
            <Skeleton className="size-8 rounded-lg" decorative />
            <div className="flex-1 space-y-1.5">
              <Skeleton className="h-3.5 w-24" decorative />
              <Skeleton className="h-3 w-16" decorative />
            </div>
          </div>
          <div className="flex-1 space-y-1.5">
            {Array.from({ length: NAV_ROWS }, (_, i) => (
              <Skeleton
                key={i}
                className="h-8 w-full rounded-full"
                decorative
              />
            ))}
          </div>
          <div className="flex items-center gap-2 px-1">
            <Skeleton className="size-7 rounded-full" decorative />
            <Skeleton className="h-3.5 w-24" decorative />
          </div>
        </div>
      </div>
      <div className="flex min-w-0 flex-1 flex-col">
        <div
          className="@container flex min-h-14 shrink-0 items-center gap-2 border-b border-border px-4"
          aria-hidden="true"
        >
          <Skeleton className="size-7 rounded-md" decorative />
          <Skeleton className="h-9 w-44 rounded-full @3xl:w-64" decorative />
          <div className="flex-1" />
          <Skeleton className="size-8 rounded-full" decorative />
          <Skeleton className="size-8 rounded-full" decorative />
        </div>
        <main className="flex-1 px-4 py-4 md:px-(--glinr-page-pad-x) md:py-6">
          <div className="mx-auto w-full min-w-0 max-w-(--glinr-content-max)">
            <PageBody />
          </div>
        </main>
      </div>
    </div>
  )
}

// Signed-out pages (login) render full-bleed with no chrome, so a shell
// placeholder would flash a layout that never appears.
function AuthSkeleton() {
  const { t } = useTranslation('common', { useSuspense: false })
  return (
    <div
      role="status"
      aria-busy="true"
      aria-label={t('pageLoading.label', 'Loading')}
      className="flex min-h-svh items-center justify-center bg-background p-4"
    >
      <div
        className="w-full max-w-sm space-y-4 rounded-xl border border-border p-6"
        aria-hidden="true"
      >
        <Skeleton className="h-6 w-32" decorative />
        <Skeleton className="h-10 w-full rounded-lg" decorative />
        <Skeleton className="h-10 w-full rounded-lg" decorative />
        <Skeleton className="h-10 w-full rounded-full" decorative />
      </div>
    </div>
  )
}

/** Root Suspense fallback: the shell when a session exists, a centered card otherwise. */
export function RootFallback() {
  return getStoredUsername() !== null ? <ShellSkeleton /> : <AuthSkeleton />
}
