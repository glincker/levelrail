import { Link, useRouterState } from '@tanstack/react-router'
import { ArrowLeftIcon, SquaresFourIcon } from '@phosphor-icons/react/dist/ssr'
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
// Deliberately one nav item, not eight: Databases has exactly one real
// section today (Overview, which already folds in the engine/version/node
// summary and ConditionsPanel's reconcile status). There is no
// domains/environment/health/resources/metrics/logs/alerts equivalent for
// a database resource, and no frontend connection-string/credentials
// display exists to justify a second nested route either (grepped for
// one, the only "credentials" hit in the whole frontend is the unrelated
// change-password flow; cmd/levelrail's database_credentials.go is a CLI
// subcommand, not a web UI surface). A one-item nav that looks sparse is
// more honest than padding this out to match Apps's section count.
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
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>
    </>
  )
}
