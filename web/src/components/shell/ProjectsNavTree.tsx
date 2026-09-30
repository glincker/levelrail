import { Link } from '@tanstack/react-router'
import { FolderIcon, StackIcon } from '@phosphor-icons/react/dist/ssr'
import {
  SidebarMenuAction,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
} from '@/components/ui/sidebar'
import { useAppListOptional } from '@/queries/apps'
import { useProjectListOptional } from '@/queries/projects'
import type { AppListEntry } from '@/types/appDetail'
import type { ProjectResource } from '@/types/projectDetail'
import { NavChevron, NavCollapsePanel } from './CollapsibleSection'
import { isGlobalItemActive, resolveOpen, type GlobalNavItem } from './navModel'
import { usePersistedToggles } from './usePersistedToggles'

// Separate from GLOBAL_NAV_STORAGE_KEY (GlobalNav.tsx) on purpose: that
// key stores top-level group open/closed state, this stores the
// Projects tree's own expand state. Sharing one key would let toggling
// a global nav group corrupt a project's expand state and vice versa.
const PROJECTS_TREE_STORAGE_KEY = 'shell.nav.projectsTree.open'

// Sentinel id for "is the Projects tree itself expanded", stored
// alongside per-project ids (project.id) in the same toggle record.
// Real project ids are opaque, mint-time-random strings (see
// migrations/0022_projects.sql), so collision with this literal is not
// a practical concern.
const TREE_ROOT_ID = '__root__'

function isAppActive(pathname: string, name: string): boolean {
  return pathname === `/apps/${name}` || pathname.startsWith(`/apps/${name}/`)
}

function isProjectActive(pathname: string, id: string): boolean {
  return (
    pathname === `/projects/${id}` || pathname.startsWith(`/projects/${id}/`)
  )
}

function ProjectApps({
  projectId,
  apps,
  pathname,
}: {
  projectId: string
  apps: AppListEntry[]
  pathname: string
}) {
  const matching = apps.filter((app) => app.project_id === projectId)
  if (matching.length === 0) {
    return (
      <li className="px-2 py-1 text-xs text-sidebar-foreground/50">
        No apps yet
      </li>
    )
  }
  return (
    <>
      {matching.map((app) => (
        <SidebarMenuSubItem key={app.name}>
          <SidebarMenuSubButton
            render={<Link to="/apps/$name" params={{ name: app.name }} />}
            size="sm"
            isActive={isAppActive(pathname, app.name)}
          >
            <StackIcon />
            <span>{app.name}</span>
          </SidebarMenuSubButton>
        </SidebarMenuSubItem>
      ))}
    </>
  )
}

function ProjectTreeRow({
  project,
  apps,
  pathname,
  open,
  onToggle,
}: {
  project: ProjectResource
  apps: AppListEntry[]
  pathname: string
  open: boolean
  onToggle: () => void
}) {
  const panelId = `nav-panel-project-${project.id}`
  return (
    <SidebarMenuSubItem>
      <SidebarMenuSubButton
        render={<Link to="/projects/$id" params={{ id: project.id }} />}
        isActive={isProjectActive(pathname, project.id)}
        className="pr-7"
      >
        <FolderIcon />
        <span>{project.name}</span>
      </SidebarMenuSubButton>
      <button
        type="button"
        aria-expanded={open}
        aria-controls={panelId}
        aria-label={`${open ? 'Collapse' : 'Expand'} ${project.name}`}
        onClick={onToggle}
        className="absolute top-0.5 right-0.5 flex size-6 items-center justify-center rounded-full text-sidebar-foreground/60 outline-hidden hover:bg-sidebar-accent hover:text-sidebar-foreground focus-visible:ring-2 focus-visible:ring-sidebar-ring"
      >
        <NavChevron open={open} />
      </button>
      <NavCollapsePanel id={panelId} open={open}>
        <SidebarMenuSub className="mt-1">
          <ProjectApps projectId={project.id} apps={apps} pathname={pathname} />
        </SidebarMenuSub>
      </NavCollapsePanel>
    </SidebarMenuSubItem>
  )
}

// Renders the "Projects" global nav item as an expandable tree instead
// of a flat link: the link itself still navigates to /projects
// unchanged, a separate chevron action reveals the user's real projects,
// and each project further expands to the apps actually filed under it
// (app.project_id === project.id). Apps with no project are deliberately
// never shown here, see the top-level flat "Apps" nav item for those.
export function ProjectsNavItem({
  item,
  pathname,
}: {
  item: GlobalNavItem
  pathname: string
}) {
  const projectsQuery = useProjectListOptional()
  const appsQuery = useAppListOptional()
  const projects = projectsQuery.data ?? []
  const apps = appsQuery.data ?? []
  const [stored, setOpen] = usePersistedToggles(PROJECTS_TREE_STORAGE_KEY)
  const treeOpen = resolveOpen(stored, TREE_ROOT_ID, false)
  const panelId = 'nav-panel-projects-tree'

  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        render={<Link to={item.to} />}
        isActive={isGlobalItemActive(pathname, item)}
        tooltip={item.label}
        className={projects.length > 0 ? 'pr-8' : undefined}
      >
        {item.icon}
        <span>{item.label}</span>
      </SidebarMenuButton>
      {projects.length > 0 && (
        <SidebarMenuAction
          render={
            <button
              type="button"
              aria-expanded={treeOpen}
              aria-controls={panelId}
              aria-label={`${treeOpen ? 'Collapse' : 'Expand'} projects`}
              onClick={() => setOpen(TREE_ROOT_ID, !treeOpen)}
            />
          }
        >
          <NavChevron open={treeOpen} />
        </SidebarMenuAction>
      )}
      {projects.length > 0 && (
        <NavCollapsePanel id={panelId} open={treeOpen}>
          <SidebarMenuSub className="mt-1">
            {projects.map((project) => (
              <ProjectTreeRow
                key={project.id}
                project={project}
                apps={apps}
                pathname={pathname}
                open={resolveOpen(stored, project.id, false)}
                onToggle={() =>
                  setOpen(project.id, !resolveOpen(stored, project.id, false))
                }
              />
            ))}
          </SidebarMenuSub>
        </NavCollapsePanel>
      )}
    </SidebarMenuItem>
  )
}
