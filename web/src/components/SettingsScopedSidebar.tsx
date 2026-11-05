import { useCallback, useId, useMemo, useState } from 'react'
import { Link, useRouterState } from '@tanstack/react-router'
import {
  ArrowLeftGlassIcon,
  ChevronDownGlassIcon,
  FoldVerticalGlassIcon,
  PinGlassIcon,
  SearchGlassIcon,
  SlidersHorizontalGlassIcon,
  UnfoldVerticalGlassIcon,
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
  SidebarMenuAction,
  SidebarMenuButton,
  SidebarMenuItem,
} from '@/components/ui/sidebar'
import { useTranslation } from 'react-i18next'
import {
  filterSettingsNav,
  useSettingsNav,
  type LabelledSettingsItem,
} from '../hooks/useSettingsNav'
import { maxNavPins, parsePins, togglePin } from '../lib/navPins'
import { BETA_FEATURES_PATH, lockedSearch } from '../lib/experimental'
import {
  SidebarUpgradeIcon,
  SidebarUpgradeLabel,
} from './settings/updates/SidebarUpgradeIndicator'
import { LockedFeatureMark } from './shell/LockedFeatureMark'
import { matchesPrefix } from './shell/navModel'
import { isBoolRecord, readJson, writeJson } from './shell/persist'
import { cn } from '@/lib/utils'

const OPEN_GROUPS_STORAGE_KEY = 'settings-sidebar-open-groups:v2'

const PINS_STORAGE_KEY = 'settings-sidebar-pins:v1'
const ONE_AT_A_TIME_STORAGE_KEY = 'settings-sidebar-one-at-a-time:v1'

function readStoredOpenGroups(): Record<string, boolean> {
  return readJson(OPEN_GROUPS_STORAGE_KEY, {}, isBoolRecord)
}

function readStoredPins(): string[] {
  try {
    return parsePins(localStorage.getItem(PINS_STORAGE_KEY))
  } catch {
    return []
  }
}

function readStoredOneAtATime(): boolean {
  return readJson(
    ONE_AT_A_TIME_STORAGE_KEY,
    false,
    (v): v is boolean => typeof v === 'boolean',
  )
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

interface PinControl {
  pinned: boolean
  full: boolean
  max: number
  onToggle: (id: string) => void
}

function PinButton({
  item,
  pin,
}: {
  item: LabelledSettingsItem
  pin: PinControl
}) {
  const { t } = useTranslation('settings')
  const blocked = pin.full && !pin.pinned
  const label = t(pin.pinned ? 'nav.pins.unpin' : 'nav.pins.pin', {
    label: item.title,
  })
  return (
    <SidebarMenuAction
      showOnHover={!pin.pinned}
      className={
        pin.pinned
          ? 'peer-data-active/menu-button:text-sidebar-active-foreground'
          : undefined
      }
      aria-pressed={pin.pinned}
      aria-label={label}
      aria-disabled={blocked || undefined}
      title={blocked ? t('nav.pins.limit', { max: pin.max }) : label}
      onClick={() => {
        if (!blocked) pin.onToggle(item.to)
      }}
    >
      <PinGlassIcon size="1.4em" aria-hidden="true" />
    </SidebarMenuAction>
  )
}

// Reserve room for the pin button on hover/focus so labels do not sit under it.
const ROW_PAD = 'group-focus-within/menu-item:pr-7 group-hover/menu-item:pr-7'

function SettingsNavLink({
  item,
  pathname,
  pin,
}: {
  item: LabelledSettingsItem
  pathname: string
  pin: PinControl
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
          className={cn('text-muted-foreground', ROW_PAD)}
        >
          <item.icon />
          <span title={item.title}>{item.title}</span>
          <LockedFeatureMark id={describedBy} />
        </SidebarMenuButton>
        <PinButton item={item} pin={pin} />
      </SidebarMenuItem>
    )
  }
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        render={<Link to={item.to} />}
        isActive={matchesPrefix(pathname, item.to)}
        tooltip={item.title}
        className={ROW_PAD}
      >
        {item.id === 'updates' ? (
          <SidebarUpgradeIcon icon={item.icon} />
        ) : (
          <item.icon />
        )}
        <span title={item.title}>{item.title}</span>
        {item.id === 'updates' ? <SidebarUpgradeLabel /> : null}
      </SidebarMenuButton>
      <PinButton item={item} pin={pin} />
    </SidebarMenuItem>
  )
}

const TOOL_BUTTON =
  'flex size-8 shrink-0 items-center justify-center rounded-md text-sidebar-foreground/70 ring-sidebar-ring outline-hidden transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:ring-2 aria-pressed:bg-sidebar-accent aria-pressed:text-sidebar-accent-foreground'

// Renders straight from settingsNavSections so this sidebar and the
// hub page (routes/settings/index.tsx) can never drift out of sync.
export function SettingsScopedSidebar() {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const { t } = useTranslation('settings')
  const sections = useSettingsNav()
  const [query, setQuery] = useState('')

  const [openGroups, setOpenGroups] =
    useState<Record<string, boolean>>(readStoredOpenGroups)

  const [pins, setPins] = useState<string[]>(readStoredPins)
  const [oneAtATime, setOneAtATime] = useState<boolean>(readStoredOneAtATime)
  const maxPins = maxNavPins()

  const togglePinned = useCallback((id: string) => {
    setPins((prev) => {
      const next = togglePin(prev, id)
      try {
        localStorage.setItem(PINS_STORAGE_KEY, JSON.stringify(next))
      } catch {
        // Storage can be blocked; pins last for this session only.
      }
      return next
    })
  }, [])

  function toggleOneAtATime() {
    const next = !oneAtATime
    setOneAtATime(next)
    writeJson(ONE_AT_A_TIME_STORAGE_KEY, next)
  }

  // Every group id is written so remembered state matches what the user sees.
  function applyOpenGroups(next: Record<string, boolean>) {
    setOpenGroups(next)
    writeJson(OPEN_GROUPS_STORAGE_KEY, next)
  }

  function setGroupOpen(id: string, open: boolean) {
    if (open && oneAtATime) {
      applyOpenGroups(
        Object.fromEntries(sections.map((s) => [s.id, s.id === id])),
      )
      return
    }
    applyOpenGroups({ ...openGroups, [id]: open })
  }

  function setAllOpen(open: boolean) {
    applyOpenGroups(Object.fromEntries(sections.map((s) => [s.id, open])))
  }

  const trimmedQuery = query.trim().toLowerCase()
  const isSearching = trimmedQuery.length > 0

  const visibleSections = useMemo(
    () => filterSettingsNav(sections, query),
    [sections, query],
  )

  const pinFor = (id: string): PinControl => ({
    pinned: pins.includes(id),
    full: pins.length >= maxPins,
    max: maxPins,
    onToggle: togglePinned,
  })

  // Pins resolve against what exists now; ids for removed pages are skipped.
  const pinnedItems = useMemo(() => {
    const byTo = new Map(
      sections.flatMap((section) => section.items).map((i) => [i.to, i]),
    )
    return pins.flatMap((id) => {
      const item = byTo.get(id)
      return item ? [item] : []
    })
  }, [pins, sections])

  const allOpen =
    visibleSections.length > 0 &&
    visibleSections.every((section) =>
      isGroupOpen(openGroups, section, pathname),
    )

  return (
    <>
      <SidebarGroup className="pb-1">
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
          <div className="flex items-center gap-1">
            <div className="relative min-w-0 flex-1">
              <SearchGlassIcon
                className="pointer-events-none absolute top-1/2 left-2.5 size-5 -translate-y-1/2 text-muted-foreground"
                aria-hidden="true"
              />
              <Input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder={t('nav.search')}
                aria-label={t('nav.search')}
                className="h-8 pl-9"
              />
            </div>
            <button
              type="button"
              className={TOOL_BUTTON}
              disabled={isSearching}
              aria-label={t(allOpen ? 'nav.collapseAll' : 'nav.expandAll')}
              title={t(allOpen ? 'nav.collapseAll' : 'nav.expandAll')}
              onClick={() => setAllOpen(!allOpen)}
            >
              {allOpen ? (
                <FoldVerticalGlassIcon size="1.4em" aria-hidden="true" />
              ) : (
                <UnfoldVerticalGlassIcon size="1.4em" aria-hidden="true" />
              )}
            </button>
            <button
              type="button"
              className={TOOL_BUTTON}
              aria-pressed={oneAtATime}
              aria-label={t('nav.oneAtATime')}
              title={t('nav.oneAtATime')}
              onClick={toggleOneAtATime}
            >
              <SlidersHorizontalGlassIcon size="1.4em" aria-hidden="true" />
            </button>
          </div>
        </SidebarGroupContent>
      </SidebarGroup>

      {!isSearching && pinnedItems.length > 0 ? (
        <SidebarGroup className="py-1">
          <SidebarGroupLabel>{t('nav.pins.group')}</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {pinnedItems.map((item) => (
                <SettingsNavLink
                  key={item.to}
                  item={item}
                  pathname={pathname}
                  pin={pinFor(item.to)}
                />
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      ) : null}

      {visibleSections.map((section) => {
        // While searching, every matching group is forced open and the
        // trigger is disabled so a stray click doesn't collapse a group the
        // user will see restored once they clear the query.
        const isOpen = isSearching
          ? true
          : isGroupOpen(openGroups, section, pathname)

        return (
          <SidebarGroup key={section.id} className="py-0.5">
            <Collapsible
              open={isOpen}
              disabled={isSearching}
              onOpenChange={(open) => setGroupOpen(section.id, open)}
            >
              <CollapsibleTrigger className="group/settings-section">
                <SidebarGroupLabel className="h-8 cursor-pointer justify-between">
                  <span className="flex min-w-0 items-baseline gap-1.5">
                    <span className="truncate" title={section.title}>
                      {section.title}
                    </span>
                    <span
                      className="text-xs text-sidebar-foreground/45 tabular-nums"
                      title={t('nav.groupCount', {
                        count: section.items.length,
                      })}
                    >
                      {section.items.length}
                    </span>
                  </span>
                  <ChevronDownGlassIcon
                    className="size-5 shrink-0 text-sidebar-foreground/50 transition-transform group-data-panel-open/settings-section:rotate-180"
                    aria-hidden="true"
                  />
                </SidebarGroupLabel>
              </CollapsibleTrigger>
              <CollapsiblePanel className="mb-0 pb-1">
                <SidebarGroupContent>
                  <SidebarMenu>
                    {section.items.map((item) => (
                      <SettingsNavLink
                        key={item.to}
                        item={item}
                        pathname={pathname}
                        pin={pinFor(item.to)}
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
