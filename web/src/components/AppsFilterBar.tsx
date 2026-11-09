import { TagFilter } from './TagFilter'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  EMPTY_FILTERS,
  filtersActive,
  type AppListFilters,
} from '../lib/appViews'

function toggle(list: string[], value: string): string[] {
  return list.includes(value)
    ? list.filter((v) => v !== value)
    : [...list, value]
}

export function AppsFilterBar({
  filters,
  environments,
  onChange,
}: {
  filters: AppListFilters
  environments: string[]
  onChange: (next: AppListFilters) => void
}) {
  return (
    <div className="mb-3 flex flex-wrap items-center gap-2">
      <Input
        value={filters.query}
        placeholder="Search apps"
        aria-label="Search apps"
        className="h-8 w-56"
        onChange={(e) => {
          onChange({ ...filters, query: e.target.value })
        }}
      />
      {environments.map((env) => {
        const on = filters.environments.includes(env)
        return (
          <Button
            key={env}
            type="button"
            size="sm"
            variant={on ? 'default' : 'outline'}
            aria-pressed={on}
            onClick={() => {
              onChange({
                ...filters,
                environments: toggle(filters.environments, env),
              })
            }}
          >
            {env}
          </Button>
        )
      })}
      <TagFilter
        selected={filters.tags}
        onChange={(tags) => {
          onChange({ ...filters, tags })
        }}
      />
      {filtersActive(filters) ? (
        <Button
          type="button"
          size="sm"
          variant="ghost"
          onClick={() => {
            onChange(EMPTY_FILTERS)
          }}
        >
          Clear
        </Button>
      ) : null}
    </div>
  )
}
