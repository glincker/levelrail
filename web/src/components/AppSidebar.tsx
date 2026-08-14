import { Link, useRouterState } from '@tanstack/react-router'
import {
  StackIcon,
  BookOpenIcon,
  DatabaseIcon,
  KeyIcon,
  SignOutIcon,
  HardDrivesIcon,
  GearIcon,
  ShieldIcon,
  UserIcon,
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

// The shadcn sidebar-07/dashboard-01 shape (docs-local/research/
// dashboard-redesign/cross-cutting-sidebar-patterns.md), minus the team
// switcher: Phase 1 is single-admin with nothing to switch between, so
// that header slot is deliberately not built rather than built for a
// workspace-of-one. Primary nav lives in SidebarContent, a secondary
// group (Settings, Docs) sits pinned above the account footer via
// `mt-auto` on its own SidebarGroup, matching the research's two-zone
// bottom structure.
export function AppSidebar() {
  const brand = useBrand()
  const username = useAuthUsername()
  const logout = useLogout()
  const pathname = useRouterState({ select: (s) => s.location.pathname })

  return (
    <Sidebar collapsible="icon">
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              size="lg"
              render={<Link to="/apps" />}
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
        <SidebarGroup>
          <SidebarGroupContent>
            <SidebarMenu>
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
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>

        {/* Pinned to the bottom of the content area, just above the
            account footer: the two-zone structure dashboard-01's
            NavSecondary establishes. mt-auto on the group itself, not a
            spacer element, so it works regardless of how much primary
            nav exists above it. A real "Settings" section (account,
            security, tokens, general), not just the one API-tokens link
            this group used to hold: Coolify and Dokploy both surface
            these even before a single app is deployed, and Levelrail's
            equivalent screens (routes/settings/*) exist now too. */}
        <SidebarGroup className="mt-auto">
          <SidebarGroupLabel>Settings</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton
                  render={<Link to="/settings/account" />}
                  isActive={pathname.startsWith('/settings/account')}
                  tooltip="Account"
                >
                  <UserIcon />
                  <span>Account</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
              <SidebarMenuItem>
                <SidebarMenuButton
                  render={<Link to="/settings/security" />}
                  isActive={pathname.startsWith('/settings/security')}
                  tooltip="Security"
                >
                  <ShieldIcon />
                  <span>Security</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
              <SidebarMenuItem>
                <SidebarMenuButton
                  render={<Link to="/settings/tokens" />}
                  isActive={pathname.startsWith('/settings/tokens')}
                  tooltip="API tokens"
                >
                  <KeyIcon />
                  <span>API tokens</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
              <SidebarMenuItem>
                <SidebarMenuButton
                  render={<Link to="/settings/general" />}
                  isActive={pathname.startsWith('/settings/general')}
                  tooltip="General"
                >
                  <GearIcon />
                  <span>General</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
              {/* Infra/admin config, not a deployable resource kind like
                  Apps/Databases, so it lives in Settings rather than the
                  primary group above. Multi-node is an occasional admin
                  action for this product's 1-10 machine target audience
                  (CLAUDE.md 1), not a daily-use screen, the same reasoning
                  that keeps Account/Security/API tokens/General down
                  here instead of promoted to primary nav. */}
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
