import { useEffect, useState } from 'react'
import { TagFilter } from './TagFilter'
import { useDebouncedValue } from '../hooks/useDebouncedValue'
import { SEARCH_DEBOUNCE_MS } from '../lib/searchDebounce'
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
  const [text, setText] = useState(filters.query)
  const debounced = useDebouncedValue(text, SEARCH_DEBOUNCE_MS)
  const [urlQuery, setUrlQuery] = useState(filters.query)
  if (filters.query !== urlQuery) {
    setUrlQuery(filters.query)
    setText(filters.query)
  }
  useEffect(() => {
    if (debounced !== filters.query) onChange({ ...filters, query: debounced })
    // Only a settled edit should push to the URL; a URL-driven change is handled above.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debounced])

  return (
    <div className="mb-3 flex flex-wrap items-center gap-2">
      <Input
        value={text}
        placeholder="Search apps"
        aria-label="Search apps"
        className="h-8 w-56"
        onChange={(e) => {
          setText(e.target.value)
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
