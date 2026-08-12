import type { QueryClient } from '@tanstack/react-query'
import {
  createRootRouteWithContext,
  Outlet,
  Link,
} from '@tanstack/react-router'

// Router context carries the QueryClient so every route loader can call
// queryClient.ensureQueryData / ensureInfiniteQueryData without importing
// a module-level singleton. See main.tsx for where this gets populated.
export interface RouterContext {
  queryClient: QueryClient
}

export const Route = createRootRouteWithContext<RouterContext>()({
  component: RootLayout,
})

// Thin shell: brand name, nav, <Outlet />. No data fetching of its own,
// per frontend-plan.md section 3's "cross-cutting" rule that layout
// routes reuse cached data rather than fetching. The brand name is a
// placeholder literal for this pass only ("Levelrail" must never appear
// as a hardcoded string per CLAUDE.md section 3); wiring this header to
// GET /api/v1/brand and a BrandProvider context is out of scope here and
// tracked as deferred work in the report for this pass.
function RootLayout() {
  return (
    <div className="flex min-h-full flex-col">
      <header className="border-b border-neutral-200 dark:border-neutral-800">
        <nav className="mx-auto flex max-w-6xl items-center gap-6 px-4 py-3">
          <Link to="/apps" className="text-sm font-semibold tracking-tight">
            Deployment Platform
          </Link>
          <Link
            to="/apps"
            className="text-sm text-neutral-600 hover:text-neutral-900 dark:text-neutral-400 dark:hover:text-neutral-100"
            activeProps={{
              className: 'text-neutral-900 dark:text-neutral-100 font-medium',
            }}
          >
            Apps
          </Link>
        </nav>
      </header>
      <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-6">
        <Outlet />
      </main>
    </div>
  )
}
