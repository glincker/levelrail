import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import type { RuntimeSettingCategory } from '../../types/runtimeSettings'

export interface CategoryCount {
  category: RuntimeSettingCategory
  count: number
}

/** Sticky list from lg up, a native select below it. */
export function RuntimeCategoryNav({
  counts,
  total,
  active,
  onSelect,
}: {
  counts: CategoryCount[]
  total: number
  active: RuntimeSettingCategory | undefined
  onSelect: (next: RuntimeSettingCategory | undefined) => void
}) {
  const { t } = useTranslation('runtimeSettings')
  const selectId = useId()
  const item = (
    category: RuntimeSettingCategory | undefined,
    label: string,
    count: number,
  ) => (
    <button
      key={category ?? 'all'}
      type="button"
      aria-current={active === category ? 'true' : undefined}
      className={cn(
        'flex min-h-8 w-full items-center justify-between gap-2 rounded-md px-2.5 text-left text-sm focus-visible:ring-2 focus-visible:ring-ring',
        active === category
          ? 'bg-muted font-medium text-foreground'
          : 'text-muted-foreground hover:bg-muted/60 hover:text-foreground',
      )}
      onClick={() => {
        onSelect(category)
      }}
    >
      {label}
      <span className="text-xs tabular-nums">{count}</span>
    </button>
  )
  return (
    <>
      <nav
        aria-label={t('toolbar.jumpLabel')}
        className="sticky top-24 hidden w-48 shrink-0 space-y-0.5 self-start lg:block"
      >
        {item(undefined, t('toolbar.allCategories'), total)}
        {counts.map((c) =>
          item(c.category, t(`categories.${c.category}`), c.count),
        )}
      </nav>
      <div className="lg:hidden">
        <label htmlFor={selectId} className="sr-only">
          {t('toolbar.jumpLabel')}
        </label>
        <select
          id={selectId}
          value={active ?? ''}
          className="h-9 w-full rounded-md border border-input bg-background px-2 text-sm focus-visible:ring-2 focus-visible:ring-ring"
          onChange={(e) => {
            const next = counts.find((c) => c.category === e.target.value)
            onSelect(next?.category)
          }}
        >
          <option value="">
            {t('toolbar.allCategories')} ({total})
          </option>
          {counts.map((c) => (
            <option key={c.category} value={c.category}>
              {t(`categories.${c.category}`)} ({c.count})
            </option>
          ))}
        </select>
      </div>
    </>
  )
}
