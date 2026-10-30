import { useId, useMemo, useState } from 'react'
import { Link, useRouterState } from '@tanstack/react-router'
import {
  ArrowLeftGlassIcon,
  ChevronDownGlassIcon,
  SearchGlassIcon,
} from '@thesvg/ui-react'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from '@/components/ui/sidebar'
import { useTranslation } from 'react-i18next'
import {
  filterSettingsNav,
  useSettingsNav,
  type LabelledSettingsItem,
} from '../hooks/useSettingsNav'
import { BETA_FEATURES_PATH, lockedSearch } from '../lib/experimental'
import {
  SidebarUpgradeIcon,
  SidebarUpgradeLabel,
} from './settings/updates/SidebarUpgradeIndicator'
import { LockedFeatureMark } from './shell/LockedFeatureMark'
import { matchesPrefix } from './shell/navModel'

const OPEN_GROUPS_STORAGE_KEY = 'settings-sidebar-open-groups:v2'

function readStoredOpenGroups(): Record<string, boolean> {
  try {
    const raw = localStorage.getItem(OPEN_GROUPS_STORAGE_KEY)
    const parsed: unknown = raw ? JSON.parse(raw) : null
    return parsed && typeof parsed === 'object'
      ? (parsed as Record<string, boolean>)
      : {}
  } catch {
    return {}
  }
}

function writeStoredOpenGroups(groups: Record<string, boolean>) {
  try {
    localStorage.setItem(OPEN_GROUPS_STORAGE_KEY, JSON.stringify(groups))
  } catch {
    // Storage can be blocked; the in-memory state still works.
  }
}

/** Groups start closed; only the one holding the current page opens by default. */
function isGroupOpen(
  stored: Record<string, boolean>,
  section: { id: string; items: readonly { to: string }[] },
  pathname: string,
): boolean {
  const remembered = stored[section.id]
  if (remembered !== undefined) return remembered
  return section.items.some((item) => matchesPrefix(pathname, item.to))
}

function SettingsNavLink({
  item,
  pathname,
}: {
  item: LabelledSettingsItem
  pathname: string
}) {
  const describedBy = useId()
  if (item.locked && item.feature) {
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
          tooltip={item.title}
          data-locked="true"
          className="text-muted-foreground"
        >
          <item.icon />
          <span>{item.title}</span>
          <LockedFeatureMark id={describedBy} />
        </SidebarMenuButton>
      </SidebarMenuItem>
    )
  }
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        render={<Link to={item.to} />}
        isActive={matchesPrefix(pathname, item.to)}
        tooltip={item.title}
      >
        {item.id === 'updates' ? (
          <SidebarUpgradeIcon icon={item.icon} />
        ) : (
          <item.icon />
        )}
        <span>{item.title}</span>
        {item.id === 'updates' ? <SidebarUpgradeLabel /> : null}
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}

// Renders straight from settingsNavSections so this sidebar and the
// hub page (routes/settings/index.tsx) can never drift out of sync.
export function SettingsScopedSidebar() {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const { t } = useTranslation('settings')
  const sections = useSettingsNav()
  const [query, setQuery] = useState('')

  const [openGroups, setOpenGroups] =
    useState<Record<string, boolean>>(readStoredOpenGroups)

  function setGroupOpen(id: string, open: boolean) {
    setOpenGroups((prev) => {
      const next = { ...prev, [id]: open }
      writeStoredOpenGroups(next)
      return next
    })
  }

  const trimmedQuery = query.trim().toLowerCase()
  const isSearching = trimmedQuery.length > 0

  const visibleSections = useMemo(
    () => filterSettingsNav(sections, query),
    [sections, query],
  )

  return (
    <>
      <SidebarGroup>
        <SidebarGroupContent>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                render={<Link to="/" />}
                tooltip={t('nav.back')}
              >
                <ArrowLeftGlassIcon size="1.4em" />
                <span>{t('nav.back')}</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>

      <SidebarGroup className="py-0">
        <SidebarGroupContent>
          <div className="relative">
            <SearchGlassIcon
              className="pointer-events-none absolute top-1/2 left-2.5 size-5 -translate-y-1/2 text-muted-foreground"
              aria-hidden="true"
            />
            <Input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder={t('nav.search')}
              aria-label={t('nav.search')}
              className="h-8 pl-7"
            />
          </div>
        </SidebarGroupContent>
      </SidebarGroup>

      {visibleSections.map((section) => {
        // While searching, every matching group is forced open and the
        // trigger is disabled so a stray click doesn't collapse a group the
        // user will see restored once they clear the query.
        const isOpen = isSearching
          ? true
          : isGroupOpen(openGroups, section, pathname)

        return (
          <SidebarGroup key={section.id}>
            <Collapsible
              open={isOpen}
              disabled={isSearching}
              onOpenChange={(open) => setGroupOpen(section.id, open)}
            >
              <CollapsibleTrigger className="group/settings-section">
                <SidebarGroupLabel className="cursor-pointer justify-between">
                  <span>{section.title}</span>
                  <ChevronDownGlassIcon
                    className="size-5 shrink-0 text-sidebar-foreground/50 transition-transform group-data-panel-open/settings-section:rotate-180"
                    aria-hidden="true"
                  />
                </SidebarGroupLabel>
              </CollapsibleTrigger>
              <CollapsiblePanel>
                <SidebarGroupContent>
                  <SidebarMenu>
                    {section.items.map((item) => (
                      <SettingsNavLink
                        key={item.to}
                        item={item}
                        pathname={pathname}
                      />
                    ))}
                  </SidebarMenu>
                </SidebarGroupContent>
              </CollapsiblePanel>
            </Collapsible>
          </SidebarGroup>
        )
      })}
    </>
  )
}
