import type { ComponentType, SVGProps } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ActivityGlassIcon,
  ChevronDownIcon,
  DatabaseGlassIcon,
  GlobeGlassIcon,
  RocketGlassIcon,
  SlidersHorizontalGlassIcon,
} from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import type { AppTabGroupId } from '@/lib/appTabGroups'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLinkItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { BetaBadge } from '../BetaBadge'
import { useAppTabs } from './useAppTabs'

type GroupIcon = ComponentType<SVGProps<SVGSVGElement> & { size?: number }>

const GROUP_ICONS: Record<AppTabGroupId, GroupIcon> = {
  basics: RocketGlassIcon,
  network: GlobeGlassIcon,
  data: DatabaseGlassIcon,
  observe: ActivityGlassIcon,
  advanced: SlidersHorizontalGlassIcon,
}

const FOCUS =
  'outline-none focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:ring-offset-0'
const HIDE_SCROLLBAR = '[scrollbar-width:none] [&::-webkit-scrollbar]:hidden'

/** One row: a section menu, then the pages of that section as tabs. */
export function AppTabStrip({ name }: { name: string }) {
  const { t } = useTranslation('common')
  const tabs = useAppTabs(name)
  const current = tabs.groups.find((g) => g.id === tabs.activeGroup)
  const label = (id: AppTabGroupId) => t(`appTabs.groups.${id}`)
  const CurrentIcon = current ? GROUP_ICONS[current.id] : null

  return (
    <div className="min-w-0" data-slot="app-tab-strip">
      <div className="flex min-w-0 items-center gap-2 border-b border-border">
        <DropdownMenu>
          <DropdownMenuTrigger
            aria-label={t('appTabs.sections')}
            className={cn(
              'my-1 flex shrink-0 items-center gap-1.5 rounded-md bg-muted px-2.5 py-1.5 text-sm font-medium text-foreground transition-colors hover:bg-muted/70',
              FOCUS,
            )}
          >
            {CurrentIcon ? <CurrentIcon size={16} aria-hidden="true" /> : null}
            {current ? label(current.id) : t('appTabs.sections')}
            <ChevronDownIcon
              size={14}
              className="text-muted-foreground"
              aria-hidden="true"
            />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" className="min-w-44">
            {tabs.groups.map((g) => {
              const first = g.items[0]
              if (!first) return null
              const Icon = GROUP_ICONS[g.id]
              return (
                <DropdownMenuLinkItem
                  key={g.id}
                  render={<Link to={first.to} params={{ name }} />}
                  className={cn(g.id === tabs.activeGroup && 'bg-muted')}
                >
                  <Icon size={16} aria-hidden="true" />
                  {label(g.id)}
                </DropdownMenuLinkItem>
              )
            })}
            {tabs.isStatic ? (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  onClick={() => {
                    tabs.setShowAll(!tabs.showAll)
                  }}
                >
                  {tabs.showAll
                    ? t('appTabs.showLess')
                    : t('appTabs.showAll', { count: tabs.hiddenCount })}
                </DropdownMenuItem>
              </>
            ) : null}
          </DropdownMenuContent>
        </DropdownMenu>
        {current ? (
          <nav
            aria-label={t('appTabs.pages', { section: label(current.id) })}
            className={cn(
              'flex min-w-0 gap-1 overflow-x-auto overflow-y-hidden overscroll-x-contain pb-px',
              HIDE_SCROLLBAR,
            )}
          >
            {current.items.map((item) => {
              const active = item.slug === tabs.activeSlug
              return (
                <Link
                  key={item.id}
                  to={item.to}
                  params={{ name }}
                  aria-current={active ? 'page' : undefined}
                  className={cn(
                    '-mb-px flex shrink-0 items-center gap-1 border-b-2 px-3 py-2 text-sm font-medium transition-colors',
                    FOCUS,
                    active
                      ? 'border-primary text-foreground'
                      : 'border-transparent text-muted-foreground hover:text-foreground',
                  )}
                >
                  {item.label}
                  {item.beta ? <BetaBadge /> : null}
                </Link>
              )
            })}
          </nav>
        ) : null}
      </div>
    </div>
  )
}
