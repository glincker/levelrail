import type { QueryClient } from '@tanstack/react-query'
import {
  createRootRouteWithContext,
  Outlet,
  Link,
  redirect,
} from '@tanstack/react-router'
import { getStoredUsername } from '../lib/authStore'
import { useAuthUsername } from '../hooks/useAuthUsername'
import { useBrand } from '../hooks/useBrand'
import { useLogout } from '../queries/auth'
import { brandQueryOptions } from '../queries/brand'
import { BrandProvider } from '../components/BrandProvider'
import { Button } from '../components/ui/button'

// Router context carries the QueryClient so every route loader can call
// queryClient.ensureQueryData / ensureInfiniteQueryData without importing
// a module-level singleton. See main.tsx for where this gets populated.
export interface RouterContext {
  queryClient: QueryClient
}

export const Route = createRootRouteWithContext<RouterContext>()({
  // Auth guard for the whole route tree (docs-local/research/dashboard-
  // gap-audit-and-devmode.md gaps #1/#2/#4): anything other than /login
  // requires a recorded session. This is a client-side heuristic, not the
  // real enforcement, lib/authStore.ts's own doc comment explains why:
  // the real 401 (session actually expired or the server restarted and
  // wiped its in-memory session store) is caught by the QueryCache/
  // MutationCache handler in main.tsx, which also redirects here. Between
  // the two, every unauthenticated path lands on /login: before a single
  // authenticated fetch ever fires (this check), and after one comes back
  // 401 (the global handler).
  beforeLoad: ({ location }) => {
    if (getStoredUsername() === null && location.pathname !== '/login') {
      redirect({ to: '/login', throw: true })
    }
  },
  // Primes brand (GET /api/v1/brand) once per navigation to a route this
  // layout wraps, public and needed before a session exists (the login
  // screen itself). BrandProvider below reads the same cache entry via
  // useSuspenseQuery, so this is the one network call, not two.
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(brandQueryOptions()),
  component: RootLayout,
})

// Thin shell: brand-aware header/nav, <Outlet />. No data fetching of its
// own beyond the loader above, per frontend-plan.md section 3's
// "cross-cutting" rule that layout routes reuse cached data rather than
// fetching in the component body. Brand hydration and auth-awareness were
// both flagged as deferred work on this file; both land in this pass
// (docs-local/research/dashboard-gap-audit-and-devmode.md gaps #4 and #6).
function RootLayout() {
  return (
    <BrandProvider>
      <AppShell />
    </BrandProvider>
  )
}

function AppShell() {
  const brand = useBrand()
  const username = useAuthUsername()
  const logout = useLogout()

  return (
    <div className="flex min-h-full flex-col">
      <header className="border-b border-neutral-200 dark:border-neutral-800">
        <nav className="mx-auto flex max-w-6xl items-center gap-6 px-4 py-3">
          <Link to="/apps" className="text-sm font-semibold tracking-tight">
            {brand.ShortName || brand.Name}
          </Link>
          {username ? (
            <>
              <Link
                to="/apps"
                className="text-sm text-neutral-600 hover:text-neutral-900 dark:text-neutral-400 dark:hover:text-neutral-100"
                activeProps={{
                  className:
                    'text-neutral-900 dark:text-neutral-100 font-medium',
                }}
              >
                Apps
              </Link>
              <Link
                to="/settings/tokens"
                className="text-sm text-neutral-600 hover:text-neutral-900 dark:text-neutral-400 dark:hover:text-neutral-100"
                activeProps={{
                  className:
                    'text-neutral-900 dark:text-neutral-100 font-medium',
                }}
              >
                API tokens
              </Link>
              <div className="ml-auto flex items-center gap-3">
                <span className="text-sm text-neutral-500 dark:text-neutral-400">
                  {username}
                </span>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    logout.mutate()
                  }}
                  disabled={logout.isPending}
                >
                  {logout.isPending ? 'Signing out...' : 'Sign out'}
                </Button>
              </div>
            </>
          ) : null}
        </nav>
      </header>
      <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-6">
        <Outlet />
      </main>
    </div>
  )
}
