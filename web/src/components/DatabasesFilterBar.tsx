import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { FunnelGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { TONE } from '@/components/kit/tone'
import {
  DB_STATUS_BUCKETS,
  EMPTY_DB_FILTERS,
  dbFiltersActive,
  type DatabaseListFilters,
  type DbStatusBucket,
} from '../lib/databasesFilter'

const DOT: Record<DbStatusBucket, string> = {
  healthy: TONE.success.solid,
  attention: TONE.danger.solid,
  reconciling: TONE.warning.solid,
  stopped: TONE.neutral.solid,
}

function toggle(list: string[], value: string): string[] {
  return list.includes(value)
    ? list.filter((v) => v !== value)
    : [...list, value]
}

/** DatabasesStatusStrip shows counts by status; each count is a quick filter. */
export function DatabasesStatusStrip({
  counts,
  active,
  onChange,
}: {
  counts: Record<DbStatusBucket, number>
  active: DbStatusBucket | null
  onChange: (next: DbStatusBucket | null) => void
}) {
  const { t } = useTranslation('databases')
  return (
    <div
      role="group"
      aria-label={t('filter.statusGroup')}
      className="-mx-4 flex items-center gap-2 overflow-x-auto px-4 pb-1 sm:mx-0 sm:flex-wrap sm:overflow-visible sm:px-0 sm:pb-0"
    >
      {DB_STATUS_BUCKETS.map((bucket) => {
        const on = active === bucket
        return (
          <button
            key={bucket}
            type="button"
            aria-pressed={on}
            onClick={() => {
              onChange(on ? null : bucket)
            }}
            className={cn(
              'inline-flex shrink-0 items-center gap-1.5 rounded-full border px-3 py-1 text-sm transition-colors duration-150 pointer-coarse:min-h-10 motion-reduce:transition-none focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring',
              on
                ? 'border-ring bg-tone-accent-soft text-foreground'
                : 'border-border text-muted-foreground hover:bg-muted/60 hover:text-foreground',
            )}
          >
            <span
              aria-hidden="true"
              className={cn('size-2 rounded-full', DOT[bucket])}
            />
            <span className="font-medium tabular-nums text-foreground">
              {counts[bucket]}
            </span>
            {t(`filter.status.${bucket}`)}
          </button>
        )
      })}
    </div>
  )
}

export function DatabasesFilterBar({
  filters,
  engines,
  onChange,
}: {
  filters: DatabaseListFilters
  engines: string[]
  onChange: (next: DatabaseListFilters) => void
}) {
  const { t } = useTranslation('databases')
  const [open, setOpen] = useState(false)
  const enginesId = useId()
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Input
        type="search"
        value={filters.q}
        placeholder={t('filter.search')}
        aria-label={t('filter.search')}
        className="h-8 min-w-0 flex-1 sm:w-56 sm:flex-none"
        onChange={(e) => {
          onChange({ ...filters, q: e.target.value })
        }}
      />
      {engines.length > 1 ? (
        <>
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="sm:hidden"
            aria-expanded={open}
            aria-controls={enginesId}
            onClick={() => {
              setOpen((v) => !v)
            }}
          >
            <FunnelGlassIcon size="1.4em" aria-hidden="true" />
            {filters.engines.length > 0
              ? t('filter.enginesCount', { count: filters.engines.length })
              : t('filter.engines')}
          </Button>
          <div
            id={enginesId}
            role="group"
            aria-label={t('filter.engines')}
            className={cn(
              'w-full flex-wrap items-center gap-2 sm:flex sm:w-auto',
              open ? 'flex' : 'hidden',
            )}
          >
            {engines.map((engine) => {
              const on = filters.engines.includes(engine)
              return (
                <Button
                  key={engine}
                  type="button"
                  size="sm"
                  variant={on ? 'default' : 'outline'}
                  aria-pressed={on}
                  className="capitalize"
                  onClick={() => {
                    onChange({
                      ...filters,
                      engines: toggle(filters.engines, engine),
                    })
                  }}
                >
                  {engine}
                </Button>
              )
            })}
          </div>
        </>
      ) : null}
      {dbFiltersActive(filters) ? (
        <Button
          type="button"
          size="sm"
          variant="ghost"
          onClick={() => {
            onChange(EMPTY_DB_FILTERS)
          }}
        >
          {t('filter.clear')}
        </Button>
      ) : null}
    </div>
  )
}
