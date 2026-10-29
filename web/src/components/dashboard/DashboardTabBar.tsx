import { useRef, type KeyboardEvent, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDownGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'
import { splitTabs, START_TAB } from '@/lib/dashboardTabs'

export interface TabBarProps {
  shown: string[]
  hidden: string[]
  selected: string
  labelOf: (id: string) => string
  onSelect: (id: string) => void
  /** The Manage menu, rendered at the end of the bar. */
  manage?: ReactNode
  onHide: (id: string) => void
  onShow: (id: string) => void
  onMove: (id: string, to: number) => void
}

/** The view tabs above the dashboard: arrow keys move focus, Shift+arrows reorder, Delete hides. */
export function DashboardTabBar({
  shown,
  hidden,
  selected,
  labelOf,
  onSelect,
  manage,
  onHide,
  onShow,
  onMove,
}: TabBarProps) {
  const { t } = useTranslation('dashboard')
  const refs = useRef(new Map<string, HTMLButtonElement>())
  const { visible, overflow } = splitTabs(shown, selected)

  function focusTab(id: string | undefined) {
    if (!id) return
    onSelect(id)
    refs.current.get(id)?.focus()
  }

  function onKeyDown(e: KeyboardEvent<HTMLButtonElement>, id: string) {
    const i = visible.indexOf(id)
    if (e.shiftKey && (e.key === 'ArrowLeft' || e.key === 'ArrowRight')) {
      e.preventDefault()
      onMove(id, shown.indexOf(id) + (e.key === 'ArrowLeft' ? -1 : 1))
      return
    }
    if (e.key === 'ArrowRight') {
      e.preventDefault()
      focusTab(visible[(i + 1) % visible.length])
    } else if (e.key === 'ArrowLeft') {
      e.preventDefault()
      focusTab(visible[(i - 1 + visible.length) % visible.length])
    } else if (e.key === 'Home') {
      e.preventDefault()
      focusTab(visible[0])
    } else if (e.key === 'End') {
      e.preventDefault()
      focusTab(visible[visible.length - 1])
    } else if (e.key === 'Delete' && id !== START_TAB) {
      e.preventDefault()
      onHide(id)
    }
  }

  return (
    <div className="flex items-center gap-1 border-b border-border">
      <div
        role="tablist"
        aria-label={t('tabs.label')}
        className="flex min-w-0 items-center gap-1 overflow-x-auto overflow-y-hidden overscroll-x-contain pb-px [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
      >
        {visible.map((id) => {
          const active = id === selected
          return (
            <button
              key={id}
              ref={(el) => {
                if (el) refs.current.set(id, el)
                else refs.current.delete(id)
              }}
              type="button"
              role="tab"
              id={`dashboard-tab-${id}`}
              aria-selected={active}
              aria-controls="dashboard-tabpanel"
              tabIndex={active ? 0 : -1}
              onClick={() => {
                onSelect(id)
              }}
              onKeyDown={(e) => {
                onKeyDown(e, id)
              }}
              className={cn(
                '-mb-px max-w-48 truncate border-b-2 px-3 py-2 text-sm font-medium transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring',
                active
                  ? 'border-primary text-foreground'
                  : 'border-transparent text-muted-foreground hover:text-foreground',
              )}
            >
              {labelOf(id)}
            </button>
          )
        })}
      </div>
      {overflow.length + hidden.length > 0 ? (
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                type="button"
                variant="ghost"
                size="sm"
                aria-label={t('tabs.more')}
              />
            }
          >
            <ChevronDownGlassIcon size="1.2em" aria-hidden="true" />
          </DropdownMenuTrigger>
          <DropdownMenuContent>
            {overflow.map((id) => (
              <DropdownMenuItem
                key={id}
                onClick={() => {
                  onSelect(id)
                }}
              >
                {labelOf(id)}
              </DropdownMenuItem>
            ))}
            {overflow.length > 0 && hidden.length > 0 ? (
              <DropdownMenuSeparator />
            ) : null}
            {hidden.map((id) => (
              <DropdownMenuItem
                key={id}
                onClick={() => {
                  onShow(id)
                  onSelect(id)
                }}
              >
                {t('tabs.hiddenSuffix', { name: labelOf(id) })}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      ) : null}
      <div className="ml-auto shrink-0">{manage}</div>
    </div>
  )
}
