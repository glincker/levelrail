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
import { AppScopedSidebar } from './AppScopedSidebar'
import { DatabaseScopedSidebar } from './DatabaseScopedSidebar'

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

// The shadcn sidebar-07/dashboard-01 shape (docs-local/research/
// dashboard-redesign/cross-cutting-sidebar-patterns.md), minus the team
// switcher: Phase 1 is single-admin with nothing to switch between, so
// that header slot is deliberately not built rather than built for a
// workspace-of-one. Primary nav lives in SidebarContent, a secondary
// group (Settings, Docs) sits pinned above the account footer via
// `mt-auto` on its own SidebarGroup, matching the research's two-zone
// bottom structure.
//
// Vercel-style dynamic/contextual nav: this single
// component now has three rendering paths, chosen by the current pathname
// rather than separate components swapped in by __root.tsx. That
// keeps the brand header, account footer, and outer Sidebar/SidebarRail
// chrome (identical in every mode) written once, and makes the "which
// mode" decision live in exactly one place (the two scope patterns
// below) instead of being duplicated between a parent chooser and this
// file. Databases now gets the same scoped treatment Apps got first, per
// the spec's own "fast-follow once this pattern is proven once" note.
export function AppSidebar() {
  const brand = useBrand()
  const username = useAuthUsername()
  const logout = useLogout()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const scopedAppName = pathname.match(APP_SCOPE_PATTERN)?.[1]
  const scopedDatabaseName = pathname.match(DATABASE_SCOPE_PATTERN)?.[1]

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
          <AppScopedSidebar name={decodeURIComponent(scopedAppName)} />
        ) : scopedDatabaseName ? (
          <DatabaseScopedSidebar
            name={decodeURIComponent(scopedDatabaseName)}
          />
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
                  {/* Lightweight, non-auth organizational grouping
                      (repo-plan section 6's Phase 4 note on resisting a
                      permission matrix: this is deliberately not that),
                      sitting alongside Apps/Databases since it's the
                      same "resource an operator browses day to day"
                      category those two are, not admin configuration. */}
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
                  {/* Promoted from Settings (design spec item 1,
                      2026-08-14): multi-node placement is a real
                      resource kind an operator manages day to day, the
                      same category as Apps/Databases, not admin
                      configuration. */}
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
                  {/* Centralized cross-app domain list plus the
                      platform-wide ACME/primary-domain settings
                      (internal/api/ingress_settings.go): the same
                      "resource an operator browses day to day" category
                      Apps/Databases/Projects/Nodes already sit in, not
                      folded under Settings since it aggregates real,
                      per-app routing state, not account configuration. */}
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
                </SidebarMenu>
              </SidebarGroupContent>
            </SidebarGroup>

            {/* Pinned to the bottom of the content area, just above the
                account footer: the two-zone structure dashboard-01's
                NavSecondary establishes. mt-auto on the group itself, not
                a spacer element, so it works regardless of how much
                primary nav exists above it. Single entry point into the
                settings hub (routes/settings/index.tsx) rather than
                listing every settings page here. */}
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
