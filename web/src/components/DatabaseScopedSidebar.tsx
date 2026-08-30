import { Link, useRouterState } from '@tanstack/react-router'
import {
  ArrowLeftIcon,
  SquaresFourIcon,
  CpuIcon,
  PulseIcon,
  ScrollIcon,
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
import { useDatabase, useDatabaseStatus } from '../queries/databases'
import { summarizeDatabaseStatus } from '../lib/databaseStatus'

// The database-scoped half of AppSidebar's Vercel-style dynamic nav,
// direct structural sibling of AppScopedSidebar.tsx: rendered in place of
// the global nav whenever the current route is under /databases/$name/*.
//
// Four nav items now, not two: Overview and Resources (unchanged) plus
// Metrics and Logs (internal/api/database_metrics.go,
// internal/api/database_logs.go), closing the observability gap this
// sidebar used to document as missing. Still no domains/environment/
// health/alerts equivalent, and still no frontend connection-string/
// credentials display (cmd/levelrail's database_credentials.go remains a
// CLI-only subcommand): those stay genuinely absent, not just
// undocumented.
//
// Reads database name/status from the same query cache
// routes/databases/$name.tsx's layout route loader already primed
// (queries/databases.ts): no fetch of its own, same "matched routes'
// loaders finish before their component tree renders" guarantee
// AppScopedSidebar relies on.
export function DatabaseScopedSidebar({ name }: { name: string }) {
  const { data: database } = useDatabase(name)
  const { data: conditions } = useDatabaseStatus(name)
  const status = summarizeDatabaseStatus(conditions)
  const pathname = useRouterState({ select: (s) => s.location.pathname })

  return (
    <>
      <SidebarGroup>
        <SidebarGroupContent>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                render={<Link to="/databases" />}
                tooltip="Back to Databases"
              >
                <ArrowLeftIcon />
                <span>Back to Databases</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>

      <SidebarGroup>
        <SidebarGroupLabel className="flex h-auto items-center gap-2 py-1.5">
          <span className="truncate font-semibold text-sidebar-foreground">
            {database.name}
          </span>
          <Badge variant={status.variant} className="shrink-0">
            {status.label}
          </Badge>
        </SidebarGroupLabel>
        <SidebarGroupContent>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                render={
                  <Link to="/databases/$name/overview" params={{ name }} />
                }
                isActive={pathname.endsWith('/overview')}
                tooltip="Overview"
              >
                <SquaresFourIcon />
                <span>Overview</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
            <SidebarMenuItem>
              <SidebarMenuButton
                render={
                  <Link to="/databases/$name/resources" params={{ name }} />
                }
                isActive={pathname.endsWith('/resources')}
                tooltip="Resources"
              >
                <CpuIcon />
                <span>Resources</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
            <SidebarMenuItem>
              <SidebarMenuButton
                render={
                  <Link to="/databases/$name/metrics" params={{ name }} />
                }
                isActive={pathname.endsWith('/metrics')}
                tooltip="Metrics"
              >
                <PulseIcon />
                <span>Metrics</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
            <SidebarMenuItem>
              <SidebarMenuButton
                render={<Link to="/databases/$name/logs" params={{ name }} />}
                isActive={pathname.endsWith('/logs')}
                tooltip="Logs"
              >
                <ScrollIcon />
                <span>Logs</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>
    </>
  )
}
