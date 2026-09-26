import { Link, useRouterState } from '@tanstack/react-router'
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from '@/components/ui/sidebar'
import { cn } from '@/lib/utils'
import { AppNavHeader } from './AppNavHeader'
import { CollapsibleSection } from './CollapsibleSection'
import {
  APP_NAV_SECTIONS,
  activeAppSection,
  isSingleSection,
  resolveOpen,
  type AppNavItem,
  type AppNavSection,
} from './navModel'
import { usePersistedToggles } from './usePersistedToggles'

const APP_NAV_STORAGE_KEY = 'shell.nav.app.open'

function ItemLink({
  name,
  item,
  activeId,
  label,
  sub,
}: {
  name: string
  item: AppNavItem
  activeId: string | undefined
  label?: string
  sub?: boolean
}) {
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        render={<Link to={item.to} params={{ name }} />}
        isActive={activeId === item.id}
        tooltip={label ?? item.label}
        className={cn(sub && 'h-7 pl-6 text-[13px]')}
      >
        {item.icon}
        <span>{item.label}</span>
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}

function SectionRail({
  name,
  section,
  activeSectionId,
}: {
  name: string
  section: AppNavSection
  activeSectionId: string | undefined
}) {
  const first = section.items[0]
  if (!first) return null
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        render={<Link to={first.to} params={{ name }} />}
        isActive={activeSectionId === section.id}
        tooltip={`${section.label}: ${section.items.map((i) => i.label).join(', ')}`}
      >
        {section.icon}
        <span>{section.label}</span>
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}

export function AppNav({ name }: { name: string }) {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const { state } = useSidebar()
  const rail = state === 'collapsed'
  const [stored, setOpen] = usePersistedToggles(APP_NAV_STORAGE_KEY)
  const active = activeAppSection(pathname)

  return (
    <>
      <AppNavHeader name={name} />
      <SidebarGroup>
        <SidebarGroupContent>
          <SidebarMenu>
            {APP_NAV_SECTIONS.map((section) => {
              const only = section.items[0]
              if (only && isSingleSection(section) && !rail) {
                return (
                  <ItemLink
                    key={section.id}
                    name={name}
                    item={only}
                    activeId={active?.item.id}
                  />
                )
              }
              if (rail) {
                return (
                  <SectionRail
                    key={section.id}
                    name={name}
                    section={section}
                    activeSectionId={active?.section.id}
                  />
                )
              }
              const containsActive = active?.section.id === section.id
              return (
                <li key={section.id} className="list-none">
                  <CollapsibleSection
                    id={`app-${section.id}`}
                    open={resolveOpen(stored, section.id, containsActive)}
                    onOpenChange={(open) => setOpen(section.id, open)}
                    header={
                      <span
                        className={cn(
                          'flex items-center gap-2 text-[13px]',
                          containsActive && 'text-sidebar-foreground',
                        )}
                      >
                        <span className="[&_svg]:size-4">{section.icon}</span>
                        {section.label}
                      </span>
                    }
                  >
                    <SidebarMenu>
                      {section.items.map((item) => (
                        <ItemLink
                          key={item.id}
                          name={name}
                          item={item}
                          activeId={active?.item.id}
                          sub
                        />
                      ))}
                    </SidebarMenu>
                  </CollapsibleSection>
                </li>
              )
            })}
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>
    </>
  )
}
