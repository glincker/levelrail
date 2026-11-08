import { Fragment, useEffect, useId, useState, type KeyboardEvent } from 'react'
import { Link, useRouterState } from '@tanstack/react-router'
import {
  DatabaseGlassIcon,
  LayersGlassIcon,
  PinGlassIcon,
} from '@thesvg/ui-react'
import { applyNavPrefs, type RecentEntry } from '@/lib/navPrefs'
import { useNavPrefs, useNavRecent } from './useNavPrefs'
import { useTranslation } from 'react-i18next'
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarMenu,
  SidebarMenuAction,
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
import { BetaBadge } from '../BetaBadge'
import { CollapsibleSection, NAV_ROW_CLASS } from './CollapsibleSection'
import { NavBadge } from './NavBadge'
import { cn } from '@/lib/utils'
import {
  activeGlobalItemId,
  chordFor,
  defaultGroupOpen,
  isGlobalItemActive,
  isItemDisclosed,
  isSingleItemGroup,
  itemBadgeCount,
  linkTarget,
  pinnedGlobalItems,
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
import { useNavPins } from './useNavPins'

const GLOBAL_NAV_STORAGE_KEY = 'shell.nav.global.open'

interface PinControl {
  pinned: boolean
  full: boolean
  max: number
  onToggle: (id: string) => void
}

function PinButton({
  item,
  label,
  pin,
}: {
  item: GlobalNavItem
  label: string
  pin: PinControl
}) {
  const { t } = useTranslation()
  const blocked = pin.full && !pin.pinned
  return (
    <SidebarMenuAction
      showOnHover={!pin.pinned}
      className={
        pin.pinned
          ? 'peer-data-active/menu-button:text-sidebar-active-foreground'
          : undefined
      }
      tabIndex={-1}
      aria-pressed={pin.pinned}
      aria-label={t('shell.pins.pin', { label })}
      aria-disabled={blocked || undefined}
      title={blocked ? t('shell.pins.limit', { max: pin.max }) : undefined}
      onClick={() => {
        if (!blocked) pin.onToggle(item.id)
      }}
    >
      <PinGlassIcon size="1.4em" aria-hidden="true" />
    </SidebarMenuAction>
  )
}

// P on a focused row pins it, so the pin buttons can stay out of the tab order.
function pinKeyDown(item: GlobalNavItem, pin?: PinControl) {
  if (!pin) return undefined
  return (e: KeyboardEvent<HTMLLIElement>) => {
    if (e.key.toLowerCase() !== 'p' || e.ctrlKey || e.metaKey || e.altKey)
      return
    if (!(e.target instanceof HTMLAnchorElement)) return
    e.preventDefault()
    if (!(pin.full && !pin.pinned)) pin.onToggle(item.id)
  }
}

const TRAFFIC_BADGE_KEYS = {
  'domains-attention': 'nav.domainsAttention',
  'dns-attention': 'nav.dnsAttention',
  'proxy-attention': 'nav.proxyAttention',
} as const

function LockedNavLinkItem({
  item,
  feature,
  pin,
}: {
  item: GlobalNavItem
  feature: ExperimentalFeature
  pin?: PinControl
}) {
  const { t: tc } = useTranslation()
  const describedBy = useId()
  const label = tc(`shell.nav.${item.id}`, { defaultValue: item.label })
  return (
    <SidebarMenuItem onKeyDown={pinKeyDown(item, pin)}>
      <SidebarMenuButton
        render={
          <Link
            to={BETA_FEATURES_PATH}
            search={lockedSearch(feature)}
            aria-describedby={describedBy}
          />
        }
        tooltip={label}
        title={label}
        className={cn(NAV_ROW_CLASS, 'text-sidebar-foreground/75')}
        data-locked="true"
      >
        {item.icon}
        <span>{label}</span>
        <LockedFeatureMark id={describedBy} />
      </SidebarMenuButton>
      {pin ? <PinButton item={item} label={label} pin={pin} /> : null}
    </SidebarMenuItem>
  )
}

function NavLinkItem({
  item,
  groupName,
  active,
  counts,
  experimental,
  pin,
  beta,
}: {
  item: GlobalNavItem
  groupName?: string
  active: boolean
  counts: Record<string, number>
  experimental: readonly string[]
  pin?: PinControl
  beta?: boolean
}) {
  const { t } = useTranslation('traffic')
  const { t: tc } = useTranslation()
  const label = tc(`shell.nav.${item.id}`, { defaultValue: item.label })
  if (isFeatureLocked(item.feature, experimental)) {
    return <LockedNavLinkItem item={item} feature={item.feature} pin={pin} />
  }
  const chord = chordFor(item.to)
  const count = itemBadgeCount(item, counts)
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
    <SidebarMenuItem onKeyDown={pinKeyDown(item, pin)}>
      <SidebarMenuButton
        render={
          <Link
            to={linkTarget(item)}
            aria-keyshortcuts={pin ? 'P' : undefined}
          />
        }
        isActive={active}
        className={NAV_ROW_CLASS}
        title={label}
        tooltip={{
          children: (
            <span className="flex flex-col gap-0.5">
              <span className="flex items-center gap-2">
                {label}
                {beta ? <BetaBadge /> : null}
                {count > 0 ? (
                  <span className="tabular-nums">{count}</span>
                ) : null}
                {chord ? <Kbd keys={chord} /> : null}
              </span>
              {groupName ? (
                <span className="text-xs opacity-70">{groupName}</span>
              ) : null}
            </span>
          ),
        }}
      >
        {item.icon}
        <span>{label}</span>
        {beta ? (
          <BetaBadge className="shrink-0 text-sidebar-foreground/85 group-data-[collapsible=icon]:hidden" />
        ) : null}
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
      {pin ? <PinButton item={item} label={label} pin={pin} /> : null}
    </SidebarMenuItem>
  )
}

function RecentItem({
  entry,
  active,
}: {
  entry: RecentEntry
  active: boolean
}) {
  const { t } = useTranslation()
  const isApp = entry.kind === 'app'
  const Icon = isApp ? LayersGlassIcon : DatabaseGlassIcon
  const link = isApp ? (
    <Link to="/apps/$name" params={{ name: entry.name }} />
  ) : (
    <Link to="/databases/$name" params={{ name: entry.name }} />
  )
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        render={link}
        isActive={active}
        className={NAV_ROW_CLASS}
        title={entry.name}
      >
        <Icon
          size="1.4em"
          aria-label={t(isApp ? 'shell.recent.app' : 'shell.recent.database')}
          role="img"
        />
        <span>{entry.name}</span>
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}

function GroupAttention({
  group,
  counts,
}: {
  group: { items: GlobalNavItem[] }
  counts: Record<string, number>
}) {
  const { t } = useTranslation()
  const total = group.items.reduce((n, i) => n + itemBadgeCount(i, counts), 0)
  return total > 0 ? (
    <NavBadge
      count={total}
      tone="danger"
      label={t('shell.navAttention', { count: total })}
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
  const baseGroups = visibleGlobalGroups(routeAvailable)
  const { pins, toggle, full, max } = useNavPins()
  const { prefs } = useNavPrefs()
  const { recent } = useNavRecent()
  const activeId = activeGlobalItemId(pathname, [
    ...baseGroups.flatMap((gr) => gr.items),
    ...visibleGlobalFooter(),
  ])
  const groups = applyNavPrefs(baseGroups, prefs, activeId)
  const pinned = pinnedGlobalItems(groups, pins)
  const pinFor = (id: string): PinControl => ({
    pinned: pins.includes(id),
    full,
    max,
    onToggle: toggle,
  })

  // Decided once on mount so navigating never flips a group the user did not touch.
  const [initialOpen] = useState<Record<string, boolean>>(() =>
    Object.fromEntries(
      groups.map((gr) => [
        gr.id,
        defaultGroupOpen(
          window.innerHeight,
          gr.items.some((i) => isGlobalItemActive(pathname, i)),
        ),
      ]),
    ),
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
    <div className="flex max-h-full min-h-0 flex-col">
      <div
        data-sidebar-scroll=""
        className="nav-scroll flex min-h-0 flex-col gap-0 overflow-x-hidden overflow-y-auto p-2 pt-0 group-data-[collapsible=icon]:items-center"
      >
        <GetStartedNavItem />
        {!rail && pinned.length > 0 ? (
          <CollapsibleSection
            id="global-pinned"
            open={resolveOpen(stored, 'pinned', true)}
            onOpenChange={(open) => setOpen('pinned', open)}
            header={t('shell.pins.group')}
          >
            <SidebarMenu>
              {pinned.map((item) => (
                <NavLinkItem
                  key={item.id}
                  item={item}
                  active={item.id === activeId}
                  counts={counts}
                  experimental={experimental}
                  pin={pinFor(item.id)}
                />
              ))}
            </SidebarMenu>
          </CollapsibleSection>
        ) : null}
        {!rail && recent.length > 0 ? (
          <CollapsibleSection
            id="global-recent"
            open={resolveOpen(stored, 'recent', false)}
            onOpenChange={(open) => setOpen('recent', open)}
            header={t('shell.recent.group')}
          >
            <SidebarMenu>
              {recent.map((entry) => (
                <RecentItem
                  key={`${entry.kind}:${entry.name}`}
                  entry={entry}
                  active={pathname.startsWith(
                    `/${entry.kind === 'app' ? 'apps' : 'databases'}/${encodeURIComponent(entry.name)}`,
                  )}
                />
              ))}
            </SidebarMenu>
          </CollapsibleSection>
        ) : null}
        {rail && pinned.length > 0 ? (
          <SidebarGroup className="p-0">
            <SidebarGroupContent>
              <SidebarMenu>
                {pinned.map((item) => (
                  <NavLinkItem
                    key={item.id}
                    item={item}
                    active={item.id === activeId}
                    counts={counts}
                    experimental={experimental}
                  />
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        ) : null}
        {groups.map((group, index) => {
          const groupName = t(`shell.navGroup.${group.id}`, {
            defaultValue: group.label,
          })
          const menu = (
            <SidebarMenu>
              {group.items
                .filter((item) => isItemDisclosed(item, counts, pathname))
                .filter(
                  (item) => !(rail && pinned.some((p) => p.id === item.id)),
                )
                .map((item) => (
                  <Fragment key={item.id}>
                    {item.id === 'projects' ? (
                      <ProjectsNavItem
                        item={item}
                        pathname={pathname}
                        active={item.id === activeId}
                      />
                    ) : (
                      <NavLinkItem
                        item={item}
                        groupName={group.ungrouped ? undefined : groupName}
                        active={item.id === activeId}
                        counts={counts}
                        experimental={experimental}
                        pin={pinFor(item.id)}
                      />
                    )}
                  </Fragment>
                ))}
            </SidebarMenu>
          )
          if (rail || group.ungrouped || isSingleItemGroup(group)) {
            return (
              <SidebarGroup
                key={group.id}
                className={cn(
                  'p-0',
                  rail &&
                    index > 0 &&
                    'mt-2 border-t border-sidebar-border pt-2',
                )}
              >
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
              active={group.items.some((i) => isGlobalItemActive(pathname, i))}
              hint={<GroupAttention group={group} counts={counts} />}
            >
              {menu}
            </CollapsibleSection>
          )
        })}
      </div>
      {/* The group list above scrolls on its own; this block never shrinks or clips. */}
      <SidebarGroup className="shrink-0 px-2 pb-1 pt-1">
        <SidebarGroupContent className="border-t border-sidebar-border pt-1">
          <SidebarMenu>
            {visibleGlobalFooter().map((item) => (
              <NavLinkItem
                key={item.id}
                item={item}
                active={item.id === activeId}
                counts={counts}
                experimental={experimental}
                beta={item.feature !== undefined}
              />
            ))}
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>
    </div>
  )
}
