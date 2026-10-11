import { useEffect, useMemo, useState } from 'react'
import { Link, useRouterState } from '@tanstack/react-router'
import {
  ArrowLeftIcon,
  CaretDownIcon,
  MagnifyingGlassIcon,
} from '@phosphor-icons/react/dist/ssr'
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
import { filterSettingsNav, useSettingsNav } from '../hooks/useSettingsNav'

const OPEN_GROUPS_STORAGE_KEY = 'settings-sidebar-open-groups'

function readStoredOpenGroups(): Record<string, boolean> | null {
  try {
    const raw = sessionStorage.getItem(OPEN_GROUPS_STORAGE_KEY)
    return raw ? (JSON.parse(raw) as Record<string, boolean>) : null
  } catch {
    return null
  }
}

// Renders straight from settingsNavSections so this sidebar and the
// hub page (routes/settings/index.tsx) can never drift out of sync.
export function SettingsScopedSidebar() {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const { t } = useTranslation('settings')
  const sections = useSettingsNav()
  const [query, setQuery] = useState('')

  const [openGroups, setOpenGroups] = useState<Record<string, boolean>>(() => {
    const stored = readStoredOpenGroups()
    if (stored) return stored
    // No stored preference yet: open every group by default so a first
    // visit doesn't require a click per heading just to see what's inside.
    return Object.fromEntries(sections.map((section) => [section.id, true]))
  })

  useEffect(() => {
    try {
      sessionStorage.setItem(
        OPEN_GROUPS_STORAGE_KEY,
        JSON.stringify(openGroups),
      )
    } catch {
      // sessionStorage can throw (private mode, quota); state still works.
    }
  }, [openGroups])

  function setGroupOpen(id: string, open: boolean) {
    setOpenGroups((prev) => ({ ...prev, [id]: open }))
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
                <ArrowLeftIcon />
                <span>{t('nav.back')}</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>

      <SidebarGroup className="py-0">
        <SidebarGroupContent>
          <div className="relative">
            <MagnifyingGlassIcon
              className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground"
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
        const isOpen = isSearching ? true : (openGroups[section.id] ?? false)

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
                  <CaretDownIcon
                    className="size-3.5 shrink-0 text-sidebar-foreground/50 transition-transform group-data-panel-open/settings-section:rotate-180"
                    aria-hidden="true"
                  />
                </SidebarGroupLabel>
              </CollapsibleTrigger>
              <CollapsiblePanel>
                <SidebarGroupContent>
                  <SidebarMenu>
                    {section.items.map((item) => (
                      <SidebarMenuItem key={item.to}>
                        <SidebarMenuButton
                          render={<Link to={item.to} />}
                          isActive={pathname === item.to}
                          tooltip={item.title}
                        >
                          <item.icon />
                          <span>{item.title}</span>
                        </SidebarMenuButton>
                      </SidebarMenuItem>
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
