import { lazy, Suspense, useEffect } from 'react'
import { Link, useRouterState } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { SignOutIcon } from '@phosphor-icons/react/dist/ssr'
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
import { Button } from '@/components/ui/button'
import { useBrand } from '../hooks/useBrand'
import { useAuthUsername } from '../hooks/useAuthUsername'
import { useLogout } from '../queries/auth'
import { appScopeName, databaseScopeName } from '../lib/sidebarScope'
import { GlobalNav } from './shell/GlobalNav'
import { GLOBAL_NAV_GROUPS, isGlobalItemActive } from './shell/navModel'
import { BrandMarkGlyph } from './BrandMarkGlyph'
import { SidebarStatusBadge } from './shell/SidebarStatusBadge'
import { scheduleScrollActiveNavItem } from './shell/scrollActiveNavItem'

// Lazy: exactly one of these three renders at a time (mutually exclusive
// by pathname below), so a session that never visits /databases or
// /settings should not pay for their query modules in the eagerly-loaded
// root chunk either.
const AppScopedSidebar = lazy(() =>
  import('./AppScopedSidebar').then((m) => ({ default: m.AppScopedSidebar })),
)
const DatabaseScopedSidebar = lazy(() =>
  import('./DatabaseScopedSidebar').then((m) => ({
    default: m.DatabaseScopedSidebar,
  })),
)
const SettingsScopedSidebar = lazy(() =>
  import('./SettingsScopedSidebar').then((m) => ({
    default: m.SettingsScopedSidebar,
  })),
)

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

// Anchors on /settings itself (no captured name, unlike the two
// patterns above), covering both the hub (/settings) and every leaf
// page under it (/settings/github-app, etc). Checked after the app/
// database patterns below purely by convention, since /settings/ can
// never overlap with /apps/ or /databases/.
const SETTINGS_SCOPE_PATTERN = /^\/settings(\/|$)/

// Renders one of four nav modes by pathname (global, app-scoped,
// database-scoped, settings-scoped) in a single component so the brand
// header, account footer, and outer chrome stay written once instead of
// duplicated across a parent chooser and per-mode components.
export function AppSidebar() {
  const brand = useBrand()
  const { t } = useTranslation()
  const username = useAuthUsername()
  const logout = useLogout()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const scopedAppName = appScopeName(pathname)
  const scopedDatabaseName = databaseScopeName(pathname)
  // Pages the main nav lists (registry, node providers, IAM policies...) live
  // under /settings but keep the main sidebar, so opening one never swaps it.
  const ownedByGlobalNav = GLOBAL_NAV_GROUPS.some((g) =>
    g.items.some((i) => isGlobalItemActive(pathname, i)),
  )
  const isSettingsScoped =
    SETTINGS_SCOPE_PATTERN.test(pathname) && !ownedByGlobalNav

  // Keep the current page's entry visible in a long sidebar.
  useEffect(() => scheduleScrollActiveNavItem(), [pathname])

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
              <span className="flex min-w-0 flex-col">
                <span className="truncate text-sm font-semibold">
                  {brand.ShortName || brand.Name}
                </span>
                <span className="truncate text-[11px] text-sidebar-foreground/60">
                  {t('shell.account.controlPlane')}
                </span>
              </span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      <SidebarContent>
        {scopedAppName ? (
          <Suspense fallback={null}>
            <AppScopedSidebar name={decodeURIComponent(scopedAppName)} />
          </Suspense>
        ) : scopedDatabaseName ? (
          <Suspense fallback={null}>
            <DatabaseScopedSidebar
              name={decodeURIComponent(scopedDatabaseName)}
            />
          </Suspense>
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
          <SidebarMenuItem className="flex items-center gap-2 px-1 py-1 group-data-[collapsible=icon]:flex-col">
            <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-muted text-[11px] font-semibold text-foreground">
              {(username ?? '?').charAt(0).toUpperCase()}
            </span>
            <span className="min-w-0 flex-1 truncate text-xs font-medium group-data-[collapsible=icon]:hidden">
              {username}
            </span>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="size-6 shrink-0"
              aria-label={t('shell.account.signOut')}
              title={t('shell.account.signOut')}
              disabled={logout.isPending}
              onClick={() => {
                logout.mutate()
              }}
            >
              <SignOutIcon className="size-3.5" />
            </Button>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  )
}
