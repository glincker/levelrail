import { lazy, Suspense, useEffect } from 'react'
import { CatchBoundary, Link, useRouterState } from '@tanstack/react-router'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
} from '@/components/ui/sidebar'
import { useBrand } from '../hooks/useBrand'
import { AccountMenu } from './shell/AccountMenu'
import { appScopeName, databaseScopeName } from '../lib/sidebarScope'
import { GlobalNav } from './shell/GlobalNav'
import { sidebarArea } from './shell/sidebarArea'
import { scheduleIdle } from '../i18n/prefetch'
import { BrandMarkGlyph } from './BrandMarkGlyph'
import { SidebarStatusBadge } from './shell/SidebarStatusBadge'
import { SidebarVersionLink } from './shell/SidebarVersionLink'
import { scheduleScrollActiveNavItem } from './shell/scrollActiveNavItem'

// Lazy: exactly one of these three renders at a time (mutually exclusive
// by pathname below), so a session that never visits /databases or
// /settings should not pay for their query modules in the eagerly-loaded
// root chunk either.
const loadAppScoped = () =>
  import('./AppScopedSidebar').then((m) => ({ default: m.AppScopedSidebar }))
const loadDatabaseScoped = () =>
  import('./DatabaseScopedSidebar').then((m) => ({
    default: m.DatabaseScopedSidebar,
  }))
const loadSettingsScoped = () =>
  import('./SettingsScopedSidebar').then((m) => ({
    default: m.SettingsScopedSidebar,
  }))
const AppScopedSidebar = lazy(loadAppScoped)
const DatabaseScopedSidebar = lazy(loadDatabaseScoped)
const SettingsScopedSidebar = lazy(loadSettingsScoped)

// Fetch the scoped chunks while idle so switching areas never shows an empty sidebar.
let scopedChunksWarmed = false
function warmScopedSidebars() {
  if (scopedChunksWarmed) return
  scopedChunksWarmed = true
  scheduleIdle(() => {
    void loadAppScoped()
    void loadDatabaseScoped()
    void loadSettingsScoped()
  })
}

// Matches /apps/<name> and any nested path under it, capturing <name>.
// Deliberately excludes the bare /apps list route (no trailing segment)
// so the list page keeps the global nav, only a specific app's detail
// tree switches to the scoped one below.

// Same shape, one level over for Databases (a planned fast-follow:
// Databases' own detail page gets the same treatment once the pattern
// is proven on Apps). The two patterns are mutually exclusive
// by construction, one anchors on /apps/, the other on /databases/, so a
// pathname can never match both, and the checks below stay a simple
// if/else-if rather than needing extra precedence handling.

// Renders one of four nav modes by pathname (global, app-scoped,
// database-scoped, settings-scoped) in a single component so the brand
// header, account footer, and outer chrome stay written once instead of
// duplicated across a parent chooser and per-mode components.
export function AppSidebar() {
  const brand = useBrand()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const scopedAppName = appScopeName(pathname)
  const scopedDatabaseName = databaseScopeName(pathname)
  const isSettingsScoped = sidebarArea(pathname) === 'settings'

  // Keep the current page's entry visible in a long sidebar.
  useEffect(() => scheduleScrollActiveNavItem(), [pathname])
  useEffect(warmScopedSidebars, [])

  return (
    <Sidebar collapsible="icon" variant="floating">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              size="lg"
              render={<Link to="/" />}
              className="gap-3 rounded-xl border border-sidebar-border bg-sidebar-accent/40 data-[slot=sidebar-menu-button]:!p-1.5 hover:bg-sidebar-accent"
            >
              <span className="relative flex size-8 shrink-0 items-center justify-center text-sm font-bold text-foreground">
                <BrandMarkGlyph svgWrapperClassName="flex size-7 items-center justify-center [&_svg]:size-full" />
                <SidebarStatusBadge />
              </span>
              <span className="truncate text-sm font-semibold">
                {brand.ShortName || brand.Name}
              </span>
            </SidebarMenuButton>
            <SidebarVersionLink />
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      {/* A missing app or database makes the scoped nav throw; without this the
          root boundary replaced the whole shell with the error card. */}
      <SidebarContent className="[view-transition-name:nav-panel]">
        {scopedAppName ? (
          <CatchBoundary
            getResetKey={() => pathname}
            errorComponent={GlobalNav}
          >
            <Suspense fallback={null}>
              <AppScopedSidebar name={decodeURIComponent(scopedAppName)} />
            </Suspense>
          </CatchBoundary>
        ) : scopedDatabaseName ? (
          <CatchBoundary
            getResetKey={() => pathname}
            errorComponent={GlobalNav}
          >
            <Suspense fallback={null}>
              <DatabaseScopedSidebar
                name={decodeURIComponent(scopedDatabaseName)}
              />
            </Suspense>
          </CatchBoundary>
        ) : isSettingsScoped ? (
          <Suspense fallback={null}>
            <SettingsScopedSidebar />
          </Suspense>
        ) : (
          <GlobalNav />
        )}
      </SidebarContent>

      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <AccountMenu />
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  )
}
