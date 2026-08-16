import { Link, useRouterState } from '@tanstack/react-router'
import {
  ArrowLeftIcon,
  SquaresFourIcon,
  ClockCounterClockwiseIcon,
  GlobeIcon,
  BracketsCurlyIcon,
  HeartbeatIcon,
  CpuIcon,
  PulseIcon,
  ScrollIcon,
  BellIcon,
  TerminalIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from '@/components/ui/sidebar'
import { Badge } from '@/components/ui/badge'
import { useApp } from '../queries/apps'
import { useDeployStatus } from '../queries/deploys'
import { summarizeAppStatus } from '../lib/appStatus'

// The app-scoped half of AppSidebar's Vercel-style dynamic nav: rendered
// in place of the global nav whenever the current route is under
// /apps/$name/*. The 9 section routes are grouped into Deploy
// (Overview, Deploys, Domains), Configure (Environment, Health,
// Resources), and Observe (Metrics, Logs, Alerts), matching the
// SidebarGroupLabel pattern the global sidebar's Settings group uses.
//
// Reads app name/status from the same query cache routes/apps/$name.tsx's
// layout route loader already primed (queries/apps.ts, queries/
// deploys.ts): no fetch of its own. Safe to call useApp/useDeployStatus
// unconditionally here (both are useSuspenseQuery) because AppSidebar
// only mounts this component once that loader has already resolved, the
// same "matched routes' loaders finish before their component tree
// renders" guarantee the layout route itself relies on.
export function AppScopedSidebar({ name }: { name: string }) {
  const { data: app } = useApp(name)
  const { data: conditions } = useDeployStatus(name)
  const status = summarizeAppStatus(conditions)
  const pathname = useRouterState({ select: (s) => s.location.pathname })

  return (
    <>
      <SidebarGroup>
        <SidebarGroupContent>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                render={<Link to="/apps" />}
                tooltip="Back to Apps"
              >
                <ArrowLeftIcon />
                <span>Back to Apps</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>

      <SidebarGroup>
        <SidebarGroupLabel className="flex h-auto items-center gap-2 py-1.5">
          <span className="truncate font-semibold text-sidebar-foreground">
            {app.name}
          </span>
          <Badge variant={status.variant} className="shrink-0">
            {status.label}
          </Badge>
        </SidebarGroupLabel>
      </SidebarGroup>

      <SidebarGroup>
        <SidebarGroupLabel>Deploy</SidebarGroupLabel>
        <SidebarGroupContent>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                render={<Link to="/apps/$name/overview" params={{ name }} />}
                isActive={pathname.endsWith('/overview')}
                tooltip="Overview"
              >
                <SquaresFourIcon />
                <span>Overview</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
            <SidebarMenuItem>
              <SidebarMenuButton
                render={<Link to="/apps/$name/deploys" params={{ name }} />}
                isActive={pathname.endsWith('/deploys')}
                tooltip="Deploys"
              >
                <ClockCounterClockwiseIcon />
                <span>Deploys</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
            <SidebarMenuItem>
              <SidebarMenuButton
                render={<Link to="/apps/$name/domains" params={{ name }} />}
                isActive={pathname.endsWith('/domains')}
                tooltip="Domains"
              >
                <GlobeIcon />
                <span>Domains</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>

      <SidebarGroup>
        <SidebarGroupLabel>Configure</SidebarGroupLabel>
        <SidebarGroupContent>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                render={<Link to="/apps/$name/environment" params={{ name }} />}
                isActive={pathname.endsWith('/environment')}
                tooltip="Environment"
              >
                <BracketsCurlyIcon />
                <span>Environment</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
            <SidebarMenuItem>
              <SidebarMenuButton
                render={<Link to="/apps/$name/health" params={{ name }} />}
                isActive={pathname.endsWith('/health')}
                tooltip="Health"
              >
                <HeartbeatIcon />
                <span>Health</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
            <SidebarMenuItem>
              <SidebarMenuButton
                render={<Link to="/apps/$name/resources" params={{ name }} />}
                isActive={pathname.endsWith('/resources')}
                tooltip="Resources"
              >
                <CpuIcon />
                <span>Resources</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>

      <SidebarGroup>
        <SidebarGroupLabel>Observe</SidebarGroupLabel>
        <SidebarGroupContent>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                render={<Link to="/apps/$name/metrics" params={{ name }} />}
                isActive={pathname.endsWith('/metrics')}
                tooltip="Metrics"
              >
                <PulseIcon />
                <span>Metrics</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
            <SidebarMenuItem>
              <SidebarMenuButton
                render={<Link to="/apps/$name/logs" params={{ name }} />}
                isActive={pathname.endsWith('/logs')}
                tooltip="Logs"
              >
                <ScrollIcon />
                <span>Logs</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
            <SidebarMenuItem>
              <SidebarMenuButton
                render={<Link to="/apps/$name/alerts" params={{ name }} />}
                isActive={pathname.endsWith('/alerts')}
                tooltip="Alerts"
              >
                <BellIcon />
                <span>Alerts</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
            <SidebarMenuItem>
              <SidebarMenuButton
                render={<Link to="/apps/$name/exec" params={{ name }} />}
                isActive={pathname.endsWith('/exec')}
                tooltip="Exec"
              >
                <TerminalIcon />
                <span>Exec</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>
    </>
  )
}
