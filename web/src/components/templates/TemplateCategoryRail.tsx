import { useTranslation } from 'react-i18next'
import { LayoutGridGlassIcon } from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import { ALL_CATEGORIES } from '../../lib/templateCatalogView'
import { categoryStyle } from '../../lib/templateCategoryStyle'

/** TemplateCategoryRail is the md+ vertical category filter with tinted icons and counts. */
export function TemplateCategoryRail({
  categoryCounts,
  total,
  active,
  onChange,
}: {
  categoryCounts: [string, number][]
  total: number
  active: string
  onChange: (category: string) => void
}) {
  const { t } = useTranslation('templates')
  const items: [string, string, number, typeof LayoutGridGlassIcon, string][] =
    [
      [
        ALL_CATEGORIES,
        t('marketplace.categories.all'),
        total,
        LayoutGridGlassIcon,
        'text-foreground/70',
      ],
      ...categoryCounts.map(([name, count]): (typeof items)[number] => {
        const style = categoryStyle(name)
        return [name, name, count, style.icon, style.toneClass]
      }),
    ]
  return (
    <nav
      aria-label={t('marketplace.categories.label')}
      className="hidden md:block"
    >
      <p className="px-2 pb-2 text-xs font-medium tracking-wide text-foreground/70 uppercase">
        {t('marketplace.categories.heading')}
      </p>
      <ul className="space-y-0.5">
        {items.map(([key, label, count, ItemIcon, tone]) => (
          <li key={key}>
            <button
              type="button"
              aria-pressed={active === key}
              onClick={() => {
                onChange(key)
              }}
              className={cn(
                'flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50',
                active === key
                  ? 'bg-primary/10 font-medium text-foreground'
                  : 'text-foreground/70 hover:bg-muted hover:text-foreground',
              )}
            >
              <ItemIcon
                className={cn('size-4 shrink-0', tone)}
                aria-hidden="true"
              />
              <span className="min-w-0 flex-1 truncate">{label}</span>
              <span className="text-xs tabular-nums text-foreground/60">
                {count}
              </span>
            </button>
          </li>
        ))}
      </ul>
    </nav>
  )
}
