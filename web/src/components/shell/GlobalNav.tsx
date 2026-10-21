import { useEffect, useId, useState } from 'react'
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
import {
  BETA_FEATURES_PATH,
  isFeatureLocked,
  lockedSearch,
  type ExperimentalFeature,
} from '@/lib/experimental'
import { LockedFeatureMark } from './LockedFeatureMark'
import { CollapsibleSection } from './CollapsibleSection'
import { NavBadge } from './NavBadge'
import {
  chordFor,
  defaultGroupOpen,
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
import { GetStartedNavItem } from './GetStartedNavItem'

const GLOBAL_NAV_STORAGE_KEY = 'shell.nav.global.open'

const TRAFFIC_BADGE_KEYS = {
  'domains-attention': 'nav.domainsAttention',
  'dns-attention': 'nav.dnsAttention',
  'proxy-attention': 'nav.proxyAttention',
} as const

function LockedNavLinkItem({
  item,
  feature,
}: {
  item: GlobalNavItem
  feature: ExperimentalFeature
}) {
  const { t: tc } = useTranslation()
  const describedBy = useId()
  const label = tc(`shell.nav.${item.id}`, { defaultValue: item.label })
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        render={
          <Link
            to={BETA_FEATURES_PATH}
            search={lockedSearch(feature)}
            aria-describedby={describedBy}
          />
        }
        tooltip={label}
        className="text-muted-foreground"
        data-locked="true"
      >
        {item.icon}
        <span>{label}</span>
        <LockedFeatureMark id={describedBy} />
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}

function NavLinkItem({
  item,
  pathname,
  counts,
  experimental,
}: {
  item: GlobalNavItem
  pathname: string
  counts: Record<string, number>
  experimental: readonly string[]
}) {
  const { t } = useTranslation('traffic')
  const { t: tc } = useTranslation()
  const label = tc(`shell.nav.${item.id}`, { defaultValue: item.label })
  if (isFeatureLocked(item.feature, experimental)) {
    return <LockedNavLinkItem item={item} feature={item.feature} />
  }
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
        : item.badge === 'inbox'
          ? `${String(count)} unread in inbox`
          : `${String(count)} failing apps`
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        render={<Link to={linkTarget(item)} />}
        isActive={isGlobalItemActive(pathname, item)}
        tooltip={{
          children: (
            <span className="flex items-center gap-2">
              {label}
              {chord ? <Kbd keys={chord} /> : null}
            </span>
          ),
        }}
      >
        {item.icon}
        <span>{label}</span>
        <NavBadge
          count={count}
          tone={
            item.badge === 'approvals' || item.badge === 'inbox'
              ? 'warning'
              : 'danger'
          }
          label={badgeLabel}
        />
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}

function groupAttention(
  group: { items: GlobalNavItem[] },
  counts: Record<string, number>,
) {
  const total = group.items.reduce(
    (n, i) => n + (i.badge ? (counts[i.badge] ?? 0) : 0),
    0,
  )
  return total > 0 ? (
    <NavBadge
      count={total}
      tone="danger"
      label={`${String(total)} need attention`}
    />
  ) : null
}

export function GlobalNav() {
  const { t } = useTranslation()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const { state } = useSidebar()
  const rail = state === 'collapsed'
  const [stored, setOpen] = usePersistedToggles(GLOBAL_NAV_STORAGE_KEY)
  const counts = useNavCounts()
  const experimental = useExperimentalFeatures()
  const routeAvailable = useRouteAvailable()
  const groups = visibleGlobalGroups(routeAvailable)

  // Decided once on mount so navigating never flips a group the user did not touch.
  const [initialOpen] = useState<Record<string, boolean>>(() =>
    Object.fromEntries(groups.map((gr) => [gr.id, defaultGroupOpen()])),
  )

  // Reveal the current page by opening its group, and keep it open.
  useEffect(() => {
    for (const gr of groups) {
      const active = gr.items.some((i) => isGlobalItemActive(pathname, i))
      if (active && stored[gr.id] !== true && !(initialOpen[gr.id] ?? false)) {
        setOpen(gr.id, true)
      }
    }
  }, [pathname, groups, stored, initialOpen, setOpen])

  return (
    <>
      <div className="flex flex-col gap-1 p-2">
        <GetStartedNavItem />
        {groups.map((group) => {
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
                    experimental={experimental}
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
              open={resolveOpen(
                stored,
                group.id,
                initialOpen[group.id] ?? true,
              )}
              onOpenChange={(open) => setOpen(group.id, open)}
              header={t(`shell.navGroup.${group.id}`, {
                defaultValue: group.label,
              })}
              hint={groupAttention(group, counts)}
            >
              {menu}
            </CollapsibleSection>
          )
        })}
      </div>
      <SidebarGroup className="mt-auto">
        <SidebarGroupContent>
          <SidebarMenu>
            {visibleGlobalFooter().map((item) => (
              <NavLinkItem
                key={item.id}
                item={item}
                pathname={pathname}
                counts={counts}
                experimental={experimental}
              />
            ))}
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>
    </>
  )
}
