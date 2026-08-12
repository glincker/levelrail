import { Link } from '@tanstack/react-router'
import type { AppSummary } from '../types/app'

const STATUS_STYLES: Record<AppSummary['status'], string> = {
  running:
    'bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-300',
  stopped:
    'bg-neutral-100 text-neutral-700 dark:bg-neutral-800 dark:text-neutral-300',
  deploying: 'bg-blue-100 text-blue-800 dark:bg-blue-900/40 dark:text-blue-300',
  crashloop: 'bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-300',
  unknown:
    'bg-neutral-100 text-neutral-500 dark:bg-neutral-800 dark:text-neutral-400',
}

// Links by app.name, not app.id: the detail route (routes/apps/$name.tsx)
// and its backing API (GET /api/v1/apps/{name}) both key off the app's
// name, the only identifier internal/api/apps.go's appResource actually
// carries. AppSummary.id is a separate field the list contract defines
// but the detail route has no use for.
export function AppRow({ app }: { app: AppSummary }) {
  return (
    <Link
      to="/apps/$name"
      params={{ name: app.name }}
      className="flex h-full w-full items-center gap-4 border-b border-neutral-100 px-4 py-3 hover:bg-neutral-50 dark:border-neutral-900 dark:hover:bg-neutral-900"
    >
      <span className="min-w-0 flex-1 truncate font-medium text-neutral-900 dark:text-neutral-100">
        {app.name}
      </span>
      <span className="min-w-0 flex-1 truncate text-sm text-neutral-500 dark:text-neutral-400">
        {app.primaryDomain ?? 'no domain'}
      </span>
      <span
        className={`shrink-0 rounded-full px-2 py-0.5 text-xs font-medium ${STATUS_STYLES[app.status]}`}
      >
        {app.status}
      </span>
    </Link>
  )
}

export function RowSkeleton() {
  return (
    <div className="flex h-full w-full items-center gap-4 border-b border-neutral-100 px-4 py-3 dark:border-neutral-900">
      <div className="h-4 w-32 animate-pulse rounded bg-neutral-200 dark:bg-neutral-800" />
      <div className="h-4 w-48 animate-pulse rounded bg-neutral-200 dark:bg-neutral-800" />
      <div className="h-4 w-16 animate-pulse rounded-full bg-neutral-200 dark:bg-neutral-800" />
    </div>
  )
}
