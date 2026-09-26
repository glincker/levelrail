import { Link, useRouterState } from '@tanstack/react-router'
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from '@/components/ui/sidebar'
import { Kbd } from '@/components/kit'
import { CollapsibleSection } from './CollapsibleSection'
import { NavBadge } from './NavBadge'
import {
  GLOBAL_NAV_FOOTER,
  GLOBAL_NAV_GROUPS,
  chordFor,
  isGlobalItemActive,
  resolveOpen,
  type GlobalNavItem,
} from './navModel'
import { useNavCounts } from './useNavCounts'
import { usePersistedToggles } from './usePersistedToggles'

const GLOBAL_NAV_STORAGE_KEY = 'shell.nav.global.open'

function NavLinkItem({
  item,
  pathname,
  counts,
}: {
  item: GlobalNavItem
  pathname: string
  counts: Record<string, number>
}) {
  const chord = chordFor(item.to)
  const count = item.badge ? (counts[item.badge] ?? 0) : 0
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        render={<Link to={item.to} />}
        isActive={isGlobalItemActive(pathname, item)}
        tooltip={{
          children: (
            <span className="flex items-center gap-2">
              {item.label}
              {chord ? <Kbd keys={chord} /> : null}
            </span>
          ),
        }}
      >
        {item.icon}
        <span>{item.label}</span>
        <NavBadge
          count={count}
          tone={item.badge === 'approvals' ? 'warning' : 'danger'}
          label={
            item.badge === 'approvals'
              ? `${String(count)} pending approvals`
              : `${String(count)} failing apps`
          }
        />
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}

export function GlobalNav() {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const { state } = useSidebar()
  const rail = state === 'collapsed'
  const [stored, setOpen] = usePersistedToggles(GLOBAL_NAV_STORAGE_KEY)
  const counts = useNavCounts()

  return (
    <>
      <div className="flex flex-col gap-1 p-2">
        {GLOBAL_NAV_GROUPS.map((group) => {
          const menu = (
            <SidebarMenu>
              {group.items.map((item) => (
                <NavLinkItem
                  key={item.id}
                  item={item}
                  pathname={pathname}
                  counts={counts}
                />
              ))}
            </SidebarMenu>
          )
          if (rail) {
            return (
              <SidebarGroup key={group.id} className="p-0">
                <SidebarGroupContent>{menu}</SidebarGroupContent>
              </SidebarGroup>
            )
          }
          return (
            <CollapsibleSection
              key={group.id}
              id={`global-${group.id}`}
              open={resolveOpen(stored, group.id, true)}
              onOpenChange={(open) => setOpen(group.id, open)}
              header={group.label}
            >
              {menu}
            </CollapsibleSection>
          )
        })}
      </div>
      <SidebarGroup className="mt-auto">
        <SidebarGroupContent>
          <SidebarMenu>
            {GLOBAL_NAV_FOOTER.map((item) => (
              <NavLinkItem
                key={item.id}
                item={item}
                pathname={pathname}
                counts={counts}
              />
            ))}
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>
    </>
  )
}
