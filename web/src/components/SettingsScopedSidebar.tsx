import { useCallback, useId, useMemo, useState } from 'react'
import { Link, useRouterState } from '@tanstack/react-router'
import {
  ArrowLeftGlassIcon,
  ChevronRightGlassIcon,
  PinGlassIcon,
  SearchGlassIcon,
} from '@thesvg/ui-react'
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
import { cn } from '@/lib/utils'

const PINS_STORAGE_KEY = 'settings-sidebar-pins:v1'

function readStoredPins(): string[] {
  try {
    return parsePins(localStorage.getItem(PINS_STORAGE_KEY))
  } catch {
    return []
  }
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

// Renders straight from settingsNavSections so this sidebar and the
// hub page (routes/settings/index.tsx) can never drift out of sync.
export function SettingsScopedSidebar() {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const { t } = useTranslation('settings')
  const sections = useSettingsNav()
  const [query, setQuery] = useState('')

  // The group holding the current page starts open; a click overrides it.
  const [toggled, setToggled] = useState<Record<string, boolean>>({})

  const [pins, setPins] = useState<string[]>(readStoredPins)
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

  const trimmedQuery = query.trim().toLowerCase()
  const isSearching = trimmedQuery.length > 0

  const visibleSections = useMemo(
    () => filterSettingsNav(sections, query),
    [sections, query],
  )

  const pathArea =
    sections.find((section) =>
      section.items.some((item) => matchesPrefix(pathname, item.to)),
    )?.id ?? null
  const isOpen = (id: string) => toggled[id] ?? id === pathArea

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

  const searchResults = visibleSections.flatMap((section) => section.items)

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
          <div className="relative min-w-0">
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
        </SidebarGroupContent>
      </SidebarGroup>

      {isSearching ? (
        <SidebarGroup className="py-1">
          <SidebarGroupContent>
            <SidebarMenu>
              {searchResults.map((item) => (
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
      ) : (
        <>
          {pinnedItems.length > 0 ? (
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
          <SidebarGroup className="py-1">
            <SidebarGroupContent>
              <SidebarMenu>
                {sections.map((section) => {
                  const open = isOpen(section.id)
                  const listId = `settings-group-${section.id}`
                  return (
                    <SidebarMenuItem key={section.id}>
                      <SidebarMenuButton
                        aria-expanded={open}
                        aria-controls={listId}
                        onClick={() => {
                          setToggled((prev) => ({
                            ...prev,
                            [section.id]: !open,
                          }))
                        }}
                        tooltip={section.title}
                        className={cn(open && 'font-medium')}
                      >
                        <ChevronRightGlassIcon
                          className={cn(
                            'size-5 shrink-0 text-sidebar-foreground/50 transition-transform motion-reduce:transition-none',
                            open && 'rotate-90',
                          )}
                          aria-hidden="true"
                        />
                        <span className="truncate" title={section.title}>
                          {section.title}
                        </span>
                        <span
                          className="ml-auto text-xs text-sidebar-foreground/45 tabular-nums"
                          title={t('nav.groupCount', {
                            count: section.items.length,
                          })}
                        >
                          {section.items.length}
                        </span>
                      </SidebarMenuButton>
                      {open ? (
                        <SidebarMenu
                          id={listId}
                          className="mt-0.5 mb-1 ml-4 border-l pl-1"
                        >
                          {section.items.map((item) => (
                            <SettingsNavLink
                              key={item.to}
                              item={item}
                              pathname={pathname}
                              pin={pinFor(item.to)}
                            />
                          ))}
                        </SidebarMenu>
                      ) : null}
                    </SidebarMenuItem>
                  )
                })}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </>
      )}
    </>
  )
}
