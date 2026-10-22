import { useRef, type KeyboardEvent } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { CaretDownIcon } from '@phosphor-icons/react/dist/ssr'
import { cn } from '@/lib/utils'
import { splitForNarrow, type AppTabGroupId } from '@/lib/appTabGroups'
import { useMediaQuery } from '@/hooks/useMediaQuery'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLinkItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { BetaBadge } from '../BetaBadge'
import { useAppTabs } from './useAppTabs'

const NARROW_QUERY = '(max-width: 639px)'
const TAB_BASE =
  'flex shrink-0 items-center gap-1 rounded-md px-3 py-1.5 text-sm font-medium outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring/60'

function nextIndex(key: string, i: number, n: number): number | null {
  if (key === 'ArrowRight') return (i + 1) % n
  if (key === 'ArrowLeft') return (i - 1 + n) % n
  if (key === 'Home') return 0
  if (key === 'End') return n - 1
  return null
}

/** Five section switcher plus the active section's pages, with a More menu on narrow screens. */
export function AppTabStrip({ name }: { name: string }) {
  const { t } = useTranslation('common')
  const narrow = useMediaQuery(NARROW_QUERY)
  const tabs = useAppTabs(name)
  const listRef = useRef<HTMLDivElement>(null)
  const { visible, more } = splitForNarrow(
    tabs.groups,
    tabs.activeGroup,
    narrow,
  )
  const current = tabs.groups.find((g) => g.id === tabs.activeGroup)
  const label = (id: AppTabGroupId) => t(`appTabs.groups.${id}`)

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    const nodes = Array.from(
      listRef.current?.querySelectorAll<HTMLElement>('[role="tab"]') ?? [],
    )
    const i = nodes.findIndex((n) => n === document.activeElement)
    const target = i === -1 ? null : nextIndex(e.key, i, nodes.length)
    if (target === null) return
    e.preventDefault()
    nodes[target]?.focus()
  }

  return (
    <div className="min-w-0 space-y-2" data-slot="app-tab-strip">
      <div className="flex min-w-0 items-center gap-1">
        <div
          ref={listRef}
          role="tablist"
          aria-label={t('appTabs.sections')}
          onKeyDown={onKeyDown}
          className="flex min-w-0 gap-1 overflow-x-auto"
        >
          {visible.map((g) => {
            const first = g.items[0]
            if (!first) return null
            const selected = g.id === tabs.activeGroup
            return (
              <Link
                key={g.id}
                to={first.to}
                params={{ name }}
                role="tab"
                id={`app-tab-${g.id}`}
                aria-selected={selected}
                aria-controls="app-tab-panel"
                tabIndex={selected || !tabs.activeGroup ? 0 : -1}
                className={cn(
                  TAB_BASE,
                  selected
                    ? 'bg-muted text-foreground'
                    : 'text-muted-foreground hover:text-foreground',
                )}
              >
                {label(g.id)}
              </Link>
            )
          })}
        </div>
        {more.length > 0 ? (
          <DropdownMenu>
            <DropdownMenuTrigger
              aria-label={t('appTabs.moreLabel')}
              className={cn(
                TAB_BASE,
                'text-muted-foreground hover:text-foreground',
              )}
            >
              {t('appTabs.more')}
              <CaretDownIcon className="size-3.5" aria-hidden="true" />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start">
              {more.map((g) => {
                const first = g.items[0]
                if (!first) return null
                return (
                  <DropdownMenuLinkItem
                    key={g.id}
                    render={<Link to={first.to} params={{ name }} />}
                  >
                    {label(g.id)}
                  </DropdownMenuLinkItem>
                )
              })}
            </DropdownMenuContent>
          </DropdownMenu>
        ) : null}
      </div>
      {current ? (
        <div
          id="app-tab-panel"
          role="tabpanel"
          aria-labelledby={`app-tab-${current.id}`}
        >
          <nav
            aria-label={t('appTabs.pages', { section: label(current.id) })}
            className="flex gap-1 overflow-x-auto border-b border-border pb-2"
          >
            {current.items.map((item) => (
              <Link
                key={item.id}
                to={item.to}
                params={{ name }}
                aria-current={
                  item.slug === tabs.activeSlug ? 'page' : undefined
                }
                className={cn(
                  TAB_BASE,
                  'text-[13px]',
                  item.slug === tabs.activeSlug
                    ? 'bg-primary/10 text-foreground'
                    : 'text-muted-foreground hover:text-foreground',
                )}
              >
                {item.label}
                {item.beta ? <BetaBadge /> : null}
              </Link>
            ))}
          </nav>
        </div>
      ) : null}
      {tabs.isStatic ? (
        <button
          type="button"
          onClick={() => {
            tabs.setShowAll(!tabs.showAll)
          }}
          className="text-xs text-muted-foreground underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:outline-none"
        >
          {tabs.showAll
            ? t('appTabs.showLess')
            : t('appTabs.showAll', { count: tabs.hiddenCount })}
        </button>
      ) : null}
    </div>
  )
}
