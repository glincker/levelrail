import { useTranslation } from 'react-i18next'
import { PackageIcon, SquaresFourIcon } from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import { CATEGORY_ICONS } from './ServiceTemplateGrid'
import { cn } from '@/lib/utils'

// Sentinel for "no category filter applied", distinct from any real
// catalog Category value so it can never collide with one.
export const ALL_CATEGORIES = 'All'

export type CategoryCount = [string, number]

interface RailProps {
  categoryCounts: CategoryCount[]
  total: number
  active: string
  onChange: (category: string) => void
}

function CategoryItem({
  icon: ItemIcon,
  label,
  count,
  active,
  onClick,
}: {
  icon: Icon
  label: string
  count: number
  active: boolean
  onClick: () => void
}) {
  return (
    <li>
      <button
        type="button"
        onClick={onClick}
        aria-pressed={active}
        className={cn(
          'flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50',
          active
            ? 'bg-primary/10 font-medium text-primary'
            : 'text-muted-foreground hover:bg-muted hover:text-foreground',
        )}
      >
        <ItemIcon className="size-4 shrink-0" aria-hidden="true" />
        <span className="min-w-0 flex-1 truncate">{label}</span>
        <span className="text-xs tabular-nums text-muted-foreground">
          {count}
        </span>
      </button>
    </li>
  )
}

// Desktop-only vertical filter rail. Hidden below md, where
// CategoryChips takes over, so one active/onChange state drives both.
export function CategorySidebar({
  categoryCounts,
  total,
  active,
  onChange,
}: RailProps) {
  const { t } = useTranslation('compose')
  return (
    <nav
      aria-label={t('catalog.categoriesLabel')}
      className="hidden md:sticky md:top-4 md:block md:self-start"
    >
      <p className="px-2 pb-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
        {t('catalog.categories')}
      </p>
      <ul className="space-y-0.5">
        <CategoryItem
          icon={SquaresFourIcon}
          label={t('catalog.allTemplates')}
          count={total}
          active={active === ALL_CATEGORIES}
          onClick={() => {
            onChange(ALL_CATEGORIES)
          }}
        />
        {categoryCounts.map(([category, count]) => (
          <CategoryItem
            key={category}
            icon={CATEGORY_ICONS[category] ?? PackageIcon}
            label={category}
            count={count}
            active={active === category}
            onClick={() => {
              onChange(category)
            }}
          />
        ))}
      </ul>
    </nav>
  )
}

function CategoryChip({
  label,
  active,
  onClick,
}: {
  label: string
  active: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={cn(
        'shrink-0 rounded-full border px-3 py-1 text-xs font-medium whitespace-nowrap transition-colors focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50',
        active
          ? 'border-primary bg-primary/10 text-primary'
          : 'border-border bg-card text-muted-foreground hover:text-foreground',
      )}
    >
      {label}
    </button>
  )
}

// Narrow-viewport equivalent of CategorySidebar: a horizontally
// scrollable chip row instead of a column.
export function CategoryChips({
  categoryCounts,
  total,
  active,
  onChange,
}: RailProps) {
  const { t } = useTranslation('compose')
  return (
    <div
      role="group"
      aria-label={t('catalog.filterByCategory')}
      className="flex gap-2 overflow-x-auto pb-1 md:hidden"
    >
      <CategoryChip
        label={t('catalog.allChip', { count: total })}
        active={active === ALL_CATEGORIES}
        onClick={() => {
          onChange(ALL_CATEGORIES)
        }}
      />
      {categoryCounts.map(([category, count]) => (
        <CategoryChip
          key={category}
          label={`${category} (${count})`}
          active={active === category}
          onClick={() => {
            onChange(category)
          }}
        />
      ))}
    </div>
  )
}
