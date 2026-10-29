import { useTranslation } from 'react-i18next'
import {
  BadgeCheckGlassIcon,
  LayoutGridGlassIcon,
  ListGlassIcon,
  SearchGlassIcon,
} from '@thesvg/ui-react'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'
import {
  ALL_CATEGORIES,
  type TemplateSort,
} from '../../lib/templateCatalogView'
import type { TemplateDensity } from '../../hooks/useTemplateDensity'

const SORTS: TemplateSort[] = ['relevance', 'name', 'memory']

function CategoryPill({
  label,
  count,
  active,
  onClick,
}: {
  label: string
  count: number
  active: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={cn(
        'flex shrink-0 items-center gap-1.5 rounded-full border px-3 py-1 text-sm font-medium whitespace-nowrap transition-colors focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50',
        active
          ? 'border-primary bg-primary/10 text-primary'
          : 'border-border bg-card text-foreground/70 hover:text-foreground',
      )}
    >
      {label}
      <span className="text-xs tabular-nums opacity-70">{count}</span>
    </button>
  )
}

/** TemplateToolbar is the sticky search, sort, layout and category-pill bar (pills show under md only). */
export function TemplateToolbar({
  search,
  onSearch,
  sort,
  onSort,
  verifiedOnly,
  onVerifiedOnly,
  density,
  onDensity,
  categoryCounts,
  total,
  category,
  onCategory,
  shown,
}: {
  search: string
  onSearch: (value: string) => void
  sort: TemplateSort
  onSort: (value: TemplateSort) => void
  verifiedOnly: boolean
  onVerifiedOnly: (value: boolean) => void
  density: TemplateDensity
  onDensity: (value: TemplateDensity) => void
  categoryCounts: [string, number][]
  total: number
  category: string
  onCategory: (value: string) => void
  shown: number
}) {
  const { t } = useTranslation('templates')
  return (
    <div className="sticky top-0 z-10 space-y-3 bg-background pb-3">
      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-56 flex-1">
          <SearchGlassIcon
            className="pointer-events-none absolute top-1/2 left-2.5 size-5 -translate-y-1/2 text-muted-foreground"
            aria-hidden="true"
          />
          <Input
            type="search"
            value={search}
            onChange={(e) => {
              onSearch(e.target.value)
            }}
            placeholder={t('marketplace.search.placeholder', { count: total })}
            aria-label={t('marketplace.search.label')}
            className="pl-8"
          />
        </div>
        <Select
          value={sort}
          onValueChange={(value) => {
            if (value) onSort(value)
          }}
        >
          <SelectTrigger
            aria-label={t('marketplace.sort.label')}
            className="w-44"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {SORTS.map((key) => (
              <SelectItem key={key} value={key}>
                {t(`marketplace.sort.${key}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <button
          type="button"
          aria-pressed={verifiedOnly}
          onClick={() => {
            onVerifiedOnly(!verifiedOnly)
          }}
          title={t('verified.hint')}
          className={cn(
            'flex items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-sm font-medium focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50',
            verifiedOnly
              ? 'border-primary bg-primary/10 text-primary'
              : 'border-border text-foreground/70 hover:text-foreground',
          )}
        >
          <BadgeCheckGlassIcon
            className="size-5 text-tone-success"
            aria-hidden="true"
          />
          {t('verified.filter')}
        </button>
        <div
          role="group"
          aria-label={t('marketplace.density.label')}
          className="flex rounded-lg border border-border p-0.5"
        >
          {(
            [
              ['comfortable', LayoutGridGlassIcon],
              ['compact', ListGlassIcon],
            ] as const
          ).map(([key, DensityIcon]) => (
            <button
              key={key}
              type="button"
              aria-pressed={density === key}
              onClick={() => {
                onDensity(key)
              }}
              className={cn(
                'flex items-center gap-1.5 rounded-md px-2.5 py-1 text-sm font-medium focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50',
                density === key
                  ? 'bg-muted text-foreground'
                  : 'text-foreground/70 hover:text-foreground',
              )}
            >
              <DensityIcon className="size-4" aria-hidden="true" />
              <span className="max-sm:sr-only">
                {t(`marketplace.density.${key}`)}
              </span>
            </button>
          ))}
        </div>
      </div>
      <div
        role="group"
        aria-label={t('marketplace.categories.filterLabel')}
        className="flex gap-2 overflow-x-auto pb-1 md:hidden"
      >
        <CategoryPill
          label={t('marketplace.categories.all')}
          count={total}
          active={category === ALL_CATEGORIES}
          onClick={() => {
            onCategory(ALL_CATEGORIES)
          }}
        />
        {categoryCounts.map(([name, count]) => (
          <CategoryPill
            key={name}
            label={name}
            count={count}
            active={category === name}
            onClick={() => {
              onCategory(name)
            }}
          />
        ))}
      </div>
      <p
        role="status"
        aria-live="polite"
        className="text-sm text-foreground/70"
      >
        {t('marketplace.count', { shown, total })}
      </p>
    </div>
  )
}
