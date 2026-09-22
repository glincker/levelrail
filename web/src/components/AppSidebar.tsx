import { lazy, Suspense } from 'react'
import { Link, useRouterState } from '@tanstack/react-router'
import {
  GaugeIcon,
  StackIcon,
  BookOpenIcon,
  DatabaseIcon,
  SignOutIcon,
  HardDrivesIcon,
  GearIcon,
  FolderIcon,
  GlobeIcon,
  CloudArrowUpIcon,
  RobotIcon,
  GavelIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
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
import { useDeployApprovalsOptional } from '../queries/deployApprovals'

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
const APP_SCOPE_PATTERN = /^\/apps\/([^/]+)/

// Same shape, one level over for Databases (a planned fast-follow:
// Databases' own detail page gets the same treatment once the pattern
// is proven on Apps). The two patterns are mutually exclusive
// by construction, one anchors on /apps/, the other on /databases/, so a
// pathname can never match both, and the checks below stay a simple
// if/else-if rather than needing extra precedence handling.
const DATABASE_SCOPE_PATTERN = /^\/databases\/([^/]+)/

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
  const username = useAuthUsername()
  const logout = useLogout()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const scopedAppName = pathname.match(APP_SCOPE_PATTERN)?.[1]
  const scopedDatabaseName = pathname.match(DATABASE_SCOPE_PATTERN)?.[1]
  const isSettingsScoped = SETTINGS_SCOPE_PATTERN.test(pathname)
  // Optional convenience only, the same graceful-degradation shape
  // useImageTagsOptional's own doc comment establishes: a failure or
  // empty result here must never block the sidebar rendering, so the
  // badge simply doesn't show rather than surfacing a loading/error
  // state of its own.
  const pendingApprovals = useDeployApprovalsOptional('pending')
  const pendingApprovalCount = pendingApprovals.data?.length ?? 0

  return (
    <Sidebar collapsible="icon" variant="floating">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              size="lg"
              render={<Link to="/" />}
              className="data-[slot=sidebar-menu-button]:!p-1.5"
            >
              <span className="flex size-6 shrink-0 items-center justify-center rounded-md bg-primary text-xs font-bold text-primary-foreground">
                {(brand.ShortName || brand.Name || 'L').charAt(0)}
              </span>
              <span className="truncate text-sm font-semibold">
                {brand.ShortName || brand.Name}
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
          <>
            <SidebarGroup>
              <SidebarGroupContent>
                <SidebarMenu>
                  <SidebarMenuItem>
                    <SidebarMenuButton
                      render={<Link to="/" />}
                      isActive={pathname === '/'}
                      tooltip="Dashboard"
                    >
                      <GaugeIcon />
                      <span>Dashboard</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                  <SidebarMenuItem>
                    <SidebarMenuButton
                      render={<Link to="/apps" />}
                      isActive={pathname.startsWith('/apps')}
                      tooltip="Apps"
                    >
                      <StackIcon />
                      <span>Apps</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                  <SidebarMenuItem>
                    <SidebarMenuButton
                      render={<Link to="/databases" />}
                      isActive={pathname.startsWith('/databases')}
                      tooltip="Databases"
                    >
                      <DatabaseIcon />
                      <span>Databases</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                  <SidebarMenuItem>
                    <SidebarMenuButton
                      render={<Link to="/projects" />}
                      isActive={pathname.startsWith('/projects')}
                      tooltip="Projects"
                    >
                      <FolderIcon />
                      <span>Projects</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                  <SidebarMenuItem>
                    <SidebarMenuButton
                      render={<Link to="/nodes" />}
                      isActive={pathname.startsWith('/nodes')}
                      tooltip="Nodes"
                    >
                      <HardDrivesIcon />
                      <span>Nodes</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                  <SidebarMenuItem>
                    <SidebarMenuButton
                      render={<Link to="/domains" />}
                      isActive={pathname.startsWith('/domains')}
                      tooltip="Domains"
                    >
                      <GlobeIcon />
                      <span>Domains</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                  <SidebarMenuItem>
                    <SidebarMenuButton
                      render={<Link to="/backups" />}
                      isActive={pathname.startsWith('/backups')}
                      tooltip="Backups"
                    >
                      <CloudArrowUpIcon />
                      <span>Backups</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                  <SidebarMenuItem>
                    <SidebarMenuButton
                      render={<Link to="/approvals" />}
                      isActive={pathname.startsWith('/approvals')}
                      tooltip="Deploy approvals"
                    >
                      <GavelIcon />
                      <span>Deploy approvals</span>
                      {pendingApprovalCount > 0 ? (
                        <span className="ml-auto flex h-5 min-w-5 items-center justify-center rounded-full bg-amber-500 px-1 text-[10px] font-semibold text-white group-data-[collapsible=icon]:hidden">
                          {pendingApprovalCount}
                        </span>
                      ) : null}
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                  <SidebarMenuItem>
                    <SidebarMenuButton
                      render={<Link to="/ai-assistant" />}
                      isActive={pathname.startsWith('/ai-assistant')}
                      tooltip="AI Assistant"
                    >
                      <RobotIcon />
                      <span>AI Assistant</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                </SidebarMenu>
              </SidebarGroupContent>
            </SidebarGroup>

            {/* Single entry point into the settings hub; the grouped
                sub-nav (SettingsScopedSidebar) takes over once inside
                /settings/*. */}
            <SidebarGroup className="mt-auto">
              <SidebarGroupLabel>Settings</SidebarGroupLabel>
              <SidebarGroupContent>
                <SidebarMenu>
                  <SidebarMenuItem>
                    <SidebarMenuButton
                      render={<Link to="/settings" />}
                      isActive={pathname.startsWith('/settings')}
                      tooltip="Settings"
                    >
                      <GearIcon />
                      <span>Settings</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                  {brand.DocsURL ? (
                    <SidebarMenuItem>
                      <SidebarMenuButton
                        render={
                          <a
                            href={brand.DocsURL}
                            target="_blank"
                            rel="noopener noreferrer"
                          />
                        }
                        tooltip="Documentation"
                      >
                        <BookOpenIcon />
                        <span>Documentation</span>
                      </SidebarMenuButton>
                    </SidebarMenuItem>
                  ) : null}
                </SidebarMenu>
              </SidebarGroupContent>
            </SidebarGroup>
          </>
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
              aria-label="Sign out"
              title="Sign out"
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
