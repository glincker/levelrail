import { Link, useRouterState } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
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
  chordFor,
  isGlobalItemActive,
  linkTarget,
  resolveOpen,
  visibleGlobalFooter,
  visibleGlobalGroups,
  type GlobalNavItem,
} from './navModel'
import { useExperimentalFeatures } from '@/hooks/useExperimental'
import { useRouteAvailable } from '@/lib/routeAvailability'
import { ProjectsNavItem } from './ProjectsNavTree'
import { useNavCounts } from './useNavCounts'
import { usePersistedToggles } from './usePersistedToggles'

const GLOBAL_NAV_STORAGE_KEY = 'shell.nav.global.open'

const TRAFFIC_BADGE_KEYS = {
  'domains-attention': 'nav.domainsAttention',
  'dns-attention': 'nav.dnsAttention',
  'proxy-attention': 'nav.proxyAttention',
} as const

function NavLinkItem({
  item,
  pathname,
  counts,
}: {
  item: GlobalNavItem
  pathname: string
  counts: Record<string, number>
}) {
  const { t } = useTranslation('traffic')
  const chord = chordFor(item.to)
  const count = item.badge ? (counts[item.badge] ?? 0) : 0
  const trafficKey =
    item.badge && item.badge in TRAFFIC_BADGE_KEYS
      ? TRAFFIC_BADGE_KEYS[item.badge as keyof typeof TRAFFIC_BADGE_KEYS]
      : undefined
  const badgeLabel =
    trafficKey !== undefined
      ? t(trafficKey, { count })
      : item.badge === 'approvals'
        ? `${String(count)} pending approvals`
        : `${String(count)} failing apps`
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        render={<Link to={linkTarget(item)} />}
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
          label={badgeLabel}
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
  const experimental = useExperimentalFeatures()
  const routeAvailable = useRouteAvailable()

  return (
    <>
      <div className="flex flex-col gap-1 p-2">
        {visibleGlobalGroups(experimental, routeAvailable).map((group) => {
          const menu = (
            <SidebarMenu>
              {group.items.map((item) =>
                item.id === 'projects' ? (
                  <ProjectsNavItem
                    key={item.id}
                    item={item}
                    pathname={pathname}
                  />
                ) : (
                  <NavLinkItem
                    key={item.id}
                    item={item}
                    pathname={pathname}
                    counts={counts}
                  />
                ),
              )}
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
            {visibleGlobalFooter(experimental).map((item) => (
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
