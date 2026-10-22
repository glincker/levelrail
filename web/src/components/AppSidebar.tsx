import { lazy, Suspense, useEffect } from 'react'
import { Link, useRouterState } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
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
import { updatesQueryOptions } from '../queries/updates'
import { appScopeName, databaseScopeName } from '../lib/sidebarScope'
import { GlobalNav } from './shell/GlobalNav'
import { sidebarArea } from './shell/sidebarArea'
import { scheduleIdle } from '../i18n/prefetch'
import { BrandMarkGlyph } from './BrandMarkGlyph'
import { SidebarStatusBadge } from './shell/SidebarStatusBadge'
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
  const { t } = useTranslation()
  const username = useAuthUsername()
  const logout = useLogout()
  const version = useQuery(updatesQueryOptions()).data?.current_version
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
              <span className="flex min-w-0 flex-col">
                <span className="truncate text-sm font-semibold">
                  {brand.ShortName || brand.Name}
                </span>
                <span className="truncate text-[11px] text-sidebar-foreground/60">
                  {version
                    ? `${t('shell.account.controlPlane')} v${version.replace(/^v/, '').split(/[-+]/)[0]}`
                    : t('shell.account.controlPlane')}
                </span>
              </span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      <SidebarContent className="[view-transition-name:nav-panel]">
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
