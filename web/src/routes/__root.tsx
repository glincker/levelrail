import * as React from 'react'
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
import { CommandPalette } from '../components/CommandPalette'
import { ShortcutsDialog } from '../components/ShortcutsDialog'
import { useShortcuts } from '../hooks/useShortcuts'
import { ThemeProvider } from '../components/ThemeProvider'
import { AppHeader } from '../components/shell/AppHeader'
import { ShellBanner } from '../components/shell/ShellBanner'
import { SidebarInset, SidebarProvider } from '../components/ui/sidebar'
import { Toaster } from '../components/ui/toast'

// Router context carries the QueryClient so every route loader can call
// queryClient.ensureQueryData / ensureInfiniteQueryData without importing
// a module-level singleton. See main.tsx for where this gets populated.
export interface RouterContext {
  queryClient: QueryClient
}

// Routes reachable with no recorded session: /login itself, plus every
// route a visitor reaches by clicking a link in an email while logged
// out, where redirecting to /login would strand them before they ever
// see the page the link pointed at. /reset-password predates this array
// (it was previously compared to on its own); /accept-invite
// (routes/accept-invite.tsx) joins it for the same reason.
const PUBLIC_ROUTE_PATHS = ['/login', '/reset-password', '/accept-invite']

export const Route = createRootRouteWithContext<RouterContext>()({
  // Auth guard for the whole route tree: anything other than
  // PUBLIC_ROUTE_PATHS requires a recorded session. This is a
  // client-side heuristic, not the real enforcement, lib/authStore.ts's
  // own doc comment explains why: the real 401 (session actually expired
  // or the server restarted and wiped its in-memory session store) is
  // caught by the QueryCache/MutationCache handler in main.tsx, which
  // also redirects here. Between the two, every unauthenticated path
  // lands on /login: before a single authenticated fetch ever fires
  // (this check), and after one comes back 401 (the global handler).
  beforeLoad: ({ location }) => {
    if (
      getStoredUsername() === null &&
      !PUBLIC_ROUTE_PATHS.includes(location.pathname)
    ) {
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
// own beyond the loader above: layout routes reuse cached data rather than
// fetching in the component body. Brand hydration and auth-awareness were
// both flagged as deferred work on this file; both land in this pass.
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
// SidebarInset): this replaces the previous flat top-nav bar entirely,
// not an incremental change to it.
function AppShell() {
  const username = useAuthUsername()
  const [commandPaletteOpen, setCommandPaletteOpen] = React.useState(false)
  const [shortcutsOpen, setShortcutsOpen] = React.useState(false)
  const openShortcuts = React.useCallback(() => setShortcutsOpen(true), [])
  useShortcuts({ onHelp: openShortcuts })

  if (!username) {
    return <Outlet />
  }

  return (
    <SidebarProvider>
      <AppSidebar />
      <CommandPalette
        open={commandPaletteOpen}
        onOpenChange={setCommandPaletteOpen}
        onShowShortcuts={openShortcuts}
      />
      <ShortcutsDialog open={shortcutsOpen} onOpenChange={setShortcutsOpen} />
      <SidebarInset>
        <AppHeader onSearch={() => setCommandPaletteOpen(true)} />
        <ShellBanner />
        <main className="flex-1 overflow-y-auto p-4 md:p-6">
          <div className="mx-auto w-full max-w-6xl">
            <Outlet />
          </div>
          <p className="mx-auto mt-8 w-full max-w-6xl text-xs text-muted-foreground">
            Press{' '}
            <kbd className="rounded border border-border bg-muted px-1">?</kbd>{' '}
            for keyboard shortcuts
          </p>
        </main>
      </SidebarInset>
    </SidebarProvider>
  )
}
