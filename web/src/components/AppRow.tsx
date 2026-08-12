import { Link } from '@tanstack/react-router'
import type { AppDetail } from '../types/appDetail'

// Links by app.name, not a separate id: the detail route
// (routes/apps/$name.tsx) and its backing API (GET /api/v1/apps/{name})
// both key off the app's name, the only identifier appResource actually
// carries.
//
// No status badge here: appResource has no status field, and rendering
// one would mean an extra GET /api/v1/apps/{name}/deploys per row (an
// N+1 fetch for a list that's supposed to stay cheap at 50+ rows, per
// CLAUDE.md 7's virtualization requirement). The app detail route
// (ConditionsPanel) is where current reconcile status actually lives;
// inventing a placeholder status here would be exactly the kind of
// fabricated contract this repo's own conventions warn against.
export function AppRow({ app }: { app: AppDetail }) {
  const domain = app.domains?.[0] ?? null
  const extraDomains = (app.domains?.length ?? 0) - 1

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
        {app.image}
      </span>
      <span className="shrink-0 truncate text-sm text-neutral-500 dark:text-neutral-400">
        {domain ? (
          <>
            {domain}
            {extraDomains > 0 ? ` +${extraDomains}` : null}
          </>
        ) : (
          'no domain'
        )}
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
