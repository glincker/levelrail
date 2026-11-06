import { useEffect, useId } from 'react'
import { useTranslation } from 'react-i18next'
import {
  EyeGlassIcon,
  RocketGlassIcon,
  Share2GlassIcon,
  SlidersHorizontalGlassIcon,
} from '@thesvg/ui-react'
import { Link } from '@tanstack/react-router'
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from '@/components/ui/sidebar'
import { cn } from '@/lib/utils'
import { BetaBadge } from '../BetaBadge'
import {
  BETA_FEATURES_PATH,
  isFeatureLocked,
  lockedSearch,
} from '@/lib/experimental'
import { LockedFeatureMark } from './LockedFeatureMark'
import { AppNavHeader } from './AppNavHeader'
import { CollapsibleSection } from './CollapsibleSection'
import {
  isSingleSection,
  resolveOpen,
  type AppNavItem,
  type AppNavSection,
} from './navModel'
import { useAppTabs } from '../apps/useAppTabs'
import { CONFIG_MORE_SLUGS, type AppTabGroupId } from '@/lib/appTabGroups'
import { useExperimentalFeatures } from '@/hooks/useExperimental'
import { usePersistedToggles } from './usePersistedToggles'

const GROUP_ICONS: Record<AppTabGroupId, React.ReactNode> = {
  deploy: <RocketGlassIcon size="1.4em" />,
  traffic: <Share2GlassIcon size="1.4em" />,
  config: <SlidersHorizontalGlassIcon size="1.4em" />,
  observe: <EyeGlassIcon size="1.4em" />,
}

const APP_NAV_STORAGE_KEY = 'shell.nav.app.open'

function ItemLink({
  name,
  item,
  activeId,
  label,
  sub,
  experimental,
}: {
  name: string
  item: AppNavItem
  activeId: string | undefined
  label?: string
  sub?: boolean
  experimental: readonly string[]
}) {
  const describedBy = useId()
  if (isFeatureLocked(item.feature, experimental)) {
    return (
      <SidebarMenuItem>
        <SidebarMenuButton
          render={
            <Link
              to={BETA_FEATURES_PATH}
              search={lockedSearch(item.feature)}
              aria-describedby={describedBy}
            />
          }
          tooltip={label ?? item.label}
          title={label ?? item.label}
          data-locked="true"
          className={cn('text-muted-foreground', sub && 'h-7 pl-6 text-[13px]')}
        >
          {item.icon}
          <span>{item.label}</span>
          <LockedFeatureMark id={describedBy} />
        </SidebarMenuButton>
      </SidebarMenuItem>
    )
  }
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        render={<Link to={item.to} params={{ name }} />}
        isActive={activeId === item.id}
        tooltip={label ?? item.label}
        title={label ?? item.label}
        className={cn(sub && 'h-7 pl-6 text-[13px]')}
      >
        {item.icon}
        <span>{item.label}</span>
        {item.beta && <BetaBadge className="ml-auto" />}
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}

function SectionRail({
  name,
  section,
  activeSectionId,
  experimental,
}: {
  name: string
  section: AppNavSection
  activeSectionId: string | undefined
  experimental: readonly string[]
}) {
  const first =
    section.items.find((i) => !isFeatureLocked(i.feature, experimental)) ??
    section.items[0]
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
  const { state } = useSidebar()
  const rail = state === 'collapsed'
  const [stored, setOpen] = usePersistedToggles(APP_NAV_STORAGE_KEY)
  const { t } = useTranslation('common')
  const experimental = useExperimentalFeatures()
  const tabs = useAppTabs(name)
  // Overview is a single row above the sections, so it never sits in Deploy.
  const overview = tabs.groups
    .flatMap((g) => g.items)
    .find((i) => i.slug === 'overview')
  const sections: AppNavSection[] = tabs.groups
    .map((g) => ({
      id: g.id,
      label: t(`appTabs.groups.${g.id}`),
      icon: GROUP_ICONS[g.id],
      items: g.items.filter((i) => i.slug !== 'overview'),
    }))
    .filter((g) => g.items.length > 0)
  const activeSectionId = tabs.activeGroup
  const active = {
    section: { id: activeSectionId },
    item: { id: tabs.activeSlug },
  }

  // Persist the visited group as open so leaving it never collapses it and shifts the list.
  useEffect(() => {
    if (activeSectionId && stored[activeSectionId] !== true) {
      setOpen(activeSectionId, true)
    }
    // Only a route change into a group re-opens it; a manual collapse stays collapsed.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeSectionId])

  return (
    <>
      <AppNavHeader name={name} />
      <SidebarGroup>
        <SidebarGroupContent>
          <SidebarMenu>
            {overview ? (
              <ItemLink
                name={name}
                item={overview}
                activeId={active.item.id}
                label={t('appTabs.overview')}
                experimental={experimental}
              />
            ) : null}
            {sections.map((section) => {
              const only = section.items[0]
              if (only && isSingleSection(section) && !rail) {
                return (
                  <ItemLink
                    key={section.id}
                    name={name}
                    item={only}
                    activeId={active.item.id}
                    experimental={experimental}
                  />
                )
              }
              if (rail) {
                return (
                  <SectionRail
                    key={section.id}
                    name={name}
                    section={section}
                    activeSectionId={active.section.id}
                    experimental={experimental}
                  />
                )
              }
              const containsActive = active.section.id === section.id
              const isMore = (i: AppNavItem) =>
                section.id === 'config' && CONFIG_MORE_SLUGS.includes(i.slug)
              const main = section.items.filter((i) => !isMore(i))
              const more = section.items.filter(isMore)
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
                        <span className="[&_svg]:size-5">{section.icon}</span>
                        {section.label}
                      </span>
                    }
                  >
                    <SidebarMenu>
                      {main.map((item) => (
                        <ItemLink
                          key={item.id}
                          name={name}
                          item={item}
                          activeId={active.item.id}
                          sub
                          experimental={experimental}
                        />
                      ))}
                      {more.length > 0 ? (
                        <li className="list-none">
                          <CollapsibleSection
                            id={`app-${section.id}-more`}
                            open={resolveOpen(
                              stored,
                              `${section.id}-more`,
                              more.some((i) => i.id === active.item.id),
                            )}
                            onOpenChange={(open) =>
                              setOpen(`${section.id}-more`, open)
                            }
                            header={
                              <span className="flex items-center gap-2 pl-2 text-[13px] text-sidebar-foreground/70">
                                {t('appTabs.more')}
                              </span>
                            }
                          >
                            <SidebarMenu>
                              {more.map((item) => (
                                <ItemLink
                                  key={item.id}
                                  name={name}
                                  item={item}
                                  activeId={active.item.id}
                                  sub
                                  experimental={experimental}
                                />
                              ))}
                            </SidebarMenu>
                          </CollapsibleSection>
                        </li>
                      ) : null}
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
