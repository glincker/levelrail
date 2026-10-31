import { Link } from '@tanstack/react-router'
import { cn } from '@/lib/utils'
import type { PlatformTabTo } from '@/lib/platformSettingsIndex'
import type { GlobalTo } from './navModel'

export interface SectionTabItem {
  to: GlobalTo | PlatformTabTo
  label: string
  active: boolean
}

/** SectionTabs links sibling routes under one sidebar entry. Each tab is a real route, so this is a nav, not a tablist. */
export function SectionTabs({
  ariaLabel,
  items,
}: {
  ariaLabel: string
  items: readonly SectionTabItem[]
}) {
  return (
    <nav
      aria-label={ariaLabel}
      data-slot="section-tabs"
      className="flex gap-1 overflow-x-auto"
    >
      {items.map((item) => (
        <Link
          key={item.to}
          to={item.to}
          aria-current={item.active ? 'page' : undefined}
          className={cn(
            '-mb-px flex shrink-0 items-center border-b-2 px-3 py-2 text-sm font-medium outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring/60',
            item.active
              ? 'border-foreground text-foreground'
              : 'border-transparent text-muted-foreground hover:text-foreground',
          )}
        >
          {item.label}
        </Link>
      ))}
    </nav>
  )
}
