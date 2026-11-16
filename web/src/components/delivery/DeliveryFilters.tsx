import { useTranslation } from 'react-i18next'
import { SearchGlassIcon } from '@thesvg/ui-react'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import {
  FILTERABLE_CATEGORIES,
  type DeliveryCategory,
  type DeliveryFilters as Filters,
  type StatusFilter,
} from '../../lib/deliveryTimeline'

const STATUSES: readonly StatusFilter[] = [
  'all',
  'failed',
  'in_progress',
  'succeeded',
]

export function DeliveryFilters({
  value,
  onChange,
}: {
  value: Filters
  onChange: (next: Filters) => void
}) {
  const { t } = useTranslation('delivery')

  function toggle(cat: DeliveryCategory) {
    const kinds = value.kinds.includes(cat)
      ? value.kinds.filter((k) => k !== cat)
      : [...value.kinds, cat]
    onChange({ ...value, kinds })
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      <div
        role="group"
        aria-label={t('filters.kind')}
        className="flex flex-wrap gap-1.5"
      >
        {FILTERABLE_CATEGORIES.map((cat) => {
          const on = value.kinds.includes(cat)
          return (
            <button
              key={cat}
              type="button"
              aria-pressed={on}
              onClick={() => toggle(cat)}
              className={`h-7 rounded-full border px-2.5 text-xs font-medium transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none pointer-coarse:h-11 ${
                on
                  ? 'border-primary bg-primary/10 text-foreground'
                  : 'border-border text-muted-foreground hover:text-foreground'
              }`}
            >
              {t(`category.${cat}`)}
            </button>
          )
        })}
      </div>
      <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
        <span className="sr-only sm:not-sr-only">{t('filters.status')}</span>
        <NativeSelect
          value={value.status}
          onChange={(e) =>
            onChange({ ...value, status: e.target.value as StatusFilter })
          }
        >
          {STATUSES.map((s) => (
            <option key={s} value={s}>
              {t(`filters.statusOption.${s}`)}
            </option>
          ))}
        </NativeSelect>
      </label>
      <label className="relative ml-auto min-w-40 flex-1 sm:max-w-64">
        <span className="sr-only">{t('filters.search')}</span>
        <SearchGlassIcon
          aria-hidden="true"
          className="pointer-events-none absolute top-1/2 left-2 size-4 -translate-y-1/2 text-muted-foreground"
        />
        <Input
          type="search"
          value={value.search}
          placeholder={t('filters.searchPlaceholder')}
          onChange={(e) => onChange({ ...value, search: e.target.value })}
          className="pl-8"
        />
      </label>
    </div>
  )
}
