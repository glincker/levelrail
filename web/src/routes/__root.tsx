import type { QueryClient } from '@tanstack/react-query'
import {
  createRootRouteWithContext,
  Outlet,
  redirect,
} from '@tanstack/react-router'
import { getStoredUsername } from '../lib/authStore'
import { useAuthUsername } from '../hooks/useAuthUsername'
import { brandQueryOptions } from '../queries/brand'
import { BrandProvider } from '../components/BrandProvider'
import { AppSidebar } from '../components/AppSidebar'
import { ThemeProvider } from '../components/ThemeProvider'
import { ThemeToggle } from '../components/ThemeToggle'
import {
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
} from '../components/ui/sidebar'
import { Separator } from '../components/ui/separator'
import { Toaster } from '../components/ui/toast'

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
    <ThemeProvider>
      <BrandProvider>
        <AppShell />
        <Toaster />
      </BrandProvider>
    </ThemeProvider>
  )
}

// The sidebar shell only applies once a session exists: /login itself
// renders full-bleed with no nav chrome (there is nothing to navigate to
// yet, and the login screen's own centered-card layout, LoginForm.tsx,
// already owns its full viewport). Every other route gets the real
// shadcn sidebar-07/dashboard-01 shape (SidebarProvider > AppSidebar +
// SidebarInset), per docs-local/research/dashboard-redesign: this
// replaces the previous flat top-nav bar entirely, not an incremental
// change to it.
function AppShell() {
  const username = useAuthUsername()

  if (!username) {
    return <Outlet />
  }

  return (
    <SidebarProvider>
      <AppSidebar />
      <SidebarInset>
        <header className="flex h-14 shrink-0 items-center gap-2 border-b border-border px-4">
          <SidebarTrigger className="-ml-1" />
          <Separator orientation="vertical" className="mr-2 h-4" />
          <div className="ml-auto">
            <ThemeToggle />
          </div>
        </header>
        <main className="flex-1 overflow-y-auto p-4 md:p-6">
          <div className="mx-auto w-full max-w-6xl">
            <Outlet />
          </div>
        </main>
      </SidebarInset>
    </SidebarProvider>
  )
}
